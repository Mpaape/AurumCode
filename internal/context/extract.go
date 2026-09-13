package context

import (
	"bytes"
	"path"
	"regexp"
	"sort"
	"strings"
)

// maxSymbolAlternation bounds how many distinct symbols are folded into the
// single reference-scan regex. Beyond it the symbol scan is skipped and the
// omission is recorded, keeping the scan linear in file bytes.
const maxSymbolAlternation = 4096

// Symbol-definition regexes (pure-Go heuristics, language family only).
var (
	goFuncRe   = regexp.MustCompile(`(?m)^[ \t]*(?:func|type)[ \t]+([A-Za-z_][A-Za-z0-9_]*)`)
	goMethodRe = regexp.MustCompile(`(?m)^[ \t]*func[ \t]+\([^)]*\)[ \t]+([A-Za-z_][A-Za-z0-9_]*)`)
	goVarRe    = regexp.MustCompile(`(?m)^[ \t]*(?:var|const)[ \t]+([A-Za-z_][A-Za-z0-9_]*)`)

	jsSymbolRe = regexp.MustCompile(`(?m)^[ \t]*(?:export[ \t]+)?(?:function|class|const|let|var)[ \t]+([A-Za-z_$][A-Za-z0-9_$]*)`)
	pySymbolRe = regexp.MustCompile(`(?m)^[ \t]*(?:def|class)[ \t]+([A-Za-z_][A-Za-z0-9_]*)`)
	cSymbolRe  = regexp.MustCompile(`(?m)^[ \t]*[A-Za-z_][A-Za-z0-9_ \t*]+[ \t]+([A-Za-z_][A-Za-z0-9_]*)[ \t]*\(`)
)

// Import-statement regexes.
var (
	goImportRe   = regexp.MustCompile(`(?m)^[ \t]*import[ \t]+(?:[A-Za-z_][A-Za-z0-9_]*[ \t]+)?"([^"]+)"`)
	jsImportRe   = regexp.MustCompile(`(?m)^[ \t]*import[^"']*["']([^"']+)["']`)
	jsRequireRe  = regexp.MustCompile(`(?m)\brequire[ \t]*\([ \t]*["']([^"']+)["']`)
	pyImportRe   = regexp.MustCompile(`(?m)^[ \t]*(?:import|from)[ \t]+([A-Za-z_][A-Za-z0-9_.]*)`)
	goBlockQuote = regexp.MustCompile(`"([^"]+)"`)
)

// familyOf maps a file extension to a coarse language family used to select
// extraction heuristics.
func familyOf(rel string) string {
	ext := strings.ToLower(strings.TrimPrefix(path.Ext(rel), "."))
	switch ext {
	case "go":
		return "go"
	case "js", "jsx", "ts", "tsx", "mjs", "cjs":
		return "js"
	case "py", "pyw", "pyi":
		return "py"
	case "c", "h", "cc", "cpp", "cxx", "hpp", "hxx", "hh", "cs", "java", "rs":
		return "c"
	default:
		return ""
	}
}

// extractSymbols returns the symbol names defined in content for the given
// language family, in encounter order (deduplicated by the caller).
func extractSymbols(family string, content []byte) []string {
	var out []string
	switch family {
	case "go":
		out = appendMatches(out, content, goFuncRe)
		out = appendMatches(out, content, goMethodRe)
		out = appendMatches(out, content, goVarRe)
	case "js":
		out = appendMatches(out, content, jsSymbolRe)
	case "py":
		out = appendMatches(out, content, pySymbolRe)
	case "c":
		out = appendMatches(out, content, cSymbolRe)
	}
	return out
}

// extractImports returns the import/dependency paths referenced in content for
// the given language family, in encounter order (deduplicated by the caller).
func extractImports(family string, content []byte) []string {
	var out []string
	switch family {
	case "go":
		out = appendMatches(out, content, goImportRe)
		out = append(out, goBlockImports(content)...)
	case "js":
		out = appendMatches(out, content, jsImportRe)
		out = appendMatches(out, content, jsRequireRe)
	case "py":
		out = appendMatches(out, content, pyImportRe)
	}
	return out
}

func appendMatches(out []string, content []byte, re *regexp.Regexp) []string {
	for _, m := range re.FindAllSubmatch(content, -1) {
		if len(m) > 1 && m[1] != nil {
			out = append(out, string(m[1]))
		}
	}
	return out
}

// goBlockImports collects the quoted paths inside a Go `import ( ... )` block.
func goBlockImports(content []byte) []string {
	var out []string
	inBlock := false
	for _, ln := range strings.Split(string(content), "\n") {
		trimmed := strings.TrimSpace(ln)
		if inBlock {
			if strings.HasPrefix(trimmed, ")") {
				inBlock = false
				continue
			}
			if m := goBlockQuote.FindStringSubmatch(ln); m != nil {
				out = append(out, m[1])
			}
			continue
		}
		if strings.HasPrefix(trimmed, "import") && strings.Contains(trimmed, "(") {
			inBlock = true
			if m := goBlockQuote.FindStringSubmatch(ln); m != nil {
				out = append(out, m[1])
			}
		}
	}
	return out
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
