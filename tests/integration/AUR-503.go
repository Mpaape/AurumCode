// IntegrationAUR503 builds the real aurumcode binary and proves AUR-503's
// outcome at the CLI boundary, distinct from tests/unit/AUR-503.go's
// package-boundary proof over synthetic types.Diff values.
//
// AUR-503 keeps the four command-injection branches AUR-486 added (Go
// `exec.Command`, C# `Process.Start`, PowerShell `Invoke-Expression`/`iex`,
// bash `eval`) unanchored and filters a match whose first byte sits in a
// comment or string literal (codeMask), so a textual mention produces no
// finding while the real invocation -- including one embedded in an
// expression -- still does. `--seguranca` alone (no LLM provider) runs the
// deterministic pass;
// this program asserts the security section contains exactly the real
// defects and none of the mentions.
//
// The sealed acceptance profile (bootstrap-readonly-v1) carries bash and a
// Go toolchain but no `git` binary, and this card's `paths` do not include
// tests/fixtures, so there is no committed fixture to read. Instead this
// program writes a small, valid bare Git repository directly into a temp
// directory -- loose objects (zlib + the standard "type size\0content"
// header), a tree, two commits and a refs/heads/main ref -- the exact
// format tests/fixtures/repos/git-demo/build-fixture.sh emits, and the same
// one internal/analyzer's pure-Go reader inflates when no `git` binary is
// present. Nothing here shells out to `git`.
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

