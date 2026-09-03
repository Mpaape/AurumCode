// Unit proof for card AUR-483: "A documentacao gerada e do produto, nao dos
// testes dele."
//
// This program tests internal/pipeline.IsTestScopePath and
// (*ExtractorPipeline).IncludeTests directly -- the two exported seams
// AUR-483 added to internal/pipeline/extractor_pipeline.go. It is
// deliberately narrow (pure function calls, no filesystem walk, no
// extraction): the broader file-discovery behavior against a real tree is
// tests/integration/AUR-483.go's job.
//
// TestAUR483 covers:
//   - AC-001 shape: tests/, testdata/, fixtures/ directory components and
//     *_test file stems are test scope.
//   - AC-003, the card's named trap: internal/attestation, cmd/latest and a
//     file literally named contest.go each CONTAIN the substring "test" but
//     are never caught, because the match is by whole path component / whole
//     filename stem, never by substring.
//   - AC-002 shape: IncludeTests() is false by default, true when
//     config.IncludeTests is set, and true when the AURUMCODE_-prefixed
//     environment override is set -- proving the capability was not removed,
//     only defaulted off.
package unit

import (
	"os"
	"testing"

	"github.com/Mpaape/AurumCode/internal/pipeline"
)

// TestAUR483 is this card's unit selector (tests/acceptance/AUR-483.sh
// dispatches to it via a bridge _test.go, the same technique the sibling
// AUR-450/AUR-467 unit programs use).
func TestAUR483(t *testing.T) {
	t.Run("AC-001_test_scope_excluded", func(t *testing.T) {
		cases := []string{
			"tests/sample.go",
			"a/b/tests/sample.go",
			"testdata/gen.go",
			"a/testdata/gen.go",
			"fixtures/data.go",
			"a/b/fixtures/data.go",
			"internal/core/service_test.go",
			"service_test.go",
		}
		for _, path := range cases {
			if !pipeline.IsTestScopePath(path) {
				t.Errorf("IsTestScopePath(%q) = false, want true (test/fixture scope)", path)
			}
		}
	})

	t.Run("AC-003_substring_trap_never_swallows_product_code", func(t *testing.T) {
		// Every one of these paths CONTAINS the substring "test" somewhere,
		// but none of them has a path component equal to "tests"/"testdata"/
		// "fixtures", nor a filename stem that ends in "_test". A substring
		// matcher would wrongly exclude all four; a component matcher must
		// not.
		cases := []string{
			"internal/attestation/report.go",
			"internal/attestation",
			"cmd/latest/main.go",
			"cmd/latest",
			"internal/core/contest.go",
			"internal/protest/handler.go",
			"internal/core/contest_helper.go",
		}
		for _, path := range cases {
			if pipeline.IsTestScopePath(path) {
				t.Errorf("IsTestScopePath(%q) = true, want false (product code, substring trap)", path)
			}
		}
	})

	t.Run("AC-002_include_tests_default_off_and_configurable", func(t *testing.T) {
		const envName = "AURUMCODE_INCLUDE_TEST_DOCS"
		os.Unsetenv(envName)

		cfg := &pipeline.ExtractorPipelineConfig{SourceDir: "."}
		p := pipeline.NewExtractorPipeline(cfg, nil, nil)
		if p.IncludeTests() {
			t.Fatalf("IncludeTests() = true with no config and no env set, want false (default excludes tests)")
		}

		cfgOn := &pipeline.ExtractorPipelineConfig{SourceDir: ".", IncludeTests: true}
		pOn := pipeline.NewExtractorPipeline(cfgOn, nil, nil)
		if !pOn.IncludeTests() {
			t.Fatalf("IncludeTests() = false with config.IncludeTests=true, want true")
		}

		if err := os.Setenv(envName, "true"); err != nil {
			t.Fatalf("setting %s: %v", envName, err)
		}
		defer os.Unsetenv(envName)
		pEnv := pipeline.NewExtractorPipeline(cfg, nil, nil)
		if !pEnv.IncludeTests() {
			t.Fatalf("IncludeTests() = false with %s=true, want true (env override reaches every entrypoint)", envName)
		}
	})
}
