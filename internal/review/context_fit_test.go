package review

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	codebasectx "github.com/Mpaape/AurumCode/internal/context"
	"github.com/Mpaape/AurumCode/internal/llm"
	"github.com/Mpaape/AurumCode/internal/prompt"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// hugePack is a codebase context pack of about bytes bytes: the change's
// symbol referenced from every file of a repository.
func hugePack(t *testing.T, bytes int) string {
	t.Helper()
	pack := codebasectx.Pack{Symbols: []string{"Changed"}, Dependents: []string{"other/user.go"}}
	for i := 0; len(mustJSON(t, pack)) < bytes; i++ {
		file := fmt.Sprintf("repo/file%05d.go", i)
		pack.References = append(pack.References, codebasectx.Reference{File: file, Symbol: "Changed", Line: i + 1})
		pack.Snippets = append(pack.Snippets, codebasectx.Snippet{File: file, Line: i + 1, Symbol: "Changed", Kind: "use", Text: strings.Repeat("x := Changed() // excerpt ", 20)})
	}
	return mustJSON(t, pack)
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// A codebase context larger than the window the model refused with would
// leave no room for any change, in every part of the split. It is support,
// not what is reviewed: the review reduces it, finishes, and says so.
func TestContextLargerThanTheWindowIsReducedAndTheReviewFinishes(t *testing.T) {
	model := &windowCompleter{limit: 20000}
	reviewer := NewReviewer(model, DefaultConfig())
	diff := manyFilesDiff(2, 2, 100)
	pack := hugePack(t, 400000) // about 100000 estimated tokens, five windows
	reviewContext := ReviewContext{
		CodebaseContext:    pack,
		CodebaseContextFor: func([]types.DiffFile) string { return pack },
	}

	result, err := reviewer.GenerateReviewWithContext(context.Background(), diff, reviewContext)
	if err != nil {
		t.Fatalf("the review did not finish: %v", err)
	}
	if result == nil || len(model.accepted) == 0 {
		t.Fatalf("no prompt was accepted (refused %v)", model.refused)
	}
	reduction, ok := reviewer.ContextReduction()
	if !ok || !reduction.Reduced() || reduction.KeptTokens >= reduction.OriginalTokens {
		t.Fatalf("ContextReduction() = %+v, %v: the reduction is not declared", reduction, ok)
	}
	if reduction.Discarded || reduction.SymbolsKept != 1 {
		t.Fatalf("ContextReduction() = %+v: the symbols of the change must stay when references go", reduction)
	}
	for _, size := range model.accepted {
		if size > model.limit {
			t.Fatalf("an accepted request of %d tokens is longer than the window", size)
		}
	}
}

// Without a refusal the context goes whole: nothing is reduced on a guess.
func TestContextGoesWholeWithoutARefusal(t *testing.T) {
	model := &windowCompleter{limit: 10000000}
	reviewer := NewReviewer(model, DefaultConfig())
	pack := hugePack(t, 400000)
	if _, err := reviewer.GenerateReviewWithContext(context.Background(), manyFilesDiff(1, 1, 10), ReviewContext{CodebaseContext: pack}); err != nil {
		t.Fatal(err)
	}
	if _, ok := reviewer.ContextReduction(); ok {
		t.Fatal("the context was reduced although the model took the prompt")
	}
	if len(model.accepted) != 1 || model.accepted[0] < len(pack)/3 {
		t.Fatalf("accepted %v: the whole context must go in the one prompt", model.accepted)
	}
}

// Evidence tied to no file goes with every part of a split review. When it
// alone fills the window, it is bounded, the review finishes, and the items
// left out are declared and never counted as seen by the model.
func TestEvidenceOfNoFileLargerThanTheWindowIsBoundedAndDeclared(t *testing.T) {
	model := &windowCompleter{limit: 20000}
	reviewer := NewReviewer(model, DefaultConfig())
	var evidence []prompt.EvidenceItem
	for i := 0; i < 40; i++ {
		evidence = append(evidence, prompt.EvidenceItem{
			ID: fmt.Sprintf("E%d", i+1), Origin: "test", RuleID: "quality/test", Severity: "warning",
			Snippet: strings.Repeat("repository-wide evidence ", 400), // about 2500 tokens each
		})
	}
	if _, err := reviewer.GenerateReviewWithContext(context.Background(), manyFilesDiff(2, 2, 50), ReviewContext{Evidence: evidence}); err != nil {
		t.Fatalf("the review did not finish: %v", err)
	}
	reduction, ok := reviewer.ContextReduction()
	if !ok || reduction.Evidence != 40 || reduction.EvidenceKept >= 40 {
		t.Fatalf("ContextReduction() = %+v, %v: the evidence left out is not declared", reduction, ok)
	}
}

// incidentCompleter answers a prompt longer than its limit the way the CI's
// chain did on 2026-10-14: the main provider's 400 for a string longer than
// it takes, in characters, and the reserve's 429 for a request larger than
// its input tokens per minute, both read by llm.DetectPromptTooLong and
// joined as the fallback chain joins them.
type incidentCompleter struct {
	limit    int
	accepted int
}

func (c *incidentCompleter) CompleteMessages(_ context.Context, messages []llm.Message, _ llm.Options) (llm.Response, error) {
	chars := 0
	for _, m := range messages {
		chars += len(m.Content)
	}
	if chars/3 <= c.limit {
		c.accepted++
		return llm.Response{Text: approveResponse}, nil
	}
	main := fmt.Sprintf("invalid_request_error: Invalid 'input[0].content[0].text': string too long. Expected a string with maximum length %d, but got a string with length %d instead.", c.limit*4, chars)
	reserve := fmt.Sprintf("rate_limit_error: This request would exceed your rate limit of %d input tokens per minute (org: x, model: claude-haiku-5-5).", c.limit)
	return llm.Response{}, fmt.Errorf("all providers failed: %w", errors.Join(
		&llm.APIError{Provider: "litellm", Status: 400, Message: main, TooLong: llm.DetectPromptTooLong(400, main)},
		&llm.APIError{Provider: "anthropic", Status: 429, Message: reserve, TooLong: llm.DetectPromptTooLong(429, reserve)},
	))
}

// The incident itself: both refusals are read as refusals for length, and a
// review whose codebase context is several windows long finishes.
func TestTheIncidentRefusalsSplitTheReviewUntilItFinishes(t *testing.T) {
	model := &incidentCompleter{limit: 20000}
	reviewer := NewReviewer(model, DefaultConfig())
	pack := hugePack(t, 400000)
	result, err := reviewer.GenerateReviewWithContext(context.Background(), manyFilesDiff(2, 2, 100), ReviewContext{CodebaseContext: pack})
	if err != nil || result == nil || model.accepted == 0 {
		t.Fatalf("the review did not finish: %v (accepted %d)", err, model.accepted)
	}
	if refusals := reviewer.PromptRefusals(); len(refusals) == 0 || refusals[0].Limit != 20000 {
		t.Fatalf("PromptRefusals() = %+v: want the smaller window of the chain", refusals)
	}
	if _, ok := reviewer.ContextReduction(); !ok {
		t.Fatal("the context reduction is not declared")
	}
}
