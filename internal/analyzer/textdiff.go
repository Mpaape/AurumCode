package analyzer

import (
	"bytes"
	"fmt"
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

// Input limits. A changed file is line-diffed only if BOTH sides fit inside
// all of them; otherwise the file is reported to the user and skipped, and
// no model call is made for it. See docs/specs/AUR-430.md ("Limits on what
// gets diffed") for the rationale and the exact user-visible messages.
const (
	// maxDiffLines caps lines per side.
	maxDiffLines = 10000

	// maxDiffBytes caps bytes per side. Minified or generated single-line
	// files pass the line cap trivially while still being huge, so size is
	// capped independently of line count.
	maxDiffBytes = 1 << 20 // 1 MiB

	// binarySniffLen is how much of a blob is inspected for a NUL byte.
	// This is git's own heuristic and its own window (FIRST_FEW_BYTES).
	binarySniffLen = 8000

	// maxTraceCells bounds the recorded search trace. The trace is the one
	// structure in this file whose size grows with the *edit distance*
	// rather than the file, so it is the one that needs a budget: at
	// distance d the band is d ints wide, making the whole trace d^2/2
	// cells. 4Mi cells is 32 MiB on a 64-bit build and admits every edit
	// script shorter than ~2896 edits exactly; past that diffLines falls
	// back to a whole-file replacement rather than allocating without
	// bound. Budgeting the trace instead of pre-rejecting on worst-case
	// size is what keeps a one-line change inside a 10000-line file exact
	// and nearly free -- its edit distance is 2 no matter how long the
	// file is.
	maxTraceCells = 4 << 20
)

// DiffNotice reports a changed file that was deliberately not line-diffed.
// Message is already user-facing and carries the path, so a caller can
// print it verbatim.
//
// This is a package-local type rather than a new field on types.Diff
// because pkg/types is outside this card's writable paths.
type DiffNotice struct {
	Path    string
	Message string
}

// classifyBlob reports why the changed file at path must not be line-diffed,
// or "" when it can be. It works on the raw bytes and never converts them to
// a string, so an oversized or binary blob is rejected before splitLines and
// myersDiff ever materialize per-line copies of it.
func classifyBlob(path string, content []byte) string {
	window := content
	if len(window) > binarySniffLen {
		window = window[:binarySniffLen]
	}
	if bytes.IndexByte(window, 0) >= 0 {
		return fmt.Sprintf("binary file, skipped: %s", path)
	}
	if len(content) > maxDiffBytes {
		return fmt.Sprintf("diff too large: %s (%d bytes > limit %d)", path, len(content), maxDiffBytes)
	}
	if n := countLines(content); n > maxDiffLines {
		return fmt.Sprintf("diff too large: %s (%d lines > limit %d)", path, n, maxDiffLines)
	}
	return ""
}

// countLines counts the lines in content the same way splitLines splits
// them -- one trailing newline is a terminator, not an empty final line --
// but without allocating the lines themselves.
func countLines(content []byte) int {
	if len(content) == 0 {
		return 0
	}
	body := content
	if body[len(body)-1] == '\n' {
		body = body[:len(body)-1]
	}
	return bytes.Count(body, []byte{'\n'}) + 1
}

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

// traceLevel is the recorded furthest-reaching-x vector at one edit
// distance, holding only the band of diagonals that distance can have
// reached: at distance d the forward pass has written exactly the d
// diagonals k in [-(d-1), d-1] whose parity matches d-1, so the level needs
// d ints rather than the full 2*(N+M)+1 width.
type traceLevel []int

// get returns the recorded furthest x on diagonal k at edit distance d.
//
// Reads outside the band return 0, which is exactly what the old
// full-width snapshot held there: the forward pass only ever writes inside
// the band, so every other slot still carried the zero the vector was
// created with. Keeping that behavior makes the banded trace an exact
// stand-in rather than an approximation. (The only out-of-band read the
// backtrack actually performs is d==0's probe of diagonal +1.)
//
// Callers only ever read diagonals whose parity matches d-1 -- see
// myersDiff's backtrack -- which is what makes the halved index exact.
func (t traceLevel) get(k, d int) int {
	if k < -(d-1) || k > d-1 {
		return 0
	}
	i := (k + d - 1) / 2
	if i < 0 || i >= len(t) {
		return 0
	}
	return t[i]
}

// snapshot records the current band of v for edit distance d.
func snapshot(v []int, offset, d int) traceLevel {
	level := make(traceLevel, d)
	for i := 0; i < d; i++ {
		k := -(d - 1) + 2*i
		level[i] = v[offset+k]
	}
	return level
}

// replaceAll is the bounded fallback edit script: delete every old line,
// then insert every new line. It is a correct -- merely non-minimal --
// rendering of any change, and it costs O(N+M). diffLines falls back to it
// when the minimal-script search would exceed maxTraceCells.
func replaceAll(a, b []string) []diffEdit {
	edits := make([]diffEdit, 0, len(a)+len(b))
	for _, line := range a {
		edits = append(edits, diffEdit{kind: diffDelete, text: line})
	}
	for _, line := range b {
		edits = append(edits, diffEdit{kind: diffInsert, text: line})
	}
	return edits
}

// myersDiff returns the minimal edit script transforming a into b using
// Myers' 1986 O(ND) shortest-edit-script algorithm: a forward pass records,
// for every distance d, the furthest-reaching x on each diagonal k; a
// backward pass then walks that recorded trace from (len(a), len(b)) to
// (0, 0) to recover the edits, which are reversed into forward order before
// returning. This is the standard textbook shape and is what `git diff`,
// GNU diff and most line-diff libraries build on; a naive O(N*M) LCS table
// was avoided because it would make reviewing a large changed file
// quadratically expensive.
//
// # MEMORY
//
// The recorded trace, not the algorithm, was this function's real cost.
// Snapshotting the *full* 2*(N+M)+1-wide vector at every distance -- and
// holding all of those snapshots alive until the return -- is
// O(D*(N+M)) of simultaneously-live memory. Because they are all live at
// once it is not garbage the collector or GOMEMLIMIT can do anything
// about: an 8000-line added file cost 1001 MiB, a real 12674-line one
// ~2.6 GB, and a ~22k-line file exhausted a 31 GB host. Two bounds fix
// that, and neither changes the edit script this returns for any input
// that fits inside them:
//
//   - each level stores only its reachable band (d ints, not 2*(N+M)+1),
//     which is every value the backtrack can read and nothing else;
//   - the accumulated trace is capped at maxTraceCells, past which the
//     search is abandoned for replaceAll's O(N+M) fallback.
//
// Note that Myers 1986's section 4b describes a further refinement -- a
// divide-and-conquer middle-snake search with a strict O(N+M) bound. This
// function deliberately does not implement it: 4b picks a different
// (equally minimal) script than the forward-greedy backtrack below, and
// the bounds above already remove the unbounded growth without perturbing
// any existing output.
func myersDiff(a, b []string) []diffEdit {
	n, m := len(a), len(b)
	maxD := n + m
	if maxD == 0 {
		return nil
	}

	offset := maxD
	v := make([]int, 2*maxD+1)
	trace := make([]traceLevel, 0, 64)
	cells := 0

	dFound := -1
searchLoop:
	for d := 0; d <= maxD; d++ {
		// Budget the trace before growing it, so the peak is the budget
		// rather than the budget plus one unbounded level.
		if cells+d > maxTraceCells {
			return replaceAll(a, b)
		}
		cells += d
		trace = append(trace, snapshot(v, offset, d))

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
		if k == -d || (k != d && snap.get(k-1, d) < snap.get(k+1, d)) {
			prevK = k + 1
		} else {
			prevK = k - 1
		}
		prevX := snap.get(prevK, d)
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
