package githubclient

import (
	"aurumcode/pkg/types"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// DefaultBaseURL is the GitHub API base URL
	DefaultBaseURL = "https://api.github.com"

	// UserAgent identifies AurumCode
	UserAgent = "AurumCode/1.0"

	// MaxRetries is the maximum number of retry attempts
	MaxRetries = 3

	// InitialBackoff is the initial backoff duration
	InitialBackoff = 1 * time.Second
)

// etagCacheEntry stores a cached diff with its ETag
type etagCacheEntry struct {
	etag string
	diff *types.Diff
}

// Client is a GitHub API client with retry and rate-limit handling
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client

	// ETag cache for conditional requests
	etagCache map[string]*etagCacheEntry
	cacheMu   sync.RWMutex
}

// NewClient creates a new GitHub API client
func NewClient(token string) *Client {
	return &Client{
		baseURL: DefaultBaseURL,
		token:   token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		etagCache: make(map[string]*etagCacheEntry),
	}
}

// NewClientWithBaseURL creates a client with a custom base URL (for testing)
func NewClientWithBaseURL(token, baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		token:   token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		etagCache: make(map[string]*etagCacheEntry),
	}
}

// doRequest performs an HTTP request with retry logic
func (c *Client) doRequest(ctx context.Context, req *http.Request) (*http.Response, error) {
	// Set standard headers
	req.Header.Set("User-Agent", UserAgent)
	if c.token != "" {
		req.Header.Set("Authorization", "token "+c.token)
	}
	// Only set Accept header if not already set
	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", "application/vnd.github.v3+json")
	}

	var lastErr error
	backoff := InitialBackoff

	for attempt := 0; attempt <= MaxRetries; attempt++ {
		// Check context before attempt
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		// Clone request for retry
		reqClone := req.Clone(ctx)

		resp, err := c.httpClient.Do(reqClone)
		if err != nil {
			lastErr = fmt.Errorf("request failed: %w", err)

			// Don't retry on context errors
			if ctx.Err() != nil {
				return nil, lastErr
			}

			// Backoff before retry
			if attempt < MaxRetries {
				time.Sleep(backoff)
				backoff = c.calculateBackoff(backoff, attempt)
			}
			continue
		}

		// Handle rate limiting (429)
		if resp.StatusCode == http.StatusTooManyRequests {
			retryAfter := c.getRetryAfter(resp)
			resp.Body.Close()

			if attempt < MaxRetries {
				// Wait for retry-after duration
				select {
				case <-time.After(retryAfter):
					continue
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			}

			return nil, fmt.Errorf("rate limit exceeded after %d retries", MaxRetries)
		}

		// Handle server errors (5xx) - retry
		if resp.StatusCode >= 500 && resp.StatusCode < 600 {
			resp.Body.Close()
			lastErr = fmt.Errorf("server error: %d", resp.StatusCode)

			if attempt < MaxRetries {
				time.Sleep(backoff)
				backoff = c.calculateBackoff(backoff, attempt)
				continue
			}

			return nil, lastErr
		}

		// Success or client error (4xx) - return response
		return resp, nil
	}

	return nil, fmt.Errorf("max retries exceeded: %w", lastErr)
}

// calculateBackoff calculates exponential backoff with jitter
func (c *Client) calculateBackoff(current time.Duration, attempt int) time.Duration {
	// Exponential: 1s, 2s, 4s, 8s...
	backoff := current * 2

	// Cap at 30 seconds
	if backoff > 30*time.Second {
		backoff = 30 * time.Second
	}

	// Add jitter (±25%)
	jitter := time.Duration(rand.Float64() * float64(backoff) * 0.5)
	return backoff + jitter
}

// getRetryAfter extracts retry duration from response headers
func (c *Client) getRetryAfter(resp *http.Response) time.Duration {
	// Try Retry-After header first
	if retryAfter := resp.Header.Get("Retry-After"); retryAfter != "" {
		// Try as seconds
		if seconds, err := strconv.Atoi(retryAfter); err == nil {
			return time.Duration(seconds) * time.Second
		}

		// Try as HTTP date
		if t, err := http.ParseTime(retryAfter); err == nil {
			return time.Until(t)
		}
	}

	// Try X-RateLimit-Reset
	if reset := resp.Header.Get("X-RateLimit-Reset"); reset != "" {
		if timestamp, err := strconv.ParseInt(reset, 10, 64); err == nil {
			resetTime := time.Unix(timestamp, 0)
			duration := time.Until(resetTime)
			if duration > 0 {
				return duration
			}
		}
	}

	// Default fallback
	return 60 * time.Second
}

