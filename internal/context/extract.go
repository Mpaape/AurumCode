package context

import (
	"bytes"
	"path"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/Mpaape/AurumCode/internal/grammar"
)

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
	// A dotted module path ("pkg.util", or "pkg.util.helper" for a name
	// imported from it) names the file of one of its slashed prefixes.
	if !strings.Contains(n, "/") && strings.Contains(n, ".") {
		parts := strings.Split(n, ".")
		for i := len(parts); i > 0; i-- {
			if matchesPath(strings.Join(parts[:i], "/"), dirKeys, stemKeys, dirBaseKeys) {
				return true
			}
		}
	}
	return matchesPath(n, dirKeys, stemKeys, dirBaseKeys)
}

// matchesPath is matchesImport for one normalized path.
func matchesPath(n string, dirKeys, stemKeys, dirBaseKeys map[string]struct{}) bool {
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

// symbolRegexByLanguage builds, per language, a single word-boundary regex
// matching every changed symbol that language defines, sorted for
// determinism.
func symbolRegexByLanguage(symbolLangs map[string]map[string]bool) map[string]*regexp.Regexp {
	byLang := map[string][]string{}
	for name, langs := range symbolLangs {
		for lang := range langs {
			byLang[lang] = append(byLang[lang], name)
		}
	}
	out := make(map[string]*regexp.Regexp, len(byLang))
	for lang, names := range byLang {
		sort.Strings(names)
		parts := make([]string, len(names))
		for i, name := range names {
			parts[i] = regexp.QuoteMeta(name)
		}
		out[lang] = regexp.MustCompile(`\b(?:` + strings.Join(parts, "|") + `)\b`)
	}
	return out
}

// isIdentifier reports whether a grammar's definition name is an identifier
// with a letter (a letter or underscore, then letters, digits or
// underscores; a blank "_" names nothing): punctuation
// and quoted or expanded words such as ":" or "\"$fn\"" are not symbols another
// file can reference.
func isIdentifier(name string) bool {
	letter := false
	for i, c := range name {
		switch {
		case unicode.IsLetter(c):
			letter = true
		case c == '_':
		case i > 0 && unicode.IsDigit(c):
		default:
			return false
		}
	}
	return letter
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
