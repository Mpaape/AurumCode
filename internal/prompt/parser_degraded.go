package prompt

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/Mpaape/AurumCode/pkg/types"
)

// findingLinePattern matches the degraded-mode convention this parser
// recognizes when a response contains no valid JSON at all:
//
//	path/to/file.ext:LINE: SEVERITY: message text
//
// with an optional leading "- " or "* " bullet marker and an optional
// ": SEVERITY" segment (defaulting to "warning" when absent). This is a
// convention this engine defines and documents (see docs/specs/AUR-430.md)
// for models that cannot reliably follow the JSON schema -- it is
// deliberately simple and line-oriented so it is easy for a prompt to
// describe and easy for a small local model to produce.
var findingLinePattern = regexp.MustCompile(`^(?:[-*]\s+)?([\w./-]+\.\w+):(\d+):\s*(?:(error|warning|info):\s*)?(.+)$`)

// degradedOrError is the fallback path for a response the strict JSON
// pipeline could not use. Before this card, any of these failures killed
// the whole review with no output -- fatal for a smaller/local model that
// drifts from the JSON schema on an otherwise-useful response. It now tries
// one bounded, documented recovery (see findingLinePattern) and only
// returns the typed ParseError when that recovery also finds nothing,
// rather than ever returning a silently empty, successful result: an empty
// []ReviewIssue must always mean "the model looked and found nothing," not
// "the engine gave up."
func (p *ResponseParser) degradedOrError(response string, kind ParseErrorKind, cause error) (*types.ReviewResult, error) {
	if issues := p.degradedExtract(response); len(issues) > 0 {
		return &types.ReviewResult{
			Issues:  issues,
			Summary: DegradedParseSummary,
			Metadata: map[string]string{
				ParseModeKey:  ParseModeDegraded,
				"parse_cause": string(kind),
			},
		}, nil
	}
	return nil, newParseError(kind, response, cause)
}

// AUR-519: a model reply this parser could not understand as structured
// JSON is, on its own, an inconclusive review (AC-008) -- today it still
// publishes as if the quality pass had succeeded, which is exactly the
// defect this card's gate closes. ParseModeKey/ParseModeDegraded are the
// ONLY place that marks a result this way; IsDegradedParse is the one
// forge-safe way a caller checks it, because ParseReviewResponse scrubs
// any value a model supplied under this same key on every successful
// decode (see the delete(result.Metadata, ParseModeKey) call above) before
// this function ever runs. A caller must never match on DegradedParseSummary
// or any other free-form text instead -- that text is documented wording,
// not a stable contract.
const (
	// ParseModeKey is the reserved result.Metadata key this parser uses to
	// record how a result was produced.
	ParseModeKey = "parse_mode"
	// ParseModeDegraded is ParseModeKey's value when the result came from
	// the free-form-text fallback (degradedExtract) rather than a valid
	// JSON response.
	ParseModeDegraded = "degraded"
	// PolicyGateWithheldKey is the reserved result.Metadata key AUR-519's
	// policy gate (cmd/aurumcode) sets to "true" when it decided a review
	// must not be presented as approved (a severity breach or an
	// inconclusive run under gate.inconclusive: block). It exists so
	// cmd/aurumcode's reviewVerdictForLanguage/formalReviewEvent/
	// canonicalVerdict -- which otherwise re-derive their own verdict from
	// Issues/Suggestions alone and never from the model's own Verdict
	// field -- have one forge-safe, engine-owned signal to check instead
	// of matching on the model's self-reported Verdict text (which the
	// model controls and which these functions must keep ignoring for
	// every other purpose, exactly as before this card). ParseReviewResponse
	// scrubs this key from a model's own JSON on every successful decode,
	// so only the gate itself, running well after parsing, can ever set it.
	PolicyGateWithheldKey = "policy_gate_withheld"
)

// DegradedParseSummary is the fixed Summary text a degraded-parse result
// always carries. It is documentation, not a detection mechanism -- see
// IsDegradedParse.
const DegradedParseSummary = "Degraded parse: recovered findings from free-form text; the model's response was not valid JSON."

// IsDegradedParse reports whether result came from this parser's degraded,
// free-form-text fallback rather than a valid JSON response. It is
// forge-safe: ParseReviewResponse deletes any ParseModeKey entry a model
// supplied on every successful decode, so by the time a caller sees
// result.Metadata, a true value here can only have been set by
// degradedOrError itself.
func IsDegradedParse(result *types.ReviewResult) bool {
	return result != nil && result.Metadata[ParseModeKey] == ParseModeDegraded
}

// degradedExtract scans response line by line for findingLinePattern
// matches and turns each into a types.ReviewIssue. It is intentionally
// simple: a best-effort recovery, not a second parser.
func (p *ResponseParser) degradedExtract(response string) []types.ReviewIssue {
	var issues []types.ReviewIssue
	for _, line := range strings.Split(response, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		m := findingLinePattern.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		severity := strings.ToLower(m[3])
		if severity == "" {
			severity = "warning"
		}
		lineNo := 0
		fmt.Sscanf(m[2], "%d", &lineNo)
		issues = append(issues, types.ReviewIssue{
			File:     m[1],
			Line:     lineNo,
			Severity: severity,
			Message:  strings.TrimSpace(m[4]),
		})
	}
	return issues
}
