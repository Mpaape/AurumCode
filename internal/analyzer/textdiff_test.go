package analyzer

import (
	"fmt"
	"strings"
	"testing"
)

func TestDiffLines_Identical(t *testing.T) {
	lines := []string{"a", "b", "c"}
	got := diffLines(lines, append([]string{}, lines...))

	want := []string{" a", " b", " c"}
	if !equalStrings(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestDiffLines_PureInsert(t *testing.T) {
	got := diffLines(nil, []string{"one", "two"})
	want := []string{"+one", "+two"}
	if !equalStrings(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestDiffLines_PureDelete(t *testing.T) {
	got := diffLines([]string{"one", "two"}, nil)
	want := []string{"-one", "-two"}
	if !equalStrings(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestDiffLines_BothEmpty(t *testing.T) {
	got := diffLines(nil, nil)
	if len(got) != 0 {
		t.Fatalf("expected no lines for two empty inputs, got %v", got)
	}
}

func TestDiffLines_SingleLineChange(t *testing.T) {
	old := []string{"func Greet() {", "\told code", "}"}
	new := []string{"func Greet() {", "\tnew code", "}"}

	got := diffLines(old, new)
	want := []string{" func Greet() {", "-\told code", "+\tnew code", " }"}
	if !equalStrings(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestDiffLines_CommonPrefixAndSuffix(t *testing.T) {
	old := []string{"header", "middle-old-1", "middle-old-2", "footer"}
	new := []string{"header", "middle-new", "footer"}

	got := diffLines(old, new)

	// The common prefix and suffix must survive as context, and every
	// original/replacement line must appear with the right prefix.
	joined := strings.Join(got, "\n")
	for _, want := range []string{" header", "-middle-old-1", "-middle-old-2", "+middle-new", " footer"} {
		if !strings.Contains(joined, want) {
			t.Errorf("expected diff to contain %q, got:\n%s", want, joined)
		}
	}
	if got[0] != " header" {
		t.Errorf("expected first line to be unchanged context, got %q", got[0])
	}
	if got[len(got)-1] != " footer" {
		t.Errorf("expected last line to be unchanged context, got %q", got[len(got)-1])
	}
}

func TestDiffLines_Reconstructs(t *testing.T) {
	// Property check: replaying the +/- lines against the old side must
	// reconstruct exactly the new side, for a handful of nontrivial cases.
	cases := [][2][]string{
		{{"a", "b", "c", "d", "e"}, {"a", "x", "c", "y", "e"}},
		{{"only-old"}, {"only-new"}},
		{{}, {"a", "b"}},
		{{"a", "b"}, {}},
		{{"a", "b", "c"}, {"c", "b", "a"}},
	}

	for i, c := range cases {
		edits := myersDiff(c[0], c[1])
		var rebuilt []string
		for _, e := range edits {
			if e.kind == diffEqual || e.kind == diffInsert {
				rebuilt = append(rebuilt, e.text)
			}
		}
		if !equalStrings(rebuilt, c[1]) {
			t.Errorf("case %d: reconstructed %v, want %v", i, rebuilt, c[1])
		}

		var oldSide []string
		for _, e := range edits {
			if e.kind == diffEqual || e.kind == diffDelete {
				oldSide = append(oldSide, e.text)
			}
		}
		if !equalStrings(oldSide, c[0]) {
			t.Errorf("case %d: old side reconstructed %v, want %v", i, oldSide, c[0])
		}
	}
}

func TestBuildDiffFile_Added(t *testing.T) {
	df := BuildDiffFile("new.txt", "", "line1\nline2\n")

	if df.Path != "new.txt" {
		t.Fatalf("unexpected path %q", df.Path)
	}
	if len(df.Hunks) != 1 {
		t.Fatalf("expected 1 hunk, got %d", len(df.Hunks))
	}
	h := df.Hunks[0]
	if h.OldLines != 0 || h.NewLines != 2 {
		t.Fatalf("unexpected hunk sizes: old=%d new=%d", h.OldLines, h.NewLines)
	}
	want := []string{"+line1", "+line2"}
	if !equalStrings(h.Lines, want) {
		t.Fatalf("got %v, want %v", h.Lines, want)
	}
}

func TestBuildDiffFile_Deleted(t *testing.T) {
	df := BuildDiffFile("gone.txt", "bye\n", "")

	h := df.Hunks[0]
	if h.OldLines != 1 || h.NewLines != 0 {
		t.Fatalf("unexpected hunk sizes: old=%d new=%d", h.OldLines, h.NewLines)
	}
	want := []string{"-bye"}
	if !equalStrings(h.Lines, want) {
		t.Fatalf("got %v, want %v", h.Lines, want)
	}
}

// myersDiffFullWidth is the pre-fix implementation, kept here and nowhere
// else as the reference the bounded implementation is checked against. It
// snapshots the entire 2*(N+M)+1-wide vector at every edit distance, which
// is precisely the memory blowup textdiff_memory_test.go guards against --
// so it must never be called outside a test, and at these input sizes its
// cost is trivial.
func myersDiffFullWidth(a, b []string) []diffEdit {
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

func equalEdits(a, b []diffEdit) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].kind != b[i].kind || a[i].text != b[i].text {
			return false
		}
	}
	return true
}

// wordsOverAB enumerates every string over {"a","b"} of length 0..maxLen,
// each as a slice of one-character lines.
func wordsOverAB(maxLen int) [][]string {
	out := [][]string{nil}
	frontier := [][]string{nil}
	for l := 1; l <= maxLen; l++ {
		var next [][]string
		for _, w := range frontier {
			for _, c := range []string{"a", "b"} {
				grown := make([]string, len(w)+1)
				copy(grown, w)
				grown[len(w)] = c
				next = append(next, grown)
			}
		}
		out = append(out, next...)
		frontier = next
	}
	return out
}

// TestMyersDiff_MatchesFullWidthReference is the proof that bounding the
// trace did not change a single byte of output. Restricting each recorded
// level to its reachable band is supposed to be an exact representation
// change, not a new algorithm, so for every input small enough to stay
// under maxTraceCells the bounded implementation must return the identical
// edit script -- same edits, same order, same tie-breaking.
//
// The check is near-exhaustive rather than random: every pair of strings
// over a two-letter alphabet up to length 6 (127^2 = 16129 pairs) covers
// every shape of insertion, deletion, replacement, transposition and
// repeated-line ambiguity that tie-breaking could resolve differently.
func TestMyersDiff_MatchesFullWidthReference(t *testing.T) {
	words := wordsOverAB(6)
	pairs := 0
	for _, a := range words {
		for _, b := range words {
			got := myersDiff(a, b)
			want := myersDiffFullWidth(a, b)
			if !equalEdits(got, want) {
				t.Fatalf("edit script diverged from the reference for a=%v b=%v:\n got %v\nwant %v", a, b, got, want)
			}
			pairs++
		}
	}
	t.Logf("bounded and full-width implementations agree on all %d pairs", pairs)
}

// TestMyersDiff_MatchesReferenceOnLargerInputs repeats the comparison on
// inputs long enough to exercise many trace levels while still staying well
// under maxTraceCells, where the minimal script -- not the fallback -- must
// still be returned.
func TestMyersDiff_MatchesReferenceOnLargerInputs(t *testing.T) {
	// A deterministic pseudo-random generator: the test must not vary run
	// to run, since AC-001 requires identical output for identical input.
	seed := uint64(0x9E3779B97F4A7C15)
	next := func() uint64 {
		seed ^= seed << 13
		seed ^= seed >> 7
		seed ^= seed << 17
		return seed
	}
	gen := func(n int) []string {
		out := make([]string, n)
		for i := range out {
			out[i] = fmt.Sprintf("line-%d", next()%17)
		}
		return out
	}

	for _, size := range []int{20, 50, 120, 300} {
		for trial := 0; trial < 5; trial++ {
			a, b := gen(size), gen(size)
			if !equalEdits(myersDiff(a, b), myersDiffFullWidth(a, b)) {
				t.Fatalf("edit script diverged from the reference at size %d, trial %d", size, trial)
			}
		}
	}
}

// TestMyersDiff_FallbackStillReconstructs covers the other side of the
// trace budget: once the minimal-script search is abandoned the result is
// coarser, but it must still be a *correct* edit script -- replaying the
// kept/deleted lines must rebuild the old side and replaying the
// kept/inserted lines must rebuild the new side. An input with a large edit
// distance on both sides is what forces the fallback.
func TestMyersDiff_FallbackStillReconstructs(t *testing.T) {
	const n = 6000 // edit distance ~12000, far past maxTraceCells
	a := make([]string, n)
	b := make([]string, n)
	for i := 0; i < n; i++ {
		a[i] = fmt.Sprintf("old line %d", i)
		b[i] = fmt.Sprintf("new line %d", i)
	}

	edits := myersDiff(a, b)

	var oldSide, newSide []string
	for _, e := range edits {
		if e.kind == diffEqual || e.kind == diffDelete {
			oldSide = append(oldSide, e.text)
		}
		if e.kind == diffEqual || e.kind == diffInsert {
			newSide = append(newSide, e.text)
		}
	}
	if !equalStrings(oldSide, a) {
		t.Error("the fallback script does not reconstruct the old side")
	}
	if !equalStrings(newSide, b) {
		t.Error("the fallback script does not reconstruct the new side")
	}

	// Confirm this input really did take the fallback, so the test cannot
	// quietly stop covering it if the budget changes.
	if len(edits) != 2*n {
		t.Errorf("expected the whole-file-replacement fallback (%d edits), got %d", 2*n, len(edits))
	}
}

func equalStrings(a, b []string) bool {
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
