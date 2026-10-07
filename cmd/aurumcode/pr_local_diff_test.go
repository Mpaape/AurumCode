package main

import (
	"os"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/security/redaction"
)

// absentBaseSHA is a well-formed commit id that no fixture checkout has.
const absentBaseSHA = "0123456789abcdef0123456789abcdef01234567"

// The API refuses the diff for its size and neither the event's base nor the
// base the API reports is in the verified checkout (a shallow clone): the
// range is never guessed, so the review fails closed with nothing sent to the
// model and nothing published.
func TestTooLargeDiffWithBaseAbsentFromCheckoutFailsClosed(t *testing.T) {
	_, _, head := aur594ObjectCheckout(t)
	var body struct{ Body string }
	server := aur594Server(t, absentBaseSHA, head, &body)
	capturePath := aur515Env(t, server.URL)
	t.Setenv("AURUMCODE_BASE_SHA", absentBaseSHA)
	var out, errOut strings.Builder
	code := runPRReview(reviewIO{stdout: &out, stderr: &errOut, filter: redaction.NewFilter()}, prReviewOptions{prNumber: 48, repo: "owner/repo", publicar: true,
		publicationSet: true, publication: "review"})
	captured, _ := os.ReadFile(capturePath)
	stderr, prompt, posted := out.String()+errOut.String(), string(captured), body.Body
	if code != 1 || posted != "" || prompt != "" {
		t.Fatalf("exit=%d posted=%q prompt=%d bytes, want exit 1 and nothing sent or published\n%s", code, posted, len(prompt), stderr)
	}
	// With git the range resolver refuses the base; without git (object
	// database reader) reading the absent commit fails. Either way the
	// review declares that the checkout could not yield the diff.
	if !strings.Contains(stderr, "could not be computed from the checkout either") {
		t.Fatalf("the failure must say the checkout could not yield the diff\n%s", stderr)
	}
}
