// IntegrationAUR486 builds the real aurumcode binary and proves AUR-486's
// Go/C#/PowerShell/bash outcome at the CLI boundary, distinct from
// tests/unit/AUR-486.go's package-boundary proof over synthetic types.Diff
// values.
//
// The sealed acceptance profile (bootstrap-readonly-v1) carries bash and a
// Go toolchain but no `git` binary, and this card's `paths` do not include
// tests/fixtures, so there is no committed fixture to read. Instead this
// program writes a small, valid bare Git repository directly into a temp
// directory -- loose objects (zlib + the standard "type size\0content"
// header), a tree, two commits and a refs/heads/main ref -- the exact
// format tests/fixtures/repos/git-demo/build-fixture.sh emits, and the
// same one internal/analyzer's pure-Go reader inflates when no `git`
// binary is present. Nothing here shells out to `git`.
//
// `--base HEAD~1` is diffed exactly as a user would. No LLM provider is
// configured for these runs (LLM_API_KEY, LLM_BASE_URL,
// AURUMCODE_LLM_FIXTURE are all stripped from the child environment): the
// engine's own AUR-449 behavior then runs `--seguranca` alone.
package integration

import (
	"bytes"
	"compress/zlib"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func aur486Root(t *testing.T) string {
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

func aur486FilteredEnv() []string {
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

func aur486Run(t *testing.T, bin, dir string, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	cmd.Env = aur486FilteredEnv()
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

const aur486SecurityHeader = "Security findings (standards/security-review):"

// aur486Section returns the security section of a review's stdout, so a
// regression count is scoped to the security pass and not confused by the
// separate deterministic-analysis section (analysis/sql-injection) that a
// review can also emit.
func aur486Section(out string) string {
	_, section, ok := strings.Cut(out, aur486SecurityHeader)
	if !ok {
		return ""
	}
	return section
}

// aur486WriteObject writes one loose Git object and returns its 40-hex id.
func aur486WriteObject(t *testing.T, gitDir, typ string, content []byte) string {
	t.Helper()
	full := make([]byte, 0, len(content)+32)
	full = append(full, []byte(fmt.Sprintf("%s %d\x00", typ, len(content)))...)
	full = append(full, content...)
	sum := sha1.Sum(full)
	id := hex.EncodeToString(sum[:])
	dir := filepath.Join(gitDir, "objects", id[:2])
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir objects: %v", err)
	}
	var buf bytes.Buffer
	zw := zlib.NewWriter(&buf)
	if _, err := zw.Write(full); err != nil {
		t.Fatalf("zlib write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zlib close: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, id[2:]), buf.Bytes(), 0o644); err != nil {
		t.Fatalf("write loose object: %v", err)
	}
	return id
}

func aur486Tree(t *testing.T, gitDir string, files map[string]string) string {
	t.Helper()
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	var buf bytes.Buffer
	for _, name := range names {
		blobID := aur486WriteObject(t, gitDir, "blob", []byte(files[name]))
		raw, err := hex.DecodeString(blobID)
		if err != nil {
			t.Fatalf("decode blob id: %v", err)
		}
		buf.WriteString("100644 " + name + "\x00")
		buf.Write(raw)
	}
	return aur486WriteObject(t, gitDir, "tree", buf.Bytes())
}

func aur486Commit(t *testing.T, gitDir, tree, parent, msg string, epoch int64) string {
	t.Helper()
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "tree %s\n", tree)
	if parent != "" {
		fmt.Fprintf(&buf, "parent %s\n", parent)
	}
	fmt.Fprintf(&buf, "author Aurum Test <test@aurum.invalid> %d +0000\n", epoch)
	fmt.Fprintf(&buf, "committer Aurum Test <test@aurum.invalid> %d +0000\n", epoch)
	fmt.Fprintf(&buf, "\n%s\n", msg)
	return aur486WriteObject(t, gitDir, "commit", buf.Bytes())
}

// aur486FixtureFiles returns the added-file content of the two-commit
// fixture. It MUST stay byte-identical to tests/unit/AUR-486.go's
// aur486FixtureLines and tests/e2e/AUR-486.sh's heredoc.
func aur486FixtureFiles() map[string]string {
	return map[string]string{
		"main.go":    "package main\n\nimport \"os/exec\"\n\nfunc pingUnsafe(host string) {\n\texec.Command(\"sh\", \"-c\", \"ping \"+host).Run()\n}\n\nfunc pingSafe(host string) {\n\texec.Command(\"ping\", host).Run()\n}\n",
		"App.cs":     "using System.Diagnostics;\n\nclass App {\n    static void Ping(string host) {\n        Process.Start(\"cmd.exe\", \"/c ping \" + host);\n        Process.Start(\"ping\", host);\n    }\n}\n",
		"script.ps1": "function Invoke-Ping($host) {\n    Invoke-Expression \"ping $host\"\n}\nGet-Process\n",
		"deploy.sh":  "#!/bin/bash\nset -euo pipefail\nping_unsafe() {\n    eval \"ping $1\"\n}\nping_shell_unsafe() {\n    sh -c \"ping $1\"\n}\nping_safe() {\n    ping \"$1\"\n}\n# eval \"ping $1\" in a comment must not fire\necho \"the literal eval \\\"ping $1\\\" must not fire\"\nquery=\"SELECT * FROM users WHERE name = '$1'\"\n",
	}
}

func aur486WriteRepo(t *testing.T, gitDir string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(gitDir, "refs", "heads"), 0o755); err != nil {
		t.Fatalf("mkdir refs: %v", err)
	}
	seed := aur486Tree(t, gitDir, map[string]string{"README.md": "AUR-486 ephemeral fixture. Nothing here is a real application.\n"})
	c1 := aur486Commit(t, gitDir, seed, "", "seed: add the fixture skeleton", 1700000000)
	head := aur486Tree(t, gitDir, aur486FixtureFiles())
	c2 := aur486Commit(t, gitDir, head, c1, "chore: plant go csharp powershell bash shapes", 1700000060)
	if err := os.WriteFile(filepath.Join(gitDir, "refs", "heads", "main"), []byte(c2+"\n"), 0o644); err != nil {
		t.Fatalf("write ref: %v", err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		t.Fatalf("write HEAD: %v", err)
	}
	cfg := "[core]\n\trepositoryformatversion = 0\n\tfilemode = false\n\tbare = true\n"
	if err := os.WriteFile(filepath.Join(gitDir, "config"), []byte(cfg), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

func IntegrationAUR486(t *testing.T) {
	root := aur486Root(t)

	binPath := filepath.Join(t.TempDir(), "aurumcode-aur486")
	build := exec.Command("go", "build", "-o", binPath, "./cmd/aurumcode")
	build.Dir = root
	build.Env = os.Environ()
	var buildOut bytes.Buffer
	build.Stdout = &buildOut
	build.Stderr = &buildOut
	if err := build.Run(); err != nil {
		t.Fatalf("go build ./cmd/aurumcode failed: %v\n%s", err, buildOut.String())
	}

	gitDir := filepath.Join(t.TempDir(), "repo.git")
	aur486WriteRepo(t, gitDir)

	// Without --seguranca: no security section leaks in.
	base, _, _ := aur486Run(t, binPath, gitDir, "review", "--base", "HEAD~1")
	if strings.Contains(base, aur486SecurityHeader) {
		t.Fatalf("without --seguranca no security section may exist, got:\n%s", base)
	}

	out, stderr, code := aur486Run(t, binPath, gitDir, "review", "--base", "HEAD~1", "--seguranca")
	if code != 0 {
		t.Fatalf("review --seguranca must exit 0 with no provider configured, got %d\nstderr=%s", code, stderr)
	}
	if !strings.Contains(out, aur486SecurityHeader) {
		t.Fatalf("expected the security section header, got:\n%s", out)
	}
	_, section, found := strings.Cut(out, aur486SecurityHeader)
	if !found {
		t.Fatalf("expected the security section header, got:\n%s", out)
	}

	// AC-001: the five command-injection shapes (Go, C#, PowerShell, bash
	// eval, bash sh -c) and the SQL-via-shell shape are all found.
	wantFindings := []string{
		"main.go:6: [error]",
		"App.cs:5: [error]",
		"script.ps1:2: [error]",
		"deploy.sh:4: [error]",
		"deploy.sh:7: [error]",
		"deploy.sh:14: [error]",
	}
	for _, want := range wantFindings {
		if !strings.Contains(section, want) {
			t.Fatalf("expected finding %q in the security section, got:\n%s", want, section)
		}
	}
	if got := strings.Count(section, "[error]"); got != len(wantFindings) {
		t.Fatalf("expected exactly %d findings, got %d (false positive?):\n%s", len(wantFindings), got, section)
	}
	for _, citation := range []string{
		"rule security/command-injection",
		"rule security/sql-injection",
	} {
		if !strings.Contains(section, citation) {
			t.Fatalf("expected citation %q, got:\n%s", citation, section)
		}
	}
	if got := strings.Count(section, "rule security/command-injection"); got != 5 {
		t.Fatalf("expected exactly 5 command-injection citations, got %d:\n%s", got, section)
	}
	if got := strings.Count(section, "rule security/sql-injection"); got != 1 {
		t.Fatalf("expected exactly 1 sql-injection citation, got %d:\n%s", got, section)
	}

	// Determinism.
	again, _, code := aur486Run(t, binPath, gitDir, "review", "--base", "HEAD~1", "--seguranca")
	if code != 0 || out != again {
		t.Fatalf("review --seguranca is not deterministic (exit %d):\nfirst=%q\nsecond=%q", code, out, again)
	}

	// AC-003 regression: the project's already-committed Python
	// SQL-injection fixture must still produce exactly the finding it
	// produced before this card touched the SQL rule.
	pyRepo := filepath.Join(root, "tests", "fixtures", "review", "vuln", "repo.git")
	if _, err := os.Stat(pyRepo); err != nil {
		t.Fatalf("required input missing: %s: %v", pyRepo, err)
	}
	pyOut, _, code := aur486Run(t, binPath, pyRepo, "review", "--base", "HEAD~1", "--seguranca")
	if code != 0 {
		t.Fatalf("review --seguranca on the Python vuln fixture must exit 0, got %d", code)
	}
	pySection := aur486Section(pyOut)
	if !strings.Contains(pySection, "src/db.py:8: [error]") || !strings.Contains(pySection, "rule security/sql-injection") {
		t.Fatalf("expected the pre-existing Python sql-injection finding to survive unchanged, got:\n%s", pyOut)
	}
	if strings.Count(pySection, "[error]") != 1 {
		t.Fatalf("expected exactly one security finding on the Python vuln fixture, got:\n%s", pyOut)
	}

	// AC-003 regression: the Node command-injection/xss fixture (AUR-462)
	// must still produce exactly its counts.
	node462Repo := filepath.Join(root, "tests", "fixtures", "review", "vuln", "node-xss-command-injection", "repo.git")
	node462Out, _, code := aur486Run(t, binPath, node462Repo, "review", "--base", "HEAD~1", "--seguranca")
	if code != 0 {
		t.Fatalf("review --seguranca on the node fixture must exit 0, got %d", code)
	}
	node462Section := aur486Section(node462Out)
	if got := strings.Count(node462Section, "rule security/command-injection"); got != 3 {
		t.Fatalf("expected exactly 3 command-injection citations to survive unchanged, got %d:\n%s", got, node462Out)
	}
	if got := strings.Count(node462Section, "rule security/xss"); got != 1 {
		t.Fatalf("expected exactly 1 xss citation to survive unchanged, got %d:\n%s", got, node462Out)
	}

	// AC-003 regression: the AUR-481 Rust fixture must still produce
	// exactly its six findings.
	rustRepo := filepath.Join(root, "tests", "fixtures", "review", "vuln", "rust-secret-sql-injection", "repo.git")
	rustOut, _, code := aur486Run(t, binPath, rustRepo, "review", "--base", "HEAD~1", "--seguranca")
	if code != 0 {
		t.Fatalf("review --seguranca on the Rust fixture must exit 0, got %d", code)
	}
	if got := strings.Count(aur486Section(rustOut), "[error]"); got != 6 {
		t.Fatalf("expected exactly 6 Rust findings to survive unchanged, got %d:\n%s", got, rustOut)
	}
}
