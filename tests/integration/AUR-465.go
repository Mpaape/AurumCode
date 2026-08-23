// Package integration holds AUR-465's Integration-layer proof: the
// sanitizers tests/unit/AUR-465.go checks in isolation actually run inside
// welcome.Generator.Generate() -- the real entry point cmd/regenerate-docs
// calls -- and the AC-002 asset check composes correctly with a real
// _config.yml and a real filesystem, the way the site scaffold and the
// welcome package's output meet in the published tree. The Unit layer never
// calls Generate(); this layer never calls the sanitizers directly. If the
// wiring between them broke -- Generate() stopped calling the sanitizers,
// or the asset check resolved paths against the wrong root -- this layer is
// what would catch it while the Unit layer stayed green.
//
// Not named "_test.go" on purpose, mirroring every sibling card in this
// office: tests/acceptance/AUR-465.sh stages a private copy of the module
// and bridges IntegrationAUR465 into a real `go test`.
package integration

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/documentation/welcome"
	"github.com/Mpaape/AurumCode/internal/llm"
)

// aur465MockProvider is a minimal llm.Provider stand-in: it returns a fixed
// response regardless of prompt, exactly the shape a real model's output
// takes when it echoes a consumer README's own Quick Start verbatim -- the
// documented failure mode this card fixes.
type aur465MockProvider struct{ response string }

func (m *aur465MockProvider) Complete(prompt string, opts llm.Options) (llm.Response, error) {
	return llm.Response{Text: m.response, TokensIn: 10, TokensOut: 10, Model: "mock"}, nil
}
func (m *aur465MockProvider) Tokens(input string) (int, error) { return len(input), nil }
func (m *aur465MockProvider) Name() string                     { return "aur465-mock" }

// IntegrationAUR465 is AUR-465's Integration-layer selector.
func IntegrationAUR465(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Fixture\n\nA fixture consumer README."), 0o644); err != nil {
		t.Fatalf("AUR-465/AC-001/infrastructure: write fixture README: %v", err)
	}

	// The mock LLM behaves exactly like a model that echoed the consumer's
	// own (unfixed) README Quick Start plus an invented "Section 1" link
	// straight out of the prompt template's old example structure -- the
	// two defects AC-001 and AC-003 exist to catch, landing in the same
	// response so one Generate() call proves both guardrails fire together.
	mock := &aur465MockProvider{response: "# Fixture\n\n" +
		"## Quick Start\n\n```yaml\n- uses: Mpaape/AurumCode@main\n```\n\n" +
		"## Documentation\n\n- [Guide](docs/getting-started.md)\n- [Section 1](guides/advanced/)\n"}
	orch := llm.NewOrchestrator(mock, nil, nil)
	gen := welcome.NewGenerator(orch)

	content, err := gen.Generate(context.Background(), welcome.GenerateOptions{
		ReadmePath: "README.md",
		ProjectDir: dir,
	})
	if err != nil {
		t.Fatalf("AUR-465/AC-001/infrastructure: Generate failed: %v", err)
	}

	if strings.Contains(content, "@main") {
		t.Fatalf("AUR-465/AC-001/behavior-missing: Generate()'s own output still carries @main; the sanitizer is not wired into the real entry point:\n%s", content)
	}
	if !strings.Contains(content, "Mpaape/AurumCode@v1") {
		t.Fatalf("AUR-465/AC-001/behavior-missing: Generate()'s output never carries the published v1 tag:\n%s", content)
	}
	if strings.Contains(content, "](guides/advanced/)") {
		t.Fatalf("AUR-465/AC-003/behavior-missing: Generate()'s output still carries the invented relative link:\n%s", content)
	}
	if !strings.Contains(content, "[Guide](docs/getting-started.md)") {
		t.Fatalf("AUR-465/AC-003/behavior-missing: Generate()'s output lost the legitimate getting-started link:\n%s", content)
	}

	// -- AC-002 cross-cutting: a declared logo composed against a real tree.
	// welcome.Generator writes prose, never _config.yml; this proves the
	// AC-002 primitive (DeclaredAssetPath + AssetExists) gives the right
	// answer against a filesystem laid out the way the site scaffold
	// (internal/documentation/site, read-only to this card) actually
	// produces it: DocsDir/assets/images/logo.png for a
	// "/assets/images/logo.png" declaration.
	siteRoot := t.TempDir()
	configCompliant := "title: Fixture\nlogo: \"/assets/images/logo.png\"\n"
	if err := os.MkdirAll(filepath.Join(siteRoot, "assets", "images"), 0o755); err != nil {
		t.Fatalf("AUR-465/AC-002/infrastructure: mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(siteRoot, "assets", "images", "logo.png"), []byte("png"), 0o644); err != nil {
		t.Fatalf("AUR-465/AC-002/infrastructure: write asset: %v", err)
	}
	path, declared := welcome.DeclaredAssetPath(configCompliant)
	if !declared {
		t.Fatalf("AUR-465/AC-002/behavior-missing: DeclaredAssetPath found no logo in a config that declares one")
	}
	if !welcome.AssetExists(path, siteRoot) {
		t.Fatalf("AUR-465/AC-002/behavior-missing: AssetExists says %q is missing under %s, but the integration fixture wrote it", path, siteRoot)
	}

	// The MUT-002 shape, checked here rather than assumed: a config
	// declaring a logo the site tree never received.
	configBroken := "title: Fixture\nlogo: \"/assets/images/missing.png\"\n"
	brokenPath, brokenDeclared := welcome.DeclaredAssetPath(configBroken)
	if !brokenDeclared {
		t.Fatalf("AUR-465/AC-002/behavior-missing: DeclaredAssetPath found no logo in the MUT-002 fixture config")
	}
	if welcome.AssetExists(brokenPath, siteRoot) {
		t.Fatalf("AUR-465/AC-002/MUT-002: AssetExists reports a nonexistent logo as present; AC-002 would pass a broken image through")
	}
}
