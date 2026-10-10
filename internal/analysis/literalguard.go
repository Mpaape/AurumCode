package analysis

import "regexp"

// matchOutsideLiteral reports whether re has a match in body whose first
// byte is code, not the inside of a string or raw-string literal. A hit that
// starts inside a literal is a mention (a docstring or a message that
// documents the dangerous call); the search then resumes one byte past that
// start, so a real call later on the same line is still found instead of
// being hidden behind the mention. inRaw is the raw-string state entering
// the line.
func matchOutsideLiteral(re *regexp.Regexp, body string, inRaw bool) bool {
	for start := 0; start < len(body); {
		loc := re.FindStringIndex(body[start:])
		if loc == nil {
			return false
		}
		pos := start + loc[0]
		if !isInsideStringLiteral(body, pos, inRaw) {
			return true
		}
		start = pos + 1
	}
	return false
}
