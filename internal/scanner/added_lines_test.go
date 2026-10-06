package scanner

import (
	"errors"
	"testing"
)

func TestParseAddedLines(t *testing.T) {
	diff := "diff --git a/x.go b/x.go\n--- a/x.go\n+++ b/x.go\n@@ -3 +3,2 @@\n-a\n+b\n+c\n@@ -9,2 +10,0 @@\n-d\n-e\n@@ -20 +20 @@\n-f\n+g\ndiff --git a/gone.go b/gone.go\n--- a/gone.go\n+++ /dev/null\n@@ -1 +0,0 @@\n-h\n"
	set, err := parseAddedLines(diff)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range []int{3, 4, 20} {
		if !set["x.go"][n] {
			t.Errorf("line %d not added", n)
		}
	}
	if set["x.go"][5] || set["x.go"][10] || len(set) != 1 {
		t.Errorf("set = %v", set)
	}
}

func TestParseAddedLinesRefusesBadHeader(t *testing.T) {
	if _, err := parseAddedLines("+++ b/x.go\n@@ nonsense\n"); !errors.Is(err, ErrInvalidOutput) {
		t.Fatalf("err = %v", err)
	}
}

func TestParseAddedLinesRefusesQuotedPath(t *testing.T) {
	diff := "diff --git \"a/x\\ty.go\" \"b/x\\ty.go\"\n--- \"a/x\\ty.go\"\n+++ \"b/x\\ty.go\"\n@@ -0,0 +1 @@\n+a\n"
	if _, err := parseAddedLines(diff); !errors.Is(err, ErrInvalidOutput) {
		t.Fatalf("err = %v, want ErrInvalidOutput", err)
	}
}

func TestDiffIsRelativeToTheRoot(t *testing.T) {
	args := diffArgs(Range{Base: "a", Head: "b"})
	if args[2] != "diff" || args[3] != "--relative" {
		t.Fatalf("args = %v, want git diff --relative", args)
	}
}
