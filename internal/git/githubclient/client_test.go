package githubclient

// Restored from commit c12d7ab together with client.go, with the
// adaptations the restoration demanded:
//
//   - the diff types are package-local now (see diff.go), so the historical
//     "aurumcode/pkg/types" import is gone;
//   - newTestClient shrinks the retry backoff so retry behavior is proven
//     without real one-second sleeps, and the historical Retry-After: 1
//     fixture became Retry-After: 0 for the same reason (the parsing path
//     is identical);
//   - the publishing tests route GET /repos/owner/repo to a permissions
//     fixture, because the restored client now verifies write permission
//     before every publish (card AUR-437's gate; see requireWritePermission).

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

// newTestClient builds a client against the given test server with a
// millisecond-scale backoff, so retry tests do not sleep for real seconds.
func newTestClient(token, baseURL string) *Client {
	c := NewClientWithBaseURL(token, baseURL)
	c.retryBackoff = 5 * time.Millisecond
	return c
}

// writableRepoRoute answers GET /repos/owner/repo with push permission and
// hands every other request to next, so publishing tests exercise the POST
// they always exercised while satisfying the new permission gate.
func writableRepoRoute(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/repos/owner/repo" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"permissions":{"admin":false,"maintain":false,"push":true,"pull":true}}`))
			return
		}
		next(w, r)
	}
}

func TestNewClient(t *testing.T) {
	client := NewClient("test-token")

	if client.baseURL != DefaultBaseURL {
		t.Errorf("expected baseURL %s, got %s", DefaultBaseURL, client.baseURL)
	}

	if client.token != "test-token" {
		t.Errorf("expected token 'test-token', got %s", client.token)
	}
}

func TestNewClientWithBaseURL(t *testing.T) {
	customURL := "https://custom.github.com"
	client := NewClientWithBaseURL("token", customURL)

	if client.baseURL != customURL {
		t.Errorf("expected baseURL %s, got %s", customURL, client.baseURL)
	}
}

func TestDoRequest_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify headers
		if r.Header.Get("User-Agent") != UserAgent {
			t.Errorf("expected User-Agent %s, got %s", UserAgent, r.Header.Get("User-Agent"))
		}

		if auth := r.Header.Get("Authorization"); !strings.HasPrefix(auth, "token ") {
			t.Errorf("expected Authorization header with token, got %s", auth)
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	client := newTestClient("test-token", server.URL)

	req, _ := http.NewRequest("GET", server.URL+"/test", nil)
	resp, err := client.doRequest(context.Background(), req)

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	resp.Body.Close()
}

