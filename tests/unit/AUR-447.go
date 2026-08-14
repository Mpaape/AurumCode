// Package unit holds AUR-447's Unit-layer proof: `aurumcode docs --source
// <projeto Rust ou C#>` generates real documentation pages through
// cmd/aurumcode/docs.go's registerDocsExtractors -- not the "no extractor
// registered", zero-page failure a reviewer reproduced before this card --
// while Go (already supported since AUR-426) keeps working.
//
// This file is not named "_test.go" on purpose, mirroring every sibling
// card in this office: tests/acceptance/AUR-447.sh stages a private
// writable copy of the module and writes a tiny bridge "_test.go" file that
// calls TestAUR447, so the assertions below run inside the sandboxed
// acceptance instead of being swept into an unrelated top-level
// `go test ./...`.
package unit

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// aur447Root resolves the repository root. The acceptance harness sets
// AURUMCODE_ROOT to the staged materialization root (see
// tests/acceptance/AUR-447.sh); running the test directly from a full
// checkout works too, climbing two directories from tests/unit back to the
// repository root.
func aur447Root(t *testing.T) string {
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

// aur447Build compiles cmd/aurumcode once and returns the binary's path.
func aur447Build(t *testing.T, root string) string {
	t.Helper()
	binPath := filepath.Join(t.TempDir(), "aurumcode-aur447")
	build := exec.Command("go", "build", "-o", binPath, "./cmd/aurumcode")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build ./cmd/aurumcode failed: %v\n%s", err, out)
	}
	return binPath
}

// aur447Run runs the built binary and returns its exit code, stdout and
// stderr, never failing the test on a non-zero exit (that is the behavior
// under test).
func aur447Run(t *testing.T, bin, dir string, env []string, args ...string) (int, string, string) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	if env != nil {
		cmd.Env = env
	}
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

// aur447WriteCSharpFixture writes a small, real C# project into dir. This
// card's `paths` grants a checked-in Rust fixture directory only
// (tests/fixtures/docs/rustproject), not a checked-in C# one, so C#'s proof
// uses a runtime-only input, exactly as AUR-427's own tests do.
func aur447WriteCSharpFixture(t *testing.T, dir string) {
	t.Helper()
	const csharp = `namespace Fixture
{
    /// <summary>
    /// A greeter that says hello in a configurable language.
    /// </summary>
    public class Greeter
    {
        /// <summary>
        /// Creates a greeter for the given language tag.
        /// </summary>
        /// <param name="lang">A BCP-47-ish language tag, e.g. "en" or "pt".</param>
        public Greeter(string lang)
        {
            Lang = lang;
        }

        /// <summary>
        /// The greeter's configured language tag.
        /// </summary>
        public string Lang { get; }

        /// <summary>
        /// Renders a greeting for the given name.
        /// </summary>
        /// <returns>A human-readable greeting sentence.</returns>
        public string Greet(string name)
        {
            return Lang == "pt" ? "Ola, " + name + "!" : "Hello, " + name + "!";
        }

        private bool IsPortuguese() => Lang == "pt";
    }
}
`
	if err := os.WriteFile(filepath.Join(dir, "Greeter.cs"), []byte(csharp), 0o600); err != nil {
		t.Fatalf("writing Greeter.cs: %v", err)
	}
}

