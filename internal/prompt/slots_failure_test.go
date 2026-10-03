package prompt

import (
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/analyzer"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// TestSlotFailureIsAnErrorNeverAPanic: a slot executed with data its
// template cannot read returns a failure the prompt boundary refuses; the
// process keeps running and no prompt reaches the model.
func TestSlotFailureIsAnErrorNeverAPanic(t *testing.T) {
	text := renderSlot(slotEvidenceItem, 42)
	err := slotRenderError("before", text)
	if err == nil || !strings.Contains(err.Error(), slotEvidenceItem) {
		t.Fatalf("a slot that cannot render must name itself in an error, got %v (text %q)", err, text)
	}
	if slotRenderError(renderSlot(slotCoverage, "fine")) != nil {
		t.Fatal("a slot that renders must not be reported as failed")
	}

	b := NewPromptBuilder()
	diff := &types.Diff{Files: []types.DiffFile{{Path: "main.go", Hunks: []types.DiffHunk{{NewStart: 1, Lines: []string{"+run()"}}}}}}
	metrics := analyzer.NewDiffAnalyzer().AnalyzeDiff(diff)
	opts := BuildOptions{SchemaKind: "review", RepositoryContext: text}
	if parts, err := b.BuildPrompt(diff, metrics, opts); err == nil {
		t.Fatalf("a prompt carrying a failed slot must be refused, got %q", parts.User)
	}
	if _, err := b.FixedOverheadTokens(diff, metrics, opts); err == nil {
		t.Fatal("the fixed overhead of a prompt carrying a failed slot must be refused")
	}
}
