// Package integration holds AUR-426's Integration-layer proof: `--languages`
// is a real filter over the same internal/pipeline extractor registry
// cmd/regenerate-docs drives, not a flag that parses but does nothing, and a
// mixed-language source tree produces the honest partial-run outcome the
// pipeline itself reports (one page generated, one language skipped because
// its external tool is not installed in this offline environment) instead of
// silently reporting either a false-clean success or a false-total failure.
package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func aur426IntegrationRoot(t *testing.T) string {
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

func aur426IntegrationBuild(t *testing.T, root string) string {
	t.Helper()
	binPath := filepath.Join(t.TempDir(), "aurumcode-aur426-integration")
	build := exec.Command("go", "build", "-o", binPath, "./cmd/aurumcode")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build ./cmd/aurumcode failed: %v\n%s", err, out)
	}
	return binPath
}

func aur426IntegrationRun(t *testing.T, bin, dir string, args ...string) (int, string, string) {
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

// aur426MultiLangSource writes a two-file source tree: one Go file (the
// registered Go extractor is pure standard library -- AUR-424 -- so it
// always succeeds offline) and one Python file (the registered Python
// extractor shells out to pydoc-markdown, which this sandboxed environment
// does not have installed, so it always ends as a language SKIP, never a
// hang or a crash). The combination is what makes the partial-run and
// language-filter behavior observable without network or an external
// toolchain.
func aur426MultiLangSource(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	goFile := `// Package lib is AUR-426's integration fixture.
package lib

// Hello returns a greeting.
func Hello() string { return "hello" }
`
	pyFile := `"""A tiny python module, present only to exercise the language filter."""


def hello():
    """Return a greeting."""
    return "hello"
`
	if err := os.WriteFile(filepath.Join(dir, "lib.go"), []byte(goFile), 0o600); err != nil {
		t.Fatalf("writing lib.go: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "script.py"), []byte(pyFile), 0o600); err != nil {
		t.Fatalf("writing script.py: %v", err)
	}
	return dir
}

// IntegrationAUR426 proves, through the real binary, that `--languages`
// reaches the real internal/pipeline extractor registry: unfiltered and
// filtered to "go" both document the Go file (the filter is not a silent
// no-op that always processes everything); filtered to "python" alone
// documents nothing and fails loudly, naming the real reason
// (`internal/pipeline`'s own "required tool not in PATH" report -- not a
// message this card invented); and the unfiltered run over the mixed tree
// reports the honest partial outcome instead of collapsing it into either an
// all-clean success or a total failure.
func IntegrationAUR426(t *testing.T) {
	root := aur426IntegrationRoot(t)
	bin := aur426IntegrationBuild(t, root)
	src := aur426MultiLangSource(t)

	t.Run("unfiltered: go documents, python is skipped, partial success", func(t *testing.T) {
		out := filepath.Join(t.TempDir(), "site")
		code, stdout, stderr := aur426IntegrationRun(t, bin, root, "docs", "--source", src, "--output", out)
		if code != 0 {
			t.Fatalf("expected exit 0, got %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
		}
		page := filepath.Join(out, "go", "root.md")
		if _, err := os.Stat(page); err != nil {
			t.Fatalf("expected %s to exist: %v", page, err)
		}
		if !strings.Contains(stderr, "partial run") {
			t.Fatalf("expected a partial-run note on stderr, got:\n%s", stderr)
		}
		if !strings.Contains(stderr, "1 language(s) skipped") {
			t.Fatalf("expected the skip count on stderr, got:\n%s", stderr)
		}
	})

	t.Run("--languages go: same result as unfiltered, proving the filter is a real allowlist", func(t *testing.T) {
		out := filepath.Join(t.TempDir(), "site")
		code, stdout, stderr := aur426IntegrationRun(t, bin, root, "docs", "--source", src, "--output", out, "--languages", "go")
		if code != 0 {
			t.Fatalf("expected exit 0, got %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
		}
		page := filepath.Join(out, "go", "root.md")
		if _, err := os.Stat(page); err != nil {
			t.Fatalf("expected %s to exist: %v", page, err)
		}
	})

	t.Run("--languages python: nothing is documented, fails loudly naming the real reason", func(t *testing.T) {
		out := filepath.Join(t.TempDir(), "site")
		code, stdout, stderr := aur426IntegrationRun(t, bin, root, "docs", "--source", src, "--output", out, "--languages", "python")
		if code != 1 {
			t.Fatalf("expected exit 1, got %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
		}
		if stdout != "" {
			t.Fatalf("expected no success summary on stdout for a failed run, got:\n%s", stdout)
		}
		// This exact phrase comes from internal/pipeline's own
		// LanguageSkip/ExtractionError reporting (extractor_pipeline.go),
		// not from cmd/aurumcode -- proving the flag reached the real
		// engine instead of a locally reimplemented check.
		if !strings.Contains(stderr, "required tool not in PATH") {
			t.Fatalf("expected the pipeline's own skip reason on stderr, got:\n%s", stderr)
		}
		if !strings.Contains(stderr, "python") {
			t.Fatalf("expected the skipped language named on stderr, got:\n%s", stderr)
		}
		if _, err := os.Stat(filepath.Join(out, "go", "root.md")); err == nil {
			t.Fatalf("the go page must not exist when --languages excludes go")
		}
	})
}
