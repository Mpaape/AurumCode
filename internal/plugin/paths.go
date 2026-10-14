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
)

// checkPath resolves a finding's path against root. It refuses an absolute
// path, a path with "..", a path that does not exist and one whose real
// location (symlinks resolved) leaves the tree, and returns the cleaned
// slash form.
func checkPath(root, p string) (string, string) {
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
	return filepath.ToSlash(filepath.Clean(filepath.FromSlash(p))), ""
}
