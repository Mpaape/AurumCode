package review

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	codebasectx "github.com/Mpaape/AurumCode/internal/context"
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
