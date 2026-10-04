// The deterministic security pass's local output: its findings and the
// coverage note of the categories that were applied.
package main

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/Mpaape/AurumCode/internal/security/redaction"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// printSecurityFindings prints the AUR-435 security pass section: a blank
// separator line, a header naming the project security standard, then one
// line per finding in the quality block's own "<file>:<line>: [<severity>]
// <message>" format, sorted by (file, line, rule) for determinism -- or the
// honest "No security findings." when the pass matched nothing. The section
// only exists when --seguranca was given, so the published no-flag output
// keeps its exact bytes.
//
// The file path derives from the reviewed diff -- repository-controlled
// input -- so it passes the redaction filter before reaching the sink
// (AUR-432); an ordinary path is filter-identity. The message is trusted
// catalog text (rule description, standard citation, rule citation) and is
// deliberately NOT re-filtered, for the same reason printFindings gives:
// the filter would rewrite catalog spellings like "-secret:" and change the
// published format of a secret-free review.
func printSecurityFindings(stdout io.Writer, filter *redaction.Filter, issues []types.ReviewIssue) {
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Security findings (standards/security-review):")
	if len(issues) == 0 {
		fmt.Fprintln(stdout, "No security findings.")
		return
	}

	sorted := make([]types.ReviewIssue, len(issues))
	copy(sorted, issues)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].File != sorted[j].File {
			return sorted[i].File < sorted[j].File
		}
		if sorted[i].Line != sorted[j].Line {
			return sorted[i].Line < sorted[j].Line
		}
		return sorted[i].RuleID < sorted[j].RuleID
	})
	for _, issue := range sorted {
		fmt.Fprintf(stdout, "%s:%d: [%s] %s\n", filter.Redact(issue.File), issue.Line, issue.Severity, issue.Message)
		printAssessment(stdout, issue)
	}
}

// printSecurityCoverage is the AUR-450 note: how many and which
// security-category rules of the embedded catalog the --seguranca pass
// actually applied (carry a matcher, internal/review.RulesLoader.
// PatternFor), against how many the category declares in total. It always
// prints when --seguranca runs -- before printSecurityFindings, and before
// the findings are even known -- so the found and the empty-result cases
// get the byte-identical coverage line: the figures are catalog-derived,
// never diff-derived, which is exactly why they cannot lie about "No
// security findings." meaning "the whole catalog ran."
//
// It goes to stderr, the same sink and "aurumcode review: " prefix every
// other --seguranca-adjacent note already uses (AUR-433's cost lines,
// AUR-441's cache-reuse note, AUR-448's discard warning, AUR-449's skip
// explanation) -- never stdout, because tests/acceptance/AUR-449.sh pins
// an exact sha256 of stdout for this exact command with a provider
// configured, a byte-for-byte guarantee this card does not own and must
// not disturb. See docs/specs/AUR-450.md's "Why stderr" section.
//
// applied is already sorted by rule id (RulesLoader.AppliedInCategory);
// the message names every one of them, never truncated, because the whole
// point is telling the caller exactly what ran.
func printSecurityCoverage(stderr io.Writer, applied []string, total int) {
	list := "none"
	if len(applied) > 0 {
		list = strings.Join(applied, ", ")
	}
	fmt.Fprintf(stderr, "aurumcode review: security pass applied %d of %d security rules (%s); see internal/review/rules/security.yml for the full catalog\n", len(applied), total, list)
}
