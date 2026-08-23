// IntegrationAUR462 builds the real aurumcode binary and proves AUR-462's
// outcome at the CLI boundary, distinct from tests/unit/AUR-462.go's
// package-boundary proof over synthetic types.Diff values.
//
// This card's `paths` do not include a fixtures directory (unlike AUR-442,
// which owned tests/fixtures/review/vuln/hardcoded-secret), so this program
// never commits a Node fixture to the repository tree. Instead it
// materializes one at runtime: an ephemeral, non-bare git repository (a
// second temp-dir commit adding a 27-line Node source file, the exact same
// content and line numbers as tests/unit/AUR-462.go's
// aur462NodeFixtureLines) built with `git init`/`git commit` inside
// t.TempDir(), diffed with `--base HEAD~1` exactly as a user would. No
// LLM provider is configured for these runs (LLM_API_KEY, LLM_BASE_URL,
// AURUMCODE_LLM_FIXTURE are all stripped from the child environment): the
// engine's own AUR-449 behavior then runs `--seguranca` alone, which is
// deterministic and needs no model, so this program depends on nothing
// outside the process it starts.
//
// It also proves AC-003 (regression) against the project's own already
// committed, already read-only fixture
// (tests/fixtures/review/vuln/repo.git, AUR-435's Python SQL-injection
// fixture): the finding it already produced continues to be produced,
// unchanged.
package integration

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// aur462Root resolves the repository root exactly like the sibling engine
// integration programs do (see tests/integration/AUR-442.go's aur442Root).
func aur462Root(t *testing.T) string {
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

// aur462NodeFixtureLines mirrors tests/unit/AUR-462.go's
// aur462NodeFixtureLines byte for byte: four planted defects (line 5 exec
// concat, line 9 execSync concat, line 17 spawn shell:true, line 22
// innerHTML direct write) and four deliberately benign neighbors (an argv
// exec call, a shell-less spawn -- implicit, via the argv exec reuse --
// a comment mentioning "exec", and an innerHTML literal).
var aur462NodeFixtureLines = []string{
	`"use strict";`,
	`const { exec, execSync, spawn } = require("child_process");`,
	``,
	`function pingConcat(host) {`,
	`  exec("ping -c 1 " + host);`,
	`}`,
	``,
	`function pingSyncConcat(host) {`,
	`  execSync("ping -c 1 " + host);`,
	`}`,
	``,
	`function pingArgv(host) {`,
	`  exec(["ping", host]);`,
	`}`,
	``,
	`function pingShell(host) {`,
	`  spawn("ping -c 1 " + host, { shell: true });`,
	`}`,
	``,
	`// exec is dangerous when combined with string concatenation.`,
	`function renderUser(userInput) {`,
	`  document.getElementById("out").innerHTML = userInput;`,
	`}`,
	``,
	`function renderStatic() {`,
	`  document.getElementById("out").innerHTML = "<b>Static content</b>";`,
	`}`,
	``,
	`function renderSanitized(userInput) {`,
	`  document.getElementById("out").innerHTML = DOMPurify.sanitize(userInput);`,
	`}`,
	``,
	`function renderTrustedTemplate() {`,
	`  document.getElementById("out").innerHTML = TRUSTED_TEMPLATE;`,
	`}`,
	``,
	`function renderEscaped(x) {`,
	`  document.getElementById("out").innerHTML = escapeHtml(x);`,
	`}`,
}

// filteredEnv strips the three environment variables selectProvider (see
// cmd/aurumcode/main.go) checks, so the child process reliably takes the
// AUR-449 "no provider configured, running --seguranca only" path
// regardless of what the host running this test happens to have set.
func filteredEnv() []string {
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

func aur462Git(t *testing.T, dir string, env []string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = env
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out.String())
	}
	return out.String()
}

