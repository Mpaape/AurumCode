// Package unit holds AUR-484's Unit-layer proof: internal/documentation/site's
// Scaffold, on a single generated page and no pre-existing index.md, produces
// a landing page whose content ABOVE the generated-page reference is exactly
// what the card requires - a copy-pasteable, no-credential command; one
// example per product feature; and the declared limitations - and that the
// page enumeration itself no longer lives in that body.
//
// AC-001: a fenced ```bash block carrying the exact no-credential command
// (`./aurumcode review --base HEAD~1 --seguranca`), positioned before the
// generated-page section, plus one example each for review, docs and pages.
//
// AC-002: the generated-page enumeration (per-language headings, per-symbol
// bullets) is NOT the body of index.md; it lives on its own page
// (reference.md), and index.md only references it.
//
// AC-003: index.md states, in the same block, that only 4 of 8 security rules
// carry a matcher and names the languages with a measured command-injection
// gap - content that must remain true against internal/review/rules/security.yml
// and docs/specs/AUR-481.md's measurement, not merely present as text.
//
// Not named "_test.go" on purpose, mirroring every sibling card in this
// office: tests/acceptance/AUR-484.sh stages a private copy of the module
// and bridges TestAUR484 into a real `go test`.
package unit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/documentation/site"
)

// writeFileAUR484 and readFileAUR484 are named to avoid colliding with any
// sibling card's helper in this package: tests/unit compiles every card's
// file together, and AUR-425's own writeFile does not create parent
// directories the way this fixture needs.
func writeFileAUR484(t *testing.T, path, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("AUR-484/AC-001/infrastructure: mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("AUR-484/AC-001/infrastructure: write %s: %v", path, err)
	}
}

func readFileAUR484(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("AUR-484/AC-001/infrastructure: read %s: %v", path, err)
	}
	return string(data)
}

// TestAUR484 is AUR-484's Unit-layer selector.
func TestAUR484(t *testing.T) {
	dir := t.TempDir()

	// A single generated page is enough to exercise the whole contract: the
	// scaffold's own content above it does not depend on how many pages a
	// run produced.
	writeFileAUR484(t, filepath.Join(dir, "go", "ledger.md"),
		"---\ntitle: Ledger\npermalink: /go/ledger/\n---\n\n# ledger\n\n## func AddMoney\n")

	result, err := site.NewScaffold(site.ScaffoldConfig{DocsDir: dir, OutputDir: dir, Title: "tinyrepo"}).Generate()
	if err != nil {
		t.Fatalf("AUR-484/AC-001/infrastructure: Generate failed: %v", err)
	}

	index := readFileAUR484(t, result.IndexPath)

	// -- AC-001: a copyable, no-credential command, above the API listing ---
	const noCredCmd = "./aurumcode review --base HEAD~1 --seguranca"
	if !strings.Contains(index, "```bash") || !strings.Contains(index, noCredCmd) {
		t.Fatalf("AUR-484/AC-001/behavior-missing: index.md has no copyable no-credential command %q:\n%s", noCredCmd, index)
	}

	cmdPos := strings.Index(index, noCredCmd)
	apiPos := strings.Index(index, "Generated API documentation")
	if cmdPos < 0 || apiPos < 0 || cmdPos > apiPos {
		t.Fatalf("AUR-484/AC-001/behavior-missing: the copyable command (pos=%d) is not above the API listing (pos=%d):\n%s", cmdPos, apiPos, index)
	}

	// One example per feature: review, docs, pages.
	if !strings.Contains(index, "aurumcode docs --source . --output ./site") {
		t.Fatalf("AUR-484/AC-001/behavior-missing: index.md has no documentation-feature example:\n%s", index)
	}
	if !strings.Contains(index, "uses: Mpaape/AurumCode@v1") {
		t.Fatalf("AUR-484/AC-001/behavior-missing: index.md has no pages-feature example:\n%s", index)
	}
	if strings.Contains(index, "@main") {
		t.Fatalf("AUR-484/AC-001/behavior-missing: index.md pins the Action to a mutable ref (@main):\n%s", index)
	}

	// -- AC-002: the enumeration is not the body of index.md -----------------
	if strings.Contains(index, "### Go") || strings.Contains(index, "func AddMoney") {
		t.Fatalf("AUR-484/AC-002/behavior-missing: index.md still embeds the per-page enumeration:\n%s", index)
	}
	if !strings.Contains(index, "reference.html") {
		t.Fatalf("AUR-484/AC-002/behavior-missing: index.md does not reference the standalone reference page:\n%s", index)
	}
	if result.ReferencePath == "" {
		t.Fatal("AUR-484/AC-002/behavior-missing: Generate() reported no ReferencePath")
	}
	reference := readFileAUR484(t, result.ReferencePath)
	if !strings.Contains(reference, "### Go") || !strings.Contains(reference, "func AddMoney") {
		t.Fatalf("AUR-484/AC-002/behavior-missing: reference.md does not carry the enumeration it was supposed to receive:\n%s", reference)
	}

	// -- AC-003: declared, verifiable limitations ----------------------------
	if !strings.Contains(index, "4 das 8") {
		t.Fatalf("AUR-484/AC-003/behavior-missing: index.md never states the measured 4-of-8 security-rule matcher coverage:\n%s", index)
	}
	for _, rule := range []string{"security/sql-injection", "security/command-injection", "security/hardcoded-secret", "security/xss"} {
		if !strings.Contains(index, rule) {
			t.Fatalf("AUR-484/AC-003/behavior-missing: index.md does not name the matched rule %q:\n%s", rule, index)
		}
	}
	for _, lang := range []string{"Go", "C#", "PowerShell", "Bash", "Rust"} {
		if !strings.Contains(index, lang) {
			t.Fatalf("AUR-484/AC-003/behavior-missing: index.md does not name the language-coverage gap %q:\n%s", lang, index)
		}
	}
	if !strings.Contains(index, "review de qualidade nao roda") && !strings.Contains(index, "review de qualidade não roda") {
		t.Fatalf("AUR-484/AC-003/behavior-missing: index.md does not state that quality review needs a provider:\n%s", index)
	}
}
