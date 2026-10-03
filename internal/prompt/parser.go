package prompt

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/Mpaape/AurumCode/pkg/types"
)

// ResponseParser parses LLM responses into structured data
type ResponseParser struct{}

// NewResponseParser creates a new response parser
func NewResponseParser() *ResponseParser {
	return &ResponseParser{}
}

// ParseErrorKind classifies why ParseReviewResponse could not produce a
// structured result. It exists so a caller (or an operator reading logs) can
// tell "the model said something, but not JSON we could use" apart from
// every other kind of failure, instead of matching on an error string.
type ParseErrorKind string

const (
	// ParseErrorNoJSON means no JSON object could be located in the
	// response at all (no code fence, no bare `{...}`).
	ParseErrorNoJSON ParseErrorKind = "no_json_found"
	// ParseErrorInvalidJSON means a JSON-shaped span was found but it did
	// not decode even after repairJSON's best-effort cleanup.
	ParseErrorInvalidJSON ParseErrorKind = "invalid_json"
	// ParseErrorValidation means the JSON decoded but failed structural
	// validation (missing required issue fields, out-of-range scores).
	ParseErrorValidation ParseErrorKind = "validation_failed"
	// ParseErrorNoFindings means neither the JSON path nor the degraded
	// freeform-text fallback (see degradedExtract) could recover a single
	// finding from the response.
	ParseErrorNoFindings ParseErrorKind = "no_findings_recovered"
)

// ParseError is the typed, clear failure ParseReviewResponse returns instead
// of silently degrading to an empty result. See the "Parser fallback" note
// in docs/specs/AUR-430.md for the reasoning: a caller (or a human reading
// the review) must be able to tell "no problems found" apart from "the
// engine could not understand the model," and a bare error string forces
// every caller back to substring matching to make that distinction.
type ParseError struct {
	Kind ParseErrorKind
	// Diagnostics contain no model-authored text. They distinguish truncation
	// and malformed upstream JSON from extraction/parser defects in CI logs.
	InputBytes     int
	RawJSONValid   bool
	SyntaxOffset   int64
	FinishReason   string
	TypeField      string
	ExpectedType   string
	ActualType     string
	ValidationCode string
	// Raw is the response that failed to parse, bounded to a safe preview
	// length so a large or adversarial response cannot balloon an error
	// message that ends up in logs.
	Raw string
	Err error
}

const parseErrorRawPreviewLimit = 512

func newParseError(kind ParseErrorKind, raw string, err error) *ParseError {
	inputBytes := len(raw)
	rawJSONValid := json.Valid([]byte(strings.TrimSpace(raw)))
	var syntaxErr *json.SyntaxError
	var syntaxOffset int64
	if errors.As(err, &syntaxErr) {
		syntaxOffset = syntaxErr.Offset
	}
	if len(raw) > parseErrorRawPreviewLimit {
		raw = raw[:parseErrorRawPreviewLimit] + "... (truncated)"
	}
	parseErr := &ParseError{Kind: kind, Raw: raw, Err: err, InputBytes: inputBytes, RawJSONValid: rawJSONValid, SyntaxOffset: syntaxOffset}
	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &typeErr) {
		parseErr.TypeField = typeErr.Field
		parseErr.ExpectedType = typeErr.Type.String()
		parseErr.ActualType = typeErr.Value
	}
	if kind == ParseErrorValidation && err != nil {
		switch message := err.Error(); {
		case strings.HasPrefix(message, "invalid verdict"):
			parseErr.ValidationCode = "invalid_verdict"
		case strings.Contains(message, "missing file path"):
			parseErr.ValidationCode = "issue_missing_file"
		case strings.Contains(message, "missing severity"):
			parseErr.ValidationCode = "issue_missing_severity"
		case strings.Contains(message, "missing message"):
			parseErr.ValidationCode = "issue_missing_message"
		case strings.Contains(message, "invalid severity"):
			parseErr.ValidationCode = "issue_invalid_severity"
		case strings.HasPrefix(message, "ISO score"):
			parseErr.ValidationCode = "iso_score_out_of_range"
		default:
			parseErr.ValidationCode = "other"
		}
	}
	return parseErr
}

