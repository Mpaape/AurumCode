// Package integration holds AUR-472's Integration-layer proof: the fix
// tests/unit/AUR-472.go checks in isolation is actually wired into
// welcome.Generator.Generate() -- the real entry point cmd/regenerate-docs
// calls -- so an LLM response echoing an unpublished-but-semver-shaped
// action ref (the measured defect) never survives into published content.
// The Unit layer never calls Generate(); this layer never calls
// SanitizeActionRefTags directly.
//
// Not named "_test.go" on purpose, mirroring every sibling card in this
// office: tests/acceptance/AUR-472.sh stages a private copy of the module
// and bridges IntegrationAUR472 into a real `go test`.
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

// aur472MockProvider is a minimal llm.Provider stand-in returning a fixed
// response shaped exactly like the measured defect: a Quick Start pinned to
// "v2", a ref with the correct semver SHAPE that has never actually been
// released.
type aur472MockProvider struct{ response string }

func (m *aur472MockProvider) Complete(prompt string, opts llm.Options) (llm.Response, error) {
	return llm.Response{Text: m.response, TokensIn: 10, TokensOut: 10, Model: "mock"}, nil
}
func (m *aur472MockProvider) Tokens(input string) (int, error) { return len(input), nil }
func (m *aur472MockProvider) Name() string                     { return "aur472-mock" }

// IntegrationAUR472 is AUR-472's Integration-layer selector.
func IntegrationAUR472(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Fixture\n\nA fixture consumer README."), 0o644); err != nil {
		t.Fatalf("AUR-472/AC-001/infrastructure: write fixture README: %v", err)
	}

	// The mock LLM behaves exactly like a model that echoed a workflow
	// pinned to "v2" -- a branch created for the next major that has the
	// right SHAPE to look like a release tag, before any v2 tag exists.
	mock := &aur472MockProvider{response: "# Fixture\n\n" +
		"## Quick Start\n\n```yaml\n- uses: Mpaape/AurumCode@v2\n```\n\n" +
		"## Documentation\n\n- [Guide](docs/getting-started.md)\n"}
	orch := llm.NewOrchestrator(mock, nil, nil)
	gen := welcome.NewGenerator(orch)

	content, err := gen.Generate(context.Background(), welcome.GenerateOptions{
		ReadmePath: "README.md",
		ProjectDir: dir,
	})
	if err != nil {
		t.Fatalf("AUR-472/AC-001/infrastructure: Generate failed: %v", err)
	}

	if strings.Contains(content, "@v2") {
		t.Fatalf("AUR-472/AC-001/behavior-missing: Generate()'s own output still carries the unpublished ref @v2; the tag-existence check is not wired into the real entry point:\n%s", content)
	}
	if !strings.Contains(content, "Mpaape/AurumCode@v1") {
		t.Fatalf("AUR-472/AC-001/behavior-missing: Generate()'s output never carries the published v1 tag after rewriting @v2:\n%s", content)
	}

	// A published, real tag must survive Generate() untouched -- proves the
	// wiring does not just rewrite everything unconditionally.
	mockReal := &aur472MockProvider{response: "# Fixture\n\n" +
		"## Quick Start\n\n```yaml\n- uses: Mpaape/AurumCode@v1\n```\n"}
	genReal := welcome.NewGenerator(llm.NewOrchestrator(mockReal, nil, nil))
	contentReal, err := genReal.Generate(context.Background(), welcome.GenerateOptions{
		ReadmePath: "README.md",
		ProjectDir: dir,
	})
	if err != nil {
		t.Fatalf("AUR-472/AC-002/infrastructure: Generate failed: %v", err)
	}
	if !strings.Contains(contentReal, "Mpaape/AurumCode@v1") {
		t.Fatalf("AUR-472/AC-002/behavior-missing: Generate() rewrote a real, published tag it should have left untouched:\n%s", contentReal)
	}
}
