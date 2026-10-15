package gitleaks

import (
	"strings"
	"testing"
)

// The commit gitleaks found a leak in travels with the finding, so the
// triage can show that commit's content (the reviewed tree may no longer
// have it); the secret itself is still dropped while decoding.
func TestDecodeReportCarriesTheCommit(t *testing.T) {
	commit := strings.Repeat("ab", 20)
	raw := `[{"RuleID":"generic-api-key","Description":"d","File":"./internal/plugin/aur622_test.go","StartLine":17,"Commit":"` + commit + `","Secret":"valor","Match":"token=valor"}]`
	findings, err := decodeReport([]byte(raw), "v8")
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 || findings[0].Commit != commit || findings[0].Path != "internal/plugin/aur622_test.go" {
		t.Fatalf("findings = %+v", findings)
	}
	if issue := findings[0].ToIssue("gitleaks"); issue.Commit != commit || strings.Contains(issue.Message, "valor") {
		t.Fatalf("issue = %+v", issue)
	}
}
