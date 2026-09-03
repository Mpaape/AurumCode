// Integration proof for card AUR-483: runs the real
// internal/pipeline.ExtractorPipeline end to end (file discovery, the Go
// extractor, normalization and site scaffold) against the fixture tree at
// tests/fixtures/docs/AUR-483, and inspects the markdown pages it actually
// writes to disk.
//
// This complements tests/unit/AUR-483.go, which only exercises the pure
// IsTestScopePath/IncludeTests functions in isolation: IntegrationAUR483
// proves those seams are actually wired into determineFilesToProcess and
// change what a real run documents.
//
// The fixture (tests/fixtures/docs/AUR-483) has four PRODUCT Go packages --
// internal/attestation, cmd/latest, internal/core (service.go + contest.go)
// -- and three TEST-scope packages -- tests/sample.go, testdata/gen.go,
// fixtures/data.go -- plus one TEST-scope file living beside product code,
// internal/core/service_test.go. The Go extractor's outputBaseName joins a
// package's relative directory with "__", so the expected pages are
// internal__attestation.md, cmd__latest.md and internal__core.md for
// product, and tests.md, testdata.md, fixtures.md for test scope.
package integration

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	goExtractor "github.com/Mpaape/AurumCode/internal/documentation/extractors/go"
	"github.com/Mpaape/AurumCode/internal/documentation/site"
	"github.com/Mpaape/AurumCode/internal/pipeline"
)

// aur483Root resolves the repository root the same way the sibling
// AUR-450/AUR-467 integration programs do: AURUMCODE_ROOT wins (the
// acceptance harness sets it to the staged materialization root), and a
// direct run from a full checkout climbs two directories back to the root.
func aur483Root(t *testing.T) string {
	t.Helper()
	if r := os.Getenv("AURUMCODE_ROOT"); r != "" {
		return r
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolving repository root: %v", err)
	}
	return root
}

var productPages = []string{"internal__attestation.md", "cmd__latest.md", "internal__core.md"}
var testScopePages = []string{"tests.md", "testdata.md", "fixtures.md"}

// restorablePages excludes testdata.md from the opt-in assertion: the Go
// extractor (internal/documentation/extractors/go, out of this card's
// paths/read_paths write scope) hardcodes "testdata" in its OWN package
// walk's skip list, independent of and unconditioned by AUR-483's
// IncludeTests flag. That pre-existing, unrelated exclusion means a
// testdata/ Go package never reappears for this language no matter how
// IncludeTests is set -- AUR-483 must not, and does not, claim to override
// it. tests.md and fixtures.md have no such competing exclusion and must
// reappear.
var restorablePages = []string{"tests.md", "fixtures.md"}

func runPipeline(t *testing.T, sourceDir, outputDir string, includeTests bool) []string {
	t.Helper()
	runner := site.NewDefaultRunner()
	cfg := &pipeline.ExtractorPipelineConfig{
		SourceDir:    sourceDir,
		OutputDir:    outputDir,
		DocsDir:      outputDir,
		IncludeTests: includeTests,
	}
	p := pipeline.NewExtractorPipeline(cfg, runner, nil)
	if err := p.RegisterExtractor(goExtractor.NewGoExtractor(runner)); err != nil {
		t.Fatalf("registering go extractor: %v", err)
	}
	if err := p.Run(context.Background()); err != nil {
		t.Fatalf("pipeline run (includeTests=%v): %v", includeTests, err)
	}

	var pages []string
	walkErr := filepath.Walk(outputDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		// index.md and reference.md are scaffold pages, not content extracted
		// from source: reference.md carries the API enumeration AUR-484 moved
		// out of index.md's body, and appears only with two or more pages.
		// Counting either would measure the scaffold, not the scope filter
		// this test exists to prove.
		if filepath.Ext(path) == ".md" && info.Name() != "index.md" && info.Name() != "reference.md" {
			pages = append(pages, info.Name())
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walking output dir %s: %v", outputDir, walkErr)
	}
	return pages
}

func containsAll(haystack, needles []string) bool {
	set := make(map[string]bool, len(haystack))
	for _, h := range haystack {
		set[h] = true
	}
	for _, n := range needles {
		if !set[n] {
			return false
		}
	}
	return true
}

func containsAny(haystack, needles []string) bool {
	set := make(map[string]bool, len(haystack))
	for _, h := range haystack {
		set[h] = true
	}
	for _, n := range needles {
		if set[n] {
			return true
		}
	}
	return false
}

// IntegrationAUR483 is this card's integration selector.
func IntegrationAUR483(t *testing.T) {
	root := aur483Root(t)
	fixture := filepath.Join(root, "tests", "fixtures", "docs", "AUR-483")
	if _, err := os.Stat(fixture); err != nil {
		t.Fatalf("fixture missing at %s: %v", fixture, err)
	}

	t.Run("AC-001_default_excludes_test_scope", func(t *testing.T) {
		outDir := t.TempDir()
		pages := runPipeline(t, fixture, outDir, false)

		if !containsAll(pages, productPages) {
			t.Fatalf("default run missing product page(s); got %v, want all of %v", pages, productPages)
		}
		if containsAny(pages, testScopePages) {
			t.Fatalf("default run documented test-scope page(s); got %v, must exclude %v", pages, testScopePages)
		}
		// The measured drop AC-001 requires: 4 test-scope files (tests/sample.go,
		// testdata/gen.go, fixtures/data.go, internal/core/service_test.go)
		// collapse to 3 test-scope PAGES excluded (tests.md, testdata.md,
		// fixtures.md -- service_test.go shares internal__core.md with the
		// product files in its package, so its exclusion is a file-count drop
		// visible in the log declaration, not a fourth excluded page).
		if len(pages) != len(productPages) {
			t.Fatalf("default run produced %d page(s), want exactly %d (product only): %v", len(pages), len(productPages), pages)
		}
	})

	t.Run("AC-002_config_opt_in_restores_test_scope", func(t *testing.T) {
		outDir := t.TempDir()
		pages := runPipeline(t, fixture, outDir, true)

		if !containsAll(pages, productPages) {
			t.Fatalf("opt-in run missing product page(s); got %v, want all of %v", pages, productPages)
		}
		if !containsAll(pages, restorablePages) {
			t.Fatalf("opt-in run (IncludeTests=true) missing test-scope page(s); got %v, want all of %v -- capability must not be removed", pages, restorablePages)
		}
		want := len(productPages) + len(restorablePages)
		if len(pages) != want {
			t.Fatalf("opt-in run produced %d page(s), want exactly %d (product + restorable test scope): %v", len(pages), want, pages)
		}
	})
}
