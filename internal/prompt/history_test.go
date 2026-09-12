package prompt

import (
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/analyzer"
	"github.com/Mpaape/AurumCode/pkg/types"
)

func TestHistoryIsPreservedAndAccountedForInExplicitBudget(t *testing.T) {
	b := NewPromptBuilder()
	diff := &types.Diff{Files: []types.DiffFile{{Path: "main.go", Hunks: []types.DiffHunk{{NewStart: 1, Lines: []string{"+run()"}}}}}}
	metrics := analyzer.NewDiffAnalyzer().AnalyzeDiff(diff)
	history := strings.Repeat("An attributed prior observation. ", 3000) + "LAST_AUTHOR_CORRECTION"
	opts := BuildOptions{SchemaKind: "review", ReviewHistory: history}
	parts, err := b.BuildPrompt(diff, metrics, opts)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(parts.User, history) != 1 || strings.Contains(parts.System, history) {
		t.Fatal("history must be complete and present once, as user evidence")
	}
	// Derive a budget that fits the original prompt, but not the history.
	baseline, err := b.BuildPrompt(diff, metrics, BuildOptions{SchemaKind: "review"})
	if err != nil {
		t.Fatal(err)
	}
	opts.MaxTokens = b.estimator.Estimate(baseline.System+baseline.User) * 2
	if _, err := b.BuildPrompt(diff, metrics, opts); err == nil {
		t.Fatal("history bypassed the explicit prompt budget")
	}
}
