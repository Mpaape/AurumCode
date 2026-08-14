// AUR-425 integration selector: the zero-page verdict integrated with the
// configuration knobs main derives from the environment (AURUMCODE_LANGUAGES)
// and with the extraction pipeline that decides what a run documents.
//
// Two scenarios bound the behaviour from both sides:
//
//  1. A project WITH a supported Go file, run under a language filter that
//     excludes it, produces zero pages: the run must exit 1 and the reason
//     must name the filter, because the "why" the card demands is different
//     here from the plain no-supported-files case.
//  2. The same project WITHOUT the filter documents the Go file and exits 0:
//     the fix is specific to the zero-page outcome and must not degrade a
//     working run into a failure.
//
// Like the unit selector, this drives the real binary: the promised contract
// is a raw process exit status, and the card's MUT-001 is only falsifiable at
// that boundary. This is a plain (non-_test) source in package integration;
// the acceptance stages it next to a generated bridge _test file.
package integration

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

// IntegrationAUR425 is AC-001 across the config/pipeline boundary.
func IntegrationAUR425(t *testing.T) {
	binary := buildRegenerateDocs(t)

	src := t.TempDir()
	writeFile(t, filepath.Join(src, "go.mod"), "module example.com/aur425fixture\n\ngo 1.21\n")
	writeFile(t, filepath.Join(src, "doc.go"),
		"// Package aur425fixture proves a supported language is present.\npackage aur425fixture\n\n"+
			"// Answer returns the documented answer.\nfunc Answer() int { return 42 }\n")

	t.Run("language filter that excludes every file fails with the filter named", func(t *testing.T) {
		out := filepath.Join(t.TempDir(), "docs-out")
		result := runBinary(t, binary, src, out, []string{"AURUMCODE_LANGUAGES=python"})

		if result.exitCode == 0 {
			t.Fatalf("AUR-425/AC-001/behavior-missing\nAUR-425/AC-001/MUT-001\n"+
				"zero pages under an all-excluding language filter still exited 0\n%s", result.output)
		}
		if result.exitCode != 1 {
			t.Fatalf("AUR-425/AC-001/wrong-exit: got %d, want 1\n%s", result.exitCode, result.output)
		}
		for _, want := range []string{
			"no documentation page was produced",
			"AURUMCODE_LANGUAGES",
			"aurumcode: result=empty",
		} {
			if !strings.Contains(result.output, want) {
				t.Errorf("AUR-425/AC-001/reason-missing: output lacks %q\n%s", want, result.output)
			}
		}
	})

	t.Run("the same project without the filter still documents and exits 0", func(t *testing.T) {
		out := filepath.Join(t.TempDir(), "docs-out")
		result := runBinary(t, binary, src, out, nil)

		if result.exitCode != 0 {
			t.Fatalf("AUR-425/AC-001/overshoot: a run that produced pages exited %d, want 0\n%s",
				result.exitCode, result.output)
		}
		if !strings.Contains(result.output, "aurumcode: result=ok") {
			t.Errorf("AUR-425/AC-001/overshoot: successful run lost its result=ok verdict\n%s", result.output)
		}
		if page := findGeneratedPage(t, out); page == "" {
			t.Errorf("AUR-425/AC-001/overshoot: no generated markdown found under %s\n%s", out, result.output)
		}
	})
}

// findGeneratedPage returns one markdown page under out that is not the site
// landing page, or "" when none exists.
func findGeneratedPage(t *testing.T, out string) string {
	t.Helper()

	var found string
	err := filepath.Walk(out, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if filepath.Ext(path) == ".md" && filepath.Base(path) != "index.md" {
			found = path
		}
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("AUR-425/AC-001/infrastructure/walk: %v", err)
	}
	return found
}

// ---------------------------------------------------------------------------
// helpers (deliberately self-contained: package integration and package unit
// are staged and compiled independently by the acceptance)
// ---------------------------------------------------------------------------

type runResult struct {
	exitCode int
	output   string
}

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

// runBinary executes the generator with an explicit, minimal environment; an
// inherited LLM_API_KEY or OPENAI_API_KEY could make main exit 1 for a budget
// reason unrelated to this card, masking the behaviour under test.
func runBinary(t *testing.T, binary, src, out string, extraEnv []string) runResult {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 240*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binary)
	cmd.Dir = t.TempDir()
	cmd.Env = append([]string{
		"PATH=" + t.TempDir(), // the Go extractor documents in-process (AUR-424); no tool needed
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
