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
	added   map[string]map[int]struct{}
	removed map[string]map[int]struct{}
}

func newFindingScope(diff *types.Diff) findingScope {
	scope := findingScope{added: make(map[string]map[int]struct{}), removed: make(map[string]map[int]struct{})}
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
		removed := scope.removed[filePath]
		if removed == nil {
			removed = make(map[int]struct{})
			scope.removed[filePath] = removed
		}

		for _, hunk := range file.Hunks {
			lineNumber := hunk.NewStart
			oldLine := hunk.OldStart
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
					oldLine++
				case "-":
					if oldLine > 0 {
						removed[oldLine] = struct{}{}
					}
					oldLine++
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
	return s.containsSide(file, line, "RIGHT")
}

func (s findingScope) containsSide(file string, line int, side string) bool {
	if line <= 0 {
		return false
	}
	lines := s.added[normalizeDiffPath(file)]
	switch side {
	case "", "RIGHT":
	case "LEFT":
		lines = s.removed[normalizeDiffPath(file)]
	default:
		return false
	}
	_, ok := lines[line]
	return ok
}

// IsChangedLine validates a publication anchor in its own coordinate space.
// Reading context is unrestricted by this check; only additions and deletions
// can anchor a finding. It does not establish whether the allegation is true.
func IsChangedLine(diff *types.Diff, issue types.ReviewIssue) bool {
	return newFindingScope(diff).containsSide(issue.File, issue.Line, issue.Side)
}

// FindingSide preserves compatibility with responses that predate LEFT support.
func FindingSide(issue types.ReviewIssue) string {
	if issue.Side == "" {
		return "RIGHT"
	}
	return issue.Side
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
		reasons = append(reasons, fmt.Sprintf("%d fora de linhas alteradas no diff", s.OutsideAddedLines))
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
// structurally publishable when it points to an addition or deletion and
// supplies evidence, impact and a verification proposal. Nonempty fields are
// not proof of correctness; semantic qualification remains the reviewer's job.
func filterModelIssues(diff *types.Diff, issues []types.ReviewIssue) ([]types.ReviewIssue, scopeDiscardSummary) {
	scope := newFindingScope(diff)
	kept := make([]types.ReviewIssue, 0, len(issues))
	var discarded scopeDiscardSummary
	for _, issue := range issues {
		if !scope.containsSide(issue.File, issue.Line, issue.Side) {
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
		issue.File = normalizeDiffPath(issue.File)
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
