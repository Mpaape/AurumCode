package review

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/Mpaape/AurumCode/internal/prompt"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
)

// What the dedicated triage shows the model about one finding. A finding a
// history scanner found in a commit is shown with that commit's content
// around its line: the reviewed tree may no longer have the value, and its
// line of the same number has nothing to do with the finding. A finding the
// model could not decide is shown again with a wider window, the whole file
// when it is short enough.

// FileSource reads a file at a commit (or "HEAD") of the reviewed
// repository's object database.
type FileSource interface {
	FileAt(ref, path string) ([]byte, error)
}

// EvidenceContext builds the triage's view of a finding.
type EvidenceContext struct {
	// Source reads the reviewed repository; nil reads nothing.
	Source FileSource
	// Hidden reports a path whose content never leaves the process (the
	// repository's secret or ignored paths); nil hides nothing.
	Hidden func(path string) bool
	// Redact filters every line shown; nil leaves it.
	Redact func(string) string
}

// Context sizes, in lines around the finding's.
const (
	commitRadius = 8
	wideRadius   = 60
	// wholeFileLines is the longest file shown whole on the wider pass.
	wholeFileLines = 400
)

// shortCommit is how much of a commit id the note names.
const shortCommit = 12

// Describe returns item with its snippet and note for the triage: the
// content of commit (empty for a finding of the reviewed tree) around the
// line, and with wide the larger window. What cannot be read keeps the
// item's own snippet for a finding of the tree, and shows nothing for a
// finding of a commit (never the current file at an unrelated line).
func (c EvidenceContext) Describe(item prompt.EvidenceItem, commit string, wide bool) prompt.EvidenceItem {
	ref, radius := "HEAD", commitRadius
	if wide {
		radius = wideRadius
	}
	if commit != "" {
		ref = commit
		short := commit
		if len(short) > shortCommit {
			short = short[:shortCommit]
		}
		item.Snippet = ""
		item.Note = fmt.Sprintf("valor encontrado no commit %s do intervalo revisado (histórico): pode não existir mais no arquivo atual; o trecho é o daquele commit", short)
	} else if !wide || strings.EqualFold(item.Side, "LEFT") {
		// A removed line is numbered in the old file: the current file at
		// that number is unrelated, so the item keeps its own snippet.
		return item
	}
	data, err := c.read(ref, item.File)
	if err != nil {
		if commit != "" {
			item.Note += fmt.Sprintf("; o conteúdo desse commit não pôde ser lido (%v), e o arquivo atual não é mostrado porque a linha dele não tem relação com o achado", c.redact(err.Error()))
		}
		return item
	}
	item.Snippet = c.window(string(data), item.Line, radius)
	if wide {
		item.Note = strings.TrimPrefix(item.Note+"; contexto ampliado: o trecho mostrado antes não bastou para decidir", "; ")
	}
	return item
}

// read reads path at ref unless it is hidden.
func (c EvidenceContext) read(ref, path string) ([]byte, error) {
	if c.Source == nil {
		return nil, fmt.Errorf("repositório revisado indisponível")
	}
	if c.Hidden != nil && c.Hidden(path) {
		return nil, fmt.Errorf("caminho reservado pela configuração do repositório")
	}
	return c.Source.FileAt(ref, path)
}

// window is the lines of text within radius of line, numbered; the whole
// text when it is short and radius is the wider one.
func (c EvidenceContext) window(text string, line, radius int) string {
	lines := c.redactedLines(text)
	start, end := line-radius, line+radius
	if radius == wideRadius && len(lines) <= wholeFileLines {
		start, end = 1, len(lines)
	}
	if start < 1 {
		start = 1
	}
	if end > len(lines) {
		end = len(lines)
	}
	var out strings.Builder
	for n := start; n <= end; n++ {
		marker := " "
		if n == line {
			marker = ">"
		}
		fmt.Fprintf(&out, "%s%5d| %s\n", marker, n, lines[n-1])
	}
	return strings.TrimRight(out.String(), "\n")
}

// reKeyBegin and reKeyEnd open and close a private key block.
var (
	reKeyBegin = regexp.MustCompile(`-----BEGIN[ \t][A-Z ]*PRIVATE KEY-----`)
	reKeyEnd   = regexp.MustCompile(`-----END[ \t][A-Z ]*PRIVATE KEY-----`)
)

// redactedLines is text redacted as a whole, then split into its lines, so
// a secret spanning lines (a private key's base64 body) is masked like the
// filter masks it whole. A whole-text redaction that changes the line count
// (a block collapsed into the marker) cannot keep the numbering, so then
// every line inside a private key block is masked and each line redacted
// on its own.
func (c EvidenceContext) redactedLines(text string) []string {
	raw := strings.Split(strings.TrimRight(text, "\n"), "\n")
	whole := strings.Split(strings.TrimRight(c.redact(text), "\n"), "\n")
	if len(whole) == len(raw) {
		return whole
	}
	out := make([]string, len(raw))
	inKey := false
	for i, l := range raw {
		if reKeyBegin.MatchString(l) {
			inKey = true
		}
		if inKey {
			out[i] = redaction.Marker
		} else {
			out[i] = c.redact(l)
		}
		if reKeyEnd.MatchString(l) {
			inKey = false
		}
	}
	return out
}

func (c EvidenceContext) redact(text string) string {
	if c.Redact == nil {
		return text
	}
	return c.Redact(text)
}
