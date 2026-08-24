package main

import (
	"fmt"
	"strings"

	"github.com/Mpaape/AurumCode/internal/analyzer"
	"github.com/Mpaape/AurumCode/internal/prompt"
	"github.com/Mpaape/AurumCode/pkg/types"
)

func main() {
	diff := &types.Diff{Files: []types.DiffFile{{
		Path: "src/two.mjs",
		Hunks: []types.DiffHunk{
			{Lines: []string{"+hunk0 " + strings.Repeat("a", 100)}},
			{Lines: []string{"+hunk1 " + strings.Repeat("b", 100)}},
		},
	}}}
	metrics := analyzer.NewDiffAnalyzer().AnalyzeDiff(diff)
	parts, err := prompt.NewPromptBuilder().BuildPrompt(diff, metrics, prompt.BuildOptions{
		MaxTokens: 1470, SchemaKind: "review", Role: "reviewer", ReserveReply: 20,
	})
	if err != nil {
		fmt.Println("ERR", err)
		return
	}
	fmt.Println("hunk0 present:", strings.Contains(parts.User, "hunk0"))
	fmt.Println("hunk1 present:", strings.Contains(parts.User, "hunk1"))
	fmt.Println("Meta:", parts.Meta)

	// direct primitive check
	est := prompt.NewHeuristicEstimator()
	detector := analyzer.NewLanguageDetector()
	budget := prompt.NewTokenBudget(est, 1470, 20)
	segs := budget.BuildContextSegments(diff, detector)
	for _, s := range segs {
		fmt.Printf("segment %s tokens=%d\n", s.SortKey, s.Tokens)
	}
	fmt.Println("Available (0 base):", budget.Available())
}
