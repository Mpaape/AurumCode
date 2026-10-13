package githubclient

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestAUR619APIErrorKeepsTheHTTPLine: the typed answer prints exactly the
// line the client always printed, so every reader of the text is unchanged.
func TestAUR619APIErrorKeepsTheHTTPLine(t *testing.T) {
	err := apiError(http.StatusForbidden, []byte(`{"message":"Resource not accessible by integration"}`))
	if got, want := err.Error(), `HTTP 403: {"message":"Resource not accessible by integration"}`; got != want {
		t.Fatalf("text %q, want %q", got, want)
	}
}

// TestAUR619PermissionDeniedStatuses: only a refused credential (401, 403)
// is a permission denial, through any wrapping; nil and other answers are not.
func TestAUR619PermissionDeniedStatuses(t *testing.T) {
	for status, want := range map[int]bool{401: true, 403: true, 404: false, 422: false, 500: false} {
		if got := IsPermissionDenied(apiError(status, []byte("x"))); got != want {
			t.Fatalf("status %d: denied=%v, want %v", status, got, want)
		}
	}
	wrapped := fmt.Errorf("publishing: %w", apiError(403, []byte("x")))
	if !IsPermissionDenied(wrapped) {
		t.Fatal("a wrapped 403 must still be a permission denial")
	}
	tooLarge := fmt.Errorf("%w: %w", apiError(406, []byte("big")), ErrDiffTooLarge)
	if !errors.Is(tooLarge, ErrDiffTooLarge) || IsPermissionDenied(tooLarge) {
		t.Fatalf("diff too large keeps its sentinel and is no permission denial: %v", tooLarge)
	}
	if IsPermissionDenied(nil) || IsPermissionDenied(errors.New("HTTP 403: text only")) {
		t.Fatal("nil and an untyped text are not permission denials")
	}
}

// TestAUR619ClientAnswersAreTyped: every write the client makes surfaces a
// refused answer as the typed error, so the caller can name the permission.
func TestAUR619ClientAnswersAreTyped(t *testing.T) {
	denied := `{"message":"Resource not accessible by integration","status":"403"}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/statuses/unprocessable") {
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(`{"message":"Validation Failed"}`))
			return
		}
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(denied))
	}))
	defer server.Close()
	client := NewClientWithBaseURL("token", server.URL)
	client.AllowPullRequestWrites()
	ctx := context.Background()
	status := CommitStatus{State: "failure", Context: "aurumcode/review"}
	calls := map[string]error{
		"issue comment":  client.PostIssueComment(ctx, "o", "r", 1, "body"),
		"inline comment": client.PostReviewComment(ctx, "o", "r", 1, ReviewComment{Body: "b", CommitID: "sha", Path: "a.go", Line: 1}, "key"),
		"formal review":  client.PostPullRequestReview(ctx, "o", "r", 1, PullRequestReview{Body: "b", Event: "COMMENT"}, "key"),
		"edit comment":   client.UpdateIssueComment(ctx, "o", "r", 7, "body"),
		"commit status":  client.SetStatus(ctx, "o", "r", "sha", status),
	}
	for name, err := range calls {
		if !IsPermissionDenied(err) || !strings.HasPrefix(err.Error(), "HTTP 403: ") {
			t.Fatalf("%s: %v, want a typed 403", name, err)
		}
	}
	if _, err := client.ListChangedFiles(ctx, "o", "r", 1); !IsPermissionDenied(err) {
		t.Fatalf("a read refused by permission is typed too: %v", err)
	}
	if err := client.SetStatus(ctx, "o", "r", "unprocessable", status); IsPermissionDenied(err) || !strings.HasPrefix(err.Error(), "HTTP 422: ") {
		t.Fatalf("422 is not a permission denial: %v", err)
	}
}