// buildAUR462NodeRepo materializes the ephemeral Node fixture repository
// described in the package doc above and returns its path.
func buildAUR462NodeRepo(t *testing.T) string {
	t.Helper()
	repoDir := filepath.Join(t.TempDir(), "node-repo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	gitEnv := append(filteredEnv(),
		"GIT_AUTHOR_NAME=Aurum Test", "GIT_AUTHOR_EMAIL=aurum-test@aurum.invalid",
		"GIT_COMMITTER_NAME=Aurum Test", "GIT_COMMITTER_EMAIL=aurum-test@aurum.invalid",
	)
	aur462Git(t, repoDir, gitEnv, "init", "-q", "-b", "main")
	aur462Git(t, repoDir, gitEnv, "config", "user.email", "aurum-test@aurum.invalid")
	aur462Git(t, repoDir, gitEnv, "config", "user.name", "Aurum Test")

	readme := filepath.Join(repoDir, "README.md")
	if err := os.WriteFile(readme, []byte("Ephemeral AUR-462 Node fixture. Nothing here is a real application.\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	aur462Git(t, repoDir, gitEnv, "add", "README.md")
	aur462Git(t, repoDir, gitEnv, "commit", "-q", "-m", "seed: add the fixture skeleton")

	src := filepath.Join(repoDir, "src")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}
	content := strings.Join(aur462NodeFixtureLines, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(src, "app.js"), []byte(content), 0o644); err != nil {
		t.Fatalf("write app.js: %v", err)
	}
	aur462Git(t, repoDir, gitEnv, "add", "src/app.js")
	aur462Git(t, repoDir, gitEnv, "commit", "-q", "-m", "add: node source with planted command-injection and xss shapes")

	return repoDir
}

const aur462SecurityHeader = "Security findings (standards/security-review):"

func aur462Run(t *testing.T, bin, dir string, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	cmd.Env = filteredEnv()
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

func IntegrationAUR462(t *testing.T) {
	root := aur462Root(t)

	binPath := filepath.Join(t.TempDir(), "aurumcode-aur462")
	build := exec.Command("go", "build", "-o", binPath, "./cmd/aurumcode")
	build.Dir = root
	build.Env = os.Environ()
	var buildOut bytes.Buffer
	build.Stdout = &buildOut
	build.Stderr = &buildOut
	if err := build.Run(); err != nil {
		t.Fatalf("go build ./cmd/aurumcode failed: %v\n%s", err, buildOut.String())
	}

	nodeRepo := buildAUR462NodeRepo(t)

	// Without --seguranca: no security section, regardless of exit code --
	// this card only touches the security pass's patterns, so it does not
	// change what happens without the flag. (With no provider configured
	// and no --seguranca, the published contract is exit 1 with an empty
	// stdout; that contract is untouched here, only re-asserted narrowly:
	// no security section ever leaks in without the flag.)
	base, _, _ := aur462Run(t, binPath, nodeRepo, "review", "--base", "HEAD~1")
	if strings.Contains(base, aur462SecurityHeader) {
		t.Fatalf("without --seguranca no security section may exist, got:\n%s", base)
	}

	// AC-001 + AC-002 in one CLI run: exactly the four planted lines are
	// reported, citing their rules, and nowhere else -- the fixture's four
	// deliberately benign neighbors (argv exec, exec-in-comment, innerHTML
	// literal) produce nothing.
	out, stderr, code := aur462Run(t, binPath, nodeRepo, "review", "--base", "HEAD~1", "--seguranca")
	if code != 0 {
		t.Fatalf("review --seguranca must exit 0 with no provider configured (AUR-449 skip), got %d\nstderr=%s", code, stderr)
	}
	if !strings.Contains(stderr, "quality review skipped") {
		t.Fatalf("expected the AUR-449 quality-skip note on stderr (proves this run took the no-provider path, not a live model call), got:\n%s", stderr)
	}
	if !strings.Contains(out, aur462SecurityHeader) {
		t.Fatalf("expected the security section header, got:\n%s", out)
	}
	_, section, found := strings.Cut(out, aur462SecurityHeader)
	if !found {
		t.Fatalf("expected the security section header, got:\n%s", out)
	}
	wantFindings := []string{
		"src/app.js:5: [error]",
		"src/app.js:9: [error]",
		"src/app.js:17: [error]",
		"src/app.js:22: [error]",
	}
	for _, want := range wantFindings {
		if !strings.Contains(section, want) {
			t.Fatalf("expected finding %q in the security section, got:\n%s", want, section)
		}
	}
	if got := strings.Count(section, "src/app.js:"); got != len(wantFindings) {
		t.Fatalf("expected exactly %d findings in the security section (the fixture's benign neighbors must produce none), got %d:\n%s", len(wantFindings), got, section)
	}
	// Adversarial-review proof (2026-08-23): a sanitizer call
	// (DOMPurify.sanitize), an uppercase module constant
	// (TRUSTED_TEMPLATE), and an escaping helper call (escapeHtml) must
	// never be found, even though the count check above would already
	// catch it -- named explicitly here so a future reader sees exactly
	// which lines these are.
	for _, mustNotAppear := range []string{
		"src/app.js:30: [error]",
		"src/app.js:34: [error]",
		"src/app.js:38: [error]",
	} {
		if strings.Contains(section, mustNotAppear) {
			t.Fatalf("false positive: %q must never appear, got:\n%s", mustNotAppear, section)
		}
	}
	if !strings.Contains(section, "rule security/command-injection") {
		t.Fatalf("expected a command-injection citation, got:\n%s", section)
	}
	if !strings.Contains(section, "rule security/xss") {
		t.Fatalf("expected an xss citation, got:\n%s", section)
	}

	// Determinism: same input, same bytes.
	again, _, code := aur462Run(t, binPath, nodeRepo, "review", "--base", "HEAD~1", "--seguranca")
	if code != 0 || out != again {
		t.Fatalf("review --seguranca is not deterministic (exit %d):\nfirst=%q\nsecond=%q", code, out, again)
	}

	// AC-003 regression: the project's own already-committed Python
	// SQL-injection fixture (tests/fixtures/review/vuln/repo.git, read-only
	// input to this card) must still produce exactly the finding it
	// produced before this card touched security/command-injection and
	// security/xss.
	pyRepo := filepath.Join(root, "tests/fixtures/review/vuln/repo.git")
	if _, err := os.Stat(pyRepo); err != nil {
		t.Fatalf("required regression input missing: %s: %v", pyRepo, err)
	}
	pyOut, _, code := aur462Run(t, binPath, pyRepo, "review", "--base", "HEAD~1", "--seguranca")
	if code != 0 {
		t.Fatalf("review --seguranca on the Python vuln fixture must exit 0, got %d", code)
	}
	if !strings.Contains(pyOut, "src/db.py:8: [error]") {
		t.Fatalf("expected the pre-existing Python sql-injection finding at src/db.py:8 to survive unchanged, got:\n%s", pyOut)
	}
	if !strings.Contains(pyOut, "rule security/sql-injection") {
		t.Fatalf("expected the security/sql-injection citation to survive unchanged, got:\n%s", pyOut)
	}
	if strings.Count(pyOut, "[error]") != 1 {
		t.Fatalf("expected exactly one finding on the Python vuln fixture (unchanged from before this card), got:\n%s", pyOut)
	}
}
