// Package integration holds AUR-427's Integration-layer proof: the native
// Rust and C# extractors are registered into a real
// internal/documentation/extractors.Registry -- exactly the mechanism
// cmd/regenerate-docs/main.go's registerLanguageExtractors uses -- and,
// resolved back out of that registry, document the card's own checked-in
// Rust fixture at tests/fixtures/docs/rustproject twice, byte-for-byte
// identically, plus an in-memory C# fixture (this card's `paths` grants a
// checked-in fixture directory for Rust only; see docs/specs/AUR-427.md).
//
// Not named "_test.go" for the same reason as tests/unit/AUR-427.go: see
// that file's package comment. tests/acceptance/AUR-427.sh bridges
// IntegrationAUR427 into a real `go test` run inside the sandbox.
package integration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	extractors "github.com/Mpaape/AurumCode/internal/documentation/extractors"
	csharpextractor "github.com/Mpaape/AurumCode/internal/documentation/extractors/csharp"
	rustextractor "github.com/Mpaape/AurumCode/internal/documentation/extractors/rust"
)

func readFileIntAUR427(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

// digestTreeAUR427 hashes every regular file under dir by relative path and
// content, so two runs can be compared for byte-for-byte identical output
// without depending on file iteration order.
func digestTreeAUR427(t *testing.T, dir string) string {
	t.Helper()
	var names []string
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read output dir %s: %v", dir, err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	h := sha256.New()
	for _, name := range names {
		h.Write([]byte(name))
		h.Write([]byte{0})
		h.Write([]byte(readFileIntAUR427(t, filepath.Join(dir, name))))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

const csharpIntegrationFixture = `namespace Fixture
{
    /// <summary>
    /// A greeter that says hello in a configurable language.
    /// </summary>
    public class Greeter
    {
        /// <summary>
        /// Creates a greeter for the given language tag.
        /// </summary>
        /// <param name="lang">A BCP-47-ish language tag, e.g. "en" or "pt".</param>
        public Greeter(string lang)
        {
            Lang = lang;
        }

        /// <summary>
        /// The greeter's configured language tag.
        /// </summary>
        public string Lang { get; }

        /// <summary>
        /// Renders a greeting for the given name.
        /// </summary>
        /// <param name="name">Who to greet.</param>
        /// <returns>A human-readable greeting sentence.</returns>
        public string Greet(string name)
        {
            return Lang == "pt" ? "Ola, " + name + "!" : "Hello, " + name + "!";
        }

        private bool IsPortuguese() => Lang == "pt";
    }

    /// <summary>
    /// The kind of greeting to produce.
    /// </summary>
    public enum GreetingKind
    {
        Formal,
        Casual,
    }
}
`

// IntegrationAUR427 is AUR-427's Integration-layer selector.
func IntegrationAUR427(t *testing.T) {
	root := os.Getenv("AURUMCODE_ROOT")
	if root == "" {
		t.Fatal("AURUMCODE_ROOT is required")
	}

	registry := extractors.NewRegistry()
	rustExt := rustextractor.NewNativeExtractor()
	csharpExt := csharpextractor.NewNativeExtractor()

	if err := registry.Register(rustExt); err != nil {
		t.Fatalf("AUR-427/AC-001/behavior-missing: registry rejected the Rust extractor: %v", err)
	}
	if err := registry.Register(csharpExt); err != nil {
		t.Fatalf("AUR-427/AC-001/behavior-missing: registry rejected the C# extractor: %v", err)
	}

	resolvedRust, err := registry.Get(extractors.LanguageRust)
	if err != nil {
		t.Fatalf("AUR-427/AC-001/behavior-missing: registry lost the Rust extractor: %v", err)
	}
	resolvedCSharp, err := registry.Get(extractors.LanguageCSharp)
	if err != nil {
		t.Fatalf("AUR-427/AC-001/behavior-missing: registry lost the C# extractor: %v", err)
	}

	// --- Rust: the card's own checked-in fixture, exercised twice ---

	fixture := filepath.Join(root, "tests", "fixtures", "docs", "rustproject")
	if info, statErr := os.Stat(fixture); statErr != nil || !info.IsDir() {
		t.Fatalf("card fixture missing: %s (%v)", fixture, statErr)
	}

	outDir1 := filepath.Join(t.TempDir(), "site")
	result, err := resolvedRust.Extract(context.Background(), &extractors.ExtractRequest{
		Language:  extractors.LanguageRust,
		SourceDir: fixture,
		OutputDir: outDir1,
	})
	if err != nil {
		t.Fatalf("AUR-427/AC-001/behavior-missing: Extract failed: %v", err)
	}
	if result.Stats.DocsGenerated == 0 || len(result.Files) == 0 {
		t.Fatalf("AUR-427/AC-001/behavior-missing: zero pages for the checked-in Rust fixture: %+v", result.Stats)
	}

	var combined strings.Builder
	for _, f := range result.Files {
		combined.WriteString(readFileIntAUR427(t, f))
		combined.WriteString("\n")
	}
	page := combined.String()
	for _, want := range []string{
		"pub struct Entry", "amount in cents",
		"pub fn new_entry", "Creates a new ledger entry.",
		"pub const MAX_ENTRIES_PER_PAGE",
		"pub struct Ledger", "pub fn new() -> Ledger",
		"pub fn add", "pub fn balance_cents",
		"pub enum EntryKind",
	} {
		if !strings.Contains(page, want) {
			t.Fatalf("AUR-427/AC-001/behavior-missing: generated pages missing %q", want)
		}
	}
	// entry_count is not `pub`: this parser must never claim it as public API.
	if strings.Contains(page, "entry_count") {
		t.Fatalf("AUR-427/AC-001/false-claim: generated pages mention non-public symbol entry_count")
	}
	// The fixture's own module doc legitimately names "record_internal" in
	// prose (to explain why it is absent); what must never appear is the
	// parser having turned it into a declared item heading.
	if strings.Contains(page, "### fn pub fn record_internal") {
		t.Fatalf("AUR-427/AC-001/false-claim: generated pages report record_internal as an extracted item")
	}

	// AC-001's "And" clause: repeating the run over the same input produces
	// the same output.
	outDir2 := filepath.Join(t.TempDir(), "site")
	if _, err := resolvedRust.Extract(context.Background(), &extractors.ExtractRequest{
		Language:  extractors.LanguageRust,
		SourceDir: fixture,
		OutputDir: outDir2,
	}); err != nil {
		t.Fatalf("AUR-427/AC-001/behavior-missing: second Extract failed: %v", err)
	}

	digest1 := digestTreeAUR427(t, outDir1)
	digest2 := digestTreeAUR427(t, outDir2)
	if digest1 != digest2 {
		t.Fatalf("AUR-427/AC-001/behavior-missing: repeated Rust extraction is not deterministic: %s != %s", digest1, digest2)
	}

	// --- C#: an in-memory fixture (this card's paths grant a checked-in
	// fixture directory for Rust only) ---

	csSrc := t.TempDir()
	if err := os.WriteFile(filepath.Join(csSrc, "Greeter.cs"), []byte(csharpIntegrationFixture), 0o644); err != nil {
		t.Fatalf("write C# fixture: %v", err)
	}

	csOut := filepath.Join(t.TempDir(), "site")
	csResult, err := resolvedCSharp.Extract(context.Background(), &extractors.ExtractRequest{
		Language:  extractors.LanguageCSharp,
		SourceDir: csSrc,
		OutputDir: csOut,
	})
	if err != nil {
		t.Fatalf("AUR-427/AC-001/behavior-missing: C# Extract failed: %v", err)
	}
	if csResult.Stats.DocsGenerated == 0 || len(csResult.Files) == 0 {
		t.Fatalf("AUR-427/AC-001/behavior-missing: zero pages for the C# fixture: %+v", csResult.Stats)
	}
	csPage := readFileIntAUR427(t, csResult.Files[0])
	for _, want := range []string{
		"public class Greeter", "A greeter that says hello",
		"public Greeter(string lang)", "Creates a greeter for the given language tag.",
		"public string Greet(string name)", "Renders a greeting for the given name.",
		"Returns: A human-readable greeting sentence.",
		"public enum GreetingKind",
	} {
		if !strings.Contains(csPage, want) {
			t.Fatalf("AUR-427/AC-001/behavior-missing: generated C# page missing %q:\n%s", want, csPage)
		}
	}
	if strings.Contains(csPage, "IsPortuguese") {
		t.Fatalf("AUR-427/AC-001/false-claim: generated C# page mentions non-public member IsPortuguese:\n%s", csPage)
	}

	// --- Wiring: cmd/regenerate-docs/main.go really calls these two
	// constructors by default. Read-only: this never executes main.go's own
	// entry point (its full transitive dependency graph is not guaranteed to
	// be materialized for every acceptance lane), it only proves the source
	// text still wires both native extractors the way this package's public
	// constructors expect, so a refactor here cannot silently drift from
	// that call site. ---

	mainSrc := readFileIntAUR427(t, filepath.Join(root, "cmd", "regenerate-docs", "main.go"))
	if !strings.Contains(mainSrc, "rustExtractor.NewNativeExtractor()") {
		t.Fatal("cmd/regenerate-docs/main.go no longer registers the native Rust extractor by default")
	}
	if !strings.Contains(mainSrc, "csharpExtractor.NewNativeExtractor()") {
		t.Fatal("cmd/regenerate-docs/main.go no longer registers the native C# extractor by default")
	}

	t.Logf("AUR-427/AC-001/pass registry=ok rust_docs=%d csharp_docs=%d digest=%s",
		result.Stats.DocsGenerated, csResult.Stats.DocsGenerated, digest1)
}
