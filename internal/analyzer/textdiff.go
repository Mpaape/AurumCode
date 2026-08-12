package analyzer

import (
	"strings"

	"github.com/Mpaape/AurumCode/pkg/types"
)

// This file is new: it did not exist in the restored c12d7ab engine. The
// original engine only ever consumed an already-parsed types.Diff (built
// from a GitHub PR payload upstream); it never turned raw file content into
// diff lines itself. cmd/aurumcode needs exactly that step to review a local
// git repository, so it lives here in internal/analyzer -- the package that
// already owns "what does a diff mean" -- rather than being invented inline
// in cmd/aurumcode.

// diffKind classifies one line of a computed line-diff.
type diffKind int

const (
	diffEqual diffKind = iota
	diffDelete
	diffInsert
)

type diffEdit struct {
	kind diffKind
	text string
}

// BuildDiffFile builds a types.DiffFile for path by diffing oldContent
// against newContent with a full-context, single-hunk line diff: every line
// of both sides appears, prefixed " " (context), "-" (removed) or "+"
// (added), exactly like a unified diff body without the +/-3 line windowing.
// analyzer.AnalyzeDiff and prompt.Builder only ever look at line prefixes,
// so the single full hunk is sufficient context for this engine's purposes
// and is far simpler to get right than minimal hunk splitting.
func BuildDiffFile(path, oldContent, newContent string) types.DiffFile {
	oldLines := splitLines(oldContent)
	newLines := splitLines(newContent)

	return types.DiffFile{
		Path: path,
		Hunks: []types.DiffHunk{
			{
				OldStart: 1,
				OldLines: len(oldLines),
				NewStart: 1,
				NewLines: len(newLines),
				Lines:    diffLines(oldLines, newLines),
			},
		},
	}
}

// splitLines splits content on "\n" and drops one trailing terminator, so a
// file that ends with a newline (the common case) does not produce a
// spurious empty final line. Empty content yields zero lines, so an added or
// deleted file reports the correct one-sided line count.
func splitLines(content string) []string {
	if content == "" {
		return nil
	}
	content = strings.TrimSuffix(content, "\n")
	return strings.Split(content, "\n")
}

// diffLines renders the Myers edit script between oldLines and newLines as
// unified-diff-style lines.
func diffLines(oldLines, newLines []string) []string {
	edits := myersDiff(oldLines, newLines)
	if len(edits) == 0 {
		return nil
	}
	out := make([]string, 0, len(edits))
	for _, e := range edits {
		switch e.kind {
		case diffEqual:
			out = append(out, " "+e.text)
		case diffDelete:
			out = append(out, "-"+e.text)
		case diffInsert:
			out = append(out, "+"+e.text)
		}
	}
	return out
}

// myersDiff returns the minimal edit script transforming a into b using
// Myers' 1986 O(ND) shortest-edit-script algorithm: a forward pass records,
// for every distance d, the furthest-reaching x on each diagonal k; a
// backward pass then walks that recorded trace from (len(a), len(b)) to
// (0, 0) to recover the edits, which are reversed into forward order before
// returning. This is the standard textbook shape (see Myers 1986, section
// 4b) and is what `git diff`, GNU diff and most line-diff libraries build
// on; a naive O(N*M) LCS table was avoided because it would make reviewing
// a large changed file quadratically expensive.
func myersDiff(a, b []string) []diffEdit {
	n, m := len(a), len(b)
	maxD := n + m
	if maxD == 0 {
		return nil
	}

	offset := maxD
	v := make([]int, 2*maxD+1)
	trace := make([][]int, 0, maxD+1)

	dFound := -1
searchLoop:
	for d := 0; d <= maxD; d++ {
		snap := make([]int, len(v))
		copy(snap, v)
		trace = append(trace, snap)

		for k := -d; k <= d; k += 2 {
			var x int
			if k == -d || (k != d && v[offset+k-1] < v[offset+k+1]) {
				x = v[offset+k+1]
			} else {
				x = v[offset+k-1] + 1
			}
			y := x - k

			for x < n && y < m && a[x] == b[y] {
				x++
				y++
			}

			v[offset+k] = x

			if x >= n && y >= m {
				dFound = d
				break searchLoop
			}
		}
	}
	if dFound < 0 {
		// d==maxD always finds x==n,y==m, so this is unreachable; guard it
		// anyway rather than trust that invariant silently.
		dFound = maxD
	}

	var edits []diffEdit
	x, y := n, m
	for d := dFound; d >= 0; d-- {
		snap := trace[d]
		k := x - y

		var prevK int
		if k == -d || (k != d && snap[offset+k-1] < snap[offset+k+1]) {
			prevK = k + 1
		} else {
			prevK = k - 1
		}
		prevX := snap[offset+prevK]
		prevY := prevX - prevK

		for x > prevX && y > prevY {
			edits = append(edits, diffEdit{kind: diffEqual, text: a[x-1]})
			x--
			y--
		}
		if d > 0 {
			if x == prevX {
				edits = append(edits, diffEdit{kind: diffInsert, text: b[y-1]})
			} else {
				edits = append(edits, diffEdit{kind: diffDelete, text: a[x-1]})
			}
		}
		x, y = prevX, prevY
	}

	for i, j := 0, len(edits)-1; i < j; i, j = i+1, j-1 {
		edits[i], edits[j] = edits[j], edits[i]
	}
	return edits
}
