package benchmark

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type Suggestion struct {
	DefectID string `json:"defect_id"`
	File     string `json:"file"`
	Patch    string `json:"patch"`
}

type ApplicabilityResult struct {
	Applies        bool   `json:"applies"`
	TestsPreserved bool   `json:"tests_preserved"`
	DefectFixed    bool   `json:"defect_fixed"`
	Applicable     bool   `json:"applicable"`
	Reason         string `json:"reason"`
}

type TestsPreservedOracle func(files map[string]string) bool

// EvaluateSuggestion applies the card's definition of an applicable
// suggestion: the patch applies to the head, the relevant tests are preserved,
// and the named defect is gone. Emitting a code block is not enough.
func EvaluateSuggestion(d Defect, files map[string]string, s Suggestion, oracle TestsPreservedOracle) ApplicabilityResult {
	result := ApplicabilityResult{}
	if strings.TrimSpace(s.Patch) == "" {
		result.Reason = "empty-patch"
		return result
	}
	original, ok := files[s.File]
	if !ok {
		result.Reason = "missing-target-file"
		return result
	}
	patched, err := applyUnifiedDiff(original, s.Patch)
	if err != nil {
		result.Reason = "patch-does-not-apply: " + err.Error()
		return result
	}
	result.Applies = true
	if d.Marker != "" {
		result.DefectFixed = strings.Contains(original, d.Marker) && !strings.Contains(patched, d.Marker)
	} else {
		result.DefectFixed = patched != original
	}
	updated := make(map[string]string, len(files))
	for name, content := range files {
		updated[name] = content
	}
	updated[s.File] = patched
	if oracle != nil {
		result.TestsPreserved = oracle(updated)
	}
	result.Applicable = result.Applies && result.DefectFixed && result.TestsPreserved
	if result.Applicable {
		result.Reason = "applicable"
		return result
	}
	switch {
	case !result.DefectFixed:
		result.Reason = "defect-not-fixed"
	case !result.TestsPreserved:
		result.Reason = "tests-not-preserved"
	}
	return result
}

type HumanDecision string

const (
	DecisionAccepted     HumanDecision = "accepted"
	DecisionRejected     HumanDecision = "rejected"
	DecisionInconclusive HumanDecision = "inconclusive"
)

// TriageEntry keeps active triage time, decision time and comment count in
// separate fields. Acceptance comes only from Decision; a comment is never
// scored as agreement.
type TriageEntry struct {
	FindingID      string        `json:"finding_id"`
	Decision       HumanDecision `json:"decision"`
	ActiveTriageMS int64         `json:"active_triage_ms"`
	DecisionMS     int64         `json:"decision_ms"`
	Comments       int           `json:"comments"`
}

func (t TriageEntry) Accepted() bool {
	return t.Decision == DecisionAccepted
}

func AcceptedCount(entries []TriageEntry) int {
	count := 0
	for _, e := range entries {
		if e.Accepted() {
			count++
		}
	}
	return count
}

var hunkHeader = regexp.MustCompile(`^@@ -([0-9]+)(?:,([0-9]+))? \+([0-9]+)(?:,([0-9]+))? @@`)

func applyUnifiedDiff(original, patch string) (string, error) {
	origLines := splitContent(original)
	patchLines := splitContent(patch)
	if len(patchLines) == 0 {
		return "", errors.New("empty patch")
	}
	i := 0
	for i < len(patchLines) && !strings.HasPrefix(patchLines[i], "@@") {
		i++
	}
	if i == len(patchLines) {
		return "", errors.New("patch has no hunk")
	}
	out := make([]string, 0, len(origLines))
	cursor := 0
	for i < len(patchLines) {
		match := hunkHeader.FindStringSubmatch(patchLines[i])
		if match == nil {
			return "", fmt.Errorf("malformed hunk header: %q", patchLines[i])
		}
		oldStart, err := strconv.Atoi(match[1])
		if err != nil {
			return "", err
		}
		target := oldStart - 1
		if target < cursor || target > len(origLines) {
			return "", errors.New("hunk out of range")
		}
		out = append(out, origLines[cursor:target]...)
		cursor = target
		i++
		for i < len(patchLines) && !strings.HasPrefix(patchLines[i], "@@") {
			line := patchLines[i]
			if line == "" {
				return "", errors.New("malformed hunk line")
			}
			prefix := line[0]
			content := line[1:]
			switch prefix {
			case ' ':
				if cursor >= len(origLines) || origLines[cursor] != content {
					return "", errors.New("context mismatch")
				}
				out = append(out, content)
				cursor++
			case '-':
				if cursor >= len(origLines) || origLines[cursor] != content {
					return "", errors.New("removed line mismatch")
				}
				cursor++
			case '+':
				out = append(out, content)
			case '\\':
			default:
				return "", fmt.Errorf("unknown hunk prefix %q", string(prefix))
			}
			i++
		}
	}
	out = append(out, origLines[cursor:]...)
	return strings.Join(out, "\n"), nil
}

func splitContent(content string) []string {
	trimmed := strings.TrimSuffix(content, "\n")
	if trimmed == "" {
		return []string{}
	}
	return strings.Split(trimmed, "\n")
}