// TestAUR447 proves, through the real binary, that `aurumcode docs --source
// <projeto Rust ou C#>` documents Rust and C# projects the same way
// `cmd/regenerate-docs` already does, while Go stays regression-free, a
// secret canary never leaks, and `aurumcode review`'s published contract is
// untouched. See docs/specs/AUR-447.md.
func TestAUR447(t *testing.T) {
	root := aur447Root(t)
	bin := aur447Build(t, root)

	rustFixture := filepath.Join(root, "tests/fixtures/docs/rustproject")
	if _, err := os.Stat(rustFixture); err != nil {
		t.Fatalf("required input missing: %s: %v", rustFixture, err)
	}
	goFixture := filepath.Join(root, "tests/fixtures/docs/goproject")
	if _, err := os.Stat(goFixture); err != nil {
		t.Fatalf("required input missing: %s: %v", goFixture, err)
	}

	baseEnv := func(extra ...string) []string {
		drop := map[string]bool{"AURUM_SECRET_CANARY": true}
		env := make([]string, 0, len(os.Environ())+len(extra))
		for _, kv := range os.Environ() {
			name, _, _ := strings.Cut(kv, "=")
			if !drop[name] {
				env = append(env, kv)
			}
		}
		return append(env, extra...)
	}

	t.Run("generates real rust documentation", func(t *testing.T) {
		out := filepath.Join(t.TempDir(), "site")
		code, stdout, stderr := aur447Run(t, bin, root, baseEnv(), "docs", "--source", rustFixture, "--output", out)
		if code != 0 {
			t.Fatalf("expected exit 0, got %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
		}
		page := filepath.Join(out, "rust", "src__lib.md")
		if !strings.Contains(stdout, page) {
			t.Fatalf("expected the generated page path on stdout, got:\n%s", stdout)
		}
		content, err := os.ReadFile(page)
		if err != nil {
			t.Fatalf("expected %s to exist: %v", page, err)
		}
		for _, symbol := range []string{
			"pub struct Entry", "pub fn new_entry", "Creates a new ledger entry.",
			"pub const MAX_ENTRIES_PER_PAGE", "pub struct Ledger", "pub enum EntryKind",
		} {
			if !strings.Contains(string(content), symbol) {
				t.Fatalf("expected %q in generated page, got:\n%s", symbol, content)
			}
		}
		// Honest coverage: a macro-generated function and a private method
		// are never claimed as documented (mirrors docs/specs/AUR-427.md).
		if strings.Contains(string(content), "pub fn record_internal") {
			t.Fatalf("false claim: record_internal must not appear, got:\n%s", content)
		}
		if strings.Contains(string(content), "entry_count") {
			t.Fatalf("false claim: entry_count must not appear, got:\n%s", content)
		}
	})

	t.Run("generates real csharp documentation", func(t *testing.T) {
		src := t.TempDir()
		aur447WriteCSharpFixture(t, src)
		out := filepath.Join(t.TempDir(), "site")
		code, stdout, stderr := aur447Run(t, bin, root, baseEnv(), "docs", "--source", src, "--output", out)
		if code != 0 {
			t.Fatalf("expected exit 0, got %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
		}
		page := filepath.Join(out, "csharp", "Greeter.md")
		if !strings.Contains(stdout, page) {
			t.Fatalf("expected the generated page path on stdout, got:\n%s", stdout)
		}
		content, err := os.ReadFile(page)
		if err != nil {
			t.Fatalf("expected %s to exist: %v", page, err)
		}
		for _, symbol := range []string{
			"public class Greeter", "public Greeter(string lang)",
			"public string Greet(string name)", "A human-readable greeting sentence.",
		} {
			if !strings.Contains(string(content), symbol) {
				t.Fatalf("expected %q in generated page, got:\n%s", symbol, content)
			}
		}
		if strings.Contains(string(content), "IsPortuguese") {
			t.Fatalf("false claim: IsPortuguese (private) must not appear, got:\n%s", content)
		}
	})

	t.Run("go keeps working (no regression)", func(t *testing.T) {
		out := filepath.Join(t.TempDir(), "site")
		code, stdout, stderr := aur447Run(t, bin, root, baseEnv(), "docs", "--source", goFixture, "--output", out)
		if code != 0 {
			t.Fatalf("expected exit 0, got %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
		}
		page := filepath.Join(out, "go", "root.md")
		if !strings.Contains(stdout, page) {
			t.Fatalf("expected the generated page path on stdout, got:\n%s", stdout)
		}
		content, err := os.ReadFile(page)
		if err != nil {
			t.Fatalf("expected %s to exist: %v", page, err)
		}
		for _, symbol := range []string{"Greeting", "NewGreeting", "func Add", "func Max"} {
			if !strings.Contains(string(content), symbol) {
				t.Fatalf("expected %q in generated page, got:\n%s", symbol, content)
			}
		}
	})

	t.Run("rust generation is deterministic", func(t *testing.T) {
		out := filepath.Join(t.TempDir(), "site")
		_, stdout1, _ := aur447Run(t, bin, root, baseEnv(), "docs", "--source", rustFixture, "--output", out)
		_, stdout2, _ := aur447Run(t, bin, root, baseEnv(), "docs", "--source", rustFixture, "--output", out)
		if stdout1 != stdout2 {
			t.Fatalf("repeating the run over the same input changed stdout:\nfirst:  %q\nsecond: %q", stdout1, stdout2)
		}
	})

	t.Run("a secret canary never reaches stdout or stderr", func(t *testing.T) {
		canary := "aur447-canary-6b41af"
		out := filepath.Join(t.TempDir(), canary+"-site")
		env := baseEnv("AURUM_SECRET_CANARY=" + canary)
		code, stdout, stderr := aur447Run(t, bin, root, env, "docs", "--source", rustFixture, "--output", out)
		if code != 0 {
			t.Fatalf("expected exit 0, got %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
		}
		if strings.Contains(stdout, canary) {
			t.Fatalf("the canary leaked onto stdout:\n%s", stdout)
		}
		if strings.Contains(stderr, canary) {
			t.Fatalf("the canary leaked onto stderr:\n%s", stderr)
		}
	})

	t.Run("aurumcode review's published contract is untouched", func(t *testing.T) {
		repoDir := filepath.Join(root, "tests/fixtures/repos/git-demo/repo.git")
		if _, err := os.Stat(repoDir); err != nil {
			t.Fatalf("required input missing: %s: %v", repoDir, err)
		}
		fixture := filepath.Join(t.TempDir(), "response-clean.json")
		if err := os.WriteFile(fixture, []byte(`{"issues":[],"summary":"Nothing to report."}`), 0o600); err != nil {
			t.Fatalf("writing fixture: %v", err)
		}
		env := baseEnv("AURUMCODE_LLM_FIXTURE=" + fixture)
		code, stdout, stderr := aur447Run(t, bin, repoDir, env, "review", "--base", "HEAD~1")
		if code != 0 {
			t.Fatalf("expected exit 0, got %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
		}
		if stdout != "No issues found.\n" {
			t.Fatalf("aurumcode review's published byte-for-byte contract changed: stdout=%q", stdout)
		}
	})
}
