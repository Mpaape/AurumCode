// Package grammar is AurumCode's only source of per-language structure. It
// holds no language table, no extension list and no regex per language: the
// set of languages, the file-name and shebang detection, and the symbol and
// import extraction all come from the generic, community-maintained tree-sitter
// grammars of the pure-Go runtime (github.com/odvcencio/gotreesitter, no cgo).
// A language that the runtime does not know is reported as having no
// structural context, and the reviewing model reads its text directly.
package grammar

import (
	"bytes"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

// NoStructure is the language name reported when the runtime has no grammar
// for a file.
const NoStructure = "unknown"

// MaxParseBytes bounds how much of one file is handed to the parser.
const MaxParseBytes = 1 << 20

// Languages enumerates, at run time, every grammar the linked runtime offers.
// The list is data from the runtime, never a literal in AurumCode.
func Languages() []string {
	all := grammars.AllLanguages()
	names := make([]string, 0, len(all))
	for _, e := range all {
		names = append(names, e.Name)
	}
	sort.Strings(names)
	return names
}

// Detect resolves the grammar for a file from its name and, failing that, its
// first line (shebang), both through the runtime's own detection. It returns
// "" when the runtime has no grammar for the file.
func Detect(name string, content []byte) string {
	if e := grammars.DetectLanguage(name); e != nil {
		return e.Name
	}
	first := content
	if i := bytes.IndexByte(first, '\n'); i >= 0 {
		first = first[:i]
	}
	if len(first) > 256 {
		first = first[:256]
	}
	if bytes.HasPrefix(first, []byte("#!")) {
		if e := grammars.DetectLanguageByShebang(string(first)); e != nil {
			return e.Name
		}
	}
	return ""
}

// Structure is the structural context extracted from one file.
type Structure struct {
	// Language is the runtime grammar name, or NoStructure.
	Language string
	// HasStructure reports whether the file parsed with a grammar. When false
	// the review proceeds on the text alone and must say so.
	HasStructure bool
	// Symbols are the definition names found by the grammar's tags query.
	Symbols []string
	// Imports are the dependency paths found by the runtime's extractor.
	Imports []string
	// Reason explains HasStructure == false.
	Reason string
}

// Analyze extracts structure from content using the grammar the runtime
// selects for name. It never panics and never fails: any problem becomes a
// Structure with HasStructure false and a Reason.
func Analyze(name string, content []byte) (s Structure) {
	s.Language = NoStructure
	defer func() {
		if r := recover(); r != nil {
			s = Structure{Language: s.Language, Reason: "grammar runtime failed on this file"}
		}
	}()
	langName := Detect(name, content)
	if langName == "" {
		s.Reason = "no grammar for this file in the runtime"
		return s
	}
	entry := grammars.DetectLanguageByName(langName)
	if entry == nil || entry.Language == nil {
		s.Reason = "no grammar for this file in the runtime"
		return s
	}
	s.Language = entry.Name
	if len(content) > MaxParseBytes {
		content = content[:MaxParseBytes]
	}
	lang := entry.Language()
	if lang == nil {
		s.Reason = "grammar failed to load"
		return s
	}
	tree, err := gotreesitter.NewParser(lang).Parse(content)
	if err != nil || tree == nil {
		s.Reason = "grammar could not parse the file"
		return s
	}
	s.HasStructure = true
	if q := grammars.ResolveTagsQuery(*entry); strings.TrimSpace(q) != "" {
		if o, oerr := gotreesitter.NewOutliner(lang, q); oerr == nil {
			syms, _ := o.OutlineTree(tree)
			s.Symbols = flatten(syms, nil)
		}
	}
	for _, imp := range gotreesitter.ExtractImports(tree) {
		if imp.Path != "" {
			s.Imports = append(s.Imports, imp.Path)
		}
	}
	return s
}

func flatten(syms []gotreesitter.OutlineSymbol, out []string) []string {
	for _, sym := range syms {
		if sym.Name != "" {
			out = append(out, sym.Name)
		}
		out = flatten(sym.Children, out)
	}
	return out
}

// LooksBinary decides from content alone, never from a name: a NUL byte in the
// leading window, or a high share of bytes that are neither text nor valid
// UTF-8, marks the file as binary.
func LooksBinary(content []byte) bool {
	window := content
	if len(window) > 8000 {
		window = window[:8000]
	}
	if len(window) == 0 {
		return false
	}
	if bytes.IndexByte(window, 0) >= 0 {
		return true
	}
	bad := 0
	for i := 0; i < len(window); {
		r, size := utf8.DecodeRune(window[i:])
		if r == utf8.RuneError && size <= 1 {
			// A multibyte rune cut by the window edge is not evidence.
			if i >= len(window)-3 {
				break
			}
			bad++
		} else if r < 0x20 && r != '\n' && r != '\r' && r != '\t' && r != '\f' {
			bad++
		}
		i += size
	}
	return bad*10 > len(window)
}

// LooksGenerated reports a self-declared generated file: one of its first
// lines says it was generated, language-neutrally, in the file's own words.
func LooksGenerated(content []byte) bool {
	head := content
	if len(head) > 2048 {
		head = head[:2048]
	}
	lines := strings.SplitN(strings.ToLower(string(head)), "\n", 12)
	for i, ln := range lines {
		if i == len(lines)-1 && len(lines) == 12 {
			break
		}
		if strings.Contains(ln, "@generated") ||
			(strings.Contains(ln, "generated") && (strings.Contains(ln, "do not edit") || strings.Contains(ln, "do not modify"))) ||
			strings.Contains(ln, "auto-generated") || strings.Contains(ln, "autogenerated") {
			return true
		}
	}
	return false
}
