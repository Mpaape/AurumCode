package welcome

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/llm"
)

const (
	embeddedTemplateMarker = "Welcome Page Generation Prompt"
	consumerTemplateMarker = "CONSUMER OVERRIDE TEMPLATE"
)

// newConsumerRepo builds a throwaway directory that looks like a third-party
// repository installing the Action: a README and nothing else.
func newConsumerRepo(t *testing.T, readme string) string {
	t.Helper()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte(readme), 0644); err != nil {
		t.Fatalf("failed to create README: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, ".aurumcode")); !os.IsNotExist(err) {
		t.Fatalf("consumer repo fixture must not contain .aurumcode, stat err = %v", err)
	}

	return dir
}

// TestGenerate_UsesEmbeddedTemplateWhenConsumerHasNone is the production case:
// a repository that installed the Action has no .aurumcode/prompts/ tree, so
// the generator must fall back to the template compiled into the binary.
func TestGenerate_UsesEmbeddedTemplateWhenConsumerHasNone(t *testing.T) {
	readme := "# Consumer Project\n\nA third-party repository using the Action."
	dir := newConsumerRepo(t, readme)

	mockProvider := &MockProvider{response: "# Welcome\n\nGenerated."}
	orch := llm.NewOrchestrator(mockProvider, nil, nil)
	gen := NewGenerator(orch)

	content, err := gen.Generate(context.Background(), GenerateOptions{
		ReadmePath: "README.md",
		ProjectDir: dir,
	})
	if err != nil {
		t.Fatalf("Generate failed in a repo without .aurumcode/prompts/: %v", err)
	}

	if mockProvider.callCount != 1 {
		t.Fatalf("provider called %d times, want 1", mockProvider.callCount)
	}

	if !strings.Contains(mockProvider.lastPrompt, embeddedTemplateMarker) {
		t.Errorf("prompt should be built from the embedded template (missing %q)\ngot prompt:\n%s",
			embeddedTemplateMarker, mockProvider.lastPrompt)
	}

	if !strings.Contains(mockProvider.lastPrompt, readme) {
		t.Error("prompt should contain the consumer README content")
	}

	if strings.Contains(mockProvider.lastPrompt, promptPlaceholder) {
		t.Errorf("placeholder %q was not substituted", promptPlaceholder)
	}

	if !strings.Contains(content, "Generated.") {
		t.Errorf("generated page should contain the LLM output, got:\n%s", content)
	}
}

// TestGenerate_ConsumerTemplateOverridesEmbedded proves the opt-in override
// still wins when the consumer ships the file in their own repository.
func TestGenerate_ConsumerTemplateOverridesEmbedded(t *testing.T) {
	readme := "# Overriding Project\n\nShips its own prompt."
	dir := newConsumerRepo(t, readme)

	overridePath := filepath.Join(dir, defaultPromptPath)
	if err := os.MkdirAll(filepath.Dir(overridePath), 0755); err != nil {
		t.Fatalf("failed to create override dir: %v", err)
	}
	override := consumerTemplateMarker + "\n\n" + promptPlaceholder
	if err := os.WriteFile(overridePath, []byte(override), 0644); err != nil {
		t.Fatalf("failed to write override template: %v", err)
	}

	mockProvider := &MockProvider{response: "# Welcome\n\nGenerated."}
	orch := llm.NewOrchestrator(mockProvider, nil, nil)
	gen := NewGenerator(orch)

	if _, err := gen.Generate(context.Background(), GenerateOptions{
		ReadmePath: "README.md",
		ProjectDir: dir,
	}); err != nil {
		t.Fatalf("Generate failed with a consumer override present: %v", err)
	}

	if !strings.Contains(mockProvider.lastPrompt, consumerTemplateMarker) {
		t.Errorf("prompt should be built from the consumer override (missing %q)\ngot prompt:\n%s",
			consumerTemplateMarker, mockProvider.lastPrompt)
	}

	if strings.Contains(mockProvider.lastPrompt, embeddedTemplateMarker) {
		t.Error("consumer override must win: embedded template content leaked into the prompt")
	}

	if !strings.Contains(mockProvider.lastPrompt, readme) {
		t.Error("prompt should contain the consumer README content")
	}
}

