package plugin

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
)

// IssueLabel is the label every issue of a plugin carries; the listing is
// filtered by it.
func IssueLabel(name string) string { return "aurumcode-plugin:" + name }

var safeFingerprint = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)

// issueFingerprint keeps a safe fingerprint as is and replaces any other by
// its sha256, so the marker can never be broken by the plugin's text. The
// mapping is a function of the value: the identity stays stable.
func issueFingerprint(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if safeFingerprint.MatchString(raw) {
		return raw
	}
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// IssueMarker is the hidden first line of an issue's body: the identity of
// the finding it tracks.
func IssueMarker(name, fingerprint string) string {
	return "<!-- aurumcode-plugin:" + name + ":" + fingerprint + " -->"
}

var markerLine = regexp.MustCompile(`^<!-- aurumcode-plugin:([a-z0-9][a-z0-9_-]*):([A-Za-z0-9._-]{1,128}) -->$`)

// markerOf reads the fingerprint of an issue of this plugin from the FIRST
// line of its body only; text further down (the redacted description) can
// never forge an identity. "" when the issue is not one of this plugin's.
func markerOf(name, body string) string {
	first, _, _ := strings.Cut(body, "\n")
	m := markerLine.FindStringSubmatch(strings.TrimRight(first, "\r"))
	if m == nil || m[1] != name {
		return ""
	}
	return m[2]
}

// severityLabel names a security-severity score in pt-BR.
func severityLabel(score float64) string {
	switch {
	case score >= 9:
		return "crítica"
	case score >= 7:
		return "alta"
	case score >= 4:
		return "média"
	case score > 0:
		return "baixa"
	default:
		return "informativa"
	}
}

// oneLine keeps a structured field on one line, without markdown that
// could break the table or the title.
func oneLine(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	return strings.NewReplacer("|", "/", "`", "'", "<", "‹", ">", "›", "@", "＠").Replace(s)
}

// IssueTitle is built only from structured fields (rule, file, line,
// severity): no text of the plugin's model is ever in a title.
func IssueTitle(name string, f IssueFinding) string {
	loc := oneLine(f.Path)
	if f.Line > 0 {
		loc = fmt.Sprintf("%s:%d", loc, f.Line)
	}
	return fmt.Sprintf("[plugin %s] %s em %s (severidade %s)", name, oneLine(f.Rule), loc, severityLabel(f.Score))
}

// fence returns a code fence longer than any backtick run in text, so the
// redacted description is shown as plain text: no markdown, no mention, no
// HTML comment that the page would render.
func fence(text string) string {
	longest, run := 0, 0
	for _, r := range text {
		if r == '`' {
			run++
			if run > longest {
				longest = run
			}
		} else {
			run = 0
		}
	}
	if longest < 3 {
		return "```"
	}
	return strings.Repeat("`", longest+1)
}

// IssueBody is the pt-BR body: the marker, the structured fields, the
// redacted description in a code block and a suggestion.
func IssueBody(name string, f IssueFinding) string {
	line := "não informada"
	if f.Line > 0 {
		line = fmt.Sprint(f.Line)
	}
	text := strings.TrimSpace(f.Text)
	if text == "" {
		text = "(sem descrição)"
	}
	fc := fence(text)
	var b strings.Builder
	fmt.Fprintln(&b, IssueMarker(name, f.Fingerprint))
	fmt.Fprintf(&b, "Achado do plugin agendado `%s`, sincronizado pelo AurumCode.\n\n", name)
	fmt.Fprintln(&b, "| campo | valor |")
	fmt.Fprintln(&b, "| --- | --- |")
	fmt.Fprintf(&b, "| regra | %s |\n", oneLine(f.Rule))
	fmt.Fprintf(&b, "| arquivo | %s |\n", oneLine(f.Path))
	fmt.Fprintf(&b, "| linha | %s |\n", line)
	fmt.Fprintf(&b, "| severidade | %s (%.1f) |\n\n", severityLabel(f.Score), f.Score)
	fmt.Fprintln(&b, "### Descrição (redigida)")
	fmt.Fprintf(&b, "\n%stext\n%s\n%s\n\n", fc, text, fc)
	fmt.Fprintln(&b, "### Sugestão")
	fmt.Fprintln(&b, "")
	fmt.Fprintln(&b, "Corrija o código no local indicado (a descrição acima traz a recomendação do plugin, quando houver).")
	fmt.Fprintln(&b, "Esta issue é fechada sozinha quando o achado some de uma varredura conclusiva, e reaberta se ele voltar.")
	return b.String()
}

// closeComment is the comment left on an issue whose finding disappeared.
const closeComment = "O achado não apareceu na última varredura conclusiva do plugin; a issue foi fechada pelo AurumCode. Se ele voltar, ela é reaberta."