func TestDoRequest_RateLimitWithRetryAfter(t *testing.T) {
	attempts := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++

		if attempts == 1 {
			// First attempt - rate limited
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}

		// Second attempt - success
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	client := newTestClient("test-token", server.URL)

	req, _ := http.NewRequest("GET", server.URL+"/test", nil)
	resp, err := client.doRequest(context.Background(), req)

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if attempts != 2 {
		t.Errorf("expected 2 attempts, got %d", attempts)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	resp.Body.Close()
}

func TestDoRequest_RateLimited403IsRetried(t *testing.T) {
	// Restoration defect 2: GitHub's primary rate limiter answers 403 with
	// X-RateLimit-Remaining: 0; the restored client only retried 429.
	attempts := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++

		if attempts == 1 {
			w.Header().Set("Retry-After", "0")
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.WriteHeader(http.StatusForbidden)
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	client := newTestClient("test-token", server.URL)

	req, _ := http.NewRequest("GET", server.URL+"/test", nil)
	resp, err := client.doRequest(context.Background(), req)

	if err != nil {
		t.Fatalf("expected rate-limited 403 to be retried, got: %v", err)
	}
	resp.Body.Close()

	if attempts != 2 {
		t.Errorf("expected 2 attempts, got %d", attempts)
	}
}

func TestDoRequest_Plain403IsNotRetried(t *testing.T) {
	// A permission denial must surface immediately, never be retried.
	attempts := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"message":"Forbidden"}`))
	}))
	defer server.Close()

	client := newTestClient("test-token", server.URL)

	req, _ := http.NewRequest("GET", server.URL+"/test", nil)
	resp, err := client.doRequest(context.Background(), req)

	if err != nil {
		t.Fatalf("plain 403 should be returned as a response, got error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("expected status 403, got %d", resp.StatusCode)
	}
	if attempts != 1 {
		t.Errorf("plain 403 must not be retried: expected 1 attempt, got %d", attempts)
	}
}

func TestDoRequest_ServerErrorRetry(t *testing.T) {
	attempts := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++

		if attempts < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	client := newTestClient("test-token", server.URL)

	req, _ := http.NewRequest("GET", server.URL+"/test", nil)
	resp, err := client.doRequest(context.Background(), req)

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}

	resp.Body.Close()
}

func TestDoRequest_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := newTestClient("test-token", server.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	req, _ := http.NewRequest("GET", server.URL+"/test", nil)
	_, err := client.doRequest(ctx, req)

	if err == nil {
		t.Error("expected context error, got nil")
	}

	if !strings.Contains(err.Error(), "context") {
		t.Errorf("expected context error, got: %v", err)
	}
}

func TestDoRequest_BackoffAbortsOnContextCancel(t *testing.T) {
	// Restoration defect 3: the backoff between retries used time.Sleep,
	// which ignores the caller's context. The wait must abort as soon as
	// the context is done.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewClientWithBaseURL("test-token", server.URL) // real 1s backoff on purpose

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := client.doRequest(ctx, func() *http.Request {
		req, _ := http.NewRequest("GET", server.URL+"/test", nil)
		return req
	}())
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected context error, got nil")
	}
	if !strings.Contains(err.Error(), "context") {
		t.Fatalf("expected context error, got: %v", err)
	}
	if elapsed >= 700*time.Millisecond {
		t.Fatalf("backoff ignored context cancellation: returned after %v", elapsed)
	}
}

func TestDoRequest_MaxRetriesExceeded(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := newTestClient("test-token", server.URL)

	req, _ := http.NewRequest("GET", server.URL+"/test", nil)
	_, err := client.doRequest(context.Background(), req)

	if err == nil {
		t.Error("expected error after max retries, got nil")
	}

	if !strings.Contains(err.Error(), "server error") {
		t.Errorf("expected server error message, got: %v", err)
	}
}

func TestDoRequest_RetryReplaysPostBody(t *testing.T) {
	// Restoration defect 1: http.Request.Clone shares the consumed body
	// reader, so a retried POST used to resend an empty body. The 429
	// closes the connection so net/http's own reused-connection rewind
	// cannot mask the defect (see doRequest).
	attempts := 0
	var retriedBody string

	handler := writableRepoRoute(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		body, _ := io.ReadAll(r.Body)
		attempts++
		if attempts == 1 {
			w.Header().Set("Retry-After", "0")
			w.Header().Set("Connection", "close")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		retriedBody = string(body)
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"id": 1}`))
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	client := newTestClient("test-token", server.URL)

	err := client.PostIssueComment(context.Background(), "owner", "repo", 42, "replayed body")
	if err != nil {
		t.Fatalf("expected retried POST to succeed, got: %v", err)
	}
	if attempts != 2 {
		t.Errorf("expected 2 POST attempts, got %d", attempts)
	}
	if !strings.Contains(retriedBody, "replayed body") {
		t.Errorf("retried POST resent a consumed body, got: %q", retriedBody)
	}
}

func TestCalculateBackoff(t *testing.T) {
	client := NewClient("")

	backoff := InitialBackoff
	backoff = client.calculateBackoff(backoff, 0)

	// Should be roughly 2 * InitialBackoff (with jitter)
	if backoff < InitialBackoff || backoff > 4*InitialBackoff {
		t.Errorf("expected backoff between %v and %v, got %v", InitialBackoff, 4*InitialBackoff, backoff)
	}
}

func TestGetRetryAfter_Seconds(t *testing.T) {
	client := NewClient("")

	resp := &http.Response{
		Header: http.Header{
			"Retry-After": []string{"5"},
		},
	}

	duration := client.getRetryAfter(resp)

	if duration != 5*time.Second {
		t.Errorf("expected 5s, got %v", duration)
	}
}

