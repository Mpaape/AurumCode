// IntegrationAUR481 builds the real aurumcode binary and proves AUR-481's
// Rust outcome at the CLI boundary, distinct from tests/unit/AUR-481.go's
// package-boundary proof over synthetic types.Diff values.
//
// The sealed acceptance profile (bootstrap-readonly-v1) carries bash and a
// Go toolchain but no `git` binary. So the fixture is committed instead:
// tests/fixtures/review/vuln/rust-secret-sql-injection/repo.git, a bare,
// loose-object repository built by
// tests/fixtures/repos/git-demo/build-fixture.sh -- the same git-less,
// deterministic builder that produced every sibling fixture this card's
// own AC-003 regresses against. This program only reads it: no git binary
// anywhere in this file, at build time or run time.
//
// `--base HEAD~1` is diffed exactly as a user would. No LLM provider is
// configured for these runs (LLM_API_KEY, LLM_BASE_URL,
// AURUMCODE_LLM_FIXTURE are all stripped from the child environment): the
// engine's own AUR-449 behavior then runs `--seguranca` alone.
package integration

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func aur481Root(t *testing.T) string {
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

func aur481RequireFixture(t *testing.T, root, rel string) string {
	t.Helper()
	p := filepath.Join(root, rel)
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("required input missing: %s: %v", p, err)
	}
	return p
}

func aur481FilteredEnv() []string {
	var out []string
	for _, kv := range os.Environ() {
		switch {
		case strings.HasPrefix(kv, "LLM_API_KEY="),
			strings.HasPrefix(kv, "LLM_BASE_URL="),
			strings.HasPrefix(kv, "AURUMCODE_LLM_FIXTURE="):
			continue
		}
		out = append(out, kv)
	}
	return out
}

const aur481SecurityHeader = "Security findings (standards/security-review):"

func aur481Run(t *testing.T, bin, dir string, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	cmd.Env = aur481FilteredEnv()
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	code := 0
	if err != nil {
		exitErr, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("running %v in %s: %v\nstderr=%s", args, dir, err, stderr.String())
		}
		code = exitErr.ExitCode()
	}
	return stdout.String(), stderr.String(), code
}

