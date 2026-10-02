// AUR-521: FindingIdentityFor is the one place that turns a ReviewIssue
// into a FindingIdentity by reading the REVIEWED DIFF, never the model's
// own free-text fields (Message/Evidence) -- those are not "code at
// File/Line", they are the model's account of it, which can be reworded
// (or, on an adversarial diff, steered) without the underlying code
// changing at all. AUR-494 should call this function, not Evidence/Message,
// when it builds its own identity for the same finding.
package render

import (
	"github.com/Mpaape/AurumCode/internal/security/redaction"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// FindingIdentityFor builds the canonical identity for issue from diff's own
// hunk content at (issue.File, issue.Line, issue.Side) -- the actual
// reviewed code, not the model's description of it. The returned identity's
// Context is passed through filter (when non-nil) before being returned:
// the fingerprint is published in a SARIF document a workflow uploads to
// code scanning, and a secret literally present in the reviewed line must
// not make the SAME finding fingerprint differently just because the
// secret's value was rotated between two runs.
//
// When diff does not contain the line (a finding not anchored to a diff
// coordinate at all), Context is empty -- still deterministic, still never
// the model's free text.
func FindingIdentityFor(diff *types.Diff, issue types.ReviewIssue, filter *redaction.Filter) FindingIdentity {
	line := diffLineAt(diff, issue.File, issue.Line, issue.Side)
	if filter != nil {
		line = filter.Redact(line)
	}
	return FindingIdentity{
		RuleID:  issue.RuleID,
		Path:    issue.File,
		Line:    issue.Line,
		Context: line,
	}
}

// diffLineAt returns the hunk line's body (its leading +/-/space marker
// stripped) at file/line in the given coordinate side ("LEFT" the old
// file, "RIGHT"/"" the new file -- the same convention
// internal/review.FindingSide already uses), or "" when not found.
func diffLineAt(diff *types.Diff, file string, line int, side string) string {
	if diff == nil {
		return ""
	}
	if side == "" {
		side = "RIGHT"
	}
	for _, f := range diff.Files {
		if f.Path != file {
			continue
		}
		for _, h := range f.Hunks {
			oldN, newN := h.OldStart, h.NewStart
			for _, l := range h.Lines {
				if l == "" {
					continue
				}
				marker, body := l[0], l[1:]
				switch marker {
				case '+':
					if side == "RIGHT" && newN == line {
						return body
					}
					newN++
				case '-':
					if side == "LEFT" && oldN == line {
						return body
					}
					oldN++
				default:
					// A context line exists in both coordinate spaces.
					if (side == "RIGHT" && newN == line) || (side == "LEFT" && oldN == line) {
						return body
					}
					oldN++
					newN++
				}
			}
		}
	}
	return ""
}
