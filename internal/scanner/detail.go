package scanner

import "strings"

// MaxDetailBytes bounds the failure detail an inconclusive scan carries: a
// diagnosis, never a dump of the engine's output.
const MaxDetailBytes = 240

// detailEllipsis ends a detail cut by MaxDetailBytes.
const detailEllipsis = "..."

// Summarize is err as one bounded line: whitespace (newlines included)
// collapsed to single spaces and the text cut at MaxDetailBytes on a rune
// boundary. nil is "". It never redacts: the caller applies the review's
// redaction filter before the detail reaches any output.
func Summarize(err error) string {
	if err == nil {
		return ""
	}
	line := strings.Join(strings.Fields(err.Error()), " ")
	if len(line) <= MaxDetailBytes {
		return line
	}
	cut := MaxDetailBytes - len(detailEllipsis)
	for cut > 0 && !utf8RuneStart(line[cut]) {
		cut--
	}
	return line[:cut] + detailEllipsis
}

// utf8RuneStart reports whether b starts a UTF-8 sequence.
func utf8RuneStart(b byte) bool { return b&0xC0 != 0x80 }