func IntegrationAUR481(t *testing.T) {
	root := aur481Root(t)

	binPath := filepath.Join(t.TempDir(), "aurumcode-aur481")
	build := exec.Command("go", "build", "-o", binPath, "./cmd/aurumcode")
	build.Dir = root
	build.Env = os.Environ()
	var buildOut bytes.Buffer
	build.Stdout = &buildOut
	build.Stderr = &buildOut
	if err := build.Run(); err != nil {
		t.Fatalf("go build ./cmd/aurumcode failed: %v\n%s", err, buildOut.String())
	}

	rustRepo := aur481RequireFixture(t, root, "tests/fixtures/review/vuln/rust-secret-sql-injection/repo.git")

	// Without --seguranca: no security section leaks in.
	base, _, _ := aur481Run(t, binPath, rustRepo, "review", "--base", "HEAD~1")
	if strings.Contains(base, aur481SecurityHeader) {
		t.Fatalf("without --seguranca no security section may exist, got:\n%s", base)
	}

	out, stderr, code := aur481Run(t, binPath, rustRepo, "review", "--base", "HEAD~1", "--seguranca")
	if code != 0 {
		t.Fatalf("review --seguranca must exit 0 with no provider configured, got %d\nstderr=%s", code, stderr)
	}
	if !strings.Contains(stderr, "quality review skipped") {
		t.Fatalf("expected the AUR-449 quality-skip note on stderr, got:\n%s", stderr)
	}
	if !strings.Contains(out, aur481SecurityHeader) {
		t.Fatalf("expected the security section header, got:\n%s", out)
	}
	_, section, found := strings.Cut(out, aur481SecurityHeader)
	if !found {
		t.Fatalf("expected the security section header, got:\n%s", out)
	}

	// AC-001: the six Rust shapes this card restores are all found.
	wantFindings := []string{
		"src/main.rs:1: [error]",
		"src/main.rs:6: [error]",
		"src/main.rs:11: [error]",
		"src/main.rs:15: [error]",
		"src/main.rs:24: [error]",
		"src/main.rs:28: [error]",
	}
	for _, want := range wantFindings {
		if !strings.Contains(section, want) {
			t.Fatalf("expected finding %q in the security section, got:\n%s", want, section)
		}
	}
	if got := strings.Count(section, "src/main.rs:"); got != len(wantFindings) {
		t.Fatalf("expected exactly %d findings in src/main.rs, got %d:\n%s", len(wantFindings), got, section)
	}
	for _, citation := range []string{
		"rule security/hardcoded-secret",
		"rule security/sql-injection",
		"rule security/command-injection",
	} {
		if !strings.Contains(section, citation) {
			t.Fatalf("expected citation %q, got:\n%s", citation, section)
		}
	}

	// AC-002: none of the four safe forms (lines 2, 3, 19, 32) produce a
	// finding -- confirmed above by the exact-6 count, since the fixture
	// carries exactly 6 unsafe and 4 safe lines in src/main.rs.

	// Determinism.
	again, _, code := aur481Run(t, binPath, rustRepo, "review", "--base", "HEAD~1", "--seguranca")
	if code != 0 || out != again {
		t.Fatalf("review --seguranca is not deterministic (exit %d):\nfirst=%q\nsecond=%q", code, out, again)
	}

	// AC-003 regression: the project's own already-committed Python
	// SQL-injection fixture must still produce exactly the finding it
	// produced before this card touched security/sql-injection.
	pyRepo := aur481RequireFixture(t, root, "tests/fixtures/review/vuln/repo.git")
	pyOut, _, code := aur481Run(t, binPath, pyRepo, "review", "--base", "HEAD~1", "--seguranca")
	if code != 0 {
		t.Fatalf("review --seguranca on the Python vuln fixture must exit 0, got %d", code)
	}
	if !strings.Contains(pyOut, "src/db.py:8: [error]") || !strings.Contains(pyOut, "rule security/sql-injection") {
		t.Fatalf("expected the pre-existing Python sql-injection finding to survive unchanged, got:\n%s", pyOut)
	}
	if strings.Count(pyOut, "[error]") != 1 {
		t.Fatalf("expected exactly one finding on the Python vuln fixture, got:\n%s", pyOut)
	}

	// AC-003 regression: the pre-existing hardcoded-secret fixture must
	// still produce exactly the two findings it produced before this card
	// touched security/hardcoded-secret.
	secretRepo := aur481RequireFixture(t, root, "tests/fixtures/review/vuln/hardcoded-secret/repo.git")
	secretOut, _, code := aur481Run(t, binPath, secretRepo, "review", "--base", "HEAD~1", "--seguranca")
	if code != 0 {
		t.Fatalf("review --seguranca on the hardcoded-secret fixture must exit 0, got %d", code)
	}
	if strings.Count(secretOut, "rule security/hardcoded-secret") != 2 {
		t.Fatalf("expected exactly two hardcoded-secret citations to survive unchanged, got:\n%s", secretOut)
	}
	if strings.Contains(secretOut, "AURUM-FAKE-KEY-9000-2222") || strings.Contains(secretOut, "AURUM-FAKE-PASSWORD-9000-1111") {
		t.Fatalf("secret value must never be echoed into output, got:\n%s", secretOut)
	}

	// AC-003 regression: the pre-existing Node command-injection/xss
	// fixture (AUR-462) must still produce exactly its 3 command-injection
	// and 1 xss findings, unaffected by this card leaving
	// security/command-injection's pattern untouched.
	node462Repo := aur481RequireFixture(t, root, "tests/fixtures/review/vuln/node-xss-command-injection/repo.git")
	node462Out, _, code := aur481Run(t, binPath, node462Repo, "review", "--base", "HEAD~1", "--seguranca")
	if code != 0 {
		t.Fatalf("review --seguranca on the node-xss-command-injection fixture must exit 0, got %d", code)
	}
	if got := strings.Count(node462Out, "rule security/command-injection"); got != 3 {
		t.Fatalf("expected exactly 3 command-injection citations to survive unchanged, got %d:\n%s", got, node462Out)
	}
	if got := strings.Count(node462Out, "rule security/xss"); got != 1 {
		t.Fatalf("expected exactly 1 xss citation to survive unchanged, got %d:\n%s", got, node462Out)
	}
}
