package prompt

import (
	"encoding/json"
	"strings"
)

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
	matches := jsonFencePattern.FindStringSubmatch(response)

	if len(matches) > 1 {
		return p.repairJSON(strings.TrimSpace(matches[1]))
	}

	// Try to find raw JSON (look for { ... })
	matches = rawJSONPattern.FindStringSubmatch(response)

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
	repaired = trailingCommaPattern.ReplaceAllString(repaired, "$1")

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
