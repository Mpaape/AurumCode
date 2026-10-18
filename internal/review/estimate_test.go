package review

import (
	"context"
	"strings"
	"testing"
)

// #198: the --limite pre-flight counts the prompt the review assembles --
// instructions, policy, codebase context and diff -- not the diff alone,
// and matches what the review then sends.
func TestEstimatePromptTokensCountsTheAssembledPrompt(t *testing.T) {
	model := &windowCompleter{limit: 10_000_000, charsPerToken: 4}
	reviewer := NewReviewer(model, DefaultConfig())
	diff := manyFilesDiff(1, 1, 20)
	pack := `{"symbols":["Handler"],"references":[{"file":"use.go","symbol":"Handler","line":1}],"dropped":[],"note":"` + strings.Repeat("context ", 4000) + `"}`

	bare := reviewer.EstimatePromptTokens(diff, ReviewContext{})
	withContext := reviewer.EstimatePromptTokens(diff, ReviewContext{CodebaseContext: pack})
	if bare <= 0 || withContext-bare < len(pack)/4*9/10 {
		t.Fatalf("estimate without context %d, with %d: the codebase context (about %d tokens) is not counted", bare, withContext, len(pack)/4)
	}
	if _, err := reviewer.GenerateReviewWithContext(context.Background(), diff, ReviewContext{CodebaseContext: pack}); err != nil {
		t.Fatal(err)
	}
	if len(model.accepted) != 1 {
		t.Fatalf("accepted %v", model.accepted)
	}
	if sent := model.accepted[0]; sent < withContext*9/10 || sent > withContext*11/10 {
		t.Fatalf("pre-flight %d tokens, the review sent %d", withContext, sent)
	}
}

// Under a declared window that splits the review, the estimate is the sum
// of the batches, not the first prompt alone.
func TestEstimatePromptTokensSumsTheBatches(t *testing.T) {
	reviewer := NewReviewer(&windowCompleter{limit: 20000}, DefaultConfig())
	diff := manyFilesDiff(3, 2, 400)
	whole := reviewer.EstimatePromptTokens(diff, ReviewContext{})
	reviewer.SetModelWindow(20000)
	split := reviewer.EstimatePromptTokens(diff, ReviewContext{})
	if split <= whole {
		t.Fatalf("split estimate %d, whole %d: the batches each repeat the instructions", split, whole)
	}
	if len(reviewer.Batches()) != 0 {
		t.Fatal("an estimate must leave no batch behind")
	}
}
