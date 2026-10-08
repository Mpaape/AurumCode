package verify

import (
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Mpaape/AurumCode/internal/prompt"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// Bounds of what one verification shows.
const (
	// citedRadius is how many lines around the cited line are shown.
	citedRadius = 12
	// definitionLines is how many lines from a symbol occurrence are shown.
	definitionLines = 15
	// maxSymbols bounds the symbols of one finding looked up.
	maxSymbols = 6
	// maxOccurrences bounds the windows of one symbol in one file.
	maxOccurrences = 3
	// maxSiblingFiles bounds the files of the cited file's directory read.
	maxSiblingFiles = 40
	// maxExcerptBytes bounds the code of one verification prompt.
	maxExcerptBytes = 24 << 10
)

// identifier is a name the finding's text may mention.
var identifier = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]{2,}`)

// Source is the reviewed revision the verification reads
// (internal/review/tools.Revision).
type Source interface {
	Read(rel string) ([]byte, error)
	Paths() []string
}

// Declarer names the symbols a file declares (the grammar provider's
// structure); nil declares nothing, so only the cited window is shown.
type Declarer func(path string, data []byte) []string

// window is one excerpt under construction.
type window struct {
	path       string
	start, end int
	label      string
}

// excerpts is the code shown for issue: the window around the cited line,
// then the occurrences of the symbols the finding names in the files of the
// cited file's directory that declare them. files holds the full content of
// every file shown, which the quote is checked against.
func excerpts(src Source, declared Declarer, issue types.ReviewIssue) ([]prompt.VerificationExcerpt, map[string]string, error) {
	data, err := src.Read(issue.File)
	if err != nil {
		return nil, nil, err
	}
	files := map[string]string{issue.File: string(data)}
	lines := map[string][]string{issue.File: splitLines(string(data))}
	cited := citedWindow(issue, len(lines[issue.File]))
	windows := []window{cited}
	if declared != nil {
		windows = append(windows, symbolWindows(src, declared, issue, cited, files, lines)...)
	}
	var out []prompt.VerificationExcerpt
	size := 0
	for _, w := range windows {
		text := strings.Join(lines[w.path][w.start-1:w.end], "\n")
		if size+len(text) > maxExcerptBytes && len(out) > 0 {
			break
		}
		size += len(text)
		out = append(out, prompt.VerificationExcerpt{Path: w.path, StartLine: w.start, EndLine: w.end, Label: w.label, Text: text})
	}
	shown := map[string]string{}
	for _, e := range out {
		shown[e.Path] = files[e.Path]
	}
	return out, shown, nil
}

// citedWindow is the lines around the cited line (the file's start when
// the line is outside it).
func citedWindow(issue types.ReviewIssue, n int) window {
	line := issue.Line
	if line < 1 || line > n {
		line = 1
	}
	start, end := max(1, line-citedRadius), min(n, line+citedRadius)
	if n == 0 {
		start, end = 1, 0
	}
	return window{path: issue.File, start: start, end: end, label: fmt.Sprintf("linha citada %d", issue.Line)}
}

// symbolWindows finds the symbols the finding names among those the files
// of its directory declare, and shows where each occurs.
func symbolWindows(src Source, declared Declarer, issue types.ReviewIssue, cited window, files map[string]string, lines map[string][]string) []window {
	names := mentioned(issue)
	if len(names) == 0 {
		return nil
	}
	var out []window
	looked := 0
	for _, p := range siblings(src, issue.File) {
		data, err := src.Read(p)
		if err != nil {
			continue
		}
		for _, name := range declaredAmong(declared(p, data), names) {
			if looked == maxSymbols {
				return out
			}
			looked++
			if _, ok := files[p]; !ok {
				files[p], lines[p] = string(data), splitLines(string(data))
			}
			out = append(out, occurrences(p, name, lines[p], cited)...)
		}
	}
	return out
}

// mentioned is the set of identifiers in the finding's own text.
func mentioned(issue types.ReviewIssue) map[string]bool {
	out := map[string]bool{}
	for _, m := range identifier.FindAllString(issue.Message+" "+issue.Evidence, -1) {
		out[m] = true
	}
	return out
}

// declaredAmong is the declared symbols the finding names, sorted.
func declaredAmong(symbols []string, names map[string]bool) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range symbols {
		if names[s] && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

// siblings is the cited file first, then the other files of its directory.
func siblings(src Source, file string) []string {
	out := []string{file}
	dir := path.Dir(file)
	for _, p := range src.Paths() {
		if len(out) == maxSiblingFiles {
			break
		}
		if p != file && path.Dir(p) == dir {
			out = append(out, p)
		}
	}
	return out
}

// occurrences is a window from each line naming name, outside the cited
// window, up to maxOccurrences.
func occurrences(p, name string, lines []string, cited window) []window {
	var out []window
	for i, line := range lines {
		n := i + 1
		if p == cited.path && n >= cited.start && n <= cited.end {
			continue
		}
		if !hasWord(line, name) {
			continue
		}
		out = append(out, window{path: p, start: n, end: min(len(lines), n+definitionLines-1), label: "símbolo " + name})
		if len(out) == maxOccurrences {
			break
		}
	}
	return out
}

// splitLines splits text into lines without the final empty one.
func splitLines(text string) []string {
	return strings.Split(strings.TrimSuffix(text, "\n"), "\n")
}

// hasWord reports name in line with no identifier rune touching either end.
func hasWord(line, name string) bool {
	for from := 0; ; {
		i := strings.Index(line[from:], name)
		if i < 0 {
			return false
		}
		start, end := from+i, from+i+len(name)
		before, _ := utf8.DecodeLastRuneInString(line[:start])
		after, _ := utf8.DecodeRuneInString(line[end:])
		if (start == 0 || !isIdentRune(before)) && (end == len(line) || !isIdentRune(after)) {
			return true
		}
		from = start + 1
	}
}

func isIdentRune(r rune) bool { return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) }
