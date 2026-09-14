package integration

// Integration program for card AUR-479, selector IntegrationAUR479.
//
// Proves the split-secret fix through the REAL configured-provider stack:
// a secret is cut between two files on disk (the repository prompt
// .aurumcode/prompt.md and a configured docs/context.md), both loaded by
// config.ConfiguredProviders, assembled by WrapProvider, and the exact
// prompt handed to the underlying llm.Provider must not contain the
// reconstituted secret.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/llm"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
)

type aur479IntCapture struct{ prompt string }

func (c *aur479IntCapture) Complete(prompt string, _ llm.Options) (llm.Response, error) {
	c.prompt = prompt
	return llm.Response{Text: `{"issues":[]}`}, nil
}
func (c *aur479IntCapture) Tokens(input string) (int, error) { return len(input), nil }
func (c *aur479IntCapture) Name() string                     { return "aur479-int-capture" }

func aur479IntWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func IntegrationAUR479(t *testing.T) {
	const secret = "AURUM-real-provider-split-canary-4c1d9e07"
	root := t.TempDir()

	// The secret is split across two distinct on-disk providers; neither
	// half is a secret on its own.
	aur479IntWrite(t, filepath.Join(root, ".aurumcode", "prompt.md"), secret[:14])
	aur479IntWrite(t, filepath.Join(root, "docs", "context.md"), secret[14:])

	cfg := &config.Config{Review: config.ReviewConfig{Context: config.ReviewContextConfig{
		Docs: []string{"docs/context.md"},
	}}}

	filter := redaction.NewFilter(secret)
	base := &aur479IntCapture{}
	wrapped, err := config.WrapProvider(context.Background(), base,
		config.ConfiguredProviders(root, cfg), []string{"internal/config/provider.go"}, filter)
	if err != nil {
		t.Fatalf("WrapProvider: %v", err)
	}
	if _, err := wrapped.Complete("BASE", llm.Options{}); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if strings.Contains(strings.NewReplacer("\n", "", "\r", "").Replace(base.prompt), secret) {
		t.Fatalf("the secret split across .aurumcode/prompt.md and a configured docs file was reconstituted: %q", base.prompt)
	}
	if !strings.Contains(base.prompt, redaction.Marker) {
		t.Fatalf("expected the redaction marker in the assembled prompt: %q", base.prompt)
	}
}
