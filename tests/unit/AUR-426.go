// Package unit holds AUR-426's Unit-layer proof: `aurumcode docs` is a real,
// flag-driven subcommand of cmd/aurumcode that reuses internal/pipeline and
// internal/documentation/extractors to generate documentation -- not a stub
// that only parses flags -- and that its --help output, docs/specs/AUR-426.md
// and the flag set actually registered in the code all agree (MUT-001: the
// three must never silently drift apart).
//
// This file is not named "_test.go" on purpose, mirroring every sibling card
// in this office: tests/acceptance/AUR-426.sh stages a private writable copy
// of the module and writes a tiny bridge "_test.go" file that calls
// TestAUR426, so the assertions below run inside the sandboxed acceptance
// instead of being swept into an unrelated top-level `go test ./...`.
package unit

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// aur426Root resolves the repository root. The acceptance harness sets
// AURUMCODE_ROOT to the staged materialization root (see
// tests/acceptance/AUR-426.sh); running the test directly from a full
// checkout works too, climbing two directories from tests/unit back to the
// repository root.
func aur426Root(t *testing.T) string {
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

// aur426Build compiles cmd/aurumcode once and returns the binary's path.
func aur426Build(t *testing.T, root string) string {
	t.Helper()
	binPath := filepath.Join(t.TempDir(), "aurumcode-aur426")
	build := exec.Command("go", "build", "-o", binPath, "./cmd/aurumcode")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build ./cmd/aurumcode failed: %v\n%s", err, out)
	}
	return binPath
}

// aur426Run runs the built binary and returns its exit code, stdout and
// stderr, never failing the test on a non-zero exit (that is the behavior
// under test).
func aur426Run(t *testing.T, bin, dir string, env []string, args ...string) (int, string, string) {
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

// aur426HelpFlagRE matches a flag.PrintDefaults() flag-introducer line, e.g.
// "  -languages string" -- exactly two leading spaces then a dash, which
// distinguishes it from the indented description line that follows
// ("    \tcomma-separated ..."). This is what makes the extracted set the
// REAL registered flags rather than a hand-authored copy: flag.PrintDefaults
// derives this text straight from the fs.String/fs.Bool calls in
// cmd/aurumcode/docs.go, so a mutant that deletes one of those calls changes
// this output by construction, with no separate help string to keep in sync.
var aur426HelpFlagRE = regexp.MustCompile(`(?m)^  -([a-zA-Z][\w-]*)`)

// aur426SpecFlagRE matches a flag row in docs/specs/AUR-426.md's "## Command"
// table, e.g. "| `--source <dir>` | `.` | ...".
var aur426SpecFlagRE = regexp.MustCompile("(?m)^\\|\\s*`--([a-zA-Z][\\w-]*)")

func aur426ExtractFlags(re *regexp.Regexp, text string) []string {
	matches := re.FindAllStringSubmatch(text, -1)
	names := make([]string, 0, len(matches))
	seen := map[string]bool{}
	for _, m := range matches {
		if !seen[m[1]] {
			seen[m[1]] = true
			names = append(names, m[1])
		}
	}
	sort.Strings(names)
	return names
}

func aur426AssertFlagSetsEqual(t *testing.T, label string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: flag set mismatch\n  got:  %v\n  want: %v", label, got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("%s: flag set mismatch\n  got:  %v\n  want: %v", label, got, want)
		}
	}
}

