package prompt

import (
	"fmt"

	"github.com/Mpaape/AurumCode/pkg/types"
)

// findingsListKey is the one top-level field of the answer schema that
// carries findings. Every finding must arrive there, where the template
// requires rule_id, evidence, impact and verification.
const findingsListKey = "issues"

// parseDiscardWarningKey is the result.Metadata key whose value
// cmd/aurumcode prints verbatim to stderr when the parser dropped part of a
// reply. It is engine-written; a value the model supplied is overwritten.
const parseDiscardWarningKey = "parse_discard_warning"

// discardLineComments drops every entry a reply placed under
// "line_comments" and announces the drop.
//
// "line_comments" is a PR-comment vocabulary (path, line, body), not a
// finding: it carries no evidence, impact or verification, so a finding
// converted from it could never pass the evidence gate, and the review
// template does not teach it. Converting it would either reintroduce a
// second findings schema or admit findings without evidence; neither is
// acceptable, so the entries never reach result.Issues. They are also
// cleared from result.LineComments, which types.ReviewResult still decodes
// for legacy input, so no later stage (rendering, batch merging) can carry
// them. A reply with ONLY "line_comments" has no findings list and is
// already rejected as inconclusive by hasFindingsList; this function only
// sees replies that also carry "issues".
func discardLineComments(result *types.ReviewResult) {
	ignored := len(result.LineComments)
	result.LineComments = nil
	if ignored == 0 {
		return
	}
	if result.Metadata == nil {
		result.Metadata = make(map[string]string)
	}
	result.Metadata[parseDiscardWarningKey] = fmt.Sprintf(
		"%d line comment(s) ignored: line_comments is not part of the answer schema; findings must be reported under %q with evidence",
		ignored, findingsListKey)
}
