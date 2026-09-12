package review

import (
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/Mpaape/AurumCode/pkg/types"
)

// findingScope is derived from the parsed patch that was actually sent to the
// model. It deliberately contains no language or repository-specific rules:
// the model remains responsible for understanding the code, while this small
// boundary prevents a response from acquiring authority over untouched code.
type findingScope struct {
	added map[string]map[int]struct{}
}

func newFindingScope(diff *types.Diff) findingScope {
	scope := findingScope{added: make(map[string]map[int]struct{})}
	if diff == nil {
		return scope
	}

	for _, file := range diff.Files {
		filePath := normalizeDiffPath(file.Path)
		if filePath == "" {
			continue
		}
		lines := scope.added[filePath]
		if lines == nil {
			lines = make(map[int]struct{})
			scope.added[filePath] = lines
		}

		for _, hunk := range file.Hunks {
			lineNumber := hunk.NewStart
			for _, raw := range hunk.Lines {
				if raw == "" {
					continue
				}
				marker, _ := splitDiffMarker(raw)
				switch marker {
				case "+":
					if lineNumber > 0 {
						lines[lineNumber] = struct{}{}
					}
					lineNumber++
				case " ":
					lineNumber++
				case "-":
					// Deletions have no current-side line to annotate.
				default:
					// A malformed hunk line cannot establish a safe location.
				}
			}
		}
	}
	return scope
}

func normalizeDiffPath(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	if value == "" {
		return ""
	}
	clean := path.Clean(value)
	if clean == "." || path.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, "../") {
		return ""
	}
	return clean
}

func (s findingScope) contains(file string, line int) bool {
	if line <= 0 {
		return false
	}
	lines := s.added[normalizeDiffPath(file)]
	_, ok := lines[line]
	return ok
}

type scopeDiscardSummary struct {
	OutsideAddedLines   int
	MissingEvidence     int
	MissingImpact       int
	MissingVerification int
}

func (s scopeDiscardSummary) total() int {
	return s.OutsideAddedLines + s.MissingEvidence + s.MissingImpact + s.MissingVerification
}

func (s scopeDiscardSummary) warning() string {
	if s.total() == 0 {
		return ""
	}
	reasons := make([]string, 0, 4)
	if s.OutsideAddedLines > 0 {
		reasons = append(reasons, fmt.Sprintf("%d fora de linhas adicionadas no diff", s.OutsideAddedLines))
	}
	if s.MissingEvidence > 0 {
		reasons = append(reasons, fmt.Sprintf("%d sem evidencia concreta", s.MissingEvidence))
	}
	if s.MissingImpact > 0 {
		reasons = append(reasons, fmt.Sprintf("%d sem impacto explicado", s.MissingImpact))
	}
	if s.MissingVerification > 0 {
		reasons = append(reasons, fmt.Sprintf("%d sem verificacao proposta", s.MissingVerification))
	}
	return fmt.Sprintf("%d finding(s) descartado(s) pelo gate de escopo e evidencia: %s", s.total(), strings.Join(reasons, ", "))
}

// filterModelIssues enforces the review's precision contract after parsing
// and redaction, before rule citations or publication. A finding is useful
// only when it points to an added line and explains what in that line proves
// the problem, why it matters, and how the author can verify the correction.
// Empty proof is rejected instead of being promoted into a warning by prose.
func filterModelIssues(diff *types.Diff, issues []types.ReviewIssue) ([]types.ReviewIssue, scopeDiscardSummary) {
	scope := newFindingScope(diff)
	kept := make([]types.ReviewIssue, 0, len(issues))
	var discarded scopeDiscardSummary
	for _, issue := range issues {
		if !scope.contains(issue.File, issue.Line) {
			discarded.OutsideAddedLines++
			continue
		}
		if strings.TrimSpace(issue.Evidence) == "" {
			discarded.MissingEvidence++
			continue
		}
		if strings.TrimSpace(issue.Impact) == "" {
			discarded.MissingImpact++
			continue
		}
		if strings.TrimSpace(issue.Verification) == "" {
			discarded.MissingVerification++
			continue
		}
		kept = append(kept, issue)
	}
	return kept, discarded
}

// addedLinesForTesting exposes only the deterministic location set to
// package tests. It keeps the production gate's data model private.
func addedLinesForTesting(diff *types.Diff) map[string][]int {
	scope := newFindingScope(diff)
	result := make(map[string][]int, len(scope.added))
	for file, lines := range scope.added {
		for line := range lines {
			result[file] = append(result[file], line)
		}
		sort.Ints(result[file])
	}
	return result
}
