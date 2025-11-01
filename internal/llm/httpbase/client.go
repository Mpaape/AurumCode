package httpbase

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"log"
)

// Client is an HTTP client with retry, backoff, and secret redaction
type Client struct {
	httpClient *http.Client
	timeout    time.Duration
	maxRetries int
	baseURL    string
}

// NewClient creates a new HTTP client with default settings
func NewClient(baseURL string) *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		timeout:    30 * time.Second,
		maxRetries: 3,
		baseURL:    baseURL,
	}
}

// Request represents an HTTP request
type Request struct {
	Method  string
	Path    string
	Headers map[string]string
	Body    interface{}
}

// Do performs an HTTP request with retries and backoff
func (c *Client) Do(ctx context.Context, req *Request) (*http.Response, error) {
	var lastErr error
	
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff with jitter
			backoff := time.Duration(attempt*attempt) * time.Second
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
		}
		
		resp, err := c.doAttempt(ctx, req)
		if err == nil && c.shouldRetry(resp) {
			if resp != nil && resp.Body != nil {
				resp.Body.Close()
			}
			lastErr = fmt.Errorf("retryable error (attempt %d/%d)", attempt+1, c.maxRetries+1)
			continue
		}
		
		return resp, err
	}
	
	return nil, fmt.Errorf("max retries exceeded: %w", lastErr)
}

// doAttempt performs a single HTTP request
func (c *Client) doAttempt(ctx context.Context, req *Request) (*http.Response, error) {
	var body io.Reader
	
	if req.Body != nil {
		bodyBytes, err := json.Marshal(req.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		body = bytes.NewReader(bodyBytes)
	}
	
	url := c.baseURL + req.Path
	httpReq, err := http.NewRequestWithContext(ctx, req.Method, url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	
	// Set headers
	if req.Headers != nil {
		for k, v := range req.Headers {
			httpReq.Header.Set(k, v)
		}
	}
	
	// Set Content-Type if body present
	if body != nil {
		httpReq.Header.Set("Content-Type", "application/json")
	}
	
	// Log request (with redaction)
	c.logRequest(req)
	
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	
	// Log response
	c.logResponse(resp)
	
	return resp, nil
}

// shouldRetry determines if a response indicates a retryable error
func (c *Client) shouldRetry(resp *http.Response) bool {
	if resp == nil {
		return true
	}
	
	// Retry on 5xx errors
	if resp.StatusCode >= 500 && resp.StatusCode < 600 {
		return true
	}
	
	// Retry on 429 (rate limit)
	if resp.StatusCode == 429 {
		return true
	}
	
	return false
}

// RedactSecret removes sensitive information from strings
func RedactSecret(s string) string {
	// Redact API keys, tokens, and other secrets
	for _, prefix := range []string{"sk-", "bearer ", "x-api-key:", "api_key"} {
		if idx := strings.Index(strings.ToLower(s), prefix); idx != -1 {
			// Redact everything after the prefix
			return s[:idx+len(prefix)] + "***REDACTED***"
		}
	}
	return s
}

// logRequest logs the request with redaction
func (c *Client) logRequest(req *Request) {
	headers := make(map[string]string)
	for k, v := range req.Headers {
		headers[k] = RedactSecret(v)
	}
	
	bodyStr := ""
	if req.Body != nil {
		bodyBytes, _ := json.Marshal(req.Body)
		bodyStr = RedactSecret(string(bodyBytes))
	}
	
	log.Printf("[HTTP Request] %s %s\nHeaders: %+v\nBody: %s", req.Method, req.Path, headers, bodyStr)
}

// logResponse logs the response
func (c *Client) logResponse(resp *http.Response) {
	if resp == nil {
		return
	}
	log.Printf("[HTTP Response] %s - Status: %d", resp.Request.URL.Path, resp.StatusCode)
}

// DecodeJSON decodes JSON response into target
func DecodeJSON(resp *http.Response, target interface{}) error {
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(bodyBytes))
	}
	
	return json.NewDecoder(resp.Body).Decode(target)
}

