package prompt

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Mpaape/AurumCode/pkg/types"
)

// lineComment is the shape of one entry of a "line_comments" array as a
// model actually emits it. It is deliberately NOT types.ReviewComment:
// that type carries no severity and no rule_id, and both are needed to
// turn a comment into a finding the rest of the pipeline can handle. Both
// spellings of the location and the text are accepted because a model that
// mixes the two vocabularies is exactly the case this path exists for.
type lineComment struct {
	Path       string `json:"path"`
	File       string `json:"file"`
	Line       int    `json:"line"`
	Body       string `json:"body"`
	Message    string `json:"message"`
	Severity   string `json:"severity"`
	RuleID     string `json:"rule_id"`
	Suggestion string `json:"suggestion"`
}

// reviewEnvelope reads only the alias fields types.ReviewResult cannot
// carry losslessly, from the same repaired JSON the main decode used.
type reviewEnvelope struct {
	LineComments []lineComment `json:"line_comments"`
}

// adoptLineComments folds a "line_comments" array into result.Issues and
// reports how many entries it converted.
//
// Mapping: path (or file) -> File, line -> Line, body (or message) ->
// Message, and rule_id/severity/suggestion straight through when the model
// supplied them.
//
// Merge, not replace, and the merge key is (file, line, normalized
// message) -- NOT (file, line). Keying on the location alone was this
// card's own first answer and it was wrong in a way only execution shows:
// a style nit reported under "issues" at a.go:42 swallowed a SECRET LEAK
// reported under "line_comments" at the same line, and two distinct
// comments on one line collapsed into one. Several findings share a line
// routinely; only the SAME finding restated in the other vocabulary may
// collapse, which is exactly what the message-aware key collapses (case
// and whitespace are normalized so a restatement is still recognized).
//
// Every path that does NOT adopt a comment is counted and named -- a
// repeat of a finding already reported, a comment with no path, a comment
// with a blank body -- and rendered into
// Metadata["parse_discard_warning"], which cmd/aurumcode prints to stderr
// beside the AUR-448 rule-gate warning. A card whose chosen answer is
// "announce the discard" cannot itself discard in silence.
//
// ABOUT THE RULE GATE (AUR-434, internal/review.enforceRuleCitations): a
// converted comment usually carries no rule_id, and a finding without a
// resolvable rule_id is discarded before it reaches stdout. That discard
// is deliberately NOT silent: enforceRuleCitations counts it and
// formatDiscardWarning (AUR-448) renders the count and the reason, which
// cmd/aurumcode prints verbatim to stderr. So the user is told "N
// finding(s) discarded: N with no rule_id" instead of being told "No
// issues found." while the model actually reported something. Requiring
// rule_id inside line_comments instead was rejected: the template does not
// show line_comments at all any more (it shows one schema, "issues",
// where rule_id IS required and explained), so teaching a required field
// inside a shape the prompt never asks for would document a contract
// nobody is offered. Announcing the discard is the honest half of the
// pair, and it already exists.
//
// Robustness: a malformed or partial line_comments block must never turn
// a usable response into a hard parse failure. A block that does not
// decode is ignored, and an entry without a path or without a body is
// skipped rather than passed to validateReviewResult, which would reject
// the whole response over one junk entry.
func (p *ResponseParser) adoptLineComments(jsonContent string, result *types.ReviewResult) int {
	var envelope reviewEnvelope
	if err := json.Unmarshal([]byte(jsonContent), &envelope); err != nil {
		return 0
	}
	if len(envelope.LineComments) == 0 {
		return 0
	}

	type finding struct {
		file    string
		line    int
		message string
	}
	seen := make(map[finding]bool, len(result.Issues))
	for _, issue := range result.Issues {
		seen[finding{file: issue.File, line: issue.Line, message: normalizeFindingMessage(issue.Message)}] = true
	}

	converted := 0
	var repeated, noPath, emptyBody int
	for _, c := range envelope.LineComments {
		file := c.Path
		if file == "" {
			file = c.File
		}
		message := c.Body
		if message == "" {
			message = c.Message
		}
		if file == "" {
			noPath++
			continue
		}
		if strings.TrimSpace(message) == "" {
			emptyBody++
			continue
		}
		key := finding{file: file, line: c.Line, message: normalizeFindingMessage(message)}
		if seen[key] {
			repeated++
			continue
		}
		seen[key] = true

		severity := strings.ToLower(strings.TrimSpace(c.Severity))
		if severity != "error" && severity != "warning" && severity != "info" {
			// A comment carries no severity of its own in the shape a
			// model emits. "warning" is the same default degradedExtract
			// already uses for a finding whose severity is unstated: it is
			// reported, never invented as an error.
			severity = "warning"
		}

		result.Issues = append(result.Issues, types.ReviewIssue{
			File:       file,
			Line:       c.Line,
			Severity:   severity,
			RuleID:     c.RuleID,
			Message:    strings.TrimSpace(message),
			Suggestion: c.Suggestion,
		})
		converted++
	}

	discarded := repeated + noPath + emptyBody
	if converted > 0 || discarded > 0 {
		if result.Metadata == nil {
			result.Metadata = make(map[string]string)
		}
		result.Metadata["line_comments_converted"] = fmt.Sprintf("%d", converted)
		result.Metadata["line_comments_discarded"] = fmt.Sprintf("%d", discarded)
		if warning := formatLineCommentDiscards(repeated, noPath, emptyBody); warning != "" {
			result.Metadata["parse_discard_warning"] = warning
		}
	}
	return converted
}

// normalizeFindingMessage folds the accidental differences between the
// same finding restated in the other vocabulary -- letter case and
// whitespace/newline layout -- so a restatement collapses while two
// genuinely different findings at one line both survive.
func normalizeFindingMessage(message string) string {
	return strings.ToLower(strings.Join(strings.Fields(message), " "))
}

// formatLineCommentDiscards renders the one-line explanation of every
// comment adoptLineComments did not turn into a finding, in the shape of
// AUR-448's rule-gate warning (count first, then the reasons), for
// cmd/aurumcode to print verbatim on stderr. Returns "" when nothing was
// discarded, so the ordinary run never receives a byte.
func formatLineCommentDiscards(repeated, noPath, emptyBody int) string {
	total := repeated + noPath + emptyBody
	if total == 0 {
		return ""
	}
	var reasons []string
	if repeated > 0 {
		reasons = append(reasons, fmt.Sprintf("%d already reported as an issue", repeated))
	}
	if noPath > 0 {
		reasons = append(reasons, fmt.Sprintf("%d with no path", noPath))
	}
	if emptyBody > 0 {
		reasons = append(reasons, fmt.Sprintf("%d with an empty body", emptyBody))
	}
	return fmt.Sprintf("%d line comment(s) not converted: %s", total, strings.Join(reasons, ", "))
}
