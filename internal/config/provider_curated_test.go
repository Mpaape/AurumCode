package config

import (
	"context"
	"strings"
	"testing"
)

// curatedTextProvider is a text provider that declares itself the policy's
// own file.
type curatedTextProvider struct{ pendingTextProvider }

func (curatedTextProvider) Curated() {}

// A contribution of any size reaches the model (issue #179): neither the
// repository's and the policy's own files nor an external source are
// refused or cut by size; the old 64 KiB ceiling of an external source is
// gone.
func TestNoProviderIsBoundedByAContributionCeiling(t *testing.T) {
	big := strings.Repeat("x", 64*1024+1)

	block, _, err := BuildContextBlockWithWarnings(context.Background(),
		[]ContextProvider{curatedTextProvider{pendingTextProvider{name: "skills", text: big}}}, []string{"a.go"}, nil)
	if err != nil {
		t.Fatalf("a curated provider past the old ceiling was refused: %v", err)
	}
	if !strings.Contains(block, big) {
		t.Fatalf("the curated contribution was cut: the block has %d bytes", len(block))
	}

	block, warnings, err := BuildContextBlockWithWarnings(context.Background(),
		[]ContextProvider{pendingTextProvider{name: "mcp", text: big}}, []string{"a.go"}, nil)
	if err != nil || len(warnings) != 0 {
		t.Fatalf("an external provider past the old ceiling was refused: err=%v warnings=%v", err, warnings)
	}
	if !strings.Contains(block, big) {
		t.Fatalf("the external contribution was cut: the block has %d bytes", len(block))
	}
}
