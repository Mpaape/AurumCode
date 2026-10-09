package render

import (
	"strings"

	"github.com/Mpaape/AurumCode/internal/gate/reasons"
	"github.com/Mpaape/AurumCode/internal/i18n"
)

// NoIssuesLine is the English verdict line of a review that found nothing
// AND whose every source concluded.
const NoIssuesLine = "No issues found."

// NoFindingsLine is NoFindingsLineIn for the default (English) language.
func NoFindingsLine(reason string) string {
	return NoFindingsLineIn("", reason)
}

// NoFindingsLineIn is the closing line of a review with no findings
// (AUR-572), in language. reason is the gate's comma-joined,
// machine-readable inconclusive reasons ("sast_execution_error",
// "partial_coverage", ...); empty means every source concluded and the line
// is the catalog's "no issues" text. A source that did not conclude cannot
// vouch that nothing is there, so the line names what stayed without a
// conclusion instead of claiming a clean review.
func NoFindingsLineIn(language, reason string) string {
	var names []string
	seen := map[string]bool{}
	for _, r := range strings.Split(reason, ",") {
		r = strings.TrimSpace(r)
		if r == "" || seen[r] {
			continue
		}
		seen[r] = true
		names = append(names, r)
	}
	if len(names) == 0 {
		return i18n.Text(language, "terminal.no_issues")
	}
	return i18n.Format(language, "terminal.no_findings_inconclusive", reasons.List(language, strings.Join(names, ",")))
}
