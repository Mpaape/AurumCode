package apply

import (
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/Mpaape/AurumCode/pkg/types"
)

// change is one validated suggestion reduced to its real edit: the ops cover
// only the changed region (leading and trailing unchanged lines are trimmed
// away, to be replaced by real file context), start is the 1-based line of
// the first op in the current file.
type change struct {
	path  string
	start int
	ops   []edit
	order int
}

// oldCount is how many current-file lines the change consumes.
func (c change) oldCount() int {
	n := 0
	for _, op := range c.ops {
		if op.kind != '+' {
			n++
		}
	}
	return n
}

// fileText is a file read once from the source: its lines and whether the
// last line ends in a newline.
type fileText struct {
	lines     []string
	lastEndNL bool
}

func readFileText(fsys fs.FS, name string) (*fileText, error) {
	data, err := fs.ReadFile(fsys, name)
	if err != nil {
		return nil, err
	}
	content := string(data)
	ft := &fileText{lastEndNL: strings.HasSuffix(content, "\n")}
	if content != "" {
		ft.lines = strings.Split(strings.TrimSuffix(content, "\n"), "\n")
	}
	return ft, nil
}

// fileGroup is every change that targets one file.
type fileGroup struct {
	path    string
	text    *fileText
	changes []change
}

// collectChanges validates every suggestion and groups the surviving changes
// by file, returning the file names in sorted order.
func collectChanges(suggestions []types.ReviewSuggestion, fsys fs.FS) (map[string]*fileGroup, []string, error) {
	groups := map[string]*fileGroup{}
	for i, s := range suggestions {
		name := normalizePath(s.File)
		if name == "" {
			continue
		}
		g := groups[name]
		if g == nil {
			g = &fileGroup{path: name}
			groups[name] = g
			text, err := readFileText(fsys, name)
			switch {
			case err == nil:
				g.text = text
			case !errors.Is(err, fs.ErrNotExist):
				return nil, nil, fmt.Errorf("%s: cannot read the working tree file: %w", name, err)
			}
		}
		c, ok, err := buildChange(s, name, i, g)
		if err != nil {
			return nil, nil, err
		}
		if ok {
			g.changes = append(g.changes, c)
		}
	}
	order := make([]string, 0, len(groups))
	for name, g := range groups {
		if len(g.changes) == 0 {
			continue
		}
		sort.SliceStable(g.changes, func(a, b int) bool {
			if g.changes[a].start != g.changes[b].start {
				return g.changes[a].start < g.changes[b].start
			}
			return g.changes[a].order < g.changes[b].order
		})
		order = append(order, name)
	}
	sort.Strings(order)
	return groups, order, nil
}

// buildChange validates a single suggestion against its file. ok is false
// when the suggestion must be skipped; an error means the working tree
// contradicts it.
func buildChange(s types.ReviewSuggestion, name string, order int, g *fileGroup) (change, bool, error) {
	proposed := splitLines(s.ProposedCode)
	if strings.TrimSpace(s.CurrentCode) == "" {
		return change{}, false, nil
	}
	current := splitLines(s.CurrentCode)
	if len(current) == 0 || equalLines(current, proposed) {
		return change{}, false, nil
	}
	start := anchorLine(s)
	if start <= 0 || (s.EndLine > 0 && start > s.EndLine) {
		return change{}, false, nil
	}
	if g.text == nil {
		return change{}, false, fmt.Errorf("%s: not found in the working tree", name)
	}
	ops := lcsDiff(current, proposed)
	lead, trail := 0, 0
	for lead < len(ops) && ops[lead].kind == ' ' {
		lead++
	}
	for trail < len(ops)-lead && ops[len(ops)-1-trail].kind == ' ' {
		trail++
	}
	c := change{path: name, start: start + lead, ops: ops[lead : len(ops)-trail], order: order}
	if err := checkAgainstFile(s, start, current, g.text); err != nil {
		return change{}, false, err
	}
	return c, true, nil
}

// checkAgainstFile confirms the suggestion's current_code is exactly what the
// file holds at the claimed lines.
func checkAgainstFile(s types.ReviewSuggestion, start int, current []string, ft *fileText) error {
	for i, want := range current {
		n := start + i
		if n > len(ft.lines) {
			return fmt.Errorf("%s:%d: line does not exist in the working tree (file has %d lines)", normalizePath(s.File), n, len(ft.lines))
		}
		if strings.TrimSuffix(ft.lines[n-1], "\r") != want {
			return fmt.Errorf("%s:%d: working tree does not match the suggestion's current_code", normalizePath(s.File), n)
		}
	}
	return nil
}
