package gitleaks

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Mpaape/AurumCode/internal/scanner"
)

// commitShown is how many characters of a commit id a finding names.
const commitShown = 12

// leak is the allowlist of gitleaks report fields this adapter reads.
// There is deliberately no field for Secret, Match, Line, Message (the
// commit message), Author, Email, Fingerprint or Entropy: json.Unmarshal
// drops them while decoding, so the secret is discarded before anything
// else touches the report.
type leak struct {
	RuleID      string `json:"RuleID"`
	Description string `json:"Description"`
	File        string `json:"File"`
	StartLine   int    `json:"StartLine"`
	Commit      string `json:"Commit"`
}

// decodeReport turns the JSON report (a top-level array; `[]` is a clean
// scan) into findings. Anything else, or an entry without rule, file or
// line, is invalid output, never zero findings.
func decodeReport(raw []byte, version string) ([]scanner.Finding, error) {
	var leaks []leak
	if err := json.Unmarshal(raw, &leaks); err != nil || leaks == nil {
		return nil, fmt.Errorf("gitleaks: report is not a JSON array: %w", scanner.ErrInvalidOutput)
	}
	findings := make([]scanner.Finding, 0, len(leaks))
	for _, l := range leaks {
		if strings.TrimSpace(l.RuleID) == "" || strings.TrimSpace(l.File) == "" || l.StartLine <= 0 {
			return nil, fmt.Errorf("gitleaks: report entry without rule, file or line: %w", scanner.ErrInvalidOutput)
		}
		findings = append(findings, scanner.Finding{
			Path:     strings.TrimPrefix(l.File, "./"),
			Line:     l.StartLine,
			Side:     sideRight,
			RuleID:   RulePrefix + strings.TrimSpace(l.RuleID),
			Severity: severity,
			Message:  message(l, version),
		})
	}
	return findings, nil
}

// message names the rule's own description from the pinned rule base, the
// commit that introduced the leak (it may be gone from the final tree) and
// the engine identity. It never carries reviewed content.
func message(l leak, version string) string {
	desc := strings.TrimSpace(l.Description)
	if desc == "" {
		desc = "Secret detected"
	}
	commit := strings.TrimSpace(l.Commit)
	if len(commit) > commitShown {
		commit = commit[:commitShown]
	}
	return fmt.Sprintf("%s in commit %s (gitleaks %s)", desc, commit, version)
}