func TestGetRetryAfter_RateLimitReset(t *testing.T) {
	client := NewClient("")

	// Use a longer future time to avoid timing issues
	future := time.Now().Add(30 * time.Second)
	timestamp := strconv.FormatInt(future.Unix(), 10)

	resp := &http.Response{
		Header: http.Header{
			"X-RateLimit-Reset": []string{timestamp},
		},
	}

	duration := client.getRetryAfter(resp)

	// Should be around 30 seconds (with tolerance for execution time)
	// If it returns 60s, it means parsing failed - that's acceptable as a fallback
	if duration != 60*time.Second && (duration < 25*time.Second || duration > 35*time.Second) {
		t.Errorf("expected ~30s or 60s (default fallback), got %v", duration)
	}
}

func TestGetRetryAfter_Default(t *testing.T) {
	client := NewClient("")

	resp := &http.Response{
		Header: http.Header{},
	}

	duration := client.getRetryAfter(resp)

	if duration != 60*time.Second {
		t.Errorf("expected default 60s, got %v", duration)
	}
}

func TestDecodeJSON_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"key":"value"}`))
	}))
	defer server.Close()

	resp, _ := http.Get(server.URL)

	var result map[string]string
	err := decodeJSON(resp, &result)

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if result["key"] != "value" {
		t.Errorf("expected 'value', got: %s", result["key"])
	}
}

func TestDecodeJSON_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"bad request"}`))
	}))
	defer server.Close()

	resp, _ := http.Get(server.URL)

	var result map[string]string
	err := decodeJSON(resp, &result)

	if err == nil {
		t.Error("expected error for 400 status, got nil")
	}
}

func TestGetPullRequestDiff_Success(t *testing.T) {
	diffContent := `diff --git a/file.go b/file.go
index 1234567..abcdefg 100644
--- a/file.go
+++ b/file.go
@@ -1,3 +1,4 @@
 package main

+import "fmt"
 func main() {
`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify headers
		if accept := r.Header.Get("Accept"); accept != "application/vnd.github.v3.diff" {
			t.Errorf("expected Accept header application/vnd.github.v3.diff, got %s", accept)
		}

		// Return diff with ETag
		w.Header().Set("ETag", `"abc123"`)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(diffContent))
	}))
	defer server.Close()

	client := newTestClient("test-token", server.URL)

	diff, err := client.GetPullRequestDiff(context.Background(), "owner", "repo", 42)

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if len(diff.Files) != 1 {
		t.Errorf("expected 1 file, got %d", len(diff.Files))
	}

	if diff.Files[0].Path != "file.go" {
		t.Errorf("expected path 'file.go', got %s", diff.Files[0].Path)
	}

	if len(diff.Files[0].Hunks) != 1 {
		t.Errorf("expected 1 hunk, got %d", len(diff.Files[0].Hunks))
	}
}

func TestGetPullRequestDiff_CachedETag(t *testing.T) {
	diffContent := `diff --git a/test.go b/test.go
@@ -1,1 +1,2 @@
+added line
 existing line
`

	attempts := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++

		if attempts == 1 {
			// First request - return with ETag
			w.Header().Set("ETag", `"xyz789"`)
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(diffContent))
			return
		}

		// Second request - check for If-None-Match
		if ifNoneMatch := r.Header.Get("If-None-Match"); ifNoneMatch != `"xyz789"` {
			t.Errorf("expected If-None-Match header with ETag, got %s", ifNoneMatch)
		}

		// Return 304 Not Modified
		w.WriteHeader(http.StatusNotModified)
	}))
	defer server.Close()

	client := newTestClient("test-token", server.URL)

	// First request - should cache
	diff1, err := client.GetPullRequestDiff(context.Background(), "owner", "repo", 100)
	if err != nil {
		t.Fatalf("first request failed: %v", err)
	}

	// Second request - should use cache
	diff2, err := client.GetPullRequestDiff(context.Background(), "owner", "repo", 100)
	if err != nil {
		t.Fatalf("second request failed: %v", err)
	}

	if attempts != 2 {
		t.Errorf("expected 2 attempts, got %d", attempts)
	}

	// Diffs should be identical (cached)
	if len(diff1.Files) != len(diff2.Files) {
		t.Errorf("cached diff differs from original")
	}
}

