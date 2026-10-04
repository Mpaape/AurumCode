package govet

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/Mpaape/AurumCode/internal/scanner"
)

const gitBinary = "git"

// lineSet is the lines the reviewed range added, per path.
type lineSet map[string]map[int]bool

// keep returns the findings on added lines, in their order.
func (s lineSet) keep(findings []scanner.Finding) []scanner.Finding {
	var out []scanner.Finding
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
func diffArgs(r scanner.Range) []string {
	return []string{
		"-c", "core.quotePath=false",
		"diff", "--no-color", "--no-ext-diff", "--no-renames", "--no-textconv",
		"--src-prefix=a/", "--dst-prefix=b/", "--unified=0",
		r.Base + "..." + r.Head, "--",
	}
}

// addedLines runs git diff over the range and reads the added lines.
func addedLines(ctx context.Context, run scanner.Command, root string, r scanner.Range) (lineSet, error) {
	stdout, _, err := run(ctx, root, gitBinary, diffArgs(r)...)
	if err != nil {
		return nil, fmt.Errorf("govet: git diff: %w", err)
	}
	return parseAddedLines(stdout)
}

// parseAddedLines reads the "+++ b/<path>" and "@@ -a,b +c,d @@" lines of a
// zero-context unified diff.
func parseAddedLines(diff string) (lineSet, error) {
	set := lineSet{}
	path := ""
	for _, line := range strings.Split(diff, "\n") {
		switch {
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
		return 0, 0, fmt.Errorf("govet: hunk header %q: %w", header, scanner.ErrInvalidOutput)
	}
	startRaw, countRaw, hasCount := strings.Cut(strings.TrimPrefix(fields[2], "+"), ",")
	start, err := strconv.Atoi(startRaw)
	count := 1
	if err == nil && hasCount {
		count, err = strconv.Atoi(countRaw)
	}
	if err != nil || start < 0 || count < 0 {
		return 0, 0, fmt.Errorf("govet: hunk header %q: %w", header, scanner.ErrInvalidOutput)
	}
	return start, count, nil
}
