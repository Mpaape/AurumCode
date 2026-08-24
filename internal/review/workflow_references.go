package review

import (
	"strings"

	"github.com/Mpaape/AurumCode/pkg/types"
)

// suppressWorkflowReferenceFindings removes only model-authored hardcoded
// secret findings that point at an unmistakably non-secret GitHub Actions
// configuration line. The model must not turn a workflow's permission scope,
// event list, action reference, or secret indirection into a blocking issue.
// Literal values remain untouched and are still covered by the deterministic
// security pass.
func suppressWorkflowReferenceFindings(diff *types.Diff, issues []types.ReviewIssue) []types.ReviewIssue {
	kept := make([]types.ReviewIssue, 0, len(issues))
	for _, issue := range issues {
		line := workflowDiffLine(diff, issue.File, issue.Line)
		if issue.RuleID == "security/hardcoded-secret" && safeWorkflowReference(issue.File, line) {
			continue
		}
		kept = append(kept, issue)
	}
	return kept
}

func workflowDiffLine(diff *types.Diff, path string, wanted int) string {
	if diff == nil || wanted <= 0 {
		return ""
	}
	for _, file := range diff.Files {
		if file.Path != path {
			continue
		}
		for _, hunk := range file.Hunks {
			lineNumber := hunk.NewStart
			for _, raw := range hunk.Lines {
				if raw == "" {
					continue
				}
				marker, body := splitDiffMarker(raw)
				if marker == "-" {
					continue
				}
				if lineNumber == wanted {
					return body
				}
				lineNumber++
			}
		}
	}
	return ""
}

func safeWorkflowReference(path, line string) bool {
	if !strings.HasPrefix(path, ".github/workflows/") {
		return false
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return false
	}

	// These are references resolved by GitHub Actions, never credential
	// material committed in the source tree.
	if strings.Contains(line, "${{") &&
		(strings.Contains(line, "secrets.") || strings.Contains(line, "github.")) {
		return true
	}
	if strings.HasPrefix(line, "types:") ||
		strings.HasPrefix(line, "uses:") ||
		strings.HasPrefix(line, "permissions:") {
		return true
	}

	// Permission names are capability labels. They are safe even when they
	// contain a digit or punctuation that resembles a token fragment.
	for _, key := range []string{"contents", "pull-requests", "statuses", "checks", "actions", "packages", "deployments", "issues"} {
		prefix := key + ":"
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		value := strings.TrimSpace(strings.TrimPrefix(line, prefix))
		return value == "read" || value == "write" || value == "none"
	}
	return false
}
