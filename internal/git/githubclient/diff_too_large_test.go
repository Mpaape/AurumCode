package githubclient

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Only GitHub's size refusal (406 with "too_large") is ErrDiffTooLarge; any
// other 406 stays an ordinary failure, and neither is ever an empty diff.
func TestDiffRefusedAsTooLargeIsTyped(t *testing.T) {
	for name, tc := range map[string]struct {
		body     string
		tooLarge bool
	}{
		"too_large": {`{"errors":[{"resource":"PullRequest","field":"diff","code":"too_large"}]}`, true},
		"other_406": {`{"message":"Unsupported media type"}`, false},
	} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNotAcceptable)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			diff, err := NewClientWithBaseURL("token", server.URL).GetPullRequestDiff(context.Background(), "owner", "repo", 1)
			if err == nil || diff != nil {
				t.Fatalf("a 406 must fail, never yield a diff: diff=%v err=%v", diff, err)
			}
			if errors.Is(err, ErrDiffTooLarge) != tc.tooLarge {
				t.Fatalf("errors.Is(err, ErrDiffTooLarge) = %v, want %v (%v)", !tc.tooLarge, tc.tooLarge, err)
			}
		})
	}
}

// A side without a count is one line, and a deleted one-line file's header
// ("@@ -1 +0,0 @@") parses instead of panicking.
func TestParseHunkHeaderWithoutCount(t *testing.T) {
	for line, want := range map[string][4]int{
		"@@ -1 +0,0 @@":            {1, 1, 0, 0},
		"@@ -0,0 +1 @@":            {0, 0, 1, 1},
		"@@ -10,5 +10,7 @@ func x": {10, 5, 10, 7},
		"@@ malformed":             {0, 0, 0, 0},
	} {
		h := parseHunkHeader(line)
		if got := [4]int{h.OldStart, h.OldLines, h.NewStart, h.NewLines}; got != want {
			t.Errorf("parseHunkHeader(%q) = %v, want %v", line, got, want)
		}
	}
}
