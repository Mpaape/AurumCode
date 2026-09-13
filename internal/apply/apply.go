// Package apply turns validated review suggestions into a safe, applyable
// unified diff. It is the "one-click fix" seam between the reviewer's
// suggestions and a patch (or GitHub suggested-change comment) that a human
// or tool can apply.
//
// Every decision here fails closed. A suggestion is skipped outright when its
// current code is empty, its proposed code does not actually change anything,
// its file path is empty or unsafe (absolute, or containing a ".." segment),
// or its hunk location cannot be anchored to a positive line number. Nothing
// is ever partially applied: a suggestion is either rendered whole as a
// standard unified diff hunk or dropped from the plan.
//
// BuildPlan and BuildPatch are pure and side-effect free. They read only the
// suggestions handed to them, write nothing to disk, perform no network
// access, and depend on nothing beyond the standard library and the read-only
// types.ReviewSuggestion model.
package apply

import (
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/Mpaape/AurumCode/pkg/types"
)

// Line is a single source line to add or remove, carrying its 1-based
// position in the file it belongs to. Additions use the new file's numbering;
// Removals use the current file's numbering.
type Line struct {
	Number int
	Text   string
}

// FileEdit is one safe, applyable change to a single file. Additions and
// Removals describe the semantic edit; Hunk is the rendered unified diff hunk
// (the "@@" header and its lines, without the "---/+++" file header).
type FileEdit struct {
	Path      string
	Additions []Line
	Removals  []Line
	Hunk      string
}

// Plan is the set of safe file edits derived from a batch of suggestions.
type Plan struct {
	Files []FileEdit
}

// BuildPlan reduces a batch of review suggestions to the subset that can be
// applied safely. Unsafe or ambiguous suggestions are skipped; the returned
// plan is deterministic and ordered by path, then hunk start line.
func BuildPlan(suggestions []types.ReviewSuggestion) (*Plan, error) {
	type located struct {
		file      FileEdit
		path      string
		startLine int
		order     int
	}
	var edits []located
	for i, s := range suggestions {
		fe, startLine, ok := buildEdit(s)
		if !ok {
			continue
		}
		edits = append(edits, located{file: fe, path: fe.Path, startLine: startLine, order: i})
	}
	sort.SliceStable(edits, func(i, j int) bool {
		if edits[i].path != edits[j].path {
			return edits[i].path < edits[j].path
		}
		if edits[i].startLine != edits[j].startLine {
			return edits[i].startLine < edits[j].startLine
		}
		return edits[i].order < edits[j].order
	})
	plan := &Plan{Files: make([]FileEdit, 0, len(edits))}
	for _, e := range edits {
		plan.Files = append(plan.Files, e.file)
	}
	return plan, nil
}

// BuildPatch renders the safe suggestions as a single standard unified diff
// (one "---/+++" file header per path, followed by that file's hunks in line
// order). It returns an empty string when no suggestion can be applied.
func BuildPatch(suggestions []types.ReviewSuggestion) (string, error) {
	plan, err := BuildPlan(suggestions)
	if err != nil {
		return "", err
	}
	if len(plan.Files) == 0 {
		return "", nil
	}
	var sb strings.Builder
	lastPath := ""
	for _, fe := range plan.Files {
		if fe.Path != lastPath {
			fmt.Fprintf(&sb, "--- a/%s\n", fe.Path)
			fmt.Fprintf(&sb, "+++ b/%s\n", fe.Path)
			lastPath = fe.Path
		}
		sb.WriteString(fe.Hunk)
	}
	return sb.String(), nil
}

// edit is one step of a line-level diff: ' ' context, '-' removal, '+'
// addition. The LCS backtrack below is deterministic, so equal inputs always
// yield the same edit script.
type edit struct {
	kind byte
	text string
}

