package analyzer

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// These tests cover the input cap: which changed files are refused before
// they are ever split into lines, what the user is told about them, and the
// fact that refusing them costs nothing. See docs/specs/AUR-430.md,
// "Limits on what gets diffed".

func TestClassifyBlob_AcceptsOrdinaryFile(t *testing.T) {
	content := []byte(strings.Repeat("a line of source\n", 100))
	if got := classifyBlob("src/main.go", content); got != "" {
		t.Errorf("expected an ordinary file to be diffable, got %q", got)
	}
	if got := classifyBlob("empty.txt", nil); got != "" {
		t.Errorf("expected an empty/absent side to be diffable, got %q", got)
	}
}

func TestClassifyBlob_AcceptsExactlyAtTheLimits(t *testing.T) {
	// The limits are maxima, not exclusive bounds: a file of exactly
	// maxDiffLines lines must still be reviewed.
	atLineLimit := []byte(strings.Repeat("x\n", maxDiffLines))
	if got := classifyBlob("at-limit.txt", atLineLimit); got != "" {
		t.Errorf("a file of exactly %d lines must be diffable, got %q", maxDiffLines, got)
	}
	if n := countLines(atLineLimit); n != maxDiffLines {
		t.Errorf("countLines = %d, want %d", n, maxDiffLines)
	}

	atByteLimit := make([]byte, maxDiffBytes)
	for i := range atByteLimit {
		atByteLimit[i] = 'x'
	}
	if got := classifyBlob("at-bytes.txt", atByteLimit); got != "" {
		t.Errorf("a file of exactly %d bytes must be diffable, got %q", maxDiffBytes, got)
	}
}

