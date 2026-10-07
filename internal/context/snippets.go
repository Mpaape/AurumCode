package context

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Mpaape/AurumCode/internal/analyzer"
)

// Snippet bounds (AUR-470): how many excerpts a pack carries, how many lines
// of context surround the referencing line, and how long one line may be.
const (
	DefaultMaxSnippets = 24
	snippetContext     = 2
	snippetLineBytes   = 240
)

// Snippet kinds. A snippet is context, never a verdict: a reference to a
// changed symbol says the file uses it, not that the use is broken.
const (
	SnippetUse  = "uso"
	SnippetTest = "teste"
)

// Snippet is the excerpt around one reference to the change, in a file
// outside the change set: the model reads it to explain the impact of the
// change on that file. Text holds numbered lines ("<n>: <line>").
type Snippet struct {
	File   string `json:"file"`
	Line   int    `json:"line"`
	Symbol string `json:"symbol"`
	Kind   string `json:"kind"`
	Text   string `json:"text"`
}

// WithExclude makes the resolver skip every path exclude reports (the
// policy's ignore globs and the secret-file catalog): such a file is never
// read, never referenced and never excerpted; the omission is declared in
// Pack.Dropped.
func (r *Resolver) WithExclude(exclude func(rel string) bool) *Resolver {
	r.exclude = exclude
	return r
}

// excluded reports a path the policy keeps out of the context.
func (r *Resolver) excluded(rel string) bool {
	return r.exclude != nil && r.exclude(rel)
}

// keepIncluded drops excluded paths, declaring each omission.
func (r *Resolver) keepIncluded(paths []string, pack *Pack) []string {
	if r.exclude == nil {
		return paths
	}
	out := paths[:0:0]
	for _, p := range paths {
		if r.excluded(p) {
			pack.Dropped = appendDropped(pack.Dropped, "excluded by policy "+p)
			continue
		}
		out = append(out, p)
	}
	return out
}

// attachSnippets excerpts the references, one snippet per file and line,
// ordered by file then line (the same revision always yields the same
// excerpts), bounded by DefaultMaxSnippets; what is left out is declared
// in Dropped.
func (r *Resolver) attachSnippets(root string, pack *Pack) {
	tests := analyzer.NewLanguageDetectorWith(r.grammar)
	contents := map[string][]string{}
	seen := map[string]bool{}
	refs := append([]Reference(nil), pack.References...)
	sort.SliceStable(refs, func(i, j int) bool {
		if refs[i].File != refs[j].File {
			return refs[i].File < refs[j].File
		}
		return refs[i].Line < refs[j].Line
	})
	for _, ref := range refs {
		key := fmt.Sprintf("%s:%d", ref.File, ref.Line)
		if seen[key] || r.excluded(ref.File) {
			continue
		}
		seen[key] = true
		if len(pack.Snippets) == DefaultMaxSnippets {
			pack.Dropped = appendDropped(pack.Dropped, "snippets truncated at "+itoa(DefaultMaxSnippets))
			return
		}
		lines, ok := contents[ref.File]
		if !ok {
			data, _, err := r.readFile(root, ref.File)
			if err != nil {
				pack.Dropped = appendDropped(pack.Dropped, "unreadable "+ref.File+": "+err.Error())
				contents[ref.File] = nil
				continue
			}
			lines = strings.Split(string(data), "\n")
			contents[ref.File] = lines
		}
		if lines == nil || ref.Line < 1 || ref.Line > len(lines) {
			continue
		}
		kind := SnippetUse
		if tests.IsTestFile(ref.File) {
			kind = SnippetTest
		}
		pack.Snippets = append(pack.Snippets, Snippet{File: ref.File, Line: ref.Line, Symbol: ref.Symbol, Kind: kind, Text: excerpt(lines, ref.Line)})
	}
}

// excerpt is lines around line (1-based), numbered, each clipped.
func excerpt(lines []string, line int) string {
	first, last := line-snippetContext, line+snippetContext
	if first < 1 {
		first = 1
	}
	if last > len(lines) {
		last = len(lines)
	}
	var b strings.Builder
	for i := first; i <= last; i++ {
		text := strings.TrimRight(lines[i-1], " \t\r")
		if len(text) > snippetLineBytes {
			cut := snippetLineBytes
			for cut > 0 && (text[cut]&0xC0) == 0x80 {
				cut--
			}
			text = text[:cut] + "…"
		}
		fmt.Fprintf(&b, "%d: %s\n", i, text)
	}
	return b.String()
}