func TestGetPullRequestDiff_404(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"message":"Not Found"}`))
	}))
	defer server.Close()

	client := newTestClient("test-token", server.URL)

	_, err := client.GetPullRequestDiff(context.Background(), "owner", "repo", 999)

	if err == nil {
		t.Error("expected error for 404, got nil")
	}

	if !strings.Contains(err.Error(), "404") {
		t.Errorf("expected 404 in error message, got: %v", err)
	}
}

func TestParseDiff_MultipleFiles(t *testing.T) {
	diffContent := `diff --git a/file1.go b/file1.go
@@ -1,2 +1,3 @@
 line 1
+added line
 line 2
diff --git a/file2.go b/file2.go
@@ -10,1 +10,2 @@
 old line
+new line
`

	diff, err := parseDiff(diffContent)

	if err != nil {
		t.Fatalf("parseDiff failed: %v", err)
	}

	if len(diff.Files) != 2 {
		t.Errorf("expected 2 files, got %d", len(diff.Files))
	}

	if diff.Files[0].Path != "file1.go" {
		t.Errorf("expected file1.go, got %s", diff.Files[0].Path)
	}

	if diff.Files[1].Path != "file2.go" {
		t.Errorf("expected file2.go, got %s", diff.Files[1].Path)
	}
}

func TestParseDiff_EmptyDiff(t *testing.T) {
	diff, err := parseDiff("")

	if err != nil {
		t.Fatalf("parseDiff failed on empty: %v", err)
	}

	if len(diff.Files) != 0 {
		t.Errorf("expected 0 files for empty diff, got %d", len(diff.Files))
	}
}

func TestExtractFilePath_DirectoryEndingInB(t *testing.T) {
	// Restoration defect 4: the first literal "b/" in the header lives
	// inside "cmdb/", so the restored version returned "settings.go".
	tests := []struct {
		line     string
		expected string
	}{
		{"diff --git a/cmdb/settings.go b/cmdb/settings.go", "cmdb/settings.go"},
		{"diff --git a/b/nested.go b/b/nested.go", "b/nested.go"},
		{"diff --git a/file.go b/file.go", "file.go"},
		{"no separator here", ""},
	}

	for _, test := range tests {
		if got := extractFilePath(test.line); got != test.expected {
			t.Errorf("extractFilePath(%q): expected %q, got %q", test.line, test.expected, got)
		}
	}
}

func TestParseHunkHeader(t *testing.T) {
	tests := []struct {
		line     string
		expected DiffHunk
	}{
		{
			line: "@@ -10,5 +10,7 @@ func main() {",
			expected: DiffHunk{
				OldStart: 10,
				OldLines: 5,
				NewStart: 10,
				NewLines: 7,
				Lines:    []string{},
			},
		},
		{
			line: "@@ -1,1 +1,2 @@",
			expected: DiffHunk{
				OldStart: 1,
				OldLines: 1,
				NewStart: 1,
				NewLines: 2,
				Lines:    []string{},
			},
		},
	}

	for _, test := range tests {
		hunk := parseHunkHeader(test.line)

		if hunk.OldStart != test.expected.OldStart {
			t.Errorf("OldStart: expected %d, got %d", test.expected.OldStart, hunk.OldStart)
		}
		if hunk.OldLines != test.expected.OldLines {
			t.Errorf("OldLines: expected %d, got %d", test.expected.OldLines, hunk.OldLines)
		}
		if hunk.NewStart != test.expected.NewStart {
			t.Errorf("NewStart: expected %d, got %d", test.expected.NewStart, hunk.NewStart)
		}
		if hunk.NewLines != test.expected.NewLines {
			t.Errorf("NewLines: expected %d, got %d", test.expected.NewLines, hunk.NewLines)
		}
	}
}

func TestListChangedFiles_SinglePage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[
			{"filename": "file1.go"},
			{"filename": "file2.go"},
			{"filename": "file3.go"}
		]`))
	}))
	defer server.Close()

	client := newTestClient("test-token", server.URL)

	files, err := client.ListChangedFiles(context.Background(), "owner", "repo", 42)

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if len(files) != 3 {
		t.Errorf("expected 3 files, got %d", len(files))
	}

	expected := []string{"file1.go", "file2.go", "file3.go"}
	for i, file := range files {
		if file != expected[i] {
			t.Errorf("file %d: expected %s, got %s", i, expected[i], file)
		}
	}
}

