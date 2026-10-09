package main

import (
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/security/redaction"
)

// TestOutsideDiffFindingNeverCountsForTheGate runs the real --pr session
// with one proved, error-severity finding on an unchanged line (app.go:1)
// under --fail-on error. The finding is published as a general comment
// headed by its location, while the session approves and exits 0: it never
// reached the threshold, the verdict or the policy gate (docs/specs/AUR-545.md).
func TestOutsideDiffFindingNeverCountsForTheGate(t *testing.T) {
	const marker = "AUR545-OUTSIDE-DIFF"
	response := strings.Replace(aur517Response(marker, 1), `"severity": "warning"`, `"severity": "error"`, 1)
	aur517Env(t, response)
	server, posted := aur517Server(t)
	aur517PREnv(t, server)

	var stdout, stderr strings.Builder
	code := runPRReview(reviewIO{stdout: &stdout, stderr: &stderr, filter: redaction.NewFilter()}, prReviewOptions{
		prNumber: 48, repo: "owner/repo", publicar: true, naLinha: true,
		failOnSet: true, failOn: "error",
	})
	body := strings.Join(*posted, "\n")
	if code != 0 {
		t.Fatalf("an outside-diff finding counted for the gate: exit=%d\nstdout=%s\nstderr=%s\nposted=%s", code, stdout.String(), stderr.String(), body)
	}
	if !strings.Contains(body, "`app.go:1`") || !strings.Contains(body, "ReadAll ignores the error fetch returns") || !strings.Contains(body, "outside the changed lines") {
		t.Fatalf("the proved outside-diff finding was not listed among the observations of the parecer:\n%s", body)
	}
	if strings.Contains(body, `"path":"app.go"`) {
		t.Fatalf("an outside-diff finding was anchored inline:\n%s", body)
	}
	if !strings.Contains(body, "[!NOTE]") || !strings.Contains(body, "Approved with 1 observation that does not block") {
		t.Fatalf("an outside-diff finding changed the decision:\n%s", body)
	}
	if line := firstLineWith(stdout.String(), "app.go:1: [error] ReadAll ignores the error fetch returns"); !strings.HasSuffix(line, inParecerMarker) {
		t.Fatalf("stdout does not report the finding read in the parecer:\n%s", stdout.String())
	}
}

// firstLineWith is the first line of text containing needle, or "".
func firstLineWith(text, needle string) string {
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(line, needle) {
			return line
		}
	}
	return ""
}