func (e *ParseError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("prompt: %s: %v", e.Kind, e.Err)
	}
	return fmt.Sprintf("prompt: %s", e.Kind)
}

func (e *ParseError) Unwrap() error { return e.Err }

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

// canonicalReviewFields is the ONE findings schema the review prompt
// template (templates/review.md) is allowed to show the model: exactly the
// top-level JSON fields this parser turns into a types.ReviewResult a user
// can see. AUR-459 exists because the template used to show two findings
// schemas -- "line_comments" first, "issues" second -- while this parser
// read only "issues": the model picked the first one it was shown and a
// diff with three planted secrets printed "No issues found." with exit 0.
//
// TestReviewTemplateMatchesParser extracts the REAL top-level keys of the
// template's JSON example and requires this exact set, so a future edit
// that teaches the model a field nobody reads breaks the build instead of
// turning back into silence.
var canonicalReviewFields = []string{
	"ci_analysis",
	"issues",
	"iso_scores",
	"limitations",
	"strengths",
	"suggestions",
	"summary",
	"test_plan",
	"verdict",
}

// acceptedReviewFields is what this parser consumes: the canonical set
// plus "line_comments", kept as a tolerated alias rather than a taught
// schema. The template no longer shows it, but a model that saw an older
// prompt, a user-supplied .aurumcode/prompts/review.md, or a model that
// simply drifts to the PR-comment vocabulary still gets its findings read
// instead of dropped (see adoptLineComments). Every name here is proven to
// change the parse result by TestAcceptedReviewFieldsAreConsumed -- the
// list is a claim, that test is the fact.
var acceptedReviewFields = []string{
	"ci_analysis",
	"issues",
	"iso_scores",
	"limitations",
	"line_comments",
	"strengths",
	"suggestions",
	"summary",
	"test_plan",
	"verdict",
}

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

