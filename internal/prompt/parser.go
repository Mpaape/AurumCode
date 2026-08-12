package prompt

import (
	"encoding/json"
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
	// Raw is the response that failed to parse, bounded to a safe preview
	// length so a large or adversarial response cannot balloon an error
	// message that ends up in logs.
	Raw string
	Err error
}

const parseErrorRawPreviewLimit = 512

func newParseError(kind ParseErrorKind, raw string, err error) *ParseError {
	if len(raw) > parseErrorRawPreviewLimit {
		raw = raw[:parseErrorRawPreviewLimit] + "... (truncated)"
	}
	return &ParseError{Kind: kind, Raw: raw, Err: err}
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

// ParseReviewResponse parses a review response from LLM
func (p *ResponseParser) ParseReviewResponse(response string) (*types.ReviewResult, error) {
	// Extract JSON from response (handle markdown code blocks)
	jsonContent := p.extractJSON(response)

	if jsonContent == "" {
		return p.degradedOrError(response, ParseErrorNoJSON, nil)
	}

	// Parse JSON
	var result types.ReviewResult
	if err := json.Unmarshal([]byte(jsonContent), &result); err != nil {
		return p.degradedOrError(response, ParseErrorInvalidJSON, err)
	}

	// Validate
	if err := p.validateReviewResult(&result); err != nil {
		return p.degradedOrError(response, ParseErrorValidation, err)
	}

	return &result, nil
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
			Summary: "Degraded parse: recovered findings from free-form text; the model's response was not valid JSON.",
			Metadata: map[string]string{
				"parse_mode":  "degraded",
				"parse_cause": string(kind),
			},
		}, nil
	}
	return nil, newParseError(kind, response, cause)
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
