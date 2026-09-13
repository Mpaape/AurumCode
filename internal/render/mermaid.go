package render

import (
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/Mpaape/AurumCode/pkg/types"
)

// Mermaid renders a deterministic Mermaid flowchart describing the changed
// flow of a diff. Each changed file becomes a node, and a directed edge is
// added from file A to file B when A's diff text references B's path or
// base name (e.g. a Go import). When no dependency can be inferred it falls
// back to a simple flowchart that lists the changed files with no edges.
func Mermaid(diff *types.Diff) (string, error) {
	if diff == nil || len(diff.Files) == 0 {
		return "", fmt.Errorf("cannot render a diagram for an empty diff")
	}

	byPath := make(map[string]types.DiffFile, len(diff.Files))
	files := make([]string, 0, len(diff.Files))
	for _, f := range diff.Files {
		if f.Path == "" {
			continue
		}
		if _, seen := byPath[f.Path]; !seen {
			files = append(files, f.Path)
			byPath[f.Path] = f
		}
	}
	if len(files) == 0 {
		return "", fmt.Errorf("cannot render a diagram for a diff with no file paths")
	}
	sort.Strings(files)

	index := make(map[string]int, len(files))
	content := make(map[string]string, len(files))
	for i, p := range files {
		index[p] = i
		content[p] = fileText(byPath[p])
	}

	var b strings.Builder
	b.WriteString("flowchart TD\n")
	for i, p := range files {
		fmt.Fprintf(&b, "    n%d[%s]\n", i, strconv.Quote(p))
	}
	for i, src := range files {
		for _, dep := range files {
			if dep == src {
				continue
			}
			if references(content[src], dep) {
				fmt.Fprintf(&b, "    n%d --> n%d\n", i, index[dep])
			}
		}
	}
	return b.String(), nil
}

// fileText flattens a file's hunks into a single string, stripping the
// leading '+', '-', or ' ' diff marker from each line so imports and
// references are searchable.
func fileText(f types.DiffFile) string {
	var b strings.Builder
	for _, h := range f.Hunks {
		for _, line := range h.Lines {
			if len(line) > 0 && (line[0] == '+' || line[0] == '-' || line[0] == ' ') {
				line = line[1:]
			}
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
	return b.String()
}

// references reports whether content mentions the given path by its full
// path, its extension-less path, or its base name. Needles shorter than
// three characters are ignored to avoid trivial matches.
func references(content, path string) bool {
	base := filepath.Base(path)
	base = strings.TrimSuffix(base, filepath.Ext(base))
	noExt := strings.TrimSuffix(path, filepath.Ext(path))
	for _, needle := range []string{path, noExt, base} {
		if len(needle) < 3 {
			continue
		}
		if strings.Contains(content, needle) {
			return true
		}
	}
	return false
}
