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

// GitHub refuses (422) an issue whose title or body is longer than these
// (characters); the text is fitted to them, never the sync failed by them.
const (
	maxIssueTitle = 256
	maxIssueBody  = 65536
)

// IssueTitle is built only from structured fields (rule, file, line,
// severity): no text of the plugin's model is ever in a title. A path too
// long for GitHub's title is shortened from the left ("…" plus its end).
func IssueTitle(name string, f IssueFinding) string {
	suffix := ""
	if f.Line > 0 {
		suffix = fmt.Sprintf(":%d", f.Line)
	}
	head := fmt.Sprintf("[plugin %s] %s em ", name, oneLine(f.Rule))
	tail := fmt.Sprintf("%s (severidade %s)", suffix, severityLabel(f.Score))
	path := []rune(oneLine(f.Path))
	if room := maxIssueTitle - len([]rune(head)) - len([]rune(tail)); len(path) > room {
		if room < 2 {
			room = 2
		}
		path = append([]rune("…"), path[len(path)-room+1:]...)
	}
	title := []rune(head + string(path) + tail)
	if len(title) > maxIssueTitle {
		title = title[:maxIssueTitle]
	}
	return string(title)
}

// fitDescription cuts the description so the whole body stays within
// GitHub's limit, saying so in the body. The cut is deterministic: the same
// finding always gives the same body.
func fitDescription(text string, room int) string {
	r := []rune(text)
	if len(r) <= room {
		return text
	}
	const note = "\n[descrição cortada no limite de tamanho do corpo da issue no GitHub]"
	keep := room - len([]rune(note))
	if keep < 0 {
		keep = 0
	}
	return string(r[:keep]) + note
}

// IssueBody is the pt-BR body: the marker, the structured fields, the
// redacted description in a code block (shown as plain text: no markdown,
// no mention, no HTML comment the page would render) and a suggestion.
func IssueBody(name string, f IssueFinding) string {
	line := "não informada"
	if f.Line > 0 {
		line = fmt.Sprint(f.Line)
	}
	// A backtick in the description could close the code block and let
	// the plugin's text render as markdown: it is shown as a look-alike.
	text := strings.ReplaceAll(strings.TrimSpace(f.Text), "`", "ˋ")
	if text == "" {
		text = "(sem descrição)"
	}
	// The fixed parts take a few hundred characters plus the path; the
	// description gets the rest of the limit, with a margin.
	reserved := 2048 + 2*len([]rune(oneLine(f.Path))) + 2*len([]rune(oneLine(f.Rule)))
	text = fitDescription(text, maxIssueBody-reserved)
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
	fmt.Fprintf(&b, "\n```text\n%s\n```\n\n", text)
	fmt.Fprintln(&b, "### Sugestão")
	fmt.Fprintln(&b, "")
	fmt.Fprintln(&b, "Corrija o código no local indicado (a descrição acima traz a recomendação do plugin, quando houver).")
	fmt.Fprintln(&b, "Esta issue é fechada sozinha quando o achado some de uma varredura conclusiva, e reaberta se ele voltar.")
	return b.String()
}

// closeComment is the comment left on an issue whose finding disappeared.
const closeComment = "O achado não apareceu na última varredura conclusiva do plugin; a issue foi fechada pelo AurumCode. Se ele voltar, ela é reaberta."