func TestClassifyBlob_RejectsTooManyLines(t *testing.T) {
	content := []byte(strings.Repeat("x\n", maxDiffLines+1))
	got := classifyBlob("generated/huge.txt", content)
	want := fmt.Sprintf("diff too large: generated/huge.txt (%d lines > limit %d)", maxDiffLines+1, maxDiffLines)
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestClassifyBlob_RejectsTooManyBytes(t *testing.T) {
	// One very long line: it passes the line cap trivially, so only the
	// byte cap can catch it.
	content := append([]byte(strings.Repeat("x", maxDiffBytes+1)), '\n')
	got := classifyBlob("dist/bundle.min.js", content)
	if !strings.HasPrefix(got, "diff too large: dist/bundle.min.js (") || !strings.Contains(got, "bytes > limit") {
		t.Errorf("expected a byte-limit rejection, got %q", got)
	}
}

func TestClassifyBlob_RejectsBinary(t *testing.T) {
	content := append([]byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR"), make([]byte, 64)...)
	got := classifyBlob("assets/logo.png", content)
	if got != "binary file, skipped: assets/logo.png" {
		t.Errorf("got %q, want the binary-skip message", got)
	}
}

// TestClassifyBlob_BinaryBeatsSize pins the ordering: a blob that is both
// binary and oversized is reported as binary, which is the more useful
// diagnosis and, unlike the size check, is decided from a fixed-size
// window at the front of the blob.
func TestClassifyBlob_BinaryBeatsSize(t *testing.T) {
	content := make([]byte, maxDiffBytes+1) // all NUL
	if got := classifyBlob("big.bin", content); got != "binary file, skipped: big.bin" {
		t.Errorf("got %q, want the binary-skip message", got)
	}
}

// TestClassifyBlob_NULAfterSniffWindow documents the deliberate limit of
// the heuristic: this is git's rule (a NUL inside the first 8000 bytes),
// not a full scan, so a NUL past that window does not make the file
// binary. The byte and line caps still bound what such a file can cost.
func TestClassifyBlob_NULAfterSniffWindow(t *testing.T) {
	content := append([]byte(strings.Repeat("x", binarySniffLen)), 0x00)
	if got := classifyBlob("late-nul.txt", content); got != "" {
		t.Errorf("a NUL past the %d-byte sniff window must not count as binary, got %q", binarySniffLen, got)
	}
}

// TestClassifyBlob_OversizedIsCheap is the "no OOM" half of the cap proof:
// rejecting a file far past the limit must cost essentially nothing,
// because the decision happens on the raw bytes before any per-line copy
// exists. Running this under the suite's ulimit is the real assertion --
// the pre-fix code would have tried to diff this file and died.
func TestClassifyBlob_OversizedIsCheap(t *testing.T) {
	// 200k lines but only ~400 KB, so the *line* cap is what rejects it:
	// the byte cap would otherwise fire first and never exercise
	// countLines on a large input.
	content := []byte(strings.Repeat("x\n", 200000))

	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	got := classifyBlob("generated/enormous.txt", content)
	runtime.ReadMemStats(&after)

	if !strings.HasPrefix(got, "diff too large: generated/enormous.txt (200000 lines > limit ") {
		t.Fatalf("expected a line-limit rejection, got %q", got)
	}
	if used := after.TotalAlloc - before.TotalAlloc; used > 1<<20 {
		t.Errorf("rejecting an oversized file allocated %d bytes; it must not materialize the content", used)
	}
}

// TestDiff_SkipsOversizedAndBinaryFiles is the end-to-end half: a real
// repository whose second commit adds one reviewable file, one oversized
// file and one binary file must return only the reviewable one in the diff
// -- so no model call is ever made for the other two -- and must report
// both skips as notices, with a normal (non-error) return.
func TestDiff_SkipsOversizedAndBinaryFiles(t *testing.T) {
	requireGitBinary(t)

	dir := t.TempDir()
	runGit(t, dir, "init", "-q", "-b", "main")
	writeFileOrFail(t, filepath.Join(dir, "keep.txt"), "one\n")
	runGit(t, dir, "add", "keep.txt")
	runGit(t, dir, "commit", "-q", "-m", "one")

	writeFileOrFail(t, filepath.Join(dir, "keep.txt"), "two\n")
	writeFileOrFail(t, filepath.Join(dir, "huge.txt"), strings.Repeat("x\n", maxDiffLines+500))
	if err := os.WriteFile(filepath.Join(dir, "logo.png"), append([]byte("\x89PNG\x00\x01\x02"), make([]byte, 128)...), 0o644); err != nil {
		t.Fatalf("writing binary fixture: %v", err)
	}
	runGit(t, dir, "add", "keep.txt", "huge.txt", "logo.png")
	runGit(t, dir, "commit", "-q", "-m", "two")

	repo, err := OpenRepo(dir)
	if err != nil {
		t.Fatalf("OpenRepo: %v", err)
	}
	diff, notices, err := repo.Diff("HEAD~1", "HEAD")
	if err != nil {
		t.Fatalf("Diff must not fail on skipped files: %v", err)
	}

	if len(diff.Files) != 1 || diff.Files[0].Path != "keep.txt" {
		var paths []string
		for _, f := range diff.Files {
			paths = append(paths, f.Path)
		}
		t.Fatalf("only keep.txt may reach the model, got %v", paths)
	}

	if len(notices) != 2 {
		t.Fatalf("expected 2 notices, got %d: %+v", len(notices), notices)
	}
	// Notices keep the diff's sorted path order.
	if notices[0].Path != "huge.txt" || notices[1].Path != "logo.png" {
		t.Fatalf("notices are not in sorted path order: %+v", notices)
	}
	wantHuge := fmt.Sprintf("diff too large: huge.txt (%d lines > limit %d)", maxDiffLines+500, maxDiffLines)
	if notices[0].Message != wantHuge {
		t.Errorf("got %q, want %q", notices[0].Message, wantHuge)
	}
	if notices[1].Message != "binary file, skipped: logo.png" {
		t.Errorf("got %q, want the binary-skip message", notices[1].Message)
	}
}

// TestDiff_AllFilesSkipped covers the degenerate case: when every changed
// file is skipped the diff is empty rather than an error, so the command
// still exits normally after printing the notices.
func TestDiff_AllFilesSkipped(t *testing.T) {
	requireGitBinary(t)

	dir := t.TempDir()
	runGit(t, dir, "init", "-q", "-b", "main")
	writeFileOrFail(t, filepath.Join(dir, "seed.txt"), "seed\n")
	runGit(t, dir, "add", "seed.txt")
	runGit(t, dir, "commit", "-q", "-m", "one")

	writeFileOrFail(t, filepath.Join(dir, "huge.txt"), strings.Repeat("y\n", maxDiffLines+1))
	runGit(t, dir, "add", "huge.txt")
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
