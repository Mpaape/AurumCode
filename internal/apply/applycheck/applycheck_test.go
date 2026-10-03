package applycheck

import (
	"strings"
	"testing"
)

func TestRejectsZeroContextAwayFromEdges(t *testing.T) {
	files := map[string]string{"a": "1\n2\n3\n4\n5\n"}
	if _, err := Apply(files, "--- a/a\n+++ b/a\n@@ -3,1 +3,1 @@\n-3\n+X\n"); err == nil {
		t.Fatal("zero-context hunk in the middle must be rejected")
	}
	got, err := Apply(files, "--- a/a\n+++ b/a\n@@ -2,3 +2,3 @@\n 2\n-3\n+X\n 4\n")
	if err != nil || got["a"] != "1\n2\nX\n4\n5\n" {
		t.Fatalf("got %q, %v", got["a"], err)
	}
}

func TestRejectsStaleAndBadCounts(t *testing.T) {
	files := map[string]string{"a": "1\n2\n3\n"}
	if _, err := Apply(files, "--- a/a\n+++ b/a\n@@ -1,2 +1,2 @@\n 1\n-9\n+X\n"); err == nil {
		t.Fatal("stale removal must be rejected")
	}
	if _, err := Apply(files, "--- a/a\n+++ b/a\n@@ -1,3 +1,2 @@\n 1\n-2\n+X\n 3\n"); err == nil {
		t.Fatal("wrong header count must be rejected")
	}
}

func TestCreateAndDelete(t *testing.T) {
	got, err := Apply(map[string]string{"old": "x\n"}, "--- /dev/null\n+++ b/new\n@@ -0,0 +1,2 @@\n+a\n+b\n--- a/old\n+++ /dev/null\n@@ -1,1 +0,0 @@\n-x\n")
	if err != nil || got["new"] != "a\nb\n" {
		t.Fatalf("got %v, %v", got, err)
	}
	if _, ok := got["old"]; ok {
		t.Fatal("old must be removed")
	}
}

// Well-formed hunks (right counts, leading and trailing context) whose
// content differs from the file must be rejected by the comparison itself.
func TestRejectsContentMismatchInWellFormedHunk(t *testing.T) {
	files := map[string]string{"a": "1\n2\n3\n4\n5\n"}
	for name, patch := range map[string]string{
		"context differs":  "--- a/a\n+++ b/a\n@@ -2,3 +2,3 @@\n 9\n-3\n+X\n 4\n",
		"removal differs":  "--- a/a\n+++ b/a\n@@ -2,3 +2,3 @@\n 2\n-9\n+X\n 4\n",
		"trailing differs": "--- a/a\n+++ b/a\n@@ -2,3 +2,3 @@\n 2\n-3\n+X\n 9\n",
	} {
		_, err := Apply(files, patch)
		if err == nil || !strings.Contains(err.Error(), "file differs") {
			t.Errorf("%s: err = %v, want a content mismatch", name, err)
		}
	}
}

func TestToleratesNoNewlineMarker(t *testing.T) {
	got, err := Apply(map[string]string{"a": "1\n2\n3"}, "--- a/a\n+++ b/a\n@@ -1,3 +1,3 @@\n 1\n 2\n-3\n\\ No newline at end of file\n+X\n\\ No newline at end of file\n")
	if err != nil || !strings.HasPrefix(got["a"], "1\n2\nX") {
		t.Fatalf("got %q, %v", got["a"], err)
	}
}
