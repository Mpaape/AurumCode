package githubclient

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// End-to-end integration tests for GitHub client

func TestIntegration_PullRequestReviewFlow(t *testing.T) {
	// Simulate a complete PR review flow:
	// 1. Get PR diff
	// 2. List changed files
	// 3. Post review comments
	// 4. Set commit status

	prNumber := 42
	commitSHA := "abc123def456"
	requestCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++

		switch {
		// GetPullRequestDiff
		case strings.Contains(r.URL.Path, fmt.Sprintf("/pulls/%d", prNumber)) && !strings.Contains(r.URL.Path, "/files") && !strings.Contains(r.URL.Path, "/comments"):
			w.Header().Set("ETag", `"test-etag"`)
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`diff --git a/main.go b/main.go
@@ -1,1 +1,2 @@
+package main
 func main() {}`))

		// ListChangedFiles
		case strings.Contains(r.URL.Path, fmt.Sprintf("/pulls/%d/files", prNumber)):
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`[
				{"filename": "main.go"},
				{"filename": "utils.go"}
			]`))

		// PostReviewComment
		case strings.Contains(r.URL.Path, fmt.Sprintf("/pulls/%d/comments", prNumber)):
			if r.Method != "POST" {
				t.Errorf("expected POST for comment, got %s", r.Method)
			}
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"id": 123}`))

		// SetStatus
		case strings.Contains(r.URL.Path, fmt.Sprintf("/statuses/%s", commitSHA)):
			if r.Method != "POST" {
				t.Errorf("expected POST for status, got %s", r.Method)
			}
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"state": "success"}`))

		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := NewClientWithBaseURL("test-token", server.URL)
	ctx := context.Background()

	// Step 1: Get PR diff
	diff, err := client.GetPullRequestDiff(ctx, "owner", "repo", prNumber)
	if err != nil {
		t.Fatalf("GetPullRequestDiff failed: %v", err)
	}
	if len(diff.Files) == 0 {
		t.Error("expected files in diff, got none")
	}

	// Step 2: List changed files
	files, err := client.ListChangedFiles(ctx, "owner", "repo", prNumber)
	if err != nil {
		t.Fatalf("ListChangedFiles failed: %v", err)
	}
	if len(files) != 2 {
		t.Errorf("expected 2 files, got %d", len(files))
	}

	// Step 3: Post review comments
	for _, file := range files {
		comment := ReviewComment{
			Body:     fmt.Sprintf("Review comment for %s", file),
			CommitID: commitSHA,
			Path:     file,
			Line:     1,
		}

		err := client.PostReviewComment(ctx, "owner", "repo", prNumber, comment, fmt.Sprintf("key-%s", file))
		if err != nil {
			t.Errorf("PostReviewComment failed for %s: %v", file, err)
		}
	}

	// Step 4: Set commit status
	status := CommitStatus{
		State:       "success",
		Description: "All checks passed",
		Context:     "aurumcode/review",
	}

	err = client.SetStatus(ctx, "owner", "repo", commitSHA, status)
	if err != nil {
		t.Fatalf("SetStatus failed: %v", err)
	}

	// Verify all requests were made
	expectedRequests := 5 // 1 diff + 1 files + 2 comments + 1 status
	if requestCount != expectedRequests {
		t.Errorf("expected %d requests, got %d", expectedRequests, requestCount)
	}
}

func TestIntegration_ETagCachingAcrossRequests(t *testing.T) {
	// Test that ETag caching works across multiple requests

	prNumber := 100
	requestCount := 0
	etag := `"cache-test-etag"`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++

		if requestCount == 1 {
			// First request - return with ETag
			w.Header().Set("ETag", etag)
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`diff --git a/file.go b/file.go
@@ -1,1 +1,1 @@
-old line
+new line`))
			return
		}

		// Subsequent requests - check for If-None-Match
		if ifNoneMatch := r.Header.Get("If-None-Match"); ifNoneMatch != etag {
			t.Errorf("expected If-None-Match %s, got %s", etag, ifNoneMatch)
		}

		// Return 304 Not Modified
		w.WriteHeader(http.StatusNotModified)
	}))
	defer server.Close()

	client := NewClientWithBaseURL("test-token", server.URL)
	ctx := context.Background()

	// First request - should fetch and cache
	diff1, err := client.GetPullRequestDiff(ctx, "owner", "repo", prNumber)
	if err != nil {
		t.Fatalf("first request failed: %v", err)
	}

	// Second request - should use cache (304)
	diff2, err := client.GetPullRequestDiff(ctx, "owner", "repo", prNumber)
	if err != nil {
		t.Fatalf("second request failed: %v", err)
	}

	// Third request - should also use cache
	diff3, err := client.GetPullRequestDiff(ctx, "owner", "repo", prNumber)
	if err != nil {
		t.Fatalf("third request failed: %v", err)
	}

	// All diffs should be identical
	if len(diff1.Files) != len(diff2.Files) || len(diff1.Files) != len(diff3.Files) {
		t.Error("cached diffs differ from original")
	}

	if requestCount != 3 {
		t.Errorf("expected 3 requests, got %d", requestCount)
	}
}

