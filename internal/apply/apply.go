// Package apply turns validated review suggestions into a safe, applyable
// unified diff. It is the "one-click fix" seam between the reviewer's
// suggestions and a patch (or GitHub suggested-change comment) that a human
// or tool can apply.
//
// The patch is a standard unified diff: "--- a/x" / "+++ b/x" headers (or
// /dev/null for a new or removed file, see CreateFilePatch and
// DeleteFilePatch), "@@ -l,n +l,n @@" hunks, and three
// lines of real file context around every change, so that plain `git apply`
// and `patch -p1` accept it without any flag. The context is read from the
// file the suggestion names, through an fs.FS handed in by the caller.
//
// Every decision fails closed. A suggestion is skipped outright when its
// proposed code does not actually change anything, its file path is empty or
// unsafe (absolute, or containing a ".." segment), or its hunk location
// cannot be anchored to a positive line number. A suggestion whose
// current_code does not match the file, or that overlaps another one, makes
// the whole plan fail: nothing is ever partially applied.
//
// BuildPlan and BuildPatch read only the suggestions and the given fs.FS;
// they write nothing, perform no network access, and depend on nothing
// beyond the standard library and the read-only types.ReviewSuggestion model.
package apply

import (
	"fmt"
	"io/fs"
	"path"
	"strings"

	"github.com/Mpaape/AurumCode/pkg/types"
)

// contextLines is the number of unchanged lines kept around every change,
// the unified diff default that `git apply` and `patch` require.
const contextLines = 3

// Line is a single source line to add or remove, carrying its 1-based
// position in the file it belongs to. Additions use the new file's numbering;
// Removals use the current file's numbering.
type Line struct {
	Number int
	Text   string
}

// FileEdit is one hunk of a safe, applyable change to a single file.
// Additions and Removals describe the semantic edit; Hunk is the rendered
// unified diff hunk (the "@@" header, context and change lines, without the
// "---/+++" file header).
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
// applied safely, reading each named file from fsys to render real context.
// The plan is deterministic and ordered by path, then hunk start line.
func BuildPlan(suggestions []types.ReviewSuggestion, fsys fs.FS) (*Plan, error) {
	if fsys == nil {
		return nil, fmt.Errorf("apply: no file source to read context from")
	}
	byFile, order, err := collectChanges(suggestions, fsys)
	if err != nil {
		return nil, err
	}
	plan := &Plan{Files: []FileEdit{}}
	for _, name := range order {
		edits, err := renderFile(byFile[name])
		if err != nil {
			return nil, err
		}
		plan.Files = append(plan.Files, edits...)
	}
	return plan, nil
}

// BuildPatch renders the safe suggestions as a single standard unified diff
// (one file header per path, followed by that file's hunks in line order).
// It returns an empty string when no suggestion can be applied.
func BuildPatch(suggestions []types.ReviewSuggestion, fsys fs.FS) (string, error) {
	plan, err := BuildPlan(suggestions, fsys)
	if err != nil {
		return "", err
	}
	var sb strings.Builder
	lastPath := ""
	for _, fe := range plan.Files {
		if fe.Path != lastPath {
			fmt.Fprintf(&sb, "--- a/%s\n+++ b/%s\n", fe.Path, fe.Path)
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
