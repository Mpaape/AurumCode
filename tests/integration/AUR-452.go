package integration

// Integration program for card AUR-452, selector IntegrationAUR452.
//
// tests/unit/AUR-452.go proves internal/config's functions in isolation
// with in-memory values. This program proves the same seam against REAL
// files on disk under a temp repository root: Load parsing an actual
// .aurumcode/config.yml, RepoPromptProvider reading an actual
// .aurumcode/prompt.md, and PathInstructionsProvider matching an actual
// .aurumcode/instructions/*.md file's `applyTo` front matter against
// real changed paths -- the exact directory layout AUR-468/469/470/471
// build their own providers next to.
import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/config"
)


func IntegrationAUR452(t *testing.T) {
	t.Run("ZeroConfigDefaultProvidersContributeNothing", testAUR452ZeroConfigDefaultProviders)
	t.Run("RepoPromptProviderReadsRealFile", testAUR452RepoPromptProviderReadsRealFile)
	t.Run("PathInstructionsScopedByApplyTo", testAUR452PathInstructionsScopedByApplyTo)
	t.Run("LoadParsesRealYAMLConfig", testAUR452LoadParsesRealYAMLConfig)
}

func testAUR452ZeroConfigDefaultProviders(t *testing.T) {
	root := t.TempDir()
	providers := config.DefaultProviders(root)
	if len(providers) != 2 {
		t.Fatalf("Camada 1 ships exactly two file providers, got %d", len(providers))
	}
	block, err := config.BuildContextBlock(context.Background(), providers, []string{"a.go", "docs/readme.md"}, nil)
	if err != nil {
		t.Fatalf("BuildContextBlock over an empty repo must not error: %v", err)
	}
	if block != "" {
		t.Fatalf("zero-config providers must contribute nothing, got %q", block)
	}
}

func testAUR452RepoPromptProviderReadsRealFile(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, ".aurumcode", "prompt.md"),
		"Prefer table-driven tests in this repository.\n")

	p := config.NewRepoPromptProvider(root)
	got, err := p.Provide(context.Background(), []string{"any/path.go"})
	if err != nil {
		t.Fatalf("Provide must not error: %v", err)
	}
	if !strings.Contains(got, "table-driven tests") {
		t.Fatalf("RepoPromptProvider must return the file's content, got %q", got)
	}
}

func testAUR452PathInstructionsScopedByApplyTo(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, ".aurumcode", "instructions", "go-style.md"),
		"---\napplyTo: \"**/*.go\"\n---\nUse early returns; avoid deep nesting.\n")
	mustWriteFile(t, filepath.Join(root, ".aurumcode", "instructions", "docs-style.md"),
		"---\napplyTo: \"docs/**\"\n---\nWrite documentation in plain language.\n")

	p := config.NewPathInstructionsProvider(root)

	// A changed Go file matches only the Go-scoped instructions.
	got, err := p.Provide(context.Background(), []string{"internal/foo/bar.go"})
	if err != nil {
		t.Fatalf("Provide must not error: %v", err)
	}
	if !strings.Contains(got, "early returns") {
		t.Fatalf("a *.go change must pull in the applyTo:**/*.go instructions, got %q", got)
	}
	if strings.Contains(got, "plain language") {
		t.Fatalf("a *.go change must NOT pull in the docs/** instructions, got %q", got)
	}

	// A changed doc file matches only the docs-scoped instructions.
	got2, err := p.Provide(context.Background(), []string{"docs/spec.md"})
	if err != nil {
		t.Fatalf("Provide must not error: %v", err)
	}
	if !strings.Contains(got2, "plain language") || strings.Contains(got2, "early returns") {
		t.Fatalf("a docs/** change must pull in only the docs-scoped instructions, got %q", got2)
	}

	// A path matching neither glob contributes nothing.
	got3, err := p.Provide(context.Background(), []string{"README.md"})
	if err != nil {
		t.Fatalf("Provide must not error: %v", err)
	}
	if got3 != "" {
		t.Fatalf("an unmatched path must contribute nothing, got %q", got3)
	}
}

func testAUR452LoadParsesRealYAMLConfig(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, ".aurumcode", "config.yml"), strings.TrimSpace(`
rules:
  security/hardcoded-secret:
    enabled: false
  quality/dead-code:
    severity: error
ignore:
  - "vendor/**"
`)+"\n")

	cfg, err := config.Load(root)
	if err != nil {
		t.Fatalf("Load must parse a real, valid config.yml: %v", err)
	}
	rc, ok := cfg.Rules["security/hardcoded-secret"]
	if !ok || rc.Enabled == nil || *rc.Enabled {
		t.Fatalf("security/hardcoded-secret must be loaded as explicitly disabled, got %+v", rc)
	}
	rc2, ok := cfg.Rules["quality/dead-code"]
	if !ok || rc2.Severity != "error" {
		t.Fatalf("quality/dead-code must load its severity override, got %+v", rc2)
	}
	if len(cfg.Ignore) != 1 || cfg.Ignore[0] != "vendor/**" {
		t.Fatalf("ignore list must load exactly the one glob, got %+v", cfg.Ignore)
	}
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
