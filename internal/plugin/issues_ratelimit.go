package plugin

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strconv"
	"time"
)

// issuesRetries is how many times a rate-limited request is sent again.
// GitHub rejected the request, so resending a write never duplicates it.
const issuesRetries = 3

// sendWithRetry sends one request and, when GitHub answers with its rate
// limiter (429, or 403 with Retry-After or X-RateLimit-Remaining: 0), waits
// what it asks and sends it again, as the pull-request client does.
func (g *GitHubIssues) sendWithRetry(ctx context.Context, method, u string, data []byte) (*http.Response, error) {
	for attempt := 0; ; attempt++ {
		resp, err := g.send(ctx, method, u, data)
		if err != nil || attempt == issuesRetries || !rateLimited(resp) {
			return resp, err
		}
		wait := retryAfter(resp)
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

func (g *GitHubIssues) send(ctx context.Context, method, u string, data []byte) (*http.Response, error) {
	var body io.Reader
	if data != nil {
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "AurumCode/1.0")
	if data != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if g.Token != "" {
		req.Header.Set("Authorization", "token "+g.Token)
	}
	return g.HTTP.Do(req)
}

func rateLimited(resp *http.Response) bool {
	if resp.StatusCode == http.StatusTooManyRequests {
		return true
	}
	return resp.StatusCode == http.StatusForbidden &&
		(resp.Header.Get("Retry-After") != "" || resp.Header.Get("X-RateLimit-Remaining") == "0")
}

// retryAfter is the wait GitHub asks for: Retry-After seconds, else until
// X-RateLimit-Reset, else one minute (GitHub's advice for the secondary
// limit).
func retryAfter(resp *http.Response) time.Duration {
	if s, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && s >= 0 {
		return time.Duration(s) * time.Second
	}
	if r, err := strconv.ParseInt(resp.Header.Get("X-RateLimit-Reset"), 10, 64); err == nil {
		if d := time.Until(time.Unix(r, 0)); d > 0 {
			return d
		}
	}
	return time.Minute
}
