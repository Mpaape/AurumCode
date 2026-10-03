package render

import "fmt"

// GateSARIFFindings builds the SARIF results from the gate's blocking
// findings, the same records the audit's blocking_findings is written from,
// so the two artifacts never disagree about what counted:
//
//   - each blocking finding gives its typed origin to the review issue at
//     the same rule, path and line (the issue keeps its message, context,
//     assessment and suppression);
//   - a blocking finding no review issue carries (a Dependency-Track
//     breach, which has no line in the diff) becomes its own result with
//     that origin, instead of being lost.
func GateSARIFFindings(issues []SARIFFinding, blocking []AuditFinding) []SARIFFinding {
	index := make(map[string]int, len(blocking))
	for i, b := range blocking {
		key := gateFindingKey(b.RuleID, b.Path, b.Line)
		if _, seen := index[key]; !seen {
			index[key] = i
		}
	}
	carried := make([]bool, len(blocking))
	out := make([]SARIFFinding, 0, len(issues)+len(blocking))
	for _, f := range issues {
		if i, ok := index[gateFindingKey(f.RuleID, f.Path, f.Line)]; ok {
			f.Origin = blocking[i].Origin
			carried[i] = true
		}
		out = append(out, f)
	}
	for i, b := range blocking {
		if carried[i] {
			continue
		}
		if dup, ok := index[gateFindingKey(b.RuleID, b.Path, b.Line)]; ok && dup != i {
			continue
		}
		out = append(out, SARIFFinding{
			RuleID:   b.RuleID,
			Path:     b.Path,
			Line:     b.Line,
			Severity: b.Severity,
			Message:  fmt.Sprintf("%s (origem %s)", b.RuleID, b.Origin),
			Origin:   b.Origin,
		})
	}
	return out
}

// gateFindingKey identifies a finding by rule, path and line.
func gateFindingKey(ruleID, path string, line int) string {
	return fmt.Sprintf("%s|%s|%d", ruleID, path, line)
}
