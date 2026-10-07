package config

import (
	"context"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/llm"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
)

type aur526ToolBase struct {
	pendingCaptureProvider
	messages []llm.Message
}

func (p *aur526ToolBase) CompleteWithTools(m []llm.Message, _ []llm.ToolSpec, _ llm.Options) (llm.ToolResponse, error) {
	p.messages = m
	return llm.ToolResponse{}, nil
}

// AC-007: the repository-context decorator keeps tool calling visible when
// the provider has it, injects the same block into the tool conversation,
// and never claims tool calling for a text-only provider.
func TestAUR526ContextWrapperForwardsToolCalling(t *testing.T) {
	sources := []ContextProvider{pendingTextProvider{name: "repo", text: "contexto do repositorio"}}
	base := &aur526ToolBase{}
	wrapped, err := WrapProvider(context.Background(), base, sources, []string{"a.go"}, redaction.NewFilter())
	if err != nil {
		t.Fatal(err)
	}
	caller, ok := llm.AsToolCaller(wrapped)
	if !ok {
		t.Fatal("the context decorator hid the provider's tool calling")
	}
	if _, err := caller.CompleteWithTools(llm.SystemUserMessages("sys", "diff"), nil, llm.Options{}); err != nil {
		t.Fatal(err)
	}
	if len(base.messages) != 2 || base.messages[0].Content != "sys" || !strings.HasPrefix(base.messages[1].Content, "diff\n\n") || !strings.Contains(base.messages[1].Content, "contexto do repositorio") {
		t.Fatalf("the tool conversation lost the context block: %+v", base.messages)
	}
	text, err := WrapProvider(context.Background(), &pendingCaptureProvider{}, sources, []string{"a.go"}, redaction.NewFilter())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := llm.AsToolCaller(text); ok {
		t.Fatal("the context decorator claimed tool calling for a text-only provider")
	}
}

// AC-003 configuration: the embedded secret catalog, the policy's ignore
// globs and deliberation.secret_paths decide what the tools refuse.
func TestAUR526SecretAndIgnoredPaths(t *testing.T) {
	cfg, err := Parse([]byte("ignore:\n  - \"vendor/**\"\ndeliberation:\n  enabled: true\n  max_read_bytes: 1024\n  secret_paths:\n    - \"**/*.secret\"\n"), "test")
	if err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]bool{".env": true, "app/.env.local": true, "deploy/tls.pem": true, "home/.ssh/config": true, "x/a.secret": true, "src/Main.java": false} {
		if got := cfg.IsSecretPath(path); got != want {
			t.Errorf("IsSecretPath(%q) = %v, want %v", path, got, want)
		}
	}
	if !cfg.IgnoresPath("vendor/lib/a.go") || cfg.IgnoresPath("src/a.go") {
		t.Fatal("IgnoresPath does not follow the ignore globs")
	}
	if cfg.Deliberation.EffectiveMaxReadBytes() != 1024 || (&DeliberationConfig{}).EffectiveMaxReadBytes() != DefaultDeliberationMaxReadBytes {
		t.Fatal("max_read_bytes not applied or not defaulted")
	}
	if _, err := Parse([]byte("deliberation:\n  max_read_bytes: -1\n"), "test"); err == nil || !strings.Contains(err.Error(), "max_read_bytes") {
		t.Fatalf("a negative max_read_bytes was accepted: %v", err)
	}
}

// AUR-469 hardening: a relative MCP command is a configuration error; an
// absolute path or a bare name from PATH is accepted.
func TestAUR469RelativeMCPCommandIsRefused(t *testing.T) {
	for _, cmd := range []string{"./tools/mcp", "tools/mcp", "..\\\\mcp"} {
		yml := "review:\n  context:\n    mcp:\n      - name: a\n        command: [\"" + cmd + "\"]\n        tool: t\n"
		if _, err := Parse([]byte(yml), "t"); err == nil || !strings.Contains(err.Error(), "absolute path") {
			t.Errorf("%q: err = %v", cmd, err)
		}
	}
	for _, cmd := range []string{"/usr/bin/adr-mcp", "adr-mcp"} {
		yml := "review:\n  context:\n    mcp:\n      - name: a\n        command: [\"" + cmd + "\"]\n        tool: t\n"
		if _, err := Parse([]byte(yml), "t"); err != nil {
			t.Errorf("%q refused: %v", cmd, err)
		}
	}
}

// Secret paths match ignoring case, and the catalog covers cluster and
// registry credentials and service-account keys.
func TestAUR526SecretPathsIgnoreCaseAndCoverCloudCredentials(t *testing.T) {
	cfg := &Config{}
	for _, path := range []string{".ENV", "app/Credentials", "deploy/TLS.PEM", "ops/kubeconfig", "home/.kube/config", "ci/.docker/config.json", "gcp/prod-service-account-key.json", "x/My-Service-Account.JSON"} {
		if !cfg.IsSecretPath(path) {
			t.Errorf("IsSecretPath(%q) = false", path)
		}
	}
	if cfg.IsSecretPath("src/Main.java") || cfg.IsSecretPath("docs/accounts.md") {
		t.Fatal("an ordinary file counted as secret")
	}
}
