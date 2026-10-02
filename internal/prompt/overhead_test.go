package prompt

import (
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/analyzer"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// TestAUR539FixedOverheadRefusalBoundary is AUR-539's AC-002/AC-003 anchor:
// FixedOverheadTokens measures exactly the floor BuildPrompt refuses at (a
// budget with zero room above it still refuses -- the AUR-467 guard this
// card must not weaken), and a budget with real room above that SAME
// measured floor succeeds. Any future growth in the fixed prompt content
// (instructions, rule catalog, schema) moves this boundary automatically,
// because both sides of the assertion are derived from the one live
// measurement -- never a literal that can drift out of sync with it.
func TestAUR539FixedOverheadRefusalBoundary(t *testing.T) {
	b := NewPromptBuilder()
	opts := BuildOptions{SchemaKind: "review", Role: "reviewer"}

	empty := &types.Diff{}
	emptyMetrics := analyzer.NewDiffAnalyzer().AnalyzeDiff(empty)
	fixed, err := b.FixedOverheadTokens(empty, emptyMetrics, opts)
	if err != nil {
		t.Fatalf("FixedOverheadTokens failed: %v", err)
	}
	if fixed <= 0 {
		t.Fatalf("FixedOverheadTokens = %d, want > 0", fixed)
	}

	// AC-002: at MaxTokens == the measured fixed overhead there is zero room
	// left for a single code change -- BuildPrompt must still refuse, named
	// clearly, not silently ship a diff-less prompt.
	oneFile := &types.Diff{Files: []types.DiffFile{{
		Path:  "a.go",
		Hunks: []types.DiffHunk{{Lines: []string{"+x"}}},
	}}}
	oneFileMetrics := analyzer.NewDiffAnalyzer().AnalyzeDiff(oneFile)
	_, err = b.BuildPrompt(oneFile, oneFileMetrics, BuildOptions{
		MaxTokens: fixed, SchemaKind: "review", Role: "reviewer",
	})
	if err == nil {
		t.Fatalf("expected refusal at MaxTokens == FixedOverheadTokens (%d): no room for the diff", fixed)
	}
	if !strings.Contains(err.Error(), "no room for a single code change") {
		t.Fatalf("refusal message changed, update this anchor: %v", err)
	}

	// AC-003: the SAME measured floor, with real room above it, must admit
	// the diff -- proving the budget tracks the builder's actual fixed
	// content instead of a number that can fall out of date with it.
	fixed2, err := b.FixedOverheadTokens(oneFile, oneFileMetrics, opts)
	if err != nil {
		t.Fatalf("FixedOverheadTokens failed: %v", err)
	}
	parts, err := b.BuildPrompt(oneFile, oneFileMetrics, BuildOptions{
		MaxTokens: fixed2 + 500, SchemaKind: "review", Role: "reviewer",
	})
	if err != nil {
		t.Fatalf("unexpected refusal with room above the measured floor (fixed2=%d): %v", fixed2, err)
	}
	if got := parts.Meta["code_files_complete"]; got != "1" {
		t.Fatalf("Meta[code_files_complete] = %q, want \"1\" once there is room", got)
	}
}