// decodeJSON is a helper to decode JSON responses
func decodeJSON(resp *http.Response, v interface{}) error {
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	return json.NewDecoder(resp.Body).Decode(v)
}

// readBody reads the full response body
func readBody(resp *http.Response) ([]byte, error) {
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

// GetPullRequestDiff retrieves the diff for a pull request with ETag caching
func (c *Client) GetPullRequestDiff(ctx context.Context, owner, repo string, number int) (*types.Diff, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/pulls/%d", c.baseURL, owner, repo, number)
	cacheKey := fmt.Sprintf("%s/%s/%d", owner, repo, number)

	// Check cache for ETag
	c.cacheMu.RLock()
	cached, hasCached := c.etagCache[cacheKey]
	c.cacheMu.RUnlock()

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Accept header for diff format
	req.Header.Set("Accept", "application/vnd.github.v3.diff")

	// Send If-None-Match if we have cached ETag
	if hasCached && cached.etag != "" {
		req.Header.Set("If-None-Match", cached.etag)
	}

	resp, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// 304 Not Modified - return cached diff
	if resp.StatusCode == http.StatusNotModified {
		if hasCached {
			return cached.diff, nil
		}
		return nil, fmt.Errorf("received 304 but no cached diff available")
	}

	// Read diff body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	// Parse diff
	diff, err := parseDiff(string(body))
	if err != nil {
		return nil, fmt.Errorf("failed to parse diff: %w", err)
	}

	// Cache with ETag
	etag := resp.Header.Get("ETag")
	if etag != "" {
		c.cacheMu.Lock()
		c.etagCache[cacheKey] = &etagCacheEntry{
			etag: etag,
			diff: diff,
		}
		c.cacheMu.Unlock()
	}

	return diff, nil
}

// ListChangedFiles retrieves the list of changed files in a pull request with pagination
func (c *Client) ListChangedFiles(ctx context.Context, owner, repo string, number int) ([]string, error) {
	var allFiles []string
	url := fmt.Sprintf("%s/repos/%s/%s/pulls/%d/files", c.baseURL, owner, repo, number)

	for url != "" {
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		resp, err := c.doRequest(ctx, req)
		if err != nil {
			return nil, err
		}

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
		}

		// Parse JSON response
		var files []struct {
			Filename string `json:"filename"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&files); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("failed to decode response: %w", err)
		}
		resp.Body.Close()

		// Extract filenames
		for _, f := range files {
			allFiles = append(allFiles, f.Filename)
		}

		// Check for next page in Link header
		url = c.parseNextLink(resp.Header.Get("Link"))
	}

	return allFiles, nil
}

// ReviewComment represents a review comment to be posted
type ReviewComment struct {
	Body     string `json:"body"`
	CommitID string `json:"commit_id"`
	Path     string `json:"path"`
	Line     int    `json:"line,omitempty"`     // For single-line comments
	Position int    `json:"position,omitempty"` // Alternative to Line (deprecated by GitHub)
}

// PostReviewComment posts a review comment on a pull request with idempotency
func (c *Client) PostReviewComment(ctx context.Context, owner, repo string, number int, comment ReviewComment, idempotencyKey string) error {
	url := fmt.Sprintf("%s/repos/%s/%s/pulls/%d/comments", c.baseURL, owner, repo, number)

	// Marshal comment to JSON
	jsonData, err := json.Marshal(comment)
	if err != nil {
		return fmt.Errorf("failed to marshal comment: %w", err)
	}

	req, err := http.NewRequest("POST", url, strings.NewReader(string(jsonData)))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set Content-Type for JSON
	req.Header.Set("Content-Type", "application/json")

	// Set idempotency key header if provided
	if idempotencyKey != "" {
		req.Header.Set("X-GitHub-Idempotency-Key", idempotencyKey)
	}

	resp, err := c.doRequest(ctx, req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// PostIssueComment posts a general comment on a pull request (not tied to a specific line)
func (c *Client) PostIssueComment(ctx context.Context, owner, repo string, number int, body string) error {
	url := fmt.Sprintf("%s/repos/%s/%s/issues/%d/comments", c.baseURL, owner, repo, number)

	comment := map[string]string{
		"body": body,
	}

	jsonData, err := json.Marshal(comment)
	if err != nil {
		return fmt.Errorf("failed to marshal comment: %w", err)
	}

	req, err := http.NewRequest("POST", url, strings.NewReader(string(jsonData)))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.doRequest(ctx, req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// CommitStatus represents a commit status update
type CommitStatus struct {
	State       string `json:"state"`                  // "pending", "success", "error", or "failure"
	TargetURL   string `json:"target_url,omitempty"`   // Optional URL
	Description string `json:"description,omitempty"`  // Short description
	Context     string `json:"context"`                // Label to differentiate this status
}

// SetStatus sets the commit status for a specific SHA
func (c *Client) SetStatus(ctx context.Context, owner, repo, sha string, status CommitStatus) error {
	url := fmt.Sprintf("%s/repos/%s/%s/statuses/%s", c.baseURL, owner, repo, sha)

	// Validate state
	validStates := map[string]bool{
		"pending": true,
		"success": true,
		"error":   true,
		"failure": true,
	}

	if !validStates[status.State] {
		return fmt.Errorf("invalid state: %s (must be pending, success, error, or failure)", status.State)
	}

	// Marshal status to JSON
	jsonData, err := json.Marshal(status)
	if err != nil {
		return fmt.Errorf("failed to marshal status: %w", err)
	}

	req, err := http.NewRequest("POST", url, strings.NewReader(string(jsonData)))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set Content-Type for JSON
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.doRequest(ctx, req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// parseNextLink extracts the "next" URL from the Link header
// Example: <https://api.github.com/repos/owner/repo/pulls/123/files?page=2>; rel="next"
func (c *Client) parseNextLink(linkHeader string) string {
	if linkHeader == "" {
		return ""
	}

	// Split by comma to get individual links
	links := splitByComma(linkHeader)

	for _, link := range links {
		// Check if this is the "next" link
		if !contains(link, `rel="next"`) {
			continue
		}

		// Extract URL between < and >
		start := stringIndexByte(link, '<')
		end := stringIndexByte(link, '>')

		if start != -1 && end != -1 && start < end {
			return link[start+1 : end]
		}
	}

	return ""
}

// Helper function to split by comma (for Link header parsing)
func splitByComma(s string) []string {
	var parts []string
	start := 0

	for i := 0; i < len(s); i++ {
		if s[i] == ',' {
			parts = append(parts, trimSpace(s[start:i]))
			start = i + 1
		}
	}

	// Add last part
	if start < len(s) {
		parts = append(parts, trimSpace(s[start:]))
	}

	return parts
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && indexOf(s, substr) != -1
}

func indexOf(s, substr string) int {
	sLen := len(s)
	subLen := len(substr)

	if subLen > sLen {
		return -1
	}

	for i := 0; i <= sLen-subLen; i++ {
		match := true
		for j := 0; j < subLen; j++ {
			if s[i+j] != substr[j] {
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

// maxBackoff returns the maximum possible backoff duration
func maxBackoff(initial time.Duration, maxRetries int) time.Duration {
	backoff := initial
	for i := 0; i < maxRetries; i++ {
		backoff = backoff * 2
		if backoff > 30*time.Second {
			return 30 * time.Second
		}
	}
	return backoff
}

// jitter adds random jitter to duration (±25%)
func jitter(d time.Duration) time.Duration {
	variance := 0.25
	jitter := (rand.Float64()*2 - 1) * variance * float64(d)
	result := float64(d) + jitter
	return time.Duration(math.Max(0, result))
}

// parseDiff parses a unified diff format into types.Diff
func parseDiff(diffText string) (*types.Diff, error) {
	diff := &types.Diff{
		Files: []types.DiffFile{},
	}

	if diffText == "" {
		return diff, nil
	}

	lines := splitLines(diffText)
	var currentFile *types.DiffFile
	var currentHunk *types.DiffHunk

	for i := 0; i < len(lines); i++ {
		line := lines[i]

		// New file header: diff --git a/path b/path
		if len(line) >= 11 && line[:11] == "diff --git " {
			// Save previous file if exists
			if currentFile != nil && currentHunk != nil {
				currentFile.Hunks = append(currentFile.Hunks, *currentHunk)
			}
			if currentFile != nil {
				diff.Files = append(diff.Files, *currentFile)
			}

			// Extract file path
			path := extractFilePath(line)
			currentFile = &types.DiffFile{
				Path:  path,
				Hunks: []types.DiffHunk{},
			}
			currentHunk = nil
			continue
		}

		// Hunk header: @@ -old_start,old_lines +new_start,new_lines @@
		if len(line) >= 2 && line[:2] == "@@" {
			// Save previous hunk if exists
			if currentHunk != nil && currentFile != nil {
				currentFile.Hunks = append(currentFile.Hunks, *currentHunk)
			}

			hunk := parseHunkHeader(line)
			currentHunk = &hunk
			continue
		}

		// Hunk content lines
		if currentHunk != nil {
			// Skip metadata lines
			if len(line) > 0 && (line[0] == '+' || line[0] == '-' || line[0] == ' ' || line[0] == '\\') {
				currentHunk.Lines = append(currentHunk.Lines, line)
			}
		}
	}

	// Save last hunk and file
	if currentFile != nil && currentHunk != nil {
		currentFile.Hunks = append(currentFile.Hunks, *currentHunk)
	}
	if currentFile != nil {
		diff.Files = append(diff.Files, *currentFile)
	}

	return diff, nil
}

// extractFilePath extracts the file path from a diff header
// Example: "diff --git a/path/to/file.go b/path/to/file.go" -> "path/to/file.go"
func extractFilePath(line string) string {
	// Find "b/" prefix (new file path)
	idx := -1
	for i := 0; i < len(line)-2; i++ {
		if line[i:i+2] == "b/" {
			idx = i + 2
			break
		}
	}

	if idx == -1 {
		return ""
	}

	// Extract path from "b/" to end or space
	path := line[idx:]
	if spaceIdx := stringIndexByte(path, ' '); spaceIdx != -1 {
		path = path[:spaceIdx]
	}

	return path
}

// parseHunkHeader parses a hunk header line
// Example: "@@ -10,5 +10,7 @@ func main() {" -> DiffHunk{OldStart: 10, OldLines: 5, NewStart: 10, NewLines: 7}
func parseHunkHeader(line string) types.DiffHunk {
	hunk := types.DiffHunk{
		Lines: []string{},
	}

	// Find positions of key characters
	minusIdx := stringIndexByte(line, '-')
	commaIdx1 := stringIndexByte(line[minusIdx:], ',')
	plusIdx := stringIndexByte(line, '+')
	commaIdx2 := stringIndexByte(line[plusIdx:], ',')
	atIdx := stringIndexByte(line[plusIdx:], '@')

	if minusIdx == -1 || plusIdx == -1 {
		return hunk
	}

	// Parse old start
	if commaIdx1 != -1 {
		commaIdx1 += minusIdx
		hunk.OldStart = parseInt(line[minusIdx+1 : commaIdx1])

		// Parse old lines
		endIdx := plusIdx
		if spaceIdx := stringIndexByte(line[commaIdx1:plusIdx], ' '); spaceIdx != -1 {
			endIdx = commaIdx1 + spaceIdx
		}
		hunk.OldLines = parseInt(line[commaIdx1+1 : endIdx])
	} else {
		// No comma, single line
		hunk.OldStart = parseInt(line[minusIdx+1 : plusIdx-1])
		hunk.OldLines = 1
	}

	// Parse new start
	if commaIdx2 != -1 {
		commaIdx2 += plusIdx
		hunk.NewStart = parseInt(line[plusIdx+1 : commaIdx2])

		// Parse new lines
		endIdx := len(line)
		if atIdx != -1 {
			endIdx = plusIdx + atIdx
		}
		if spaceIdx := stringIndexByte(line[commaIdx2:endIdx], ' '); spaceIdx != -1 {
			endIdx = commaIdx2 + spaceIdx
		}
		hunk.NewLines = parseInt(line[commaIdx2+1 : endIdx])
	} else {
		// No comma, single line
		endIdx := len(line)
		if atIdx != -1 {
			endIdx = plusIdx + atIdx
		}
		hunk.NewStart = parseInt(line[plusIdx+1 : endIdx])
		hunk.NewLines = 1
	}

	return hunk
}

// Helper functions for string parsing
func splitLines(s string) []string {
	var lines []string
	start := 0

	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}

	// Add last line if not empty
	if start < len(s) {
		lines = append(lines, s[start:])
	}

	return lines
}

func stringIndexByte(s string, c byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}

func parseInt(s string) int {
	s = trimSpace(s)
	if s == "" {
		return 0
	}

	n := 0
	for i := 0; i < len(s); i++ {
		if s[i] >= '0' && s[i] <= '9' {
			n = n*10 + int(s[i]-'0')
		} else {
			break
		}
	}
	return n
}

func trimSpace(s string) string {
	start := 0
	end := len(s)

	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}

	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}

	return s[start:end]
}
