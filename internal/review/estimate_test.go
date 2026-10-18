package review

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/pkg/types"
)

// #198: the --limite pre-flight counts the prompt the review assembles --
// instructions, policy, codebase context and diff -- not the diff alone,
// and matches what the review then sends.
func TestEstimatePromptTokensCountsTheAssembledPrompt(t *testing.T) {
	model := &windowCompleter{limit: 10_000_000, charsPerToken: 4}
	reviewer := NewReviewer(model, DefaultConfig())
	diff := manyFilesDiff(1, 1, 20)
	pack := `{"symbols":["Handler"],"references":[{"file":"use.go","symbol":"Handler","line":1}],"dropped":[],"note":"` + strings.Repeat("context ", 4000) + `"}`

	bare := mustEstimate(t, reviewer, diff, ReviewContext{})
	withContext := mustEstimate(t, reviewer, diff, ReviewContext{CodebaseContext: pack})
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
	whole := mustEstimate(t, reviewer, diff, ReviewContext{})
	reviewer.SetModelWindow(20000)
	split := mustEstimate(t, reviewer, diff, ReviewContext{})
	if split <= whole {
		t.Fatalf("split estimate %d, whole %d: the batches each repeat the instructions", split, whole)
	}
	if len(reviewer.Batches()) != 0 {
		t.Fatal("an estimate must leave no batch behind")
	}
}

func mustEstimate(t *testing.T, reviewer *Reviewer, diff *types.Diff, reviewContext ReviewContext) int {
	t.Helper()
	tokens, err := reviewer.EstimatePromptTokens(diff, reviewContext)
	if err != nil {
		t.Fatal(err)
	}
	return tokens
}

// A batches.max_prompt_tokens that admits no batch leaves nothing to price:
// the estimate says why instead of a count of zero, and the reason is the
// one the review itself would stop with.
func TestEstimatePromptTokensSaysWhyNoBatchIsAdmitted(t *testing.T) {
	reviewer := NewReviewer(&windowCompleter{limit: 20000}, DefaultConfig())
	reviewer.SetModelWindow(20000)
	reviewer.SetBatchLimits(BatchLimits{MaxPromptTokens: 10})
	tokens, err := reviewer.EstimatePromptTokens(manyFilesDiff(3, 2, 400), ReviewContext{})
	if !errors.Is(err, ErrNoBatchReviewed) {
		t.Fatalf("estimate %d tokens, error %v: want ErrNoBatchReviewed", tokens, err)
	}
	if !strings.Contains(err.Error(), " files left out") {
		t.Fatalf("the reason does not count the files left out: %v", err)
	}
}

func TestCountOfAgreesWithTheNumber(t *testing.T) {
	for n, want := range map[int]string{0: "0 files", 1: "1 file", 2: "2 files"} {
		if got := countOf(n, "file", "files"); got != want {
			t.Fatalf("countOf(%d) = %q, want %q", n, got, want)
		}
	}
}