// TestAUR426 proves `aurumcode docs` behaviorally, through the real binary:
// --help is self-documenting and its flags match docs/specs/AUR-426.md and
// the code's own registered set (MUT-001's target); the command actually
// drives internal/pipeline to generate real documentation, not a stub;
// invalid input and a run that documents nothing both fail loudly instead of
// exiting 0; a secret canary never reaches stdout or stderr; the run is
// deterministic; and `aurumcode review`'s published contract is untouched.
// See docs/specs/AUR-426.md.
func TestAUR426(t *testing.T) {
	root := aur426Root(t)
	bin := aur426Build(t, root)
	goproject := filepath.Join(root, "tests/fixtures/docs/goproject")
	if _, err := os.Stat(goproject); err != nil {
		t.Fatalf("required input missing: %s: %v", goproject, err)
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

	t.Run("--help documents usage on stdout and exits 0", func(t *testing.T) {
		code, stdout, stderr := aur426Run(t, bin, root, baseEnv(), "docs", "--help")
		if code != 0 {
			t.Fatalf("expected exit 0, got %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
		}
		if stderr != "" {
			t.Fatalf("expected empty stderr for --help, got:\n%s", stderr)
		}
		if !strings.Contains(stdout, "usage: aurumcode docs") {
			t.Fatalf("expected a usage line on stdout, got:\n%s", stdout)
		}
		for _, want := range []string{"-source", "-output", "-languages"} {
			if !strings.Contains(stdout, want) {
				t.Fatalf("expected %q documented in --help, got:\n%s", want, stdout)
			}
		}
	})

	t.Run("-h behaves the same as --help", func(t *testing.T) {
		code, stdout, _ := aur426Run(t, bin, root, baseEnv(), "docs", "-h")
		if code != 0 {
			t.Fatalf("expected exit 0 for -h, got %d", code)
		}
		if !strings.Contains(stdout, "usage: aurumcode docs") {
			t.Fatalf("expected -h to print usage, got:\n%s", stdout)
		}
	})

	// MUT-001's guard: the flags actually printed by --help (which
	// flag.PrintDefaults derives from the real registered flag.FlagSet, not
	// a hand-authored string) must equal the flags docs/specs/AUR-426.md
	// documents. Removing an fs.String(...) registration in docs.go shrinks
	// the --help set while the spec (untouched by that code mutation)
	// still lists three, so the sets diverge and this fails.
	t.Run("help flags match the documented spec table (MUT-001 guard)", func(t *testing.T) {
		_, stdout, _ := aur426Run(t, bin, root, baseEnv(), "docs", "--help")
		helpFlags := aur426ExtractFlags(aur426HelpFlagRE, stdout)

		specPath := filepath.Join(root, "docs/specs/AUR-426.md")
		specBytes, err := os.ReadFile(specPath)
		if err != nil {
			t.Fatalf("required input missing: %s: %v", specPath, err)
		}
		specFlags := aur426ExtractFlags(aur426SpecFlagRE, string(specBytes))

		if len(helpFlags) == 0 {
			t.Fatalf("--help produced no parseable flags at all:\n%s", stdout)
		}
		if len(specFlags) == 0 {
			t.Fatalf("docs/specs/AUR-426.md's Command table produced no parseable flags")
		}
		aur426AssertFlagSetsEqual(t, "--help vs docs/specs/AUR-426.md", helpFlags, specFlags)

		want := []string{"languages", "output", "source"}
		aur426AssertFlagSetsEqual(t, "--help vs the three flags the Outcome names", helpFlags, want)
	})

	t.Run("an unknown flag is a usage error on stderr, exit 2", func(t *testing.T) {
		code, stdout, stderr := aur426Run(t, bin, root, baseEnv(), "docs", "--bogus")
		if code != 2 {
			t.Fatalf("expected exit 2, got %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
		}
		if stdout != "" {
			t.Fatalf("expected empty stdout for a usage error, got:\n%s", stdout)
		}
		if !strings.Contains(stderr, "usage: aurumcode docs") {
			t.Fatalf("expected usage on stderr, got:\n%s", stderr)
		}
	})

	t.Run("generates real documentation by reusing the pipeline", func(t *testing.T) {
		out := filepath.Join(t.TempDir(), "site")
		code, stdout, stderr := aur426Run(t, bin, root, baseEnv(), "docs", "--source", goproject, "--output", out)
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
		// Real symbols parsed from the fixture's real source, not a
		// placeholder -- proves internal/pipeline + the Go extractor
		// actually ran, mirroring docs/specs/AUR-424.md's own proof.
		for _, symbol := range []string{"Greeting", "NewGreeting", "func Add", "func Max"} {
			if !strings.Contains(string(content), symbol) {
				t.Fatalf("expected %q in generated page, got:\n%s", symbol, content)
			}
		}
	})

	t.Run("same input produces the same output (determinism)", func(t *testing.T) {
		out := filepath.Join(t.TempDir(), "site")
		_, stdout1, _ := aur426Run(t, bin, root, baseEnv(), "docs", "--source", goproject, "--output", out)
		_, stdout2, _ := aur426Run(t, bin, root, baseEnv(), "docs", "--source", goproject, "--output", out)
		if stdout1 != stdout2 {
			t.Fatalf("repeating the run over the same input changed stdout:\nfirst:  %q\nsecond: %q", stdout1, stdout2)
		}
	})

	t.Run("a missing --source fails clearly, exit 1", func(t *testing.T) {
		missing := filepath.Join(t.TempDir(), "does-not-exist")
		code, stdout, stderr := aur426Run(t, bin, root, baseEnv(), "docs", "--source", missing, "--output", filepath.Join(t.TempDir(), "out"))
		if code != 1 {
			t.Fatalf("expected exit 1, got %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
		}
		if !strings.Contains(stderr, "source directory") {
			t.Fatalf("expected a clear source-directory error, got:\n%s", stderr)
		}
	})

	t.Run("a --source that is a file, not a directory, fails clearly, exit 1", func(t *testing.T) {
		file := filepath.Join(t.TempDir(), "notadir.txt")
		if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
			t.Fatalf("writing fixture file: %v", err)
		}
		code, _, stderr := aur426Run(t, bin, root, baseEnv(), "docs", "--source", file, "--output", filepath.Join(t.TempDir(), "out"))
		if code != 1 {
			t.Fatalf("expected exit 1, got %d\nstderr=%s", code, stderr)
		}
		if !strings.Contains(stderr, "is not a directory") {
			t.Fatalf("expected the not-a-directory error, got:\n%s", stderr)
		}
	})

	t.Run("zero documented pages is never a silent green", func(t *testing.T) {
		out := filepath.Join(t.TempDir(), "site")
		code, stdout, stderr := aur426Run(t, bin, root, baseEnv(), "docs", "--source", goproject, "--output", out, "--languages", "nonexistent-language")
		if code != 1 {
			t.Fatalf("expected exit 1 for a filter matching nothing, got %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
		}
		if !strings.Contains(stderr, "no documentation page was generated") {
			t.Fatalf("expected the empty-run message, got:\n%s", stderr)
		}
	})

	t.Run("a secret canary never reaches stdout or stderr", func(t *testing.T) {
		canary := "aur426-canary-2c9f5e"
		out := filepath.Join(t.TempDir(), canary+"-site")
		env := baseEnv("AURUM_SECRET_CANARY=" + canary)
		code, stdout, stderr := aur426Run(t, bin, root, env, "docs", "--source", goproject, "--output", out)
		if code != 0 {
			t.Fatalf("expected exit 0, got %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
		}
		if strings.Contains(stdout, canary) {
			t.Fatalf("the canary leaked onto stdout:\n%s", stdout)
		}
		if strings.Contains(stderr, canary) {
			t.Fatalf("the canary leaked onto stderr:\n%s", stderr)
		}
		if !strings.Contains(stdout, "[REDACTED]") {
			t.Fatalf("expected the redaction marker in place of the canary, got:\n%s", stdout)
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
		code, stdout, stderr := aur426Run(t, bin, repoDir, env, "review", "--base", "HEAD~1")
		if code != 0 {
			t.Fatalf("expected exit 0, got %d\nstdout=%s\nstderr=%s", code, stdout, stderr)
		}
		if stdout != "No issues found.\n" {
			t.Fatalf("aurumcode review's published byte-for-byte contract changed: stdout=%q", stdout)
		}
	})
}
