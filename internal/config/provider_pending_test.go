package config

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/llm"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
)

type pendingCaptureProvider struct {
	prompt string
}

func (p *pendingCaptureProvider) Complete(prompt string, _ llm.Options) (llm.Response, error) {
	p.prompt = prompt
	return llm.Response{Text: `{"issues":[]}`}, nil
}

func (p *pendingCaptureProvider) Tokens(input string) (int, error) { return len(input), nil }
func (p *pendingCaptureProvider) Name() string                     { return "pending-capture" }

type pendingTextProvider struct {
	name string
	text string
	err  error
}

func (p pendingTextProvider) Name() string { return p.name }
func (p pendingTextProvider) Provide(context.Context, []string) (string, error) {
	return p.text, p.err
}

func TestAUR479SplitSecretIsRedactedAfterAssembly(t *testing.T) {
	const secret = "AURUM-provider-split-canary"
	filter := redaction.NewFilter(secret)
	base := &pendingCaptureProvider{}

	wrapped, err := WrapProvider(context.Background(), base, []ContextProvider{
		pendingTextProvider{name: "provider-a", text: secret[:12]},
		pendingTextProvider{name: "provider-b", text: secret[12:]},
	}, []string{"changed.go"}, filter)
	if err != nil {
		t.Fatalf("split contributions must remain usable: %v", err)
	}
	if _, err := wrapped.Complete("base", llm.Options{}); err != nil {
		t.Fatalf("wrapped provider failed: %v", err)
	}
	if strings.Contains(base.prompt, secret) {
		t.Fatalf("a secret split across providers reached the model: %q", base.prompt)
	}
	if !strings.Contains(base.prompt, redaction.Marker) {
		t.Fatalf("assembled redaction must leave the marker in the prompt: %q", base.prompt)
	}
}

func TestAUR480ProviderFailureWarnsAndContinues(t *testing.T) {
	base := &pendingCaptureProvider{}
	wrapped, warnings, err := WrapProviderWithWarnings(context.Background(), base, []ContextProvider{
		pendingTextProvider{name: "unavailable-mcp", err: errors.New("connection refused")},
		pendingTextProvider{name: "healthy-source", text: "safe background"},
	}, []string{"changed.go"}, redaction.NewFilter())
	if err != nil {
		t.Fatalf("an unavailable optional source must not abort the review: %v", err)
	}
	if len(warnings) != 1 || warnings[0].Provider != "unavailable-mcp" || !strings.Contains(warnings[0].Reason, "connection refused") {
		t.Fatalf("expected one actionable provider warning, got %+v", warnings)
	}
	if _, err := wrapped.Complete("base", llm.Options{}); err != nil {
		t.Fatalf("review should continue after optional provider failure: %v", err)
	}
	if !strings.Contains(base.prompt, "safe background") {
		t.Fatalf("healthy context must survive a different provider failure: %q", base.prompt)
	}
}

func TestConfiguredProvidersLoadPromptSkillAndDocs(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".aurumcode", "skills"), 0o755); err != nil {
		t.Fatalf("mkdir skills: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	files := map[string]string{
		".aurumcode/prompt.md":             "Repository review context",
		".aurumcode/skills/code-review.md": "Prefer behavior-focused findings",
		"docs/architecture.md":             "The service boundary is explicit",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(name)), []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	cfg := &Config{Review: ReviewConfig{Context: ReviewContextConfig{
		Skills: []string{".aurumcode/skills/code-review.md"},
		Docs:   []string{"docs/architecture.md"},
	}}}
	base := &pendingCaptureProvider{}
	wrapped, warnings, err := WrapProviderWithWarnings(context.Background(), base, ConfiguredProviders(root, cfg), []string{"service.go"}, redaction.NewFilter())
	if err != nil {
		t.Fatalf("ConfiguredProviders: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected provider warnings: %+v", warnings)
	}
	if _, err := wrapped.Complete("base", llm.Options{}); err != nil {
		t.Fatalf("wrapped provider: %v", err)
	}
	for _, want := range []string{"Repository review context", "Prefer behavior-focused findings", "The service boundary is explicit"} {
		if !strings.Contains(base.prompt, want) {
			t.Errorf("prompt missing configured context %q:\n%s", want, base.prompt)
		}
	}
}
