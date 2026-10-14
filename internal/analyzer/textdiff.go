package analyzer

import (
	"fmt"
	"strings"

	"github.com/Mpaape/AurumCode/internal/grammar"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// This file is new: it did not exist in the restored c12d7ab engine. The
// original engine only ever consumed an already-parsed types.Diff (built
// from a GitHub PR payload upstream); it never turned raw file content into
// diff lines itself. cmd/aurumcode needs exactly that step to review a local
// git repository, so it lives here in internal/analyzer -- the package that
// already owns "what does a diff mean" -- rather than being invented inline
// in cmd/aurumcode.

// Search budgets. No changed text file is ever left out of the review for
// its size (the owner's rule: no limit may drop code from the review); what
// is budgeted is the minimal-edit-script search, never the file. Past either
// budget the diff falls back to replaceAll, a correct whole-file replacement
// that costs O(N+M), so a huge file is reviewed with a coarser diff instead
// of being skipped, and memory stays bounded. See docs/specs/AUR-430.md.
const (
	// maxSearchSteps bounds the search's work (diagonal visits plus
	// snake comparisons), so time is bounded the same way memory is. An
	// ordinary change costs about N+M steps (edit distance times the file),
	// so 128Mi steps keeps every realistic diff exact; only a search that
	// would grind for seconds falls back.
	maxSearchSteps = 1 << 27

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
	// Reason is the short, stable cause: "binary", "generated" or "no patch".
	// A file with a Reason was NOT reviewed; it must never count as approved.
	Reason string
	// DeclaredFormat: the binary content is a format the catalog declares
	// out of the review's scope (grammar.DeclaredBinaryFormat: listed
	// extension, its signature and binary content). Only such a file may be
	// declared ignored instead of partial.
	DeclaredFormat bool
}

// ClassifyInspectBytes is how much of a file's start the content check
// reads: the binary sniff window, the generated-marker header and every
// catalog signature sit inside it. A caller that only classifies (the --pr
// path) reads this prefix, never the whole file. A signature past it can
// only fail to match, which keeps the file unreviewed (fail closed).
const ClassifyInspectBytes = 64 << 10

// classifyBlob reports why the changed file at path must not be line-diffed,
// or "" when it can be. It works on the raw bytes and never converts them to
// a string, so a binary blob is rejected before splitLines and myersDiff ever
// materialize per-line copies of it. Size is never a reason: a large text
// file is diffed (see maxSearchSteps and maxTraceCells).
func classifyBlob(path string, content []byte) string {
	window := content
	if len(window) > binarySniffLen {
		window = window[:binarySniffLen]
	}
	// Binary is decided from the content (a NUL byte, or mostly non-text
	// bytes), never from the file name.
	if grammar.LooksBinary(window) {
		return fmt.Sprintf("binary file, skipped: %s", path)
	}
	// A file that says, in its own words, that it was generated is out of the
	// review's reach: the source that generates it is what a human reviews.
	if grammar.LooksGenerated(content) {
		return fmt.Sprintf("generated file, skipped: %s", path)
	}
	return ""
}

// NoticeReasonNoPatch is the Reason of a changed file whose content nobody
// inspected: the API gave no patch and no verified copy was available.
const NoticeReasonNoPatch = "no patch"

// NoticeReasonBinary is the Reason of a changed file whose content is not
// text. Nothing in it is reviewable by reading, so the review declares it
// instead of counting it as partially covered.
const NoticeReasonBinary = "binary"

// ClassifyBlob is the one content check both review paths share: it returns a
// notice when the file must not be line-diffed or reviewed (binary or
// generated), or nil when its content is reviewable text, whatever its size.
func ClassifyBlob(path string, content []byte) *DiffNotice {
	msg := classifyBlob(path, content)
	if msg == "" {
		return nil
	}
	return &DiffNotice{Path: path, Message: msg, Reason: noticeReason(msg), DeclaredFormat: grammar.DeclaredBinaryFormat(path, content)}
}

// noticeReasonGenerated is the Reason of a file that says it was generated.
const noticeReasonGenerated = "generated"

// noticeReason maps a classifyBlob message to the stable reason token. Only
// a binary message is "binary"; anything else is "generated", which is never
// declared ignored, so an unexpected message fails closed (withholds).
func noticeReason(message string) string {
	if strings.HasPrefix(message, "binary file") {
		return NoticeReasonBinary
	}
	return noticeReasonGenerated
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

// searchDistanceLimit is the largest edit distance myersDiff can reach
// before it gives up: maxD itself, or the last distance whose trace still
// fits maxTraceCells (the trace through distance d holds d(d+1)/2 cells).
func searchDistanceLimit(maxD int) int {
	d := 0
	for d < maxD && (d+1)*(d+2)/2 <= maxTraceCells {
		d++
	}
	return d
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
// With the input cap gone (no file is skipped for size), two more bounds
// keep a huge file safe without changing any output inside the old cap:
//
//   - the furthest-reaching vector v is only as wide as the distances the
//     trace budget admits (searchDistanceLimit), not 2*(N+M)+1, so the
//     search's own memory is a constant independent of the file length;
//   - the search's work is capped at maxSearchSteps, past which it also
//     falls back to replaceAll, so a pathological file cannot stall it.
//
// Note that Myers 1986's section 4b describes a further refinement -- a
// divide-and-conquer middle-snake search with a strict O(N+M) bound. This
// function deliberately does not implement it: 4b picks a different
// (equally minimal) script than the forward-greedy backtrack below, and
// the bounds above already remove the unbounded growth without perturbing
// any existing output.
func myersDiff(a, b []string) []diffEdit {
	return myersSearch(a, b, maxSearchSteps)
}

// myersSearch is myersDiff with the work budget as a parameter, so a test can
// prove the fallback with a small budget instead of a slow input.
func myersSearch(a, b []string, stepBudget int) []diffEdit {
	n, m := len(a), len(b)
	maxD := n + m
	if maxD == 0 {
		return nil
	}

	limitD := searchDistanceLimit(maxD)
	// Diagonals k-1..k+1 with |k| <= limitD are read, hence the +1 margin.
	offset := limitD + 1
	v := make([]int, 2*offset+1)
	trace := make([]traceLevel, 0, 64)
	cells, steps := 0, 0

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

			steps++
			for x < n && y < m && a[x] == b[y] {
				x++
				y++
				steps++
			}
			if steps > stepBudget {
				return replaceAll(a, b)
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