func TestIntegration_PaginationWithMultiplePages(t *testing.T) {
	// Test pagination across multiple pages

	prNumber := 200
	page := 0
	var baseURL string

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page++

		w.Header().Set("Content-Type", "application/json")

		switch page {
		case 1:
			// Page 1 - link to page 2
			w.Header().Set("Link", fmt.Sprintf(`<%s/repos/owner/repo/pulls/200/files?page=2>; rel="next"`, baseURL))
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`[
				{"filename": "file1.go"},
				{"filename": "file2.go"}
			]`))

		case 2:
			// Page 2 - link to page 3
			w.Header().Set("Link", fmt.Sprintf(`<%s/repos/owner/repo/pulls/200/files?page=3>; rel="next"`, baseURL))
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`[
				{"filename": "file3.go"}
			]`))

		case 3:
			// Page 3 - no more pages
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`[
				{"filename": "file4.go"},
				{"filename": "file5.go"}
			]`))

		default:
			t.Errorf("unexpected page %d", page)
			w.WriteHeader(http.StatusInternalServerError)
		}
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	baseURL = server.URL

	client := NewClientWithBaseURL("test-token", server.URL)

	files, err := client.ListChangedFiles(context.Background(), "owner", "repo", prNumber)
	if err != nil {
		t.Fatalf("ListChangedFiles failed: %v", err)
	}

	// Should have collected all files from all pages
	if len(files) != 5 {
		t.Errorf("expected 5 files from 3 pages, got %d", len(files))
	}

	expected := []string{"file1.go", "file2.go", "file3.go", "file4.go", "file5.go"}
	for i, file := range files {
		if file != expected[i] {
			t.Errorf("file %d: expected %s, got %s", i, expected[i], file)
		}
	}

	if page != 3 {
		t.Errorf("expected 3 pages, got %d", page)
	}
}

func TestIntegration_ErrorHandlingAndRetry(t *testing.T) {
	// Test that errors are handled correctly and retries work

	attempts := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++

		if attempts < 3 {
			// First two attempts - return server error (should retry)
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"message":"Internal Server Error"}`))
			return
		}

		// Third attempt - success
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[{"filename": "retried.go"}]`))
	}))
	defer server.Close()

	client := NewClientWithBaseURL("test-token", server.URL)

	files, err := client.ListChangedFiles(context.Background(), "owner", "repo", 42)

	if err != nil {
		t.Fatalf("expected success after retries, got error: %v", err)
	}

	if len(files) != 1 {
		t.Errorf("expected 1 file, got %d", len(files))
	}

	// Should have retried
	if attempts != 3 {
		t.Errorf("expected 3 attempts (2 failures + 1 success), got %d", attempts)
	}
}

func TestIntegration_ConcurrentRequests(t *testing.T) {
	// Test that concurrent requests work correctly

	requestCount := 0
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requestCount++
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`[{"filename": "concurrent.go"}]`))
	}))
	defer server.Close()

	client := NewClientWithBaseURL("test-token", server.URL)
	ctx := context.Background()

	// Launch 10 concurrent requests
	concurrency := 10
	done := make(chan error, concurrency)

	for i := 0; i < concurrency; i++ {
		go func(prNum int) {
			_, err := client.ListChangedFiles(ctx, "owner", "repo", prNum)
			done <- err
		}(i)
	}

	// Wait for all to complete
	for i := 0; i < concurrency; i++ {
		if err := <-done; err != nil {
			t.Errorf("concurrent request %d failed: %v", i, err)
		}
	}

	mu.Lock()
	finalCount := requestCount
	mu.Unlock()

	if finalCount != concurrency {
		t.Errorf("expected %d requests, got %d", concurrency, finalCount)
	}
}
