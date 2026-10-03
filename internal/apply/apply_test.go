package apply

import (
	"github.com/Mpaape/AurumCode/internal/apply/applycheck"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/Mpaape/AurumCode/pkg/types"
)

// numbered returns a file of n lines "l1".."ln", each ending in a newline.
func numbered(n int) string {
	var sb strings.Builder
	for i := 1; i <= n; i++ {
		sb.WriteString("l" + strconv.Itoa(i) + "\n")
	}
	return sb.String()
}

func tree(files map[string]string) fstest.MapFS {
	m := fstest.MapFS{}
	for k, v := range files {
		m[k] = &fstest.MapFile{Data: []byte(v)}
	}
	return m
}

func mustPatch(t *testing.T, s []types.ReviewSuggestion, files map[string]string) string {
	t.Helper()
	p, err := BuildPatch(s, tree(files))
	if err != nil {
		t.Fatalf("BuildPatch: %v", err)
	}
	return p
}

// AC-002: a one-line replacement in the middle carries exactly three lines of
// real context on each side and correct headers and counts.
func TestHunkHasThreeLinesOfContext(t *testing.T) {
	p := mustPatch(t, []types.ReviewSuggestion{{File: "a.go", Line: 10, CurrentCode: "l10", ProposedCode: "L10"}},
		map[string]string{"a.go": numbered(20)})
	want := "--- a/a.go\n+++ b/a.go\n@@ -7,7 +7,7 @@\n l7\n l8\n l9\n-l10\n+L10\n l11\n l12\n l13\n"
	if p != want {
		t.Fatalf("patch:\n%s\nwant:\n%s", p, want)
	}
}

func TestHunkContextIsClampedAtFileEdges(t *testing.T) {
	p := mustPatch(t, []types.ReviewSuggestion{{File: "a.go", Line: 1, CurrentCode: "l1", ProposedCode: "L1"}},
		map[string]string{"a.go": numbered(3)})
	want := "--- a/a.go\n+++ b/a.go\n@@ -1,3 +1,3 @@\n-l1\n+L1\n l2\n l3\n"
	if p != want {
		t.Fatalf("patch:\n%s\nwant:\n%s", p, want)
	}
}

func TestInsertionAndDeletionHunkCounts(t *testing.T) {
	files := map[string]string{"a.go": numbered(12)}
	p := mustPatch(t, []types.ReviewSuggestion{{File: "a.go", Line: 6, CurrentCode: "l6", ProposedCode: "l6\nextra1\nextra2"}}, files)
	want := "--- a/a.go\n+++ b/a.go\n@@ -4,6 +4,8 @@\n l4\n l5\n l6\n+extra1\n+extra2\n l7\n l8\n l9\n"
	if p != want {
		t.Fatalf("insertion:\n%s\nwant:\n%s", p, want)
	}
	p = mustPatch(t, []types.ReviewSuggestion{{File: "a.go", Line: 6, CurrentCode: "l6\nl7", ProposedCode: ""}}, files)
	want = "--- a/a.go\n+++ b/a.go\n@@ -3,8 +3,6 @@\n l3\n l4\n l5\n-l6\n-l7\n l8\n l9\n l10\n"
	if p != want {
		t.Fatalf("deletion:\n%s\nwant:\n%s", p, want)
	}
}

func TestNearbyChangesMergeIntoOneHunkAndFarOnesDoNot(t *testing.T) {
	files := map[string]string{"a.go": numbered(40)}
	near := mustPatch(t, []types.ReviewSuggestion{
		{File: "a.go", Line: 10, CurrentCode: "l10", ProposedCode: "A\nB"},
		{File: "a.go", Line: 14, CurrentCode: "l14", ProposedCode: "X"},
	}, files)
	if n := strings.Count(near, "@@ -"); n != 1 {
		t.Fatalf("near changes: %d hunks, want 1:\n%s", n, near)
	}
	far := mustPatch(t, []types.ReviewSuggestion{
		{File: "a.go", Line: 30, CurrentCode: "l30", ProposedCode: "Z"},
		{File: "a.go", Line: 10, CurrentCode: "l10", ProposedCode: "A\nB"},
	}, files)
	want := "--- a/a.go\n+++ b/a.go\n" +
		"@@ -7,7 +7,8 @@\n l7\n l8\n l9\n-l10\n+A\n+B\n l11\n l12\n l13\n" +
		"@@ -27,7 +28,7 @@\n l27\n l28\n l29\n-l30\n+Z\n l31\n l32\n l33\n"
	if far != want {
		t.Fatalf("far changes:\n%s\nwant:\n%s", far, want)
	}
}