func TestListChangedFiles_MultiplePages(t *testing.T) {
	page := 0
	var baseURL string

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page++

		if page == 1 {
			// First page with Link header to next page
			w.Header().Set("Link", fmt.Sprintf(`<%s/repos/owner/repo/pulls/42/files?page=2>; rel="next"`, baseURL))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`[
				{"filename": "page1_file1.go"},
				{"filename": "page1_file2.go"}
			]`))
			return
		}

		// Second page (no more pages)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[
			{"filename": "page2_file1.go"}
		]`))
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	baseURL = server.URL

	client := newTestClient("test-token", server.URL)

	files, err := client.ListChangedFiles(context.Background(), "owner", "repo", 42)

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if len(files) != 3 {
		t.Errorf("expected 3 files across pages, got %d", len(files))
	}

	expected := []string{"page1_file1.go", "page1_file2.go", "page2_file1.go"}
	for i, file := range files {
		if file != expected[i] {
			t.Errorf("file %d: expected %s, got %s", i, expected[i], file)
		}
	}

	if page != 2 {
		t.Errorf("expected 2 requests, got %d", page)
	}
}

func TestListChangedFiles_EmptyResult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[]`))
	}))
	defer server.Close()

	client := newTestClient("test-token", server.URL)

	files, err := client.ListChangedFiles(context.Background(), "owner", "repo", 42)

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if len(files) != 0 {
		t.Errorf("expected 0 files, got %d", len(files))
	}
}

func TestListChangedFiles_404(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"message":"Not Found"}`))
	}))
	defer server.Close()

	client := newTestClient("test-token", server.URL)

	_, err := client.ListChangedFiles(context.Background(), "owner", "repo", 999)

	if err == nil {
		t.Error("expected error for 404, got nil")
	}

	if !strings.Contains(err.Error(), "404") {
		t.Errorf("expected 404 in error message, got: %v", err)
	}
}

func TestParseNextLink(t *testing.T) {
	client := NewClient("")

	tests := []struct {
		name     string
		header   string
		expected string
	}{
		{
			name:     "with next link",
			header:   `<https://api.github.com/repos/owner/repo/pulls/42/files?page=2>; rel="next", <https://api.github.com/repos/owner/repo/pulls/42/files?page=3>; rel="last"`,
			expected: "https://api.github.com/repos/owner/repo/pulls/42/files?page=2",
		},
		{
			name:     "no next link",
			header:   `<https://api.github.com/repos/owner/repo/pulls/42/files?page=1>; rel="prev"`,
			expected: "",
		},
		{
			name:     "empty header",
			header:   "",
			expected: "",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := client.parseNextLink(test.header)
			if result != test.expected {
				t.Errorf("expected %s, got %s", test.expected, result)
			}
		})
	}
}