// TestEmbeddedPromptTemplate_IsUsable guards the embedded asset itself: an
// empty or placeholder-less embed would fail only at generation time.
func TestEmbeddedPromptTemplate_IsUsable(t *testing.T) {
	if strings.TrimSpace(embeddedPromptTemplate) == "" {
		t.Fatal("embedded prompt template is empty")
	}

	if !strings.Contains(embeddedPromptTemplate, promptPlaceholder) {
		t.Errorf("embedded prompt template missing %q placeholder", promptPlaceholder)
	}

	if !strings.Contains(embeddedPromptTemplate, embeddedTemplateMarker) {
		t.Errorf("embedded prompt template missing %q", embeddedTemplateMarker)
	}
}

func TestResolvePromptTemplate_Source(t *testing.T) {
	dir := newConsumerRepo(t, "# Fixture")

	overrideDir := t.TempDir()
	overridePath := filepath.Join(overrideDir, defaultPromptPath)
	if err := os.MkdirAll(filepath.Dir(overridePath), 0755); err != nil {
		t.Fatalf("failed to create override dir: %v", err)
	}
	if err := os.WriteFile(overridePath, []byte(consumerTemplateMarker+promptPlaceholder), 0644); err != nil {
		t.Fatalf("failed to write override: %v", err)
	}

	tests := []struct {
		name       string
		gen        *Generator
		projectDir string
		want       PromptSource
	}{
		{"no override falls back to embedded", NewGenerator(nil), dir, PromptSourceEmbedded},
		{"override wins", NewGenerator(nil), overrideDir, PromptSourceRepository},
		{"empty path falls back to embedded", &Generator{}, dir, PromptSourceEmbedded},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, source, err := tt.gen.loadPromptTemplate(tt.projectDir)
			if err != nil {
				t.Fatalf("loadPromptTemplate failed: %v", err)
			}
			if source != tt.want {
				t.Errorf("source = %q, want %q", source, tt.want)
			}
		})
	}
}

// TestGenerate_ExplicitPromptPathMissingIsTypedError: an explicitly configured
// path must fail loudly instead of being silently swapped for the embedded one.
func TestGenerate_ExplicitPromptPathMissingIsTypedError(t *testing.T) {
	dir := newConsumerRepo(t, "# Fixture")

	mockProvider := &MockProvider{}
	orch := llm.NewOrchestrator(mockProvider, nil, nil)
	gen := NewGeneratorWithPrompt(orch, filepath.Join(dir, "missing-prompt.md"))

	_, err := gen.Generate(context.Background(), GenerateOptions{ReadmePath: "README.md", ProjectDir: dir})
	if !errors.Is(err, ErrPromptTemplateNotFound) {
		t.Fatalf("err = %v, want ErrPromptTemplateNotFound", err)
	}

	if mockProvider.callCount != 0 {
		t.Errorf("provider called %d times on a template error, want 0", mockProvider.callCount)
	}
}

func TestGenerate_InvalidTemplateIsTypedError(t *testing.T) {
	dir := newConsumerRepo(t, "# Fixture")

	overridePath := filepath.Join(dir, defaultPromptPath)
	if err := os.MkdirAll(filepath.Dir(overridePath), 0755); err != nil {
		t.Fatalf("failed to create override dir: %v", err)
	}
	if err := os.WriteFile(overridePath, []byte("no placeholder here"), 0644); err != nil {
		t.Fatalf("failed to write override: %v", err)
	}

	orch := llm.NewOrchestrator(&MockProvider{}, nil, nil)
	_, err := NewGenerator(orch).Generate(context.Background(), GenerateOptions{ReadmePath: "README.md", ProjectDir: dir})
	if !errors.Is(err, ErrPromptTemplateInvalid) {
		t.Fatalf("err = %v, want ErrPromptTemplateInvalid", err)
	}
}

// TestGenerate_NoLLMProvider proves the LLM stays optional: no provider yields
// a typed error the caller can log as a warning, not a panic and not a
// silently empty success.
func TestGenerate_NoLLMProvider(t *testing.T) {
	dir := newConsumerRepo(t, "# Fixture")

	content, err := NewGenerator(nil).Generate(context.Background(), GenerateOptions{
		ReadmePath: "README.md",
		ProjectDir: dir,
		OutputPath: filepath.Join(dir, "docs", "index.md"),
	})
	if !errors.Is(err, ErrNoLLMProvider) {
		t.Fatalf("err = %v, want ErrNoLLMProvider", err)
	}

	if content != "" {
		t.Errorf("content = %q, want empty", content)
	}

	if _, statErr := os.Stat(filepath.Join(dir, "docs", "index.md")); !os.IsNotExist(statErr) {
		t.Errorf("no output file must be written without a provider, stat err = %v", statErr)
	}
}