func aur503Root(t *testing.T) string {
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

func aur503FilteredEnv() []string {
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

func aur503Run(t *testing.T, bin, dir string, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	cmd.Env = aur503FilteredEnv()
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

const aur503SecurityHeader = "Security findings (standards/security-review):"

func aur503Section(out string) string {
	_, section, ok := strings.Cut(out, aur503SecurityHeader)
	if !ok {
		return ""
	}
	return section
}

func aur503WriteObject(t *testing.T, gitDir, typ string, content []byte) string {
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

func aur503Tree(t *testing.T, gitDir string, files map[string]string) string {
	t.Helper()
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	var buf bytes.Buffer
	for _, name := range names {
		blobID := aur503WriteObject(t, gitDir, "blob", []byte(files[name]))
		raw, err := hex.DecodeString(blobID)
		if err != nil {
			t.Fatalf("decode blob id: %v", err)
		}
		buf.WriteString("100644 " + name + "\x00")
		buf.Write(raw)
	}
	return aur503WriteObject(t, gitDir, "tree", buf.Bytes())
}

func aur503Commit(t *testing.T, gitDir, tree, parent, msg string, epoch int64) string {
	t.Helper()
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "tree %s\n", tree)
	if parent != "" {
		fmt.Fprintf(&buf, "parent %s\n", parent)
	}
	fmt.Fprintf(&buf, "author Aurum Test <test@aurum.invalid> %d +0000\n", epoch)
	fmt.Fprintf(&buf, "committer Aurum Test <test@aurum.invalid> %d +0000\n", epoch)
	fmt.Fprintf(&buf, "\n%s\n", msg)
	return aur503WriteObject(t, gitDir, "commit", buf.Bytes())
}

// aur503FixtureFiles returns the added-file content of the two-commit
// fixture. It MUST stay byte-identical to tests/unit/AUR-503.go's
// aur503FixtureLines and tests/acceptance/AUR-503.sh's heredoc.
func aur503FixtureFiles() map[string]string {
	return map[string]string{
		"main.go":    "package main\n\nimport \"os/exec\"\n\nfunc pingUnsafe(host string) {\n\texec.Command(\"sh\", \"-c\", \"ping \"+host).Run()\n}\n\nfunc pingSafe(host string) {\n\texec.Command(\"ping\", host).Run()\n}\n\n// exec.Command(\"sh\", \"-c\", \"ping \"+host) in a comment must not fire\n",
		"App.cs":     "using System.Diagnostics;\n\nclass App {\n    static void Ping(string host) {\n        Process.Start(\"cmd.exe\", \"/c ping \" + host);\n        Process.Start(\"ping\", host);\n        // Process.Start(\"cmd.exe\", \"/c ping \" + host) must not fire\n    }\n}\n",
		"script.ps1": "function Invoke-Ping($host) {\n    Invoke-Expression \"ping $host\"\n}\n# Invoke-Expression \"ping $host\" in a comment must not fire\niex $payload\nWrite-Host \"iex is a string literal, not a call\"\n",
		"deploy.sh":  "#!/bin/bash\nset -euo pipefail\nping_unsafe() {\n    eval \"ping $1\"\n}\nping_shell_unsafe() {\n    sh -c \"ping $1\"\n}\n# eval \"ping $1\" in a comment must not fire\nmsg=\"eval ping \\$1 in a variable\"\neval := compute()\nquery=\"SELECT * FROM users WHERE name = '$1'\"\n",
	}
}

func aur503WriteRepo(t *testing.T, gitDir string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(gitDir, "refs", "heads"), 0o755); err != nil {
		t.Fatalf("mkdir refs: %v", err)
	}
	seed := aur503Tree(t, gitDir, map[string]string{"README.md": "AUR-503 ephemeral fixture. Nothing here is a real application.\n"})
	c1 := aur503Commit(t, gitDir, seed, "", "seed: add the fixture skeleton", 1700000000)
	head := aur503Tree(t, gitDir, aur503FixtureFiles())
	c2 := aur503Commit(t, gitDir, head, c1, "chore: plant mentions and real invocations", 1700000060)
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

func IntegrationAUR503(t *testing.T) {
	root := aur503Root(t)

	binPath := filepath.Join(t.TempDir(), "aurumcode-aur503")
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
	aur503WriteRepo(t, gitDir)

	// Without --seguranca: no security section leaks in.
	base, _, _ := aur503Run(t, binPath, gitDir, "review", "--base", "HEAD~1")
	if strings.Contains(base, aur503SecurityHeader) {
		t.Fatalf("without --seguranca no security section may exist, got:\n%s", base)
	}

	out, stderr, code := aur503Run(t, binPath, gitDir, "review", "--base", "HEAD~1", "--seguranca")
	if code != 0 {
		t.Fatalf("review --seguranca must exit 0 with no provider configured, got %d\nstderr=%s", code, stderr)
	}
	if !strings.Contains(out, aur503SecurityHeader) {
		t.Fatalf("expected the security section header, got:\n%s", out)
	}
	section := aur503Section(out)

	// AC-002: the six real command-injection invocations and the SQL-by-shell
	// line are found.
	wantFindings := []string{
		"App.cs:5:",
		"deploy.sh:4:",
		"deploy.sh:7:",
		"deploy.sh:12:",
		"main.go:6:",
		"script.ps1:2:",
		"script.ps1:5:",
	}
	for _, want := range wantFindings {
		if !strings.Contains(section, want) {
			t.Fatalf("expected finding %q in the security section, got:\n%s", want, section)
		}
	}
	if got := strings.Count(section, "[error]"); got != len(wantFindings) {
		t.Fatalf("expected exactly %d findings, got %d (a mention fired?):\n%s", len(wantFindings), got, section)
	}
	if got := strings.Count(section, "rule security/command-injection"); got != 6 {
		t.Fatalf("expected exactly 6 command-injection citations, got %d:\n%s", got, section)
	}
	if got := strings.Count(section, "rule security/sql-injection"); got != 1 {
		t.Fatalf("expected exactly 1 sql-injection citation, got %d:\n%s", got, section)
	}

	// AC-001/AC-003: no finding may land on a comment/string/declaration line.
	for _, forbidden := range []string{
		"main.go:13:",
		"App.cs:7:",
		"script.ps1:4:",
		"script.ps1:6:",
		"deploy.sh:9:",
		"deploy.sh:10:",
		"deploy.sh:11:",
	} {
		if strings.Contains(section, forbidden) {
			t.Fatalf("a mention/declaration line %q must not produce a finding, got:\n%s", forbidden, section)
		}
	}

	// Determinism.
	again, _, code := aur503Run(t, binPath, gitDir, "review", "--base", "HEAD~1", "--seguranca")
	if code != 0 || out != again {
		t.Fatalf("review --seguranca is not deterministic (exit %d):\nfirst=%q\nsecond=%q", code, out, again)
	}

	// Regression: the Node (AUR-462) command-injection fixture still produces
	// exactly its three citations, and the Rust (AUR-481) fixture its finding
	// count, so the unanchored branches plus the codeMask filter did not
	// weaken the earlier coverage.
	nodeRepo := filepath.Join(root, "tests", "fixtures", "review", "vuln", "node-xss-command-injection", "repo.git")
	if _, err := os.Stat(nodeRepo); err != nil {
		t.Fatalf("required input missing: %s: %v", nodeRepo, err)
	}
	nodeOut, _, code := aur503Run(t, binPath, nodeRepo, "review", "--base", "HEAD~1", "--seguranca")
	if code != 0 {
		t.Fatalf("review --seguranca on the node fixture must exit 0, got %d", code)
	}
	if got := strings.Count(aur503Section(nodeOut), "rule security/command-injection"); got != 3 {
		t.Fatalf("expected exactly 3 node command-injection citations, got %d:\n%s", got, nodeOut)
	}

	rustRepo := filepath.Join(root, "tests", "fixtures", "review", "vuln", "rust-secret-sql-injection", "repo.git")
	if _, err := os.Stat(rustRepo); err != nil {
		t.Fatalf("required input missing: %s: %v", rustRepo, err)
	}
	rustOut, _, code := aur503Run(t, binPath, rustRepo, "review", "--base", "HEAD~1", "--seguranca")
	if code != 0 {
		t.Fatalf("review --seguranca on the Rust fixture must exit 0, got %d", code)
	}
	if got := strings.Count(aur503Section(rustOut), "rule security/command-injection"); got != 2 {
		t.Fatalf("expected exactly 2 Rust command-injection citations, got %d:\n%s", got, rustOut)
	}
}
