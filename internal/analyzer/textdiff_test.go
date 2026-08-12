package analyzer

import (
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
