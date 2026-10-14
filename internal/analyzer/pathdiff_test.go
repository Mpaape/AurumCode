package analyzer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestPathDiffsFromObjects covers each shape a line-less --pr file can have:
// changed text (small and 2 MiB) comes back with hunks, a pure rename, a
// mode change and an empty file are declared with no content change, and a
// binary stays binary.
func TestPathDiffsFromObjects(t *testing.T) {
	requireGitBinary(t)

	wide := strings.Repeat("w", bigFileBytes/4)
	dir := t.TempDir()
	runGit(t, dir, "init", "-q", "-b", "main")
	writeFileOrFail(t, filepath.Join(dir, "small.txt"), "one\ntwo\n")
	writeFileOrFail(t, filepath.Join(dir, "wide.txt"), strings.Repeat(wide+"\n", 4))
	writeFileOrFail(t, filepath.Join(dir, "old-name.txt"), "renamed as is\n")
	writeFileOrFail(t, filepath.Join(dir, "run.sh"), "echo hi\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-q", "-m", "base")

	writeFileOrFail(t, filepath.Join(dir, "small.txt"), "one\nTWO\n")
	writeFileOrFail(t, filepath.Join(dir, "wide.txt"), wide+"\n"+wide+"\nchanged\n"+wide+"\n"+wide+"\n")
	runGit(t, dir, "mv", "old-name.txt", "new-name.txt")
	if err := os.Chmod(filepath.Join(dir, "run.sh"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFileOrFail(t, filepath.Join(dir, "empty.txt"), "")
	if err := os.WriteFile(filepath.Join(dir, "logo.png"), append([]byte("\x89PNG\x00\x01"), make([]byte, 64)...), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-q", "-m", "head")

	repo, err := OpenRepo(dir)
	if err != nil {
		t.Fatalf("OpenRepo: %v", err)
	}
	paths := []string{"small.txt", "wide.txt", "new-name.txt", "run.sh", "empty.txt", "logo.png", "absent.txt"}
	files, notices, err := repo.PathDiffsFromObjects("HEAD~1", "HEAD", paths)
	if err != nil {
		t.Fatalf("PathDiffsFromObjects: %v", err)
	}
	got := map[string]string{}
	for _, f := range files {
		if len(f.Hunks) == 0 {
			t.Fatalf("%s came back without hunks", f.Path)
		}
		got[f.Path] = strings.Join(f.Hunks[0].Lines, "\n")
	}
	if !strings.Contains(got["small.txt"], "+TWO") || !strings.Contains(got["wide.txt"], "+changed") || len(got) != 2 {
		t.Fatalf("changed text must come back with hunks, got %v", got)
	}
	reasons := map[string]string{}
	for _, n := range notices {
		reasons[n.Path] = n.Reason
	}
	want := map[string]string{
		"new-name.txt": NoticeReasonNoContentChange,
		"run.sh":       NoticeReasonNoContentChange,
		"empty.txt":    NoticeReasonNoContentChange,
		"logo.png":     NoticeReasonBinary,
	}
	if len(reasons) != len(want) {
		t.Fatalf("notices = %v, want %v", reasons, want)
	}
	for p, r := range want {
		if reasons[p] != r {
			t.Errorf("%s: reason %q, want %q", p, reasons[p], r)
		}
	}
}
