package analyzer

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// These tests cover what the content check refuses and what it no longer
// refuses: a binary or generated file is skipped, but a text file is never
// skipped for its size (the old 1 MiB / 10000-line input cap is gone). See
// docs/specs/AUR-430.md, "Limits on what gets diffed".

// The sizes the old cap refused: 2 MiB of bytes and 20000 lines.
const (
	bigFileBytes = 2 << 20
	bigFileLines = 20000
)

func TestClassifyBlob_AcceptsOrdinaryFile(t *testing.T) {
	content := []byte(strings.Repeat("a line of source\n", 100))
	if got := classifyBlob("src/main.go", content); got != "" {
		t.Errorf("expected an ordinary file to be diffable, got %q", got)
	}
	if got := classifyBlob("empty.txt", nil); got != "" {
		t.Errorf("expected an empty/absent side to be diffable, got %q", got)
	}
}

// TestClassifyBlob_AcceptsPastTheOldLimits inverts the old "at the limits"
// test: one byte or one line past the former cap is still reviewable text.
func TestClassifyBlob_AcceptsPastTheOldLimits(t *testing.T) {
	pastLines := []byte(strings.Repeat("x\n", 10001))
	if got := classifyBlob("past-lines.txt", pastLines); got != "" {
		t.Errorf("a 10001-line file must be diffable, got %q", got)
	}
	pastBytes := []byte(strings.Repeat("x", (1<<20)+1))
	if got := classifyBlob("past-bytes.txt", pastBytes); got != "" {
		t.Errorf("a file one byte past 1 MiB must be diffable, got %q", got)
	}
}

// TestClassifyBlob_AcceptsManyLines inverts the old line-cap rejection.
func TestClassifyBlob_AcceptsManyLines(t *testing.T) {
	content := []byte(strings.Repeat("x\n", bigFileLines))
	if got := classifyBlob("generated/huge.txt", content); got != "" {
		t.Errorf("a %d-line text file must be diffable, got %q", bigFileLines, got)
	}
	if n := ClassifyBlob("generated/huge.txt", content); n != nil {
		t.Errorf("a %d-line text file must produce no notice, got %+v", bigFileLines, *n)
	}
}

// TestClassifyBlob_AcceptsManyBytes inverts the old byte-cap rejection: one
// very long line, the minified-bundle shape, is reviewable text too.
func TestClassifyBlob_AcceptsManyBytes(t *testing.T) {
	content := append([]byte(strings.Repeat("x", bigFileBytes)), '\n')
	if got := classifyBlob("dist/bundle.min.js", content); got != "" {
		t.Errorf("a %d-byte text file must be diffable, got %q", bigFileBytes, got)
	}
}

func TestClassifyBlob_RejectsBinary(t *testing.T) {
	content := append([]byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR"), make([]byte, 64)...)
	got := classifyBlob("assets/logo.png", content)
	if got != "binary file, skipped: assets/logo.png" {
		t.Errorf("got %q, want the binary-skip message", got)
	}
}

// TestClassifyBlob_BinaryAtAnySize pins that a large binary blob is still
// reported as binary, decided from a fixed-size window at its front.
func TestClassifyBlob_BinaryAtAnySize(t *testing.T) {
	content := make([]byte, bigFileBytes) // all NUL
	if got := classifyBlob("big.bin", content); got != "binary file, skipped: big.bin" {
		t.Errorf("got %q, want the binary-skip message", got)
	}
}

// TestClassifyBlob_NULAfterSniffWindow documents the deliberate limit of
// the heuristic: this is git's rule (a NUL inside the first 8000 bytes),
// not a full scan, so a NUL past that window does not make the file
// binary. Such a file is diffed as text, under the search budgets.
func TestClassifyBlob_NULAfterSniffWindow(t *testing.T) {
	content := append([]byte(strings.Repeat("x", binarySniffLen)), 0x00)
	if got := classifyBlob("late-nul.txt", content); got != "" {
		t.Errorf("a NUL past the %d-byte sniff window must not count as binary, got %q", binarySniffLen, got)
	}
}

// TestClassifyBlob_LargeFileIsCheap keeps the "no OOM" half of the old cap
// proof: classifying a large file costs essentially nothing, because the
// decision happens on the raw bytes before any per-line copy exists.
func TestClassifyBlob_LargeFileIsCheap(t *testing.T) {
	content := []byte(strings.Repeat("x\n", 200000))

	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	got := classifyBlob("generated/enormous.txt", content)
	runtime.ReadMemStats(&after)

	if got != "" {
		t.Fatalf("a 200000-line text file must be diffable, got %q", got)
	}
	if used := after.TotalAlloc - before.TotalAlloc; used > 1<<20 {
		t.Errorf("classifying a large file allocated %d bytes; it must not materialize the content", used)
	}
}

// bigFixture returns n lines of distinct text, so a change has a long common
// prefix and suffix around it.
func bigFixture(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("line %d of a large file", i)
	}
	return out
}

