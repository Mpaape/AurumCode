// Package integration holds AUR-447's Integration-layer proof: Rust and C#
// are real entries in the same internal/pipeline extractor registry
// `--languages` already filters over (AUR-426), not extractors wired only
// for the unfiltered default path. A mixed Rust+C# source tree documents
// both languages unfiltered, and `--languages rust` / `--languages csharp`
// behave as a real allowlist over that same registry -- proving
// cmd/aurumcode/docs.go's registration reaches the real engine, the same
// way cmd/regenerate-docs's registration does.
package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func aur447IntegrationRoot(t *testing.T) string {
	t.Helper()
	if r := os.Getenv("AURUMCODE_ROOT"); r != "" {
		return r
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolving repository root: %v", err)
	}
	return root
}

func aur447IntegrationBuild(t *testing.T, root string) string {
	t.Helper()
	binPath := filepath.Join(t.TempDir(), "aurumcode-aur447-integration")
	build := exec.Command("go", "build", "-o", binPath, "./cmd/aurumcode")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build ./cmd/aurumcode failed: %v\n%s", err, out)
	}
	return binPath
}

func aur447IntegrationRun(t *testing.T, bin, dir string, args ...string) (int, string, string) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	code := 0
	if err != nil {
		ee, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("running %v: %v", args, err)
		}
		code = ee.ExitCode()
	}
	return code, stdout.String(), stderr.String()
}

// aur447MixedSource writes a two-file source tree: one Rust file and one C#
// file, each with a real doc-commented public item the native, tool-free
// extractors (AUR-427) recognize without any external toolchain, so both
// always succeed offline. The combination is what makes the language-filter
// behavior observable for these two specific languages.
func aur447MixedSource(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	rustFile := `//! AUR-447's integration fixture.

/// Says hello.
pub fn hello() -> &'static str {
    "hello"
}
`
	csharpFile := `namespace Fixture
{
    /// <summary>
    /// A tiny greeter, present only to exercise the language filter.
    /// </summary>
    public class Hello
    {
        /// <summary>
        /// Says hello.
        /// </summary>
        public string Greet()
        {
            return "hello";
        }
    }
}
`
	if err := os.WriteFile(filepath.Join(dir, "lib.rs"), []byte(rustFile), 0o600); err != nil {
		t.Fatalf("writing lib.rs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Hello.cs"), []byte(csharpFile), 0o600); err != nil {
		t.Fatalf("writing Hello.cs: %v", err)
	}
	return dir
}

// IntegrationAUR447 proves, through the real binary, that Rust and C# are
// real entries in the same registry `--languages` filters: unfiltered
// documents both; `--languages rust` documents only Rust; `--languages
// csharp` documents only C#. Each assertion reads the real generated
// markdown, never only the process exit code, so a filter that silently did
// nothing (or an extractor wired only for the unfiltered path) would be
// caught here.
func IntegrationAUR447(t *testing.T) {
	root := aur447IntegrationRoot(t)
	bin := aur447IntegrationBuild(t, root)
	src := aur447MixedSource(t)

	rustPage := func(outDir string) string { return filepath.Join(outDir, "rust", "lib.md") }
	csPage := func(outDir string) string { return filepath.Join(outDir, "csharp", "Hello.md") }

	t.Run("unfiltered: both rust and csharp are documented", func(t *testing.T) {
		out := filepath.Join(t.TempDir(), "site")
		code, stdout, stderr := aur447IntegrationRun(t, bin, root, "docs", "--source", src, "--output", out)
		if code != 0 {
			t.Fatalf("expected exit 0, got %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
		}
		if _, err := os.Stat(rustPage(out)); err != nil {
			t.Fatalf("expected %s to exist: %v", rustPage(out), err)
		}
		if _, err := os.Stat(csPage(out)); err != nil {
			t.Fatalf("expected %s to exist: %v", csPage(out), err)
		}
	})

	t.Run("--languages rust: only rust is documented", func(t *testing.T) {
		out := filepath.Join(t.TempDir(), "site")
		code, stdout, stderr := aur447IntegrationRun(t, bin, root, "docs", "--source", src, "--output", out, "--languages", "rust")
		if code != 0 {
			t.Fatalf("expected exit 0, got %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
		}
		if _, err := os.Stat(rustPage(out)); err != nil {
			t.Fatalf("expected %s to exist: %v", rustPage(out), err)
		}
		if _, err := os.Stat(csPage(out)); err == nil {
			t.Fatalf("the csharp page must not exist when --languages excludes csharp")
		}
	})

	t.Run("--languages csharp: only csharp is documented", func(t *testing.T) {
		out := filepath.Join(t.TempDir(), "site")
		code, stdout, stderr := aur447IntegrationRun(t, bin, root, "docs", "--source", src, "--output", out, "--languages", "csharp")
		if code != 0 {
			t.Fatalf("expected exit 0, got %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
		}
		if _, err := os.Stat(csPage(out)); err != nil {
			t.Fatalf("expected %s to exist: %v", csPage(out), err)
		}
		if _, err := os.Stat(rustPage(out)); err == nil {
			t.Fatalf("the rust page must not exist when --languages excludes rust")
		}
	})
}
