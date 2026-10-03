package applycheck

import "testing"

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