// buildEdit validates a single suggestion and, when it passes, renders its
// hunk. The second return value is the resolved hunk start line; ok is false
// when the suggestion must be skipped.
func buildEdit(s types.ReviewSuggestion) (FileEdit, int, bool) {
	var zero FileEdit
	file := normalizePath(s.File)
	if file == "" {
		return zero, 0, false
	}
	if strings.TrimSpace(s.CurrentCode) == "" {
		return zero, 0, false
	}
	current := splitLines(s.CurrentCode)
	if len(current) == 0 {
		return zero, 0, false
	}
	proposed := splitLines(s.ProposedCode)
	if equalLines(current, proposed) {
		return zero, 0, false
	}
	startLine := anchorLine(s)
	if startLine <= 0 {
		return zero, 0, false
	}
	if s.EndLine > 0 && startLine > s.EndLine {
		return zero, 0, false
	}
	fe := renderEdit(file, startLine, lcsDiff(current, proposed))
	if len(fe.Removals) == 0 && len(fe.Additions) == 0 {
		return zero, 0, false
	}
	return fe, startLine, true
}

// anchorLine resolves the hunk start line from the suggestion's location
// hints, preferring the explicit block start, then the single-line reference.
func anchorLine(s types.ReviewSuggestion) int {
	if s.StartLine > 0 {
		return s.StartLine
	}
	if s.Line > 0 {
		return s.Line
	}
	return 0
}

// renderEdit turns an edit script into a FileEdit, tracking the 1-based line
// numbers of every added and removed line.
func renderEdit(file string, startLine int, ops []edit) FileEdit {
	fe := FileEdit{Path: file}
	oldCount, newCount := 0, 0
	for _, op := range ops {
		switch op.kind {
		case ' ':
			oldCount++
			newCount++
		case '-':
			oldCount++
		case '+':
			newCount++
		}
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "@@ -%d,%d +%d,%d @@\n", startLine, oldCount, startLine, newCount)
	oldLine, newLine := startLine, startLine
	for _, op := range ops {
		switch op.kind {
		case ' ':
			sb.WriteString(" ")
			sb.WriteString(op.text)
			sb.WriteString("\n")
			oldLine++
			newLine++
		case '-':
			fe.Removals = append(fe.Removals, Line{Number: oldLine, Text: op.text})
			sb.WriteString("-")
			sb.WriteString(op.text)
			sb.WriteString("\n")
			oldLine++
		case '+':
			fe.Additions = append(fe.Additions, Line{Number: newLine, Text: op.text})
			sb.WriteString("+")
			sb.WriteString(op.text)
			sb.WriteString("\n")
			newLine++
		}
	}
	fe.Hunk = sb.String()
	return fe
}

// lcsDiff computes a deterministic longest-common-subsequence edit script
// between the current and proposed lines.
func lcsDiff(a, b []string) []edit {
	n, m := len(a), len(b)
	dp := make([][]int, n+1)
	for i := range dp {
		dp[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			switch {
			case a[i] == b[j]:
				dp[i][j] = dp[i+1][j+1] + 1
			case dp[i+1][j] >= dp[i][j+1]:
				dp[i][j] = dp[i+1][j]
			default:
				dp[i][j] = dp[i][j+1]
			}
		}
	}
	var ops []edit
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case a[i] == b[j]:
			ops = append(ops, edit{kind: ' ', text: a[i]})
			i++
			j++
		case dp[i+1][j] >= dp[i][j+1]:
			ops = append(ops, edit{kind: '-', text: a[i]})
			i++
		default:
			ops = append(ops, edit{kind: '+', text: b[j]})
			j++
		}
	}
	for ; i < n; i++ {
		ops = append(ops, edit{kind: '-', text: a[i]})
	}
	for ; j < m; j++ {
		ops = append(ops, edit{kind: '+', text: b[j]})
	}
	return ops
}

// normalizePath rejects any path that is empty, absolute, a Windows
// drive-qualified path, or that escapes the repository root via a ".."
// segment. It returns "" for every unsafe value.
func normalizePath(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	if value == "" || strings.ContainsRune(value, 0) {
		return ""
	}
	if len(value) >= 2 && value[1] == ':' &&
		((value[0] >= 'A' && value[0] <= 'Z') || (value[0] >= 'a' && value[0] <= 'z')) {
		return ""
	}
	clean := path.Clean(value)
	if clean == "." || path.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, "../") {
		return ""
	}
	return clean
}

// splitLines normalizes a code block into its lines, treating a single
// trailing newline as insignificant and accepting CRLF input.
func splitLines(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.TrimSuffix(s, "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

func equalLines(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