func TestPostReviewComment_Success(t *testing.T) {
	handler := writableRepoRoute(func(w http.ResponseWriter, r *http.Request) {
		// Verify method
		if r.Method != "POST" {
			t.Errorf("expected POST method, got %s", r.Method)
		}

		// Verify Content-Type
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", ct)
		}

		// Verify idempotency key
		if key := r.Header.Get("X-GitHub-Idempotency-Key"); key != "test-key-123" {
			t.Errorf("expected idempotency key test-key-123, got %s", key)
		}

		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"id": 12345}`))
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	client := newTestClient("test-token", server.URL)

	comment := ReviewComment{
		Body:     "This looks good!",
		CommitID: "abc123",
		Path:     "file.go",
		Line:     42,
	}

	err := client.PostReviewComment(context.Background(), "owner", "repo", 100, comment, "test-key-123")

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestPostReviewComment_WithoutIdempotencyKey(t *testing.T) {
	handler := writableRepoRoute(func(w http.ResponseWriter, r *http.Request) {
		// Verify no idempotency key
		if key := r.Header.Get("X-GitHub-Idempotency-Key"); key != "" {
			t.Errorf("expected no idempotency key, got %s", key)
		}

		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"id": 12345}`))
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	client := newTestClient("test-token", server.URL)

	comment := ReviewComment{
		Body:     "Comment without idempotency",
		CommitID: "xyz789",
		Path:     "test.go",
		Line:     10,
	}

	err := client.PostReviewComment(context.Background(), "owner", "repo", 100, comment, "")

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestPostReviewComment_422UnprocessableEntity(t *testing.T) {
	handler := writableRepoRoute(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		w.Write([]byte(`{"message":"Validation Failed","errors":[{"message":"Invalid line"}]}`))
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	client := newTestClient("test-token", server.URL)

	comment := ReviewComment{
		Body:     "Invalid comment",
		CommitID: "abc",
		Path:     "missing.go",
		Line:     -1,
	}

	err := client.PostReviewComment(context.Background(), "owner", "repo", 100, comment, "key")

	if err == nil {
		t.Error("expected error for 422, got nil")
	}

	if !strings.Contains(err.Error(), "422") {
		t.Errorf("expected 422 in error message, got: %v", err)
	}
}

func TestPostReviewComment_IdempotencyDuplicate(t *testing.T) {
	postAttempts := 0

	handler := writableRepoRoute(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			postAttempts++
		}

		// Both requests should succeed (idempotency prevents duplicate)
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"id": 12345}`))
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	client := newTestClient("test-token", server.URL)

	comment := ReviewComment{
		Body:     "Duplicate comment test",
		CommitID: "abc123",
		Path:     "file.go",
		Line:     42,
	}

	// First request
	err1 := client.PostReviewComment(context.Background(), "owner", "repo", 100, comment, "duplicate-key")
	if err1 != nil {
		t.Fatalf("first request failed: %v", err1)
	}

	// Second request with same idempotency key
	err2 := client.PostReviewComment(context.Background(), "owner", "repo", 100, comment, "duplicate-key")
	if err2 != nil {
		t.Fatalf("second request failed: %v", err2)
	}

	// Both should have been sent (GitHub handles deduplication)
	if postAttempts != 2 {
		t.Errorf("expected 2 POST attempts, got %d", postAttempts)
	}
}

func TestPublish_RefusedWithReadOnlyToken(t *testing.T) {
	// The write-permission gate: a token whose repository answer carries
	// push=false must be refused before any POST reaches the server.
	posted := false

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/repos/owner/repo" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"permissions":{"admin":false,"maintain":false,"push":false,"pull":true}}`))
			return
		}
		if r.Method == http.MethodPost {
			posted = true
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"id": 1}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := newTestClient("read-only-synthetic", server.URL)
	ctx := context.Background()

	if err := client.PostIssueComment(ctx, "owner", "repo", 42, "should not publish"); !errors.Is(err, ErrNoWritePermission) {
		t.Fatalf("PostIssueComment: expected ErrNoWritePermission, got: %v", err)
	}
	if err := client.PostReviewComment(ctx, "owner", "repo", 42, ReviewComment{Body: "x", CommitID: "sha", Path: "f.go", Line: 1}, "k"); !errors.Is(err, ErrNoWritePermission) {
		t.Fatalf("PostReviewComment: expected ErrNoWritePermission, got: %v", err)
	}
	if err := client.SetStatus(ctx, "owner", "repo", "sha", CommitStatus{State: "success", Context: "c"}); !errors.Is(err, ErrNoWritePermission) {
		t.Fatalf("SetStatus: expected ErrNoWritePermission, got: %v", err)
	}

	if posted {
		t.Fatal("a POST reached the server despite the read-only token")
	}
}

