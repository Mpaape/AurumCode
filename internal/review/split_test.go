package review

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/Mpaape/AurumCode/internal/llm"
	"github.com/Mpaape/AurumCode/internal/prompt"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// windowCompleter is a model whose context window is limit tokens. A longer
// prompt is refused the way real providers refuse it, in the provider's own
// count: one token per three bytes here, denser than the product's estimate
// of one per four, as a real tokenizer is for code.
type windowCompleter struct {
	limit int
	// charsPerToken is the provider's count, three bytes a token when zero.
	charsPerToken float64
	mu            sync.Mutex
	accepted      []int
	refused       []int
	// sawGlobal is set when an accepted request carries the marker of the
	// whole diff's codebase context.
	sawGlobal bool
}

func (w *windowCompleter) CompleteMessages(_ context.Context, messages []llm.Message, _ llm.Options) (llm.Response, error) {
	bytes := 0
	for _, m := range messages {
		bytes += len(m.Content)
	}
	perToken := w.charsPerToken
	if perToken == 0 {
		perToken = 3
	}
	tokens := int(float64(bytes) / perToken)
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, m := range messages {
		if tokens <= w.limit && strings.Contains(m.Content, "GLOBAL-CONTEXT-OF-THE-WHOLE-DIFF") {
			w.sawGlobal = true
		}
	}
	if tokens > w.limit {
		w.refused = append(w.refused, tokens)
		return llm.Response{}, &llm.APIError{
			Provider: "fake", Status: 400,
			Message: fmt.Sprintf("prompt is too long: %d tokens > %d maximum", tokens, w.limit),
			TooLong: &llm.PromptTooLong{Limit: w.limit, Requested: tokens},
		}
	}
	w.accepted = append(w.accepted, tokens)
	return llm.Response{Text: approveResponse}, nil
}

// manyFilesDiff is dirs directories of perDir Go files, each a hunk of lines
// added lines.
func manyFilesDiff(dirs, perDir, lines int) *types.Diff {
	diff := &types.Diff{}
	for d := 0; d < dirs; d++ {
		for f := 0; f < perDir; f++ {
			added := make([]string, 0, lines)
			for l := 0; l < lines; l++ {
				added = append(added, fmt.Sprintf("+var value%d_%d_%d = \"alpha beta gamma delta epsilon\"", d, f, l))
			}
			diff.Files = append(diff.Files, types.DiffFile{
				Path: fmt.Sprintf("pkg%d/file%d.go", d, f), Lang: "go",
				Hunks: []types.DiffHunk{{OldStart: 1, NewStart: 1, NewLines: lines, Lines: added}},
			})
		}
	}
	return diff
}

// The product sends the whole diff. A model that refuses it as too long gets
// it back in smaller parts, every file reviewed, and no request the model
// takes is longer than its window.
func TestRefusedPromptIsSplitUntilTheModelTakesIt(t *testing.T) {
	model := &windowCompleter{limit: 20000}
	reviewer := NewReviewer(model, DefaultConfig())
	diff := manyFilesDiff(3, 2, 400)

	result, err := reviewer.GenerateReviewWithContext(context.Background(), diff, ReviewContext{})
	if err != nil {
		t.Fatalf("a diff the model could take in parts failed: %v", err)
	}
	if len(model.refused) == 0 || len(reviewer.PromptRefusals()) == 0 {
		t.Fatalf("the model took the whole prompt: the test does not exercise the split (refused %v)", model.refused)
	}
	first := reviewer.PromptRefusals()[0]
	if first.Limit != 20000 || first.Requested != model.refused[0] || first.EstimatedTokens <= 0 {
		t.Fatalf("first refusal = %+v, want the model's own sizes", first)
	}
	covered := map[string]bool{}
	for _, b := range reviewer.Batches() {
		for _, f := range b.Files {
			covered[f] = true
		}
	}
	if len(reviewer.Batches()) < 2 || len(covered) != len(diff.Files) {
		t.Fatalf("%d batches cover %d of %d files", len(reviewer.Batches()), len(covered), len(diff.Files))
	}
	if result.Metadata[MetaOmittedPaths] != "" {
		t.Fatalf("files were left out: %q", result.Metadata[MetaOmittedPaths])
	}
	for _, tokens := range model.accepted {
		if tokens > model.limit {
			t.Fatalf("a request of %d tokens was accepted past the %d-token window", tokens, model.limit)
		}
	}
}

