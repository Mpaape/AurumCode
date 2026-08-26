package review

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/llm"
)

type testProvider struct {
	prompt string
}

func (p *testProvider) Complete(prompt string, _ llm.Options) (llm.Response, error) {
	p.prompt = prompt
	return llm.Response{Text: "# Revisão da documentação\n\n## Veredito\n\naprovada com melhorias"}, nil
}

func (p *testProvider) Tokens(input string) (int, error) { return len(input), nil }
func (p *testProvider) Name() string                     { return "test" }

func TestGenerateUsesRepositoryPromptAndDocumentationSkills(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("# Ledger\n\nUse `ledger run`."), 0o644); err != nil {
		t.Fatal(err)
	}
	siteRoot := filepath.Join(root, "site")
	if err := os.MkdirAll(filepath.Join(root, ".aurumcode", "prompts", "documentation"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".aurumcode", "skills", "documentation"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".aurumcode", "prompts", "documentation", "docs-review.md"), []byte("Review {{README_CONTENT}} {{SITE_CONTENT}} {{SKILLS_CONTENT}}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".aurumcode", "skills", "documentation", "quality.md"), []byte("Prefer executable examples."), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(siteRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(siteRoot, "guide.md"), []byte("# Guide\n\nRun `ledger run`."), 0o644); err != nil {
		t.Fatal(err)
	}

	provider := &testProvider{}
	reviewer := NewReviewer(llm.NewOrchestrator(provider, nil, nil))
	out := filepath.Join(siteRoot, "reviews", "docs-review.md")
	content, err := reviewer.Generate(context.Background(), GenerateOptions{
		ProjectDir: root,
		SiteDir:    siteRoot,
		OutputPath: out,
		Language:   "pt-BR",
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if !strings.Contains(provider.prompt, "Prefer executable examples.") {
		t.Fatalf("skill was not included in prompt: %q", provider.prompt)
	}
	if !strings.Contains(provider.prompt, "# Ledger") || !strings.Contains(provider.prompt, "# Guide") {
		t.Fatalf("repository evidence was not included: %q", provider.prompt)
	}
	if !strings.Contains(content, "permalink: /reviews/docs-review/") {
		t.Fatalf("report has no stable route: %s", content)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("report was not written: %v", err)
	}
}

func TestGenerateWithoutProviderIsOptionalError(t *testing.T) {
	_, err := NewReviewer(nil).Generate(context.Background(), GenerateOptions{})
	if !errors.Is(err, ErrNoLLMProvider) {
		t.Fatalf("error = %v, want ErrNoLLMProvider", err)
	}
}

func TestRenderReportEscapesLiquidAndOwnsFrontMatter(t *testing.T) {
	got := renderReport("---\ntitle: forged\n---\ntexto {{ perigoso }}", "pt-BR")
	if !strings.Contains(got, "title: Revisão da documentação") {
		t.Fatalf("report front matter was not controlled: %s", got)
	}
	if strings.Contains(got, "{{") || strings.Contains(got, "}}") {
		t.Fatalf("Liquid delimiters leaked into report: %s", got)
	}
}
