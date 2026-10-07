package scanner

import "strings"

// MaxDetailBytes bounds the failure detail an inconclusive scan carries: a
// diagnosis, never a dump of the engine's output.
const MaxDetailBytes = 240

// detailEllipsis ends a detail cut by MaxDetailBytes.
const detailEllipsis = "..."

// Redactor removes credentials from text that is about to be published.
type Redactor func(string) string

// Summarize is a failed scan's error as one publishable line. Order is the
// point: redact runs first, over the whole raw text with its line breaks,
// so a secret is never cut below the redaction patterns and a header on a
// line of its own still matches; only then is whitespace collapsed to
// single spaces and the text cut at MaxDetailBytes on a rune boundary. A
// nil error is ""; so is a nil redact, since an unredacted detail is never
// published.
func Summarize(err error, redact Redactor) string {
	if err == nil || redact == nil {
		return ""
	}
	line := strings.Join(strings.Fields(redact(err.Error())), " ")
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