// A model that takes the whole prompt is asked once: no refusal, no batch.
func TestPromptTheModelTakesIsSentWhole(t *testing.T) {
	model := &windowCompleter{limit: 10_000_000}
	reviewer := NewReviewer(model, DefaultConfig())
	if _, err := reviewer.GenerateReviewWithContext(context.Background(), manyFilesDiff(3, 2, 400), ReviewContext{}); err != nil {
		t.Fatal(err)
	}
	if len(model.accepted) != 1 || len(model.refused) != 0 || len(reviewer.Batches()) != 0 || len(reviewer.PromptRefusals()) != 0 {
		t.Fatalf("accepted %v refused %v batches %d: the whole prompt must go once", model.accepted, model.refused, len(reviewer.Batches()))
	}
}

// When the instructions and the policy alone are longer than the window, no
// split helps: the review fails loudly and says why.
func TestSplitEndsWhenTheInstructionsAloneDoNotFit(t *testing.T) {
	model := &windowCompleter{limit: 500}
	reviewer := NewReviewer(model, DefaultConfig())
	_, err := reviewer.GenerateReviewWithContext(context.Background(), manyFilesDiff(2, 1, 50), ReviewContext{})
	var noRoom *prompt.NoRoomError
	if err == nil || !errors.As(err, &noRoom) {
		t.Fatalf("error = %v, want one wrapping prompt.NoRoomError", err)
	}
	for _, want := range []string{"the model refused a prompt of about", "what goes with every part (instructions, policy, context and evidence)"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not say %q", err, want)
		}
	}
}

// Only a refusal for length is answered by splitting.
func TestOtherProviderErrorsAreNotRetried(t *testing.T) {
	calls := 0
	reviewer := NewReviewer(completerFunc(func() (llm.Response, error) {
		calls++
		return llm.Response{}, &llm.APIError{Provider: "fake", Status: 401, Message: "invalid_api_key"}
	}), DefaultConfig())
	_, err := reviewer.GenerateReviewWithContext(context.Background(), manyFilesDiff(2, 1, 50), ReviewContext{})
	if err == nil || calls != 1 || len(reviewer.PromptRefusals()) != 0 {
		t.Fatalf("calls %d, refusals %v, err %v: an authentication error must be returned at once", calls, reviewer.PromptRefusals(), err)
	}
}

type completerFunc func() (llm.Response, error)

func (f completerFunc) CompleteMessages(context.Context, []llm.Message, llm.Options) (llm.Response, error) {
	return f()
}

func TestLearnedCeiling(t *testing.T) {
	cases := []struct {
		name     string
		refusal  PromptRefusal
		previous int
		want     int
	}{
		{"keeps the window's share, with a margin", PromptRefusal{EstimatedTokens: 100000, Limit: 128000, Requested: 400000}, 0, 28800},
		{"halves when the provider said nothing", PromptRefusal{EstimatedTokens: 100000}, 0, 50000},
		{"never keeps the whole prompt", PromptRefusal{EstimatedTokens: 1000, Limit: 128000, Requested: 128001}, 0, 899},
		{"goes below the previous ceiling", PromptRefusal{EstimatedTokens: 28000, Limit: 128000, Requested: 128001}, 25000, 12500},
		{"nothing left", PromptRefusal{EstimatedTokens: 1}, 0, 0},
		{"only the window said: its share", PromptRefusal{EstimatedTokens: 3900000, Limit: 2000000}, 0, 1575000},
		{"only the window said, larger than the prompt", PromptRefusal{EstimatedTokens: 1000, Limit: 2000000}, 0, 999},
	}
	for _, c := range cases {
		if got := learnedCeiling(c.refusal, c.previous); got != c.want {
			t.Errorf("%s: learnedCeiling(%+v, %d) = %d, want %d", c.name, c.refusal, c.previous, got, c.want)
		}
	}
}

// The codebase context of the whole diff can fill a window by itself. Each
// batch of a split review carries the context of its own files, so the split
// is not defeated by what goes with every part.
func TestBatchesCarryTheCodebaseContextOfTheirOwnFiles(t *testing.T) {
	model := &windowCompleter{limit: 20000}
	reviewer := NewReviewer(model, DefaultConfig())
	diff := manyFilesDiff(3, 2, 400)
	global := strings.Repeat("GLOBAL-CONTEXT-OF-THE-WHOLE-DIFF ", 4000) // about 33000 tokens
	reviewContext := ReviewContext{
		CodebaseContext: global,
		CodebaseContextFor: func(files []types.DiffFile) string {
			return fmt.Sprintf("context of %d file(s)", len(files))
		},
	}

	if _, err := reviewer.GenerateReviewWithContext(context.Background(), diff, reviewContext); err != nil {
		t.Fatalf("a split whose parts carry their own context failed: %v", err)
	}
	if len(reviewer.Batches()) < 2 {
		t.Fatalf("%d batches: the test does not exercise the split", len(reviewer.Batches()))
	}
	if model.sawGlobal {
		t.Fatal("a batch carried the codebase context of the whole diff")
	}
}