func TestNewFileAndRemovalPatches(t *testing.T) {
	want := "--- /dev/null\n+++ b/new/x.go\n@@ -0,0 +1,3 @@\n+package x\n+\n+var A = 1\n"
	if p := CreateFilePatch("new/x.go", "package x\n\nvar A = 1\n"); p != want {
		t.Fatalf("new file:\n%s\nwant:\n%s", p, want)
	}
	want = "--- a/old.go\n+++ /dev/null\n@@ -1,2 +0,0 @@\n-a\n-b\n\\ No newline at end of file\n"
	if p := DeleteFilePatch("old.go", "a\nb"); p != want {
		t.Fatalf("removal:\n%s\nwant:\n%s", p, want)
	}
	if CreateFilePatch("../x", "a") != "" || DeleteFilePatch("/abs", "a") != "" || CreateFilePatch("x", "") != "" {
		t.Fatal("unsafe or empty input must yield an empty patch")
	}
}

func TestMissingNewlineAtEndOfFile(t *testing.T) {
	p := mustPatch(t, []types.ReviewSuggestion{{File: "a.txt", Line: 3, CurrentCode: "c", ProposedCode: "C"}},
		map[string]string{"a.txt": "a\nb\nc"})
	want := "--- a/a.txt\n+++ b/a.txt\n@@ -1,3 +1,3 @@\n a\n b\n-c\n\\ No newline at end of file\n+C\n\\ No newline at end of file\n"
	if p != want {
		t.Fatalf("patch:\n%s\nwant:\n%s", p, want)
	}
}

