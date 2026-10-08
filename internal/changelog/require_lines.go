package changelog

import "strings"

// addedLine is one line the new side has and the old side does not, with the
// level-2 heading it sits under in the new side.
type addedLine struct {
	number  int
	text    string
	section string
}

// normalizeLine folds runs of whitespace so indentation or trailing spaces
// never count as new information. Applied to a whole file it compares the
// two sides without any whitespace distinction.
func normalizeLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// addedLines compares the two sides as multisets of normalized lines: a line
// that only moved (reorder, consolidation) is not new, a duplicate copy of an
// existing line is new only beyond the copies the old side already had.
func addedLines(oldText, newText string) []addedLine {
	have := map[string]int{}
	for _, l := range strings.Split(oldText, "\n") {
		if n := normalizeLine(l); n != "" {
			have[n]++
		}
	}
	var out []addedLine
	section := ""
	for i, l := range strings.Split(newText, "\n") {
		n := normalizeLine(l)
		if strings.HasPrefix(n, "## ") {
			section = strings.TrimSpace(strings.TrimPrefix(n, "## "))
		}
		if n == "" {
			continue
		}
		if have[n] > 0 {
			have[n]--
			continue
		}
		out = append(out, addedLine{number: i + 1, text: l, section: section})
	}
	return out
}

// headingName strips the decorations a Keep a Changelog heading may carry:
// "[Unreleased]", "Unreleased - 2026-10-07", "[1.2.0] - 2026-10-07".
func headingName(heading string) string {
	name := strings.TrimSpace(heading)
	if i := strings.Index(name, " - "); i >= 0 {
		name = name[:i]
	}
	return strings.TrimSpace(strings.Trim(strings.TrimSpace(name), "[]"))
}

// sectionMatches compares a heading with the configured section name.
func sectionMatches(heading, section string) bool {
	return heading != "" && strings.EqualFold(headingName(heading), strings.TrimSpace(section))
}

// isReleaseHeading reports whether a heading names a version (consolidated
// release notes), using the engine's own strict semver parser.
func isReleaseHeading(heading string) bool {
	if heading == "" {
		return false
	}
	_, err := ParseVersion(headingName(heading))
	return err == nil
}