// cutShortCompleter answers a long prompt with a reply cut off by the model's
// length cap, which fails to parse, and a short one with a whole reply.
type cutShortCompleter struct {
	limit int
	cuts  int
}

func (c *cutShortCompleter) CompleteMessages(_ context.Context, messages []llm.Message, _ llm.Options) (llm.Response, error) {
	bytes := 0
	for _, m := range messages {
		bytes += len(m.Content)
	}
	if bytes/4 > c.limit {
		c.cuts++
		return llm.Response{Text: `{"verdict":"approve","issues":[{"file":"a.go","li`, FinishReason: "length"}, nil
	}
	return llm.Response{Text: approveResponse}, nil
}

// A reply the model cut short is a prompt that left too little room: the
// review splits it, as it splits a prompt the model refused.
func TestReplyCutShortSplitsTheReview(t *testing.T) {
	model := &cutShortCompleter{limit: 20000}
	reviewer := NewReviewer(model, DefaultConfig())
	diff := manyFilesDiff(3, 2, 400)

	if _, err := reviewer.GenerateReviewWithContext(context.Background(), diff, ReviewContext{}); err != nil {
		t.Fatalf("a reply cut short was not answered by splitting: %v", err)
	}
	refusals := reviewer.PromptRefusals()
	if model.cuts == 0 || len(refusals) == 0 || !refusals[0].Cut || len(reviewer.Batches()) < 2 {
		t.Fatalf("cuts %d refusals %+v batches %d: the review must split after a reply cut short", model.cuts, refusals, len(reviewer.Batches()))
	}
}

// A reply that is not cut short but does not parse is still the deterministic
// fallback's business, never a reason to split.
func TestUnparseableReplyIsNotSplit(t *testing.T) {
	calls := 0
	reviewer := NewReviewer(completerFunc(func() (llm.Response, error) {
		calls++
		return llm.Response{Text: "not json", FinishReason: "stop"}, nil
	}), DefaultConfig())
	_, _ = reviewer.GenerateReviewWithContext(context.Background(), manyFilesDiff(2, 1, 50), ReviewContext{})
	if calls != 1 || len(reviewer.PromptRefusals()) != 0 {
		t.Fatalf("calls %d refusals %v: an unparseable reply must not split the review", calls, reviewer.PromptRefusals())
	}
}

// What goes with some files can fill the ceiling by itself. The plan does not
// fail for it: those files do not fit together, and each one, whose own
// evidence fits, gets a batch.
func TestPlanBatchesTreatsEvidenceThatFillsTheCeilingAsNotFitting(t *testing.T) {
	reviewer := NewReviewer(&windowCompleter{limit: 1 << 30}, DefaultConfig())
	reviewer.learnedBudget = 12000
	diff := manyFilesDiff(1, 2, 20)
	var evidence []prompt.EvidenceItem
	for i, f := range diff.Files {
		evidence = append(evidence, prompt.EvidenceItem{
			ID: fmt.Sprintf("E%d", i+1), Origin: "test", RuleID: "quality/test", File: f.Path, Line: 1,
			Severity: "warning", Snippet: strings.Repeat("evidence ", 2700), // about 6000 tokens for each file
		})
	}

	plan, err := reviewer.planBatches(diff, ReviewContext{Evidence: evidence})
	if err != nil {
		t.Fatalf("the plan failed instead of giving each file a batch: %v", err)
	}
	if len(plan.batches) != 2 || len(plan.outside) != 0 {
		t.Fatalf("%d batches, %d files outside: want one batch per file", len(plan.batches), len(plan.outside))
	}
}

