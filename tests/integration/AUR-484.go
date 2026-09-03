// Package integration holds AUR-484's Integration-layer proof: the
// per-block guarantees tests/unit/AUR-484.go checks in isolation actually
// compose the way the real published site does - an existing, hand-written
// index.md intro (the shape `.aurumcode/index.md` carries today: a short
// "what you'll find here" paragraph, no runnable command), several
// generated pages across more than one language, and a rerun of the exact
// same Scaffold. The Unit layer proves the block's own content; this layer
// proves it survives composition with real prose, stays byte-identical on a
// second run (the same idempotency contract the pre-existing pages block
// already had), and that AC-002's move to reference.md does not silently
// drop a page the run actually produced.
//
// Not named "_test.go" on purpose, mirroring every sibling card in this
// office: tests/acceptance/AUR-484.sh stages a private copy of the module
// and bridges IntegrationAUR484 into a real `go test`.
package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/documentation/site"
)

func writeFileAUR484(t *testing.T, path, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("AUR-484/AC-001/infrastructure: mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("AUR-484/AC-001/infrastructure: write %s: %v", path, err)
	}
}

func readFileIntegrationAUR484(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("AUR-484/AC-001/infrastructure: read %s: %v", path, err)
	}
	return string(data)
}

// IntegrationAUR484 is AUR-484's Integration-layer selector.
func IntegrationAUR484(t *testing.T) {
	dir := t.TempDir()

	// The shape .aurumcode/index.md actually carries today: prose above the
	// generated block, no runnable command, no limitations. AUR-484 must add
	// its own block without disturbing this hand-written intro.
	writeFileAUR484(t, filepath.Join(dir, "index.md"),
		"---\ntitle: AurumCode\n---\n\n# AurumCode\n\nCode review, documentacao e publicacao no GitHub Pages.\n")

	writeFileAUR484(t, filepath.Join(dir, "go", "ledger.md"),
		"---\ntitle: Ledger\npermalink: /go/ledger/\n---\n\n# ledger\n\n## func AddMoney\n")
	writeFileAUR484(t, filepath.Join(dir, "python", "app.md"),
		"# app\n\n## class Ledger\n")

	scaffold := site.NewScaffold(site.ScaffoldConfig{DocsDir: dir, OutputDir: dir, Title: "AurumCode"})

	first, err := scaffold.Generate()
	if err != nil {
		t.Fatalf("AUR-484/AC-001/infrastructure: first Generate failed: %v", err)
	}

	firstIndex := readFileIntegrationAUR484(t, first.IndexPath)
	if !strings.Contains(firstIndex, "Code review, documentacao e publicacao no GitHub Pages.") {
		t.Fatalf("AUR-484/AC-001/behavior-missing: the existing hand-written intro was dropped:\n%s", firstIndex)
	}
	if !strings.Contains(firstIndex, "./aurumcode review --base HEAD~1 --seguranca") {
		t.Fatalf("AUR-484/AC-001/behavior-missing: the quickstart command did not compose with the existing intro:\n%s", firstIndex)
	}

	// Rerun: the same idempotency contract the pre-existing pages block
	// already had (TestScaffoldPreservesIntroAndReplacesListing) must hold
	// for the new quickstart block too, or every CI run would grow the file.
	second, err := scaffold.Generate()
	if err != nil {
		t.Fatalf("AUR-484/AC-001/infrastructure: second Generate failed: %v", err)
	}
	secondIndex := readFileIntegrationAUR484(t, second.IndexPath)
	if firstIndex != secondIndex {
		t.Fatalf("AUR-484/AC-001/behavior-missing: regeneration is not idempotent:\nfirst:\n%s\nsecond:\n%s", firstIndex, secondIndex)
	}
	if got := strings.Count(secondIndex, "aurumcode:quickstart:start"); got != 1 {
		t.Fatalf("AUR-484/AC-001/behavior-missing: quickstart block appears %d times after two runs, want 1", got)
	}

	// AC-002: both generated pages are accounted for on the reference page,
	// not dropped by the move off the landing page.
	reference := readFileIntegrationAUR484(t, second.ReferencePath)
	for _, want := range []string{"### Go", "func AddMoney", "### Python", "class Ledger"} {
		if !strings.Contains(reference, want) {
			t.Fatalf("AUR-484/AC-002/behavior-missing: reference.md is missing %q after composing with a real multi-language run:\n%s", want, reference)
		}
	}
	if len(second.Pages) != 2 {
		t.Fatalf("AUR-484/AC-002/behavior-missing: Generate() reports %d pages, want 2", len(second.Pages))
	}

	// The landing page states the count without the per-page detail.
	if !strings.Contains(secondIndex, "2 page(s) generated") {
		t.Fatalf("AUR-484/AC-002/behavior-missing: index.md does not state the generated-page count:\n%s", secondIndex)
	}
}