// TestDiff_ReviewsLargeFilesSkipsBinary inverts the old "skips oversized"
// end-to-end test: in a real repository, a 20000-line file and a 2 MiB file,
// each with a small change, reach the diff as files with hunks (never as a
// notice); only the binary file is skipped.
func TestDiff_ReviewsLargeFilesSkipsBinary(t *testing.T) {
	requireGitBinary(t)

	lines := bigFixture(bigFileLines)
	wide := strings.Repeat("w", bigFileBytes/4) // four 512 KiB lines: 2 MiB
	dir := t.TempDir()
	runGit(t, dir, "init", "-q", "-b", "main")
	writeFileOrFail(t, filepath.Join(dir, "keep.txt"), "one\n")
	writeFileOrFail(t, filepath.Join(dir, "huge.txt"), strings.Join(lines, "\n")+"\n")
	writeFileOrFail(t, filepath.Join(dir, "wide.txt"), strings.Repeat(wide+"\n", 4))
	runGit(t, dir, "add", "keep.txt", "huge.txt", "wide.txt")
	runGit(t, dir, "commit", "-q", "-m", "one")

	lines[12345] = "the one line that changed"
	writeFileOrFail(t, filepath.Join(dir, "keep.txt"), "two\n")
	writeFileOrFail(t, filepath.Join(dir, "huge.txt"), strings.Join(lines, "\n")+"\n")
	writeFileOrFail(t, filepath.Join(dir, "wide.txt"), wide+"\n"+wide+"\nchanged\n"+wide+"\n"+wide+"\n")
	if err := os.WriteFile(filepath.Join(dir, "logo.png"), append([]byte("\x89PNG\x00\x01\x02"), make([]byte, 128)...), 0o644); err != nil {
		t.Fatalf("writing binary fixture: %v", err)
	}
	runGit(t, dir, "add", "keep.txt", "huge.txt", "wide.txt", "logo.png")
	runGit(t, dir, "commit", "-q", "-m", "two")

	repo, err := OpenRepo(dir)
	if err != nil {
		t.Fatalf("OpenRepo: %v", err)
	}
	diff, notices, err := repo.Diff("HEAD~1", "HEAD")
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}

	byPath := map[string]int{}
	for i, f := range diff.Files {
		byPath[f.Path] = i
	}
	for _, p := range []string{"huge.txt", "keep.txt", "wide.txt"} {
		i, ok := byPath[p]
		if !ok || len(diff.Files[i].Hunks) == 0 || len(diff.Files[i].Hunks[0].Lines) == 0 {
			t.Fatalf("%s must reach the review with hunks, got files %+v", p, byPath)
		}
	}
	huge := strings.Join(diff.Files[byPath["huge.txt"]].Hunks[0].Lines, "\n")
	if !strings.Contains(huge, "+the one line that changed") || !strings.Contains(huge, "-line 12345 of a large file") {
		t.Error("huge.txt does not render its one-line change")
	}
	if !strings.Contains(strings.Join(diff.Files[byPath["wide.txt"]].Hunks[0].Lines, "\n"), "+changed") {
		t.Error("wide.txt does not render its inserted line")
	}
	if len(notices) != 1 || notices[0].Path != "logo.png" || notices[0].Message != "binary file, skipped: logo.png" {
		t.Fatalf("only logo.png may be skipped, got %+v", notices)
	}
}

// TestDiff_AllFilesSkipped covers the degenerate case: when every changed
// file is skipped the diff is empty rather than an error, so the command
// still exits normally after printing the notices. A large text file no
// longer qualifies; a binary one does.
func TestDiff_AllFilesSkipped(t *testing.T) {
	requireGitBinary(t)

	dir := t.TempDir()
	runGit(t, dir, "init", "-q", "-b", "main")
	writeFileOrFail(t, filepath.Join(dir, "seed.txt"), "seed\n")
	runGit(t, dir, "add", "seed.txt")
	runGit(t, dir, "commit", "-q", "-m", "one")

	if err := os.WriteFile(filepath.Join(dir, "blob.bin"), make([]byte, bigFileBytes), 0o644); err != nil {
		t.Fatalf("writing binary fixture: %v", err)
	}
	runGit(t, dir, "add", "blob.bin")
	runGit(t, dir, "commit", "-q", "-m", "two")

	repo, err := OpenRepo(dir)
	if err != nil {
		t.Fatalf("OpenRepo: %v", err)
	}
	diff, notices, err := repo.Diff("HEAD~1", "HEAD")
	if err != nil {
		t.Fatalf("Diff must not fail when everything is skipped: %v", err)
	}
	if len(diff.Files) != 0 {
		t.Errorf("expected an empty diff, got %+v", diff.Files)
	}
	if len(notices) != 1 {
		t.Fatalf("expected 1 notice, got %+v", notices)
	}
}
