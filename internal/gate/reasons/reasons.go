// Package reasons phrases the gate's machine-readable inconclusive reason
// codes for people, per review language. Both the gate (its decision lines)
// and the presentation (the closing line of a report) read it, the same way
// they share internal/gate/facts, and it imports neither.
package reasons

import (
	"strings"

	"github.com/Mpaape/AurumCode/internal/i18n"
)

// reasonKeyPrefix names an inconclusive reason code in the i18n catalog:
// gate.reason.<code>, or gate.reason.<name>_detail for a "name:detail" code
// (deliberation_limit:max_rounds, contributor_error:<contributor>).
const reasonKeyPrefix = "gate.reason."

// Text is one machine-readable inconclusive reason code as the
// review's language shows it. In English it is the code itself, the bytes
// earlier releases published; in Portuguese it is a sentence followed by the
// code in brackets, so support can still match on it. A code the catalog
// does not describe still keeps its own token, never a silent blank.
func Text(language, code string) string {
	code = strings.TrimSpace(code)
	if text, ok := i18n.Lookup(language, reasonKeyPrefix+code); ok {
		return text
	}
	if name, detail, found := strings.Cut(code, ":"); found {
		if _, ok := i18n.Lookup(language, reasonKeyPrefix+name+"_detail"); ok {
			return i18n.Format(language, reasonKeyPrefix+name+"_detail", detail)
		}
	}
	return i18n.Format(language, reasonKeyPrefix+"unknown", code)
}

// Texts is Text over a comma-joined reason list, in order.
func Texts(language, list string) []string {
	codes := strings.Split(list, ",")
	out := make([]string, 0, len(codes))
	for _, code := range codes {
		out = append(out, Text(language, code))
	}
	return out
}

// List joins the reasons of a comma-joined list for a sentence: the codes
// themselves in English, the sentences in Portuguese.
func List(language, list string) string {
	return strings.Join(Texts(language, list), i18n.Text(language, "gate.reason_list_separator"))
}

// IsLine reports whether line is the policy gate's inconclusive line (Line)
// in language: it starts with the catalog template's text before the
// reasons, whatever they are.
func IsLine(language, line string) bool {
	prefix, _, _ := strings.Cut(i18n.Text(language, "gate.inconclusive_line"), "%s")
	return prefix != "" && strings.HasPrefix(line, prefix)
}

// Line is the policy gate's line for an inconclusive run: "review
// inconclusive (<codes>)" in English, byte for byte as before; the reasons
// as sentences in Portuguese.
func Line(language, list string) string {
	joined := strings.Join(Texts(language, list), i18n.Text(language, "gate.reason_separator"))
	return i18n.Format(language, "gate.inconclusive_line", joined)
}
