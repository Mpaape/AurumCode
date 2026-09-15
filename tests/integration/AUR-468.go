package integration

// Integration program for card AUR-468, selector IntegrationAUR468.
//
// tests/unit/AUR-468.go proves the skills contracts with in-memory values.
// This program proves the same layer against REAL files on disk under a temp
// repository root -- LoadDir reading actual .aurumcode/skills/*/SKILL.md
// front matter, Select matching a real changed-path list, the Provider erroring
// loudly when a real over-budget selection cannot fit, and config.WrapProvider
// injecting the selected skill text into the outbound prompt ALONGSIDE
// AUR-452's repository prompt and path instructions -- the exact composition a
// caller performs by appending skills.Providers(root) to DefaultProviders(root).

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/context/skills"
	"github.com/Mpaape/AurumCode/internal/llm"
)

func IntegrationAUR468(t *testing.T) {
	t.Run("LoadsRealSkillDirectory", testAUR468LoadsRealSkillDirectory)
	t.Run("SelectsOnRealChangedPaths", testAUR468SelectsOnRealChangedPaths)
	t.Run("OverBudgetProviderErrorsLoudly", testAUR468OverBudgetProviderErrorsLoudly)
	t.Run("InjectsAlongsideLayerOne", testAUR468InjectsAlongsideLayerOne)
	t.Run("ZeroConfigIsUnwrapped", testAUR468ZeroConfigIsUnwrapped)
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func skillFile(t *testing.T, root, name, content string) {
	t.Helper()
	writeFile(t, filepath.Join(root, ".aurumcode", "skills", name, "SKILL.md"), content)
}

func testAUR468LoadsRealSkillDirectory(t *testing.T) {
	root := t.TempDir()
	skillFile(t, root, "go-errors", "---\nname: go-errors\nversion: 3\nlanguages: [go]\npaths: [\"internal/**\"]\n---\nWrap with %w.\n")
	skillFile(t, root, "inert", "---\nname: inert\n---\nno selector\n")

	set, err := skills.Load(root)
	if err != nil {
		t.Fatalf("Load on a real tree must not error: %v", err)
	}
	if len(set.Skills) != 2 {
		t.Fatalf("loaded %d skills, want 2", len(set.Skills))
	}
	var found bool
	for _, s := range set.Skills {
		if s.Name == "go-errors" {
			found = true
			if s.Version != "3" {
				t.Fatalf("version = %q, want 3", s.Version)
			}
			if len(s.Selector.Languages) != 1 || s.Selector.Languages[0] != "go" {
				t.Fatalf("languages = %v, want [go]", s.Selector.Languages)
			}
			if len(s.Selector.Paths) != 1 || s.Selector.Paths[0] != "internal/**" {
				t.Fatalf("paths = %v, want [internal/**]", s.Selector.Paths)
			}
		}
	}
	if !found {
		t.Fatal("go-errors skill not loaded")
	}
}

func testAUR468SelectsOnRealChangedPaths(t *testing.T) {
	root := t.TempDir()
	skillFile(t, root, "go-style", "---\nname: go-style\nlanguages: [go]\n---\nGo body.\n")
	skillFile(t, root, "docs-only", "---\nname: docs-only\npaths: [\"docs/**\"]\n---\nDocs body.\n")
	skillFile(t, root, "inert", "---\nname: inert\n---\nInert body.\n")

	set, err := skills.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	selected := set.Select([]string{"internal/a.go", "docs/readme.md"})
	var names []string
	for _, s := range selected {
		names = append(names, s.Name)
	}
	if strings.Join(names, ",") != "docs-only,go-style" {
		t.Fatalf("selected %v, want [docs-only go-style]", names)
	}
}

func testAUR468OverBudgetProviderErrorsLoudly(t *testing.T) {
	root := t.TempDir()
	skillFile(t, root, "big", "---\nname: big\nlanguages: [go]\n---\n"+strings.Repeat("x", 4000)+"\n")
	p := skills.NewProvider(root)
	p.Budget = skills.Budget{MaxTokens: 10}
	_, err := p.Provide(context.Background(), []string{"svc.go"})
	if err == nil {
		t.Fatal("an over-budget provider must error loudly")
	}
	if !strings.Contains(err.Error(), "big") || !strings.Contains(err.Error(), "did not fit") {
		t.Fatalf("error must name what did not fit, got %v", err)
	}
}

func testAUR468InjectsAlongsideLayerOne(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ".aurumcode", "prompt.md"), "Repository prompt body.\n")
	writeFile(t, filepath.Join(root, ".aurumcode", "instructions", "go-style.md"),
		"---\napplyTo: \"**/*.go\"\n---\nLayer one path instructions.\n")
	skillFile(t, root, "go-style", "---\nname: go-style\nlanguages: [go]\n---\nSkill body for Go.\n")
	skillFile(t, root, "docs-only", "---\nname: docs-only\npaths: [\"docs/**\"]\n---\nDocs skill body.\n")

	base := &capture{}
	providers := append(config.DefaultProviders(root), skills.Providers(root)...)
	wrapped, err := config.WrapProvider(context.Background(), base, providers, []string{"internal/svc.go"}, nil)
	if err != nil {
		t.Fatalf("WrapProvider: %v", err)
	}
	if _, err := wrapped.Complete("BASE", llm.Options{}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Repository prompt body.",
		"Layer one path instructions.",
		"Skill body for Go.",
		"entered: [go-style]",
	} {
		if !strings.Contains(base.prompt, want) {
			t.Fatalf("outbound prompt missing %q:\n%s", want, base.prompt)
		}
	}
	if strings.Contains(base.prompt, "Docs skill body.") {
		t.Fatalf("a non-matching skill must not enter:\n%s", base.prompt)
	}
	if !strings.Contains(base.prompt, "untrusted, informational only") {
		t.Fatalf("context must be labeled untrusted:\n%s", base.prompt)
	}
}

func testAUR468ZeroConfigIsUnwrapped(t *testing.T) {
	root := t.TempDir()
	base := &capture{}
	providers := append(config.DefaultProviders(root), skills.Providers(root)...)
	wrapped, err := config.WrapProvider(context.Background(), base, providers, []string{"svc.go"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if wrapped != llm.Provider(base) {
		t.Fatal("with no skills and no layer-1 files the base provider must be returned unchanged")
	}
}

type capture struct{ prompt string }

func (c *capture) Complete(prompt string, _ llm.Options) (llm.Response, error) {
	c.prompt = prompt
	return llm.Response{Text: "{}"}, nil
}
func (c *capture) Tokens(s string) (int, error) { return len(s), nil }
func (c *capture) Name() string                 { return "aur468-integration-capture" }