func TestFailsClosed(t *testing.T) {
	files := map[string]string{"a.go": numbered(5)}
	cases := map[string]types.ReviewSuggestion{
		"stale":    {File: "a.go", Line: 2, CurrentCode: "nope", ProposedCode: "x"},
		"past-eof": {File: "a.go", Line: 5, CurrentCode: "l5\nl6", ProposedCode: "x"},
		"missing":  {File: "gone.go", Line: 1, CurrentCode: "l1", ProposedCode: "x"},
	}
	for name, s := range cases {
		if _, err := BuildPatch([]types.ReviewSuggestion{s}, tree(files)); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	_, err := BuildPatch([]types.ReviewSuggestion{
		{File: "a.go", Line: 2, CurrentCode: "l2\nl3", ProposedCode: "x"},
		{File: "a.go", Line: 3, CurrentCode: "l3", ProposedCode: "y"},
	}, tree(files))
	if err == nil || !strings.Contains(err.Error(), "overlap") {
		t.Errorf("overlap: err = %v", err)
	}
	if _, err := BuildPatch(nil, nil); err == nil {
		t.Error("nil source must be an error")
	}
}

func TestSkipsUnsafeAndNoopSuggestions(t *testing.T) {
	files := map[string]string{"a.go": numbered(5), "ok.go": "x\n"}
	valid := types.ReviewSuggestion{File: "ok.go", Line: 1, CurrentCode: "x", ProposedCode: "y"}
	for name, s := range map[string]types.ReviewSuggestion{
		"noop":       {File: "a.go", Line: 1, CurrentCode: "l1\n", ProposedCode: "l1\n"},
		"abs":        {File: "/etc/passwd", Line: 1, CurrentCode: "l1", ProposedCode: "x"},
		"dotdot":     {File: "../a.go", Line: 1, CurrentCode: "l1", ProposedCode: "x"},
		"no-line":    {File: "a.go", CurrentCode: "l1", ProposedCode: "x"},
		"empty-path": {File: "", Line: 1, CurrentCode: "l1", ProposedCode: "x"},
	} {
		plan, err := BuildPlan([]types.ReviewSuggestion{s, valid}, tree(files))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(plan.Files) != 1 || plan.Files[0].Path != "ok.go" {
			t.Errorf("%s: plan = %#v, want only ok.go", name, plan.Files)
		}
	}
	if p := mustPatch(t, nil, files); p != "" {
		t.Fatalf("patch = %q, want empty", p)
	}
}

func TestPlanCarriesLineNumbers(t *testing.T) {
	plan, err := BuildPlan([]types.ReviewSuggestion{{File: "a.go", Line: 4, CurrentCode: "l4", ProposedCode: "N1\nN2"}}, tree(map[string]string{"a.go": numbered(8)}))
	if err != nil {
		t.Fatal(err)
	}
	fe := plan.Files[0]
	if len(fe.Removals) != 1 || fe.Removals[0] != (Line{4, "l4"}) {
		t.Fatalf("removals = %#v", fe.Removals)
	}
	if len(fe.Additions) != 2 || fe.Additions[0] != (Line{4, "N1"}) || fe.Additions[1] != (Line{5, "N2"}) {
		t.Fatalf("additions = %#v", fe.Additions)
	}
}

func TestBuildPatchDeterministic(t *testing.T) {
	s := []types.ReviewSuggestion{
		{File: "b.go", Line: 2, CurrentCode: "l2", ProposedCode: "x"},
		{File: "a.go", Line: 4, CurrentCode: "l4", ProposedCode: "y"},
	}
	files := map[string]string{"a.go": numbered(6), "b.go": numbered(6)}
	first := mustPatch(t, s, files)
	for i := 0; i < 5; i++ {
		if mustPatch(t, s, files) != first {
			t.Fatalf("not deterministic on run %d", i)
		}
	}
	if !strings.HasPrefix(first, "--- a/a.go\n") {
		t.Fatalf("expected files sorted by path:\n%s", first)
	}
}

// AC-001 without a git binary: the patch is applied with plain-git strictness
// (applycheck) and the result holds the correction. Zero-context hunks, the
// pre-AUR-566 output, are rejected by it.
func TestPatchAppliesWithPlainGitStrictness(t *testing.T) {
	files := map[string]string{"a.go": numbered(40), "b.go": numbered(9)}
	p := mustPatch(t, []types.ReviewSuggestion{
		{File: "a.go", Line: 10, CurrentCode: "l10", ProposedCode: "A\nB"},
		{File: "a.go", Line: 14, CurrentCode: "l14", ProposedCode: "X"},
		{File: "a.go", Line: 30, CurrentCode: "l30\nl31", ProposedCode: ""},
		{File: "b.go", Line: 1, CurrentCode: "l1", ProposedCode: "first"},
		{File: "b.go", Line: 9, CurrentCode: "l9", ProposedCode: "last"},
	}, files)
	got, err := applycheck.Apply(files, p)
	if err != nil {
		t.Fatalf("patch does not apply:\n%s\n%v", p, err)
	}
	if !strings.Contains(got["a.go"], "l9\nA\nB\nl11\nl12\nl13\nX\nl15\n") || strings.Contains(got["a.go"], "l30") || strings.Contains(got["a.go"], "l31") {
		t.Fatalf("a.go after apply:\n%s", got["a.go"])
	}
	if !strings.HasPrefix(got["b.go"], "first\nl2\n") || !strings.HasSuffix(got["b.go"], "l8\nlast\n") {
		t.Fatalf("b.go after apply:\n%s", got["b.go"])
	}
	got, err = applycheck.Apply(map[string]string{}, CreateFilePatch("n/x.go", "package x\n"))
	if err != nil || got["n/x.go"] != "package x\n" {
		t.Fatalf("new file: %v %v", got, err)
	}
	got, err = applycheck.Apply(map[string]string{"o.go": "a\nb\n"}, DeleteFilePatch("o.go", "a\nb\n"))
	if err != nil || len(got) != 0 {
		t.Fatalf("removal: %v %v", got, err)
	}
}