// ParseReviewResponse parses a review response from LLM
func (p *ResponseParser) ParseReviewResponse(response string) (*types.ReviewResult, error) {
	// Extract JSON from response (handle markdown code blocks)
	jsonContent := p.extractJSON(response)

	if jsonContent == "" {
		return p.degradedOrError(response, ParseErrorNoJSON, nil)
	}

	// Keep findings strict, but do not erase a review because an auxiliary
	// narrative section has the wrong JSON shape. The discarded section is
	// recorded in metadata so callers can diagnose the provider drift.
	var result types.ReviewResult
	var fields map[string]json.RawMessage
	var discarded []string
	for {
		result = types.ReviewResult{}
		err := json.Unmarshal([]byte(jsonContent), &result)
		if err == nil {
			break
		}
		var typeErr *json.UnmarshalTypeError
		if !errors.As(err, &typeErr) {
			return p.degradedOrError(response, ParseErrorInvalidJSON, err)
		}
		section, _, _ := strings.Cut(typeErr.Field, ".")
		switch section {
		case "strengths", "suggestions", "ci_analysis", "test_plan", "limitations", "iso_scores", "summary":
		default:
			return p.degradedOrError(response, ParseErrorInvalidJSON, err)
		}
		if fields == nil {
			if json.Unmarshal([]byte(jsonContent), &fields) != nil {
				return p.degradedOrError(response, ParseErrorInvalidJSON, err)
			}
		}
		if _, exists := fields[section]; !exists {
			return p.degradedOrError(response, ParseErrorInvalidJSON, err)
		}
		delete(fields, section)
		discarded = append(discarded, section)
		pruned, marshalErr := json.Marshal(fields)
		if marshalErr != nil {
			return p.degradedOrError(response, ParseErrorInvalidJSON, marshalErr)
		}
		jsonContent = string(pruned)
	}

	// result.Metadata decodes straight from the model's own top-level
	// "metadata" object when the response carries one: it is model-authored,
	// untrusted input, exactly like every other field json.Unmarshal just
	// populated above. AUR-519's degraded-parse detection (ParseModeKey,
	// IsDegradedParse below) must never be forgeable by a model replying
	// with a syntactically valid, otherwise-ordinary review that simply
	// includes "metadata":{"parse_mode":"degraded"} -- that would let a
	// compromised or confused provider manufacture an "inconclusive" status
	// a policy's gate.inconclusive is supposed to react to only on this
	// parser's own, engine-decided failure path (degradedOrError below).
	// Scrub the reserved key here, on every successful strict-JSON decode,
	// before any engine-owned metadata is ever written to this same map.
	delete(result.Metadata, ParseModeKey)
	// Same forgery risk, same fix, for AUR-519's gate-withheld marker
	// (cmd/aurumcode's reviewVerdictForLanguage/formalReviewEvent/
	// canonicalVerdict check PolicyGateWithheldKey, never the model's own
	// Verdict text, precisely so a model cannot talk its way into -- or
	// out of -- a gate-driven withholding by supplying
	// "metadata":{"policy_gate_withheld":"true"} itself).
	delete(result.Metadata, PolicyGateWithheldKey)
	// AUR-519 (B-C): the per-file coverage counts the prompt builder
	// computed (code_files_total/complete/partial/omitted,
	// PromptParts.Meta) are a different map the model's JSON body never
	// touches, but a model could still smuggle same-named keys into its
	// own "metadata" object and have them survive here unless scrubbed --
	// internal/review/reviewer.go overwrites these from the trusted
	// PromptParts.Meta right after this call, but scrubbing here too
	// means a caller that forgets that overwrite still fails closed
	// (absent) rather than trusting a model-supplied count.
	for _, key := range []string{"code_files_total", "code_files_complete", "code_files_partial", "code_files_omitted"} {
		delete(result.Metadata, key)
	}

	// Origin is engine-owned: a model cannot claim a finding came from a
	// deterministic analyzer. An assessment with a status outside the
	// closed set, or naming no evidence, is dropped rather than guessed.
	sanitizeModelIssueProvenance(result.Issues)

	// A finding the model reported under "line_comments" is a finding: it
	// becomes an issue here, before validation, so it travels the same
	// path as one the model reported under "issues".
	p.adoptLineComments(jsonContent, &result)
	// The example formerly showed iso_scores:{} even though the validator
	// treats every missing score as zero and rejects it. An empty object has
	// no scores to validate; normalize only that exact shape to absent.
	if result.ISOScores != nil {
		if fields == nil {
			_ = json.Unmarshal([]byte(jsonContent), &fields)
		}
		var isoFields map[string]json.RawMessage
		if raw, ok := fields["iso_scores"]; ok && json.Unmarshal(raw, &isoFields) == nil && len(isoFields) == 0 {
			result.ISOScores = nil
		}
	}

	// Validate
	if err := p.validateReviewResult(&result); err != nil {
		return p.degradedOrError(response, ParseErrorValidation, err)
	}
	if len(discarded) > 0 {
		if result.Metadata == nil {
			result.Metadata = make(map[string]string)
		}
		result.Metadata["optional_sections_discarded"] = strings.Join(discarded, ",")
	}

	return &result, nil
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

// extractJSON extracts JSON content from response, handling markdown code blocks
func (p *ResponseParser) extractJSON(response string) string {
	// JSON-mode providers return the object itself. Inspect it before looking
	// for Markdown fences: a proposed code snippet inside a JSON string may
	// contain ``` markers, which the fence regex would otherwise extract as
	// if they surrounded the whole response.
	if whole := strings.TrimSpace(response); json.Valid([]byte(whole)) {
		return whole
	}

	// Try to find JSON in markdown code blocks
	codeBlockPattern := regexp.MustCompile("```(?:json)?\\s*\\n?([\\s\\S]*?)\\n?```")
	matches := codeBlockPattern.FindStringSubmatch(response)

	if len(matches) > 1 {
		return p.repairJSON(strings.TrimSpace(matches[1]))
	}

	// Try to find raw JSON (look for { ... })
	jsonPattern := regexp.MustCompile(`\{[\s\S]*\}`)
	matches = jsonPattern.FindStringSubmatch(response)

	if len(matches) > 0 {
		return p.repairJSON(strings.TrimSpace(matches[0]))
	}

	return ""
}

// repairJSON attempts to repair common JSON malformations produced by LLMs:
// trailing commas before a closing bracket/brace, and quote characters --
// straight or "smart"/typographic -- that do not line up with valid JSON
// string boundaries.
//
// The restored c12d7ab version of this function translated typographic
// double quotes (“ ”) to straight ASCII quotes with a blind,
// global string replace, run over the *entire* response including the
// inside of already-open JSON strings. That is unsafe: when the
// substitution lands inside a string value, it inserts a bare, unescaped
// `"` that ends the string early and corrupts everything after it -- the
// exact bug this restoration must not reintroduce. Typographic single
// quotes/apostrophes (‘ ’) are not JSON delimiters at all, so
// normalizing those is always safe and is still done as a first, global
// pass. Double quotes -- straight or typographic -- go through repairQuotes
// instead, which tracks whether it is currently inside a string and only
// closes the string on a quote that looks like a real boundary (the next
// non-whitespace character is a JSON structural character, or end of
// input); every other quote it meets while inside a string is treated as
// literal content and escaped in place. See TestRepairJSON/smart_quotes for
// the exact case that motivated this rewrite: a quote character embedded in
// a string value must become an escaped literal, never a premature close.
func (p *ResponseParser) repairJSON(jsonStr string) string {
	// Typographic single quotes/apostrophes are never JSON string
	// delimiters, so normalizing them anywhere in the text is always safe.
	repaired := strings.ReplaceAll(jsonStr, "‘", "'")
	repaired = strings.ReplaceAll(repaired, "’", "'")

	repaired = p.repairQuotes(repaired)

	// Remove trailing commas before closing brackets/braces.
	repaired = regexp.MustCompile(`,\s*([}\]])`).ReplaceAllString(repaired, "$1")

	return strings.TrimSpace(repaired)
}

// repairQuotes walks s as a small state machine that tracks whether it is
// currently inside a JSON string literal, normalizing every double-quote
// character -- straight (") or typographic (“ ”) -- to a straight
// ASCII quote. A quote met while inside a string only ends that string when
// it looks like a genuine boundary: the next non-whitespace character is
// one of `, } ] :`, or end of input. Any other quote met inside a string
// (including an already-escaped one, which is left untouched) is treated as
// literal content and is escaped in place, so it can never be misread as
// closing the string early.
func (p *ResponseParser) repairQuotes(s string) string {
	runes := []rune(s)
	var out strings.Builder
	out.Grow(len(s))

	inString := false

	isDoubleQuote := func(r rune) bool {
		return r == '"' || r == '“' || r == '”'
	}

	looksLikeBoundary := func(from int) bool {
		j := from
		for j < len(runes) && (runes[j] == ' ' || runes[j] == '\t' || runes[j] == '\n' || runes[j] == '\r') {
			j++
		}
		if j >= len(runes) {
			return true
		}
		switch runes[j] {
		case ',', '}', ']', ':':
			return true
		default:
			return false
		}
	}

	for i := 0; i < len(runes); i++ {
		r := runes[i]

		if inString && r == '\\' && i+1 < len(runes) {
			// Preserve existing escape sequences (including an already
			// escaped quote) untouched.
			out.WriteRune(r)
			out.WriteRune(runes[i+1])
			i++
			continue
		}

		if isDoubleQuote(r) {
			if !inString {
				inString = true
				out.WriteByte('"')
				continue
			}
			if looksLikeBoundary(i + 1) {
				inString = false
				out.WriteByte('"')
				continue
			}
			// Embedded quote: keep it as literal content, escaped so it
			// cannot be misread as the string's end.
			out.WriteByte('\\')
			out.WriteByte('"')
			continue
		}

		out.WriteRune(r)
	}

	return out.String()
}

// validateReviewResult validates a review result
func (p *ResponseParser) validateReviewResult(result *types.ReviewResult) error {
	// Check if issues is nil (create empty slice if needed)
	if result.Issues == nil {
		result.Issues = []types.ReviewIssue{}
	}
	if result.Strengths == nil {
		result.Strengths = []string{}
	}
	if result.Suggestions == nil {
		result.Suggestions = []types.ReviewSuggestion{}
	}
	if result.CIAnalysis == nil {
		result.CIAnalysis = []types.CIAnalysis{}
	}
	if result.TestPlan == nil {
		result.TestPlan = []string{}
	}
	if result.Limitations == nil {
		result.Limitations = []string{}
	}

	if result.Verdict != "" && result.Verdict != "approve" && result.Verdict != "changes_requested" && result.Verdict != "comment" {
		return fmt.Errorf("invalid verdict %q (must be approve, changes_requested, or comment)", result.Verdict)
	}

	// Validate each issue
	for i, issue := range result.Issues {
		if issue.File == "" {
			return fmt.Errorf("issue %d: missing file path", i)
		}
		if issue.Severity == "" {
			return fmt.Errorf("issue %d: missing severity", i)
		}
		if issue.Message == "" {
			return fmt.Errorf("issue %d: missing message", i)
		}

		// Normalize severity
		severity := strings.ToLower(issue.Severity)
		if severity != "error" && severity != "warning" && severity != "info" {
			return fmt.Errorf("issue %d: invalid severity %s (must be error, warning, or info)", i, issue.Severity)
		}
	}

	// Validate ISO scores (should be 1-10) only when the response supplied
	// them. types.ReviewResult.ISOScores is *ISOScores (unlike c12d7ab's
	// value field -- see AUR-430's restoration audit) precisely so a
	// response can omit them: this card's engine never asks a model to
	// score ISO/IEC 25010 characteristics (that is internal/review/iso25010,
	// explicitly out of scope here), so treating a missing block as
	// mandatory would reject every response this card's own prompt asks
	// for. A block that *is* present is still held to the same 1-10 range.
	if result.ISOScores != nil {
		if err := p.validateISOScores(result.ISOScores); err != nil {
			return err
		}
	}

	return nil
}

// validateISOScores validates ISO/IEC 25010 scores
func (p *ResponseParser) validateISOScores(scores *types.ISOScores) error {
	scoreMap := map[string]int{
		"functionality":   scores.Functionality,
		"reliability":     scores.Reliability,
		"usability":       scores.Usability,
		"efficiency":      scores.Efficiency,
		"maintainability": scores.Maintainability,
		"portability":     scores.Portability,
		"security":        scores.Security,
		"compatibility":   scores.Compatibility,
	}

	for name, score := range scoreMap {
		if score < 1 || score > 10 {
			return fmt.Errorf("ISO score %s must be between 1-10, got %d", name, score)
		}
	}

	return nil
}

// ParseDocumentationResponse parses documentation from LLM response
func (p *ResponseParser) ParseDocumentationResponse(response string) (string, error) {
	// Documentation is typically markdown, so just clean it up
	content := strings.TrimSpace(response)

	if content == "" {
		return "", fmt.Errorf("empty documentation response")
	}

	// Remove markdown code block markers if present
	codeBlockPattern := regexp.MustCompile("```(?:markdown|md)?\\s*\\n?([\\s\\S]*?)\\n?```")
	matches := codeBlockPattern.FindStringSubmatch(content)

	if len(matches) > 1 {
		content = strings.TrimSpace(matches[1])
	}

	return content, nil
}

// ParseTestResponse parses generated tests from LLM response
func (p *ResponseParser) ParseTestResponse(response string, language string) (string, error) {
	// Tests are typically in code blocks
	content := strings.TrimSpace(response)

	if content == "" {
		return "", fmt.Errorf("empty test response")
	}

	// Try to extract from code block
	pattern := fmt.Sprintf("```(?:%s)?\\s*\\n?([\\s\\S]*?)\\n?```", language)
	codeBlockPattern := regexp.MustCompile(pattern)
	matches := codeBlockPattern.FindStringSubmatch(content)

	if len(matches) > 1 {
		return strings.TrimSpace(matches[1]), nil
	}

	// If no code block, look for any code block
	anyCodeBlock := regexp.MustCompile("```[\\w]*\\s*\\n?([\\s\\S]*?)\\n?```")
	matches = anyCodeBlock.FindStringSubmatch(content)

	if len(matches) > 1 {
		return strings.TrimSpace(matches[1]), nil
	}

	// Return as-is if no code blocks found
	return content, nil
}

// ParseSummaryResponse parses a summary from LLM response
func (p *ResponseParser) ParseSummaryResponse(response string) (string, error) {
	content := strings.TrimSpace(response)

	if content == "" {
		return "", fmt.Errorf("empty summary response")
	}

	// Remove any markdown formatting
	content = p.cleanMarkdown(content)

	return content, nil
}

// cleanMarkdown removes basic markdown formatting
func (p *ResponseParser) cleanMarkdown(text string) string {
	// Remove bold/italic
	text = regexp.MustCompile(`\*\*([^*]+)\*\*`).ReplaceAllString(text, "$1")
	text = regexp.MustCompile(`\*([^*]+)\*`).ReplaceAllString(text, "$1")

	// Remove headers
	text = regexp.MustCompile(`^#{1,6}\s+`).ReplaceAllString(text, "")

	// Remove code blocks
	text = regexp.MustCompile("```[\\w]*\\s*\\n?([\\s\\S]*?)\\n?```").ReplaceAllString(text, "$1")

	return strings.TrimSpace(text)
}

// ExtractCodeBlocks extracts all code blocks from response
func (p *ResponseParser) ExtractCodeBlocks(response string) []string {
	var blocks []string

	pattern := regexp.MustCompile("```[\\w]*\\s*\\n?([\\s\\S]*?)\\n?```")
	matches := pattern.FindAllStringSubmatch(response, -1)

	for _, match := range matches {
		if len(match) > 1 {
			blocks = append(blocks, strings.TrimSpace(match[1]))
		}
	}

	return blocks
}

// SanitizeResponse removes common LLM artifacts from response
func (p *ResponseParser) SanitizeResponse(response string) string {
	// Remove "Here is..." prefixes
	patterns := []string{
		`^Here is.*?:\s*`,
		`^Here's.*?:\s*`,
		`^I'll.*?:\s*`,
		`^I've.*?:\s*`,
		`^The.*?is:\s*`,
		`^Below is.*?:\s*`,
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		response = re.ReplaceAllString(response, "")
	}

	return strings.TrimSpace(response)
}

// sanitizeModelIssueProvenance clears the engine-owned Origin of every
// model-reported issue and drops an assessment that is not well formed.
func sanitizeModelIssueProvenance(issues []types.ReviewIssue) {
	for i := range issues {
		issues[i].Origin = ""
		a := issues[i].Assessment
		if a == nil {
			continue
		}
		a.Status = strings.ToLower(strings.TrimSpace(a.Status))
		a.EvidenceID = strings.TrimSpace(a.EvidenceID)
		if !types.IsKnownAssessmentStatus(a.Status) || a.EvidenceID == "" {
			issues[i].Assessment = nil
		}
	}
}
