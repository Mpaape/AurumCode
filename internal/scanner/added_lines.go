// The change scope of a scanner: the lines the reviewed commit range added.
// An engine whose findings are about code (sast, lint) keeps only the
// findings on these lines, so a pull request is judged by what it changed and
// never by the repository's history.
package scanner

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const gitBinary = "git"

// LineSet is the lines the reviewed range added, per path relative to the
// scan root.
type LineSet map[string]map[int]bool

// Touched reports whether the range added any line to path.
func (s LineSet) Touched(path string) bool { return len(s[path]) > 0 }

// Keep returns the findings on added lines, in their order.
func (s LineSet) Keep(findings []Finding) []Finding {
	var out []Finding
	for _, f := range findings {
		if s[f.Path][f.Line] {
			out = append(out, f)
		}
	}
	return out
}

// diffArgs compares the merge base of the range with its head (what a pull
// request shows), with fixed prefixes and no external diff driver, so the
// repository's own configuration cannot change the format read below.
// --relative: run from the review root, paths are relative to it (as go
// vet's are) even when the root is a subdirectory of the repository.
func diffArgs(r Range) []string {
	return []string{
		"-c", "core.quotePath=false",
		"diff", "--relative", "--no-color", "--no-ext-diff", "--no-renames", "--no-textconv",
		"--src-prefix=a/", "--dst-prefix=b/", "--unified=0",
		r.Base + "..." + r.Head, "--",
	}
}

// ErrNoRange: the caller gave no commit range, so no finding can be
// anchored to the change; reporting the whole tree instead would accuse the
// pull request of the repository's history.
var ErrNoRange = errors.New("scanner: no reviewed commit range")

// AddedLines runs git diff over the range in root and reads the added lines.
// An empty range is ErrNoRange: never "every line".
func AddedLines(ctx context.Context, run Command, root string, r Range) (LineSet, error) {
	if r.Empty() {
		return nil, ErrNoRange
	}
	stdout, _, err := run(ctx, root, gitBinary, diffArgs(r)...)
	if err != nil {
		return nil, fmt.Errorf("scanner: git diff: %w", err)
	}
	return parseAddedLines(stdout)
}

// parseAddedLines reads the "+++ b/<path>" and "@@ -a,b +c,d @@" lines of a
// zero-context unified diff.
func parseAddedLines(diff string) (LineSet, error) {
	set := LineSet{}
	path := ""
	for _, line := range strings.Split(diff, "\n") {
		switch {
		case strings.HasPrefix(line, "+++ \""):
			// git quotes a path with a tab, newline, quote or backslash even
			// with core.quotePath=false; dropping it would hide its lines.
			return nil, fmt.Errorf("scanner: quoted path in diff %q: %w", line, ErrInvalidOutput)
		case strings.HasPrefix(line, "+++ "):
			path = strings.TrimPrefix(strings.TrimPrefix(line, "+++ "), "b/")
			if path == "/dev/null" {
				path = ""
			}
		case strings.HasPrefix(line, "@@ ") && path != "":
			start, count, err := newRange(line)
			if err != nil {
				return nil, err
			}
			if set[path] == nil {
				set[path] = map[int]bool{}
			}
			for n := start; n < start+count; n++ {
				set[path][n] = true
			}
		}
	}
	return set, nil
}

// newRange reads the "+c,d" side of a hunk header; d defaults to 1.
func newRange(header string) (int, int, error) {
	fields := strings.Fields(header)
	if len(fields) < 3 || !strings.HasPrefix(fields[2], "+") {
		return 0, 0, fmt.Errorf("scanner: hunk header %q: %w", header, ErrInvalidOutput)
	}
	startRaw, countRaw, hasCount := strings.Cut(strings.TrimPrefix(fields[2], "+"), ",")
	start, err := strconv.Atoi(startRaw)
	count := 1
	if err == nil && hasCount {
		count, err = strconv.Atoi(countRaw)
	}
	if err != nil || start < 0 || count < 0 {
		return 0, 0, fmt.Errorf("scanner: hunk header %q: %w", header, ErrInvalidOutput)
	}
	return start, count, nil
}
