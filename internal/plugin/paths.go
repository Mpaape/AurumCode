package plugin

import (
	"os"
	"path/filepath"
	"strings"
)

// The classes of a refused path.
const (
	pathAbsolute = "absolute"
	pathEscapes  = "outside_tree"
	pathMissing  = "missing"
	pathEmpty    = "empty"
	pathOutput   = "plugin_output_dir"
)

// checkPath resolves a finding's path against root. It refuses an absolute
// path, a path with "..", a path that does not exist and one whose real
// location (symlinks resolved) leaves the tree, and returns the cleaned
// slash form. A path inside excluded (the plugin's writable output
// directory, relative to root) is refused too: what the plugin wrote is not
// the scanned code.
func checkPath(root, excluded, p string) (string, string) {
	p = strings.TrimSpace(strings.TrimPrefix(p, "file://"))
	switch {
	case p == "":
		return "", pathEmpty
	case strings.HasPrefix(p, "/") || filepath.IsAbs(p) || strings.Contains(p, `\`):
		return "", pathAbsolute
	case !filepath.IsLocal(filepath.FromSlash(p)):
		return "", pathEscapes
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", pathMissing
	}
	real, err := filepath.EvalSymlinks(filepath.Join(realRoot, filepath.FromSlash(p)))
	if err != nil {
		return "", pathMissing
	}
	rel, err := filepath.Rel(realRoot, real)
	if err != nil || !filepath.IsLocal(rel) {
		return "", pathEscapes
	}
	if _, err := os.Stat(real); err != nil {
		return "", pathMissing
	}
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(p)))
	if excluded != "" && (clean == excluded || strings.HasPrefix(clean, excluded+"/")) {
		return "", pathOutput
	}
	return clean, ""
}

// outputDir is the directory of the plugin's output relative to root, in
// slash form; "" when it is not inside root (nothing to exclude) or is root.
func outputDir(root, input string) string {
	absRoot, err1 := filepath.Abs(root)
	absDir, err2 := filepath.Abs(filepath.Dir(input))
	if err1 != nil || err2 != nil {
		return ""
	}
	rel, err := filepath.Rel(absRoot, absDir)
	if err != nil || rel == "." || !filepath.IsLocal(rel) {
		return ""
	}
	return filepath.ToSlash(rel)
}