// #198: a declared window sizes the prompt from the first request. The
// model counts 3.5 bytes a token, denser than the product's estimate of
// four, and the window declared is the model's own: the batches fill up to
// the ceiling, the model refuses nothing, every file is reviewed, and no
// request is longer than the window.
func TestDeclaredWindowSplitsWithoutARefusal(t *testing.T) {
	model := &windowCompleter{limit: 20000, charsPerToken: 3.5}
	reviewer := NewReviewer(model, DefaultConfig())
	reviewer.SetModelWindow(20000)
	// Many small files: the packing fills each batch close to the ceiling.
	diff := manyFilesDiff(8, 20, 10)

	result, err := reviewer.GenerateReviewWithContext(context.Background(), diff, ReviewContext{})
	if err != nil {
		t.Fatalf("review under a declared window failed: %v", err)
	}
	if len(model.refused) != 0 || len(reviewer.PromptRefusals()) != 0 {
		t.Fatalf("a declared window must spare the refusals: refused %v", model.refused)
	}
	if len(model.accepted) < 2 {
		t.Fatalf("the diff should have gone in batches, got %d request(s)", len(model.accepted))
	}
	largest := 0
	for _, tokens := range model.accepted {
		if tokens > 20000 {
			t.Fatalf("a request of %d tokens crossed the window", tokens)
		}
		largest = max(largest, tokens)
	}
	if largest < 16000 {
		t.Fatalf("the largest batch took %d tokens: the batches do not fill the ceiling, so the margin is not under test", largest)
	}
	if result == nil {
		t.Fatal("no result")
	}
}

func TestDeclaredCeilingKeepsTheReplyRoom(t *testing.T) {
	if got := declaredCeiling(0); got != 0 {
		t.Fatalf("no window: %d", got)
	}
	// A prompt estimated at the ceiling, four bytes a token, counted at
	// 3.5 bytes a token, still leaves the reply room in the window.
	window := 262144
	ceiling := declaredCeiling(window)
	if counted := float64(ceiling) * 4 / 3.5; counted > float64(window)*promptMargin {
		t.Fatalf("declaredCeiling(%d) = %d: counted at 3.5 bytes a token it takes %.0f tokens", window, ceiling, counted)
	}
}

// A declared window too small for what goes with every part never fails
// the review: it is set aside with a notice, and the window is learned from
// the model's refusal as if none had been declared.
func TestDeclaredWindowTooSmallIsSetAsideWithANotice(t *testing.T) {
	model := &windowCompleter{limit: 20000}
	reviewer := NewReviewer(model, DefaultConfig())
	reviewer.SetModelWindow(100)
	var notices []string
	reviewer.SetNotifier(func(s string) { notices = append(notices, s) })

	result, err := reviewer.GenerateReviewWithContext(context.Background(), manyFilesDiff(3, 2, 400), ReviewContext{})
	if err != nil {
		t.Fatalf("a declared window too small failed the review: %v", err)
	}
	if result == nil || len(model.accepted) == 0 {
		t.Fatal("the model never reviewed anything")
	}
	if len(notices) != 1 || !strings.Contains(notices[0], "declared model window (100 tokens") || !strings.Contains(notices[0], "as if no window were declared") {
		t.Fatalf("notices = %q, want one naming the window set aside", notices)
	}
	// With nothing declared, the first request is the whole diff.
	if len(model.refused) == 0 {
		t.Fatal("the review did not fall back to learning the window from a refusal")
	}
}

// An explicit prompt budget the caller set is not the declared window: its
// lack of room is not set aside.
func TestExplicitBudgetBelowTheFixedPartIsNotSetAside(t *testing.T) {
	cfg := DefaultConfig()
	cfg.PromptTokenBudget = 50
	reviewer := NewReviewer(&windowCompleter{limit: 20000}, cfg)
	reviewer.SetModelWindow(100)
	_, err := reviewer.GenerateReviewWithContext(context.Background(), manyFilesDiff(1, 1, 10), ReviewContext{})
	var noRoom *prompt.NoRoomError
	if !errors.As(err, &noRoom) {
		t.Fatalf("error = %v, want the caller's own budget's NoRoomError", err)
	}
}

// The ceiling that leaves the first batch out is named, and the error still
// takes the path of a model review that did not run.
func TestNoBatchReviewedNamesTheCeiling(t *testing.T) {
	model := &windowCompleter{limit: 20000}
	reviewer := NewReviewer(model, DefaultConfig())
	reviewer.SetBatchLimits(BatchLimits{MaxPromptTokens: 10})
	_, err := reviewer.GenerateReviewWithContext(context.Background(), manyFilesDiff(3, 2, 400), ReviewContext{})
	if !errors.Is(err, ErrNoBatchReviewed) || !errors.Is(err, llm.ErrAllProvidersFailed) {
		t.Fatalf("error = %v, want ErrNoBatchReviewed wrapping llm.ErrAllProvidersFailed", err)
	}
	if !strings.Contains(err.Error(), "batches.max_prompt_tokens allows 10") || strings.Contains(err.Error(), "max_batches") {
		t.Fatalf("error %q does not name the ceiling that left the batch out", err)
	}
}
