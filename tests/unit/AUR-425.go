// AUR-425 unit selector: a run of cmd/regenerate-docs that produces zero
// documentation pages must state the reason and exit non-zero, instead of the
// historical silent exit 0.
//
// WHY THIS DRIVES THE REAL BINARY INSTEAD OF IMPORTING THE PACKAGE:
// the verdict under test lives in report() inside package main of
// cmd/regenerate-docs; a main package cannot be imported, and the promised
// behaviour is a raw process exit status, which only the operating system
// can prove. The card's mutation (MUT-001: going back to exit 0 with zero
// pages) is only observable at that boundary, so any in-process re-model of
// the switch would pass under the mutation and be a false green.
//
// This file is a plain (non-_test) source in package unit, per the board's
// selector convention; the acceptance stages it next to a generated bridge
// _test file that calls TestAUR425.
package unit

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// TestAUR425 is AC-001 at its smallest observable seam: one project with no
// supported source file, one run, one exit status, one stated reason.
func TestAUR425(t *testing.T) {
	binary := buildRegenerateDocs(t)

	src := t.TempDir()
	writeFile(t, filepath.Join(src, "notes.txt"), "plain text, no supported language\n")
	writeFile(t, filepath.Join(src, "manual.rst"), "reStructuredText is not a supported language\n")

	out := filepath.Join(t.TempDir(), "docs-out")
	result := runBinary(t, binary, src, out, nil)

	// The heart of the card. Exit 0 with zero pages is both the pre-fix
	// behaviour (RED: behavior-missing) and the card's skeptical mutation
	// (MUT-001), so both tokens are emitted on this exact failure.
	if result.exitCode == 0 {
		t.Fatalf("AUR-425/AC-001/behavior-missing\nAUR-425/AC-001/MUT-001\n"+
			"a run that produced zero documentation pages exited 0\n%s", result.output)
	}
	if result.exitCode != 1 {
		t.Fatalf("AUR-425/AC-001/wrong-exit: got %d, want 1 (documented in docs/specs/AUR-425.md)\n%s",
			result.exitCode, result.output)
	}

	// The reason: the command must say why there is nothing to show.
	for _, want := range []string{
		"no documentation page was produced",
		"no supported source file was documented under " + src,
		"aurumcode: result=empty",
	} {
		if !strings.Contains(result.output, want) {
			t.Errorf("AUR-425/AC-001/reason-missing: output lacks %q\n%s", want, result.output)
		}
	}

	// A failing run must not carry success markers a log-scraping consumer
	// could mistake for a green verdict.
	for _, banned := range []string{"result=ok", "✅"} {
		if strings.Contains(result.output, banned) {
			t.Errorf("AUR-425/AC-001/false-success-marker: output contains %q\n%s", banned, result.output)
		}
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

type runResult struct {
	exitCode int
	output   string
}

// repoRoot resolves the tree holding cmd/regenerate-docs. AURUMCODE_ROOT wins
// because the acceptance stages this file into a scratch module, where the
// caller-derived path no longer points at the repository.
func repoRoot(t *testing.T) string {
	t.Helper()

	if root := os.Getenv("AURUMCODE_ROOT"); root != "" {
		return root
	}
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("AUR-425/AC-001/infrastructure: cannot locate the test source file")
	}
	return filepath.Join(filepath.Dir(file), "..", "..")
}

// buildRegenerateDocs compiles the real binary from the repository root. A
// missing compile closure is infrastructure, not behaviour: the failure names
// it so the acceptance's own closure probe (exit 69) stays the authority.
func buildRegenerateDocs(t *testing.T) string {
	t.Helper()

	root := repoRoot(t)
	if _, err := os.Stat(filepath.Join(root, "cmd", "regenerate-docs", "main.go")); err != nil {
		t.Fatalf("AUR-425/AC-001/infrastructure/closure: cmd/regenerate-docs not materialized under %s: %v", root, err)
	}

	binary := filepath.Join(t.TempDir(), "regenerate-docs")
	ctx, cancel := context.WithTimeout(context.Background(), 240*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "go", "build", "-o", binary, "./cmd/regenerate-docs")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GOFLAGS=-mod=mod")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("AUR-425/AC-001/infrastructure/build: %v\n%s", err, output)
	}
	return binary
}

// runBinary executes the built generator with an explicit, minimal
// environment. os.Environ is deliberately not inherited by the binary: an
// ambient LLM_API_KEY or OPENAI_API_KEY on the host would route main through
// the budget resolver and could exit 1 for a reason that has nothing to do
// with this card, turning a broken run into a false green.
func runBinary(t *testing.T, binary, src, out string, extraEnv []string) runResult {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 240*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binary)
	cmd.Dir = t.TempDir()
	cmd.Env = append([]string{
		"PATH=" + t.TempDir(), // empty PATH: a zero-page verdict needs no external tool
		"HOME=" + t.TempDir(),
		"AURUMCODE_SOURCE_DIR=" + src,
		"AURUMCODE_OUTPUT_DIR=" + out,
	}, extraEnv...)

	output, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("AUR-425/AC-001/infrastructure/timeout: generator did not finish\n%s", output)
	}

	result := runResult{output: string(output)}
	if err != nil {
		exitErr, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("AUR-425/AC-001/infrastructure/exec: %v\n%s", err, output)
		}
		result.exitCode = exitErr.ExitCode()
	}
	return result
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("AUR-425/AC-001/infrastructure/fixture: %v", err)
	}
}
