package main

// The CLI's own seam: flag handling, the verdict on stdout, and the exit-code
// mapping the spec publishes. The full process boundary (raw exits, canary,
// determinism) is tests/e2e/AUR-429.sh's job.

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/qa/browserproof"
)

func writeDocs(t *testing.T, pages map[string]string) string {
	t.Helper()
	docs := t.TempDir()
	for rel, content := range pages {
		path := filepath.Join(docs, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("fixture: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("fixture: %v", err)
		}
	}
	return docs
}

func goodDocs(t *testing.T) string {
	t.Helper()
	return writeDocs(t, map[string]string{
		"index.md": "---\ntitle: Home\npermalink: /\n---\n\n## Generated API documentation\n\n- [Guia](/guia/)\n",
		"guia.md":  "---\ntitle: Guia\npermalink: /guia/\n---\n\n# Guia\n\nO guia documenta func NewGreeting.\n",
	})
}

func runCLI(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := run(args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func TestRunProvesAPublishedTreeAndPrintsTheVerdict(t *testing.T) {
	code, stdout, stderr := runCLI(t,
		"--url", "https://usuario.github.io/projeto",
		"--docs", goodDocs(t),
		"--content", "func NewGreeting")

	if code != exitProved {
		t.Fatalf("expected exit %d, got %d\nstdout: %s\nstderr: %s", exitProved, code, stdout, stderr)
	}
	result, err := browserproof.ParseDocsVerifyResultV1([]byte(strings.TrimSpace(stdout)))
	if err != nil {
		t.Fatalf("stdout is not a valid docs verdict: %v\n%s", err, stdout)
	}
	if !result.Proved || result.FollowedRoute != "/guia" ||
		result.PublishedURL != "https://usuario.github.io/projeto" {
		t.Fatalf("verdict does not record the navigation: %+v", result)
	}
	if !strings.Contains(stderr, "scripted driver") {
		t.Fatalf("the run must say what navigated, stderr: %s", stderr)
	}
}

func TestRunRefusesATreeWithoutTheContent(t *testing.T) {
	docs := writeDocs(t, map[string]string{
		"index.md": "---\ntitle: Home\npermalink: /\n---\n\n## Generated API documentation\n\n- [Guia](/guia/)\n",
		"guia.md":  "---\ntitle: Guia\npermalink: /guia/\n---\n\n# Guia\n\nSem o simbolo prometido.\n",
	})

	code, stdout, _ := runCLI(t,
		"--url", "https://usuario.github.io/projeto",
		"--docs", docs,
		"--content", "func NewGreeting")

	if code != exitRefused {
		// The exact behavior MUT-001 deletes: a page without the expected
		// content must exit non-zero.
		t.Fatalf("AUR-429/AC-001/MUT-001: expected exit %d, got %d\n%s", exitRefused, code, stdout)
	}
	if !strings.Contains(stdout, `"proved":false`) ||
		!strings.Contains(stdout, browserproof.CodeTextMismatch) {
		t.Fatalf("refusal verdict does not say why: %s", stdout)
	}
}

func TestRunUsageErrorsExit64(t *testing.T) {
	cases := map[string][]string{
		"no flags":        {},
		"missing url":     {"--docs", "x", "--content", "y"},
		"missing docs":    {"--url", "https://u.github.io/p", "--content", "y"},
		"missing content": {"--url", "https://u.github.io/p", "--docs", "x"},
		"unknown flag":    {"--nope"},
		"stray argument":  {"--url", "https://u.github.io/p", "--docs", "x", "--content", "y", "stray"},
	}
	for name, args := range cases {
		args := args
		t.Run(name, func(t *testing.T) {
			if code, _, _ := runCLI(t, args...); code != exitUsage {
				t.Fatalf("expected exit %d, got %d", exitUsage, code)
			}
		})
	}
}

func TestRunDocsDirThatDoesNotExistIsUsage(t *testing.T) {
	code, _, _ := runCLI(t,
		"--url", "https://usuario.github.io/projeto",
		"--docs", filepath.Join(t.TempDir(), "nao-existe"),
		"--content", "x")
	if code != exitUsage {
		t.Fatalf("expected exit %d, got %d", exitUsage, code)
	}
}

func TestRunBadPublishedURLIsInconclusive(t *testing.T) {
	code, stdout, _ := runCLI(t,
		"--url", "ftp://usuario/projeto",
		"--docs", goodDocs(t),
		"--content", "func NewGreeting")
	if code != exitInconclusive {
		t.Fatalf("expected exit %d, got %d\n%s", exitInconclusive, code, stdout)
	}
	if !strings.Contains(stdout, browserproof.CodeRequestInvalid) {
		t.Fatalf("inconclusive verdict does not say why: %s", stdout)
	}
}

func TestRunUnusableConfiguredDriverIsInconclusive(t *testing.T) {
	t.Setenv(browserproof.DriverPathEnv, filepath.Join(t.TempDir(), "nao-existe"))
	code, _, stderr := runCLI(t,
		"--url", "https://usuario.github.io/projeto",
		"--docs", goodDocs(t),
		"--content", "func NewGreeting")
	if code != exitInconclusive {
		t.Fatalf("expected exit %d, got %d\n%s", exitInconclusive, code, stderr)
	}
}
