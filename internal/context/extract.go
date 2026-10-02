package context

import (
	"bytes"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/Mpaape/AurumCode/internal/grammar"
)

// maxSymbolAlternation bounds how many distinct symbols are folded into the
// single reference-scan regex. Beyond it the symbol scan is skipped and the
// omission is recorded, keeping the scan linear in file bytes.
const maxSymbolAlternation = 4096

// structureOf asks the grammar runtime (internal/grammar) for the structure of
// one file. No language is named here: which grammar applies, and whether one
// exists at all, is the runtime's answer. Binary content never reaches a
// parser.
func (r *Resolver) structureOf(rel string, content []byte) grammar.Structure {
	if grammar.LooksBinary(content) {
		return grammar.Structure{Language: grammar.NoStructure, Reason: "binary content"}
	}
	return r.grammar.Analyze(rel, content)
}

// importKeys derives the three match keys for a changed file: its directory
// path, its base name without extension, and the base name of its directory.
func importKeys(rel string) (dir, stem, dirBase string) {
	dir = path.Dir(rel)
	if dir == "." {
		dir = ""
	}
	base := path.Base(rel)
	stem = strings.TrimSuffix(base, path.Ext(base))
	dirBase = path.Base(dir)
	return dir, stem, dirBase
}

// matchesImport reports whether an import/dependency path points at a changed
// file, using the directory, stem, and directory-base keys.
func matchesImport(imp string, dirKeys, stemKeys, dirBaseKeys map[string]struct{}) bool {
	n := normalizeImport(imp)
	if n == "" {
		return false
	}
	if _, ok := dirKeys[n]; ok {
		return true
	}
	for d := range dirKeys {
		if strings.HasSuffix(n, "/"+d) {
			return true
		}
	}
	last := path.Base(n)
	if _, ok := stemKeys[last]; ok {
		return true
	}
	if _, ok := stemKeys[strings.TrimSuffix(last, path.Ext(last))]; ok {
		return true
	}
	if _, ok := dirBaseKeys[last]; ok {
		return true
	}
	return false
}

func normalizeImport(imp string) string {
	s := strings.TrimSpace(imp)
	s = strings.Trim(s, `"'`)
	for strings.HasPrefix(s, "./") {
		s = s[2:]
	}
	for strings.HasPrefix(s, "../") {
		s = s[3:]
	}
	s = strings.TrimSuffix(s, "/")
	return s
}

// symbolAlternation builds a single word-boundary regex matching any changed
// symbol, sorted for determinism. If there are too many symbols it returns nil
// and a Dropped note instead.
func symbolAlternation(symbolFiles map[string][]string) (*regexp.Regexp, string) {
	if len(symbolFiles) == 0 {
		return nil, ""
	}
	if len(symbolFiles) > maxSymbolAlternation {
		return nil, "symbol reference scan skipped: " + itoa(len(symbolFiles)) + " symbols exceed limit " + itoa(maxSymbolAlternation)
	}
	names := make([]string, 0, len(symbolFiles))
	for name := range symbolFiles {
		names = append(names, name)
	}
	sort.Strings(names)
	parts := make([]string, len(names))
	for i, name := range names {
		parts[i] = regexp.QuoteMeta(name)
	}
	return regexp.MustCompile(`\b(?:` + strings.Join(parts, "|") + `)\b`), ""
}

func indexOf(content []byte, sub []byte) int {
	if len(sub) == 0 {
		return -1
	}
	return bytes.Index(content, sub)
}

func lineOf(content []byte, offset int) int {
	if offset < 0 || offset > len(content) {
		return 0
	}
	return bytes.Count(content[:offset], []byte{'\n'}) + 1
}
