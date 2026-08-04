package httpbase

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"sort"
	"time"
)

const (
	defaultTimeout     = 30 * time.Second
	defaultMaxRetries  = 3
	defaultBackoffBase = time.Second
	maxBackoff         = 30 * time.Second

	// maxErrorBody caps how much of a remote error body is copied into an
	// error value. Bodies are remote-controlled and reach logs and Action output.
	maxErrorBody = 2048
)

// Client is an HTTP client with retry, backoff, and secret redaction
type Client struct {
	httpClient  *http.Client
	maxRetries  int
	backoffBase time.Duration
	baseURL     string
}

// NewClient creates a new HTTP client with default settings
func NewClient(baseURL string) *Client {
	return &Client{
		httpClient:  &http.Client{Timeout: defaultTimeout},
		maxRetries:  defaultMaxRetries,
		backoffBase: defaultBackoffBase,
		baseURL:     baseURL,
	}
}

// Request represents an HTTP request
type Request struct {
	Method  string
	Path    string
	Headers map[string]string
	Body    interface{}
}

// StatusError reports a response the client gave up on. It carries the status
// that caused the failure so a caller can tell a rate limit from an outage.
type StatusError struct {
	StatusCode int
	Attempts   int
	Body       string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("HTTP %d after %d attempt(s): %s", e.StatusCode, e.Attempts, e.Body)
}

// Do performs an HTTP request, retrying transport failures, 5xx and 429.
//
// On success the caller owns resp.Body and must close it. On failure the
// response is drained and closed here, and only the status survives, so no
// response body or header is handed back implicitly.
func (c *Client) Do(ctx context.Context, req *Request) (*http.Response, error) {
	body, err := encodeBody(req.Body)
	if err != nil {
		return nil, err
	}

	c.logRequest(req)

	var lastErr error

	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(c.backoff(attempt)):
			}
		}

		resp, err := c.doAttempt(ctx, req, body)
		if err != nil {
			if ctx.Err() != nil {
				return nil, err
			}
			lastErr = err
			continue
		}

		if !shouldRetry(resp.StatusCode) {
			return resp, nil
		}

		lastErr = &StatusError{
			StatusCode: resp.StatusCode,
			Attempts:   attempt + 1,
			Body:       RedactSecret(readBounded(resp.Body)),
		}
		resp.Body.Close()
	}

	return nil, fmt.Errorf("giving up after %d attempt(s): %w", c.maxRetries+1, lastErr)
}

// doAttempt performs a single HTTP request
func (c *Client) doAttempt(ctx context.Context, req *Request, body []byte) (*http.Response, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}

	httpReq, err := http.NewRequestWithContext(ctx, req.Method, c.baseURL+req.Path, reader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	for k, v := range req.Headers {
		httpReq.Header.Set(k, v)
	}
	if body != nil {
		httpReq.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	c.logResponse(req, resp)

	return resp, nil
}

// backoff grows exponentially from backoffBase and applies full jitter, so a
// fleet of runners retrying the same gateway does not resynchronise.
func (c *Client) backoff(attempt int) time.Duration {
	d := c.backoffBase << (attempt - 1)
	if d > maxBackoff || d <= 0 {
		d = maxBackoff
	}
	return time.Duration(rand.Int63n(int64(d)) + 1)
}

// shouldRetry reports whether a status is worth another attempt.
func shouldRetry(status int) bool {
	return status == http.StatusTooManyRequests || (status >= 500 && status < 600)
}

func encodeBody(body interface{}) ([]byte, error) {
	if body == nil {
		return nil, nil
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}
	return encoded, nil
}

func readBounded(r io.Reader) string {
	data, _ := io.ReadAll(io.LimitReader(r, maxErrorBody))
	return string(data)
}

// RedactSecret truncates a string at the first credential marker it recognises.
// Offsets are resolved against the original bytes: case folding can change byte
// length, and slicing a folded offset would corrupt or under-redact the value.
func RedactSecret(s string) string {
	for _, marker := range []string{"sk-", "bearer ", "x-api-key:", "api_key"} {
		if idx := indexFold(s, marker); idx != -1 {
			return s[:idx+len(marker)] + "***REDACTED***"
		}
	}
	return s
}

// indexFold reports the byte offset of lowercase marker in s, comparing ASCII
// case-insensitively. marker must already be lowercase ASCII.
func indexFold(s, marker string) int {
	if len(marker) == 0 || len(s) < len(marker) {
		return -1
	}
	for i := 0; i+len(marker) <= len(s); i++ {
		match := true
		for j := 0; j < len(marker); j++ {
			c := s[i+j]
			if 'A' <= c && c <= 'Z' {
				c += 'a' - 'A'
			}
			if c != marker[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

// logRequest records that a call happened. Header values are never printed:
// the consumer's key is one of them and carries no marker redaction can spot.
// Bodies are never printed either - they carry the prompt, which is repository
// content, and these logs land in the consumer's Action output.
func (c *Client) logRequest(req *Request) {
	names := make([]string, 0, len(req.Headers))
	for k := range req.Headers {
		names = append(names, k)
	}
	sort.Strings(names)

	log.Printf("[HTTP Request] %s %s headers=%v", req.Method, req.Path, names)
}

func (c *Client) logResponse(req *Request, resp *http.Response) {
	log.Printf("[HTTP Response] %s %s - Status: %d", req.Method, req.Path, resp.StatusCode)
}

// DecodeJSON decodes a successful JSON response into target. It always closes
// the body. On a non-2xx status it returns a StatusError carrying a bounded,
// redacted excerpt of the body.
func DecodeJSON(resp *http.Response, target interface{}) error {
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return &StatusError{
			StatusCode: resp.StatusCode,
			Attempts:   1,
			Body:       RedactSecret(readBounded(resp.Body)),
		}
	}

	return json.NewDecoder(resp.Body).Decode(target)
}