func TestPullRequestWritesUseEndpointPermission(t *testing.T) {
	// A GitHub Actions token can have pull-requests:write while the
	// repository-role response reports push=false. PR publishing must use the
	// actual endpoint instead of requiring contents:write.
	var repoPermissionProbe bool
	var posts int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/repos/owner/repo" {
			repoPermissionProbe = true
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"permissions":{"admin":false,"maintain":false,"push":false,"pull":true}}`))
			return
		}
		if r.Method == http.MethodPost && r.URL.Path == "/repos/owner/repo/issues/42/comments" {
			posts++
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":1}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := newTestClient("actions-token", server.URL)
	client.AllowPullRequestWrites()
	if err := client.PostIssueComment(context.Background(), "owner", "repo", 42, "review summary"); err != nil {
		t.Fatalf("PostIssueComment: %v", err)
	}
	if repoPermissionProbe {
		t.Fatal("PR publishing performed the repository push-permission preflight")
	}
	if posts != 1 {
		t.Fatalf("expected one comment POST, got %d", posts)
	}
}

func TestPullRequestWritesSurfaceEndpointDenial(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/repos/owner/repo/issues/42/comments" {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"message":"Resource not accessible by integration"}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := newTestClient("actions-token", server.URL)
	client.AllowPullRequestWrites()
	err := client.PostIssueComment(context.Background(), "owner", "repo", 42, "review summary")
	if err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("expected the endpoint denial, got %v", err)
	}
}

func TestSetStatus_Success(t *testing.T) {
	handler := writableRepoRoute(func(w http.ResponseWriter, r *http.Request) {
		// Verify method
		if r.Method != "POST" {
			t.Errorf("expected POST method, got %s", r.Method)
		}

		// Verify Content-Type
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", ct)
		}

		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"id": 1, "state": "success"}`))
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	client := newTestClient("test-token", server.URL)

	status := CommitStatus{
		State:       "success",
		TargetURL:   "https://example.com/build/123",
		Description: "Build passed",
		Context:     "ci/build",
	}

	err := client.SetStatus(context.Background(), "owner", "repo", "abc123", status)

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestSetStatus_AllValidStates(t *testing.T) {
	validStates := []string{"pending", "success", "error", "failure"}

	for _, state := range validStates {
		t.Run(state, func(t *testing.T) {
			handler := writableRepoRoute(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusCreated)
				w.Write([]byte(`{"state": "` + state + `"}`))
			})
			server := httptest.NewServer(handler)
			defer server.Close()

			client := newTestClient("test-token", server.URL)

			status := CommitStatus{
				State:       state,
				Description: "Test status",
				Context:     "test/status",
			}

			err := client.SetStatus(context.Background(), "owner", "repo", "sha", status)

			if err != nil {
				t.Errorf("expected no error for state %s, got: %v", state, err)
			}
		})
	}
}

func TestSetStatus_InvalidState(t *testing.T) {
	client := NewClient("test-token")

	status := CommitStatus{
		State:       "invalid_state",
		Description: "Test",
		Context:     "test",
	}

	err := client.SetStatus(context.Background(), "owner", "repo", "sha", status)

	if err == nil {
		t.Error("expected error for invalid state, got nil")
	}

	if !strings.Contains(err.Error(), "invalid state") {
		t.Errorf("expected 'invalid state' in error message, got: %v", err)
	}
}

func TestSetStatus_404NotFound(t *testing.T) {
	handler := writableRepoRoute(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"message":"Not Found"}`))
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	client := newTestClient("test-token", server.URL)

	status := CommitStatus{
		State:       "success",
		Description: "Test",
		Context:     "test",
	}

	err := client.SetStatus(context.Background(), "owner", "repo", "invalid_sha", status)

	if err == nil {
		t.Error("expected error for 404, got nil")
	}

	if !strings.Contains(err.Error(), "404") {
		t.Errorf("expected 404 in error message, got: %v", err)
	}
}

func TestSetStatus_WithoutOptionalFields(t *testing.T) {
	handler := writableRepoRoute(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"state": "pending"}`))
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	client := newTestClient("test-token", server.URL)

	// Minimal status without optional fields
	status := CommitStatus{
		State:   "pending",
		Context: "ci/test",
	}

	err := client.SetStatus(context.Background(), "owner", "repo", "sha", status)

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}
