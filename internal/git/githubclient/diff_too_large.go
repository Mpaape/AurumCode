package githubclient

import (
	"bytes"
	"errors"
	"net/http"
)

// ErrDiffTooLarge is returned by GetPullRequestDiff when the API refuses to
// render the pull request's diff because of its size (GitHub answers 406
// with the error code "too_large" above its line limit). It is never an empty
// diff: the caller either computes the same range elsewhere or fails.
var ErrDiffTooLarge = errors.New("the API refused the pull request diff as too large")

// tooLargeCode is the error code GitHub writes in the body of that refusal.
var tooLargeCode = []byte(`"too_large"`)

// diffRefusedAsTooLarge recognizes only the size refusal: status 406 with the
// "too_large" code. Any other 406 (an unsupported media type, a proxy) stays
// an ordinary failure.
func diffRefusedAsTooLarge(status int, body []byte) bool {
	return status == http.StatusNotAcceptable && bytes.Contains(body, tooLargeCode)
}

// ParseUnifiedDiff parses unified diff text with the same parser that reads
// the API's diff, so a diff computed elsewhere for the same range (the
// verified checkout, when the API refuses the size) has the same files, sides
// and hunks.
func ParseUnifiedDiff(text string) (*Diff, error) { return parseDiff(text) }
