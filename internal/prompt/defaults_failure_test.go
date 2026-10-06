package prompt

import (
	"errors"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/Mpaape/AurumCode/internal/analyzer"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// The embedded defaults are compiled into the binary, so their failure is a
// build defect. These tests pin that such a defect is returned as an error
// at every assembly entry point -- never a panic at package initialization
// and never a prompt assembled with zero, unbounded ceilings.

func TestEmbeddedDefaultsLoad(t *testing.T) {
	if err := EmbeddedDefaultsErr(); err != nil {
		t.Fatalf("embedded defaults failed to load: %v", err)
	}
	if DefaultLimits().PromptMaxTokens <= 0 || len(DefaultRuleCatalog) == 0 {
		t.Fatalf("embedded defaults are empty: %+v, %d rules", DefaultLimits(), len(DefaultRuleCatalog))
	}
}

func TestLoadSlotLimitsReturnsErrors(t *testing.T) {
	cases := map[string]fstest.MapFS{
		"missing":      {},
		"malformed":    {limitsFile: {Data: []byte("prompt_max_tokens: [unclosed\n")}},
		"non-positive": {limitsFile: {Data: []byte("prompt_max_tokens: 0\nrule_catalog_max_tokens: 1\nevidence_max_tokens: 1\ntools_max_tokens: 1\n")}},
	}
	for name, fsys := range cases {
		t.Run(name, func(t *testing.T) {
			limits, err := loadSlotLimits(fsys)
			if err == nil {
				t.Fatalf("loadSlotLimits accepted a %s limits file: %+v", name, limits)
			}
			if !strings.Contains(err.Error(), limitsFile) {
				t.Fatalf("error does not name %s: %v", limitsFile, err)
			}
		})
	}
}

func TestCatalogIDsReturnsLoaderError(t *testing.T) {
	cause := errors.New("embedded rules unreadable")
	ids, err := catalogIDs(func() ([]string, error) { return nil, cause })
	if !errors.Is(err, cause) || ids != nil {
		t.Fatalf("catalogIDs = %v, %v; want nil and the loader's error", ids, err)
	}
	if !strings.Contains(err.Error(), "review rule catalog unavailable") {
		t.Fatalf("error does not say the catalog is unavailable: %v", err)
	}
}

func TestBuilderRefusesAssemblyWhenDefaultsFailed(t *testing.T) {
	cause := errors.New("embedded defaults did not load")
	b := NewPromptBuilder()
	b.defaultsErr = cause
	diff := &types.Diff{Files: []types.DiffFile{{Path: "a.go", Hunks: []types.DiffHunk{{Lines: []string{"+x"}}}}}}
	metrics := &analyzer.DiffMetrics{}

	if _, err := b.BuildPrompt(diff, metrics, BuildOptions{SchemaKind: "review"}); !errors.Is(err, cause) {
		t.Fatalf("BuildPrompt error = %v, want %v", err, cause)
	}
	if _, err := b.FixedOverheadTokens(diff, metrics, BuildOptions{SchemaKind: "review"}); !errors.Is(err, cause) {
		t.Fatalf("FixedOverheadTokens error = %v, want %v", err, cause)
	}
	if _, err := b.FixedContentDigest(); !errors.Is(err, cause) {
		t.Fatalf("FixedContentDigest error = %v, want %v", err, cause)
	}
	if _, err := b.ruleCatalogSection(); !errors.Is(err, cause) {
		t.Fatalf("ruleCatalogSection error = %v, want %v", err, cause)
	}
}
