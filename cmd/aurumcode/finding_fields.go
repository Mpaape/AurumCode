// The actionable detail of one finding, written the same way to the
// terminal report and to the published review document.
package main

import (
	"fmt"
	"io"

	"github.com/Mpaape/AurumCode/pkg/types"
)

// writeFindingFields writes the finding's impact, evidence, suggested fix
// and verification, one labeled line each, in the review's language. An
// empty field writes nothing. Both outputs call this one writer, so the
// terminal never shows less of a finding than the pull request does.
func writeFindingFields(w io.Writer, issue types.ReviewIssue, copy reviewCopy) {
	for _, field := range []struct{ label, value string }{
		{copy.impact, issue.Impact},
		{copy.evidence, issue.Evidence},
		{copy.suggestedFix, issue.Suggestion},
		{copy.verify, issue.Verification},
	} {
		if field.value != "" {
			fmt.Fprintf(w, "  - %s: %s\n", field.label, field.value)
		}
	}
}
