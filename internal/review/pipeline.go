package review

import (
	"context"
	"errors"
	"fmt"

	"github.com/Mpaape/AurumCode/internal/analyzer"
	"github.com/Mpaape/AurumCode/internal/llm"
	"github.com/Mpaape/AurumCode/internal/prompt"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// GenerateReviewWithContext generates a review using the diff and optional
// external evidence such as completed CI check statuses. It runs four
// stages: assemble the redacted prompt, ask the model, parse its reply, and
// pass the parsed findings through the engine's gates.
func (r *Reviewer) GenerateReviewWithContext(ctx context.Context, diff *types.Diff, reviewContext ReviewContext) (*types.ReviewResult, error) {
	prepared, err := r.preparePrompt(diff, reviewContext)
	if err != nil {
		return nil, err
	}
	resp, err := r.complete(ctx, prepared.parts)
	if err != nil {
		return nil, err
	}
	result, err := r.parse(resp)
	if err != nil {
		return nil, err
	}
	outcome, err := r.applyGates(prepared.diff, result)
	if err != nil {
		return nil, err
	}
	annotateResult(result, prepared, outcome)
	return result, nil
}

// PromptDigest returns the digest of the exact system and user messages
// GenerateReviewWithContext would send for diff and reviewContext, without
// calling the model. Equal inputs give equal digests; any change to the
// rendered text -- evidence included -- moves it.
func (r *Reviewer) PromptDigest(diff *types.Diff, reviewContext ReviewContext) (string, error) {
	prepared, err := r.preparePrompt(diff, reviewContext)
	if err != nil {
		return "", err
	}
	return prepared.parts.Digest(), nil
}

// preparedPrompt is the output of the assembly stage.
type preparedPrompt struct {
	diff    *types.Diff // redacted copy: everything downstream sees this one
	metrics *analyzer.DiffMetrics
	parts   prompt.PromptParts
}

// preparePrompt redacts every untrusted input and assembles the budgeted
// prompt from the review template's slots.
func (r *Reviewer) preparePrompt(diff *types.Diff, reviewContext ReviewContext) (preparedPrompt, error) {
	// Analyze diff (counts only; metrics carry no content into the prompt)
	metrics := r.diffAnalyzer.AnalyzeDiff(diff)

	// The diff is the untrusted material that will leave this process
	// toward a model, so it passes the single AUR-009 redaction filter
	// BEFORE the prompt is assembled (AUR-432). Composition matters, not
	// just coverage: a diff line carries a +/-/space marker, and the
	// filter's header rule is anchored at line start, so redacting the
	// assembled prompt would let "+Authorization: Bearer x" through
	// verbatim. redactDiff strips each line's marker, redacts the bodies
	// as the filter would see them on their own, and re-prefixes.
	redacted := redactDiff(r.filter, diff)

	opts := prompt.BuildOptions{
		MaxTokens:         r.promptBudget(),
		SchemaKind:        "review",
		Role:              "reviewer",
		ReserveReply:      r.cfg.ReserveReply,
		CIContext:         r.filter.Redact(reviewContext.CI),
		ReviewHistory:     r.filter.Redact(reviewContext.History),
		CodebaseContext:   r.filter.Redact(reviewContext.CodebaseContext),
		MemoryNotes:       r.filter.Redact(reviewContext.MemoryNotes),
		Language:          reviewContext.Language,
		RepositoryContext: r.filter.Redact(reviewContext.RepositoryContext),
		Evidence:          redactEvidence(r.filter, reviewContext.Evidence),
		Tools:             redactTools(r.filter, reviewContext.Tools),
	}

	parts, err := r.promptBuilder.BuildPrompt(redacted, metrics, opts)
	if err != nil {
		return preparedPrompt{}, fmt.Errorf("failed to build prompt: %w", err)
	}
	return preparedPrompt{diff: redacted, metrics: metrics, parts: parts}, nil
}

// promptBudget is the explicit MaxTokens when set, else the default prompt
// ceiling.
func (r *Reviewer) promptBudget() int {
	if r.cfg.MaxTokens > 0 {
		return r.cfg.MaxTokens
	}
	return r.cfg.PromptTokenBudget
}

// complete sends the prompt as separate system and user messages. The
// system message is the trusted embedded template; everything untrusted in
// the user message was redacted by preparePrompt.
func (r *Reviewer) complete(ctx context.Context, parts prompt.PromptParts) (llm.Response, error) {
	maxReplyTokens := 0
	if r.cfg.MaxTokens > r.cfg.ReserveReply {
		maxReplyTokens = r.cfg.MaxTokens - r.cfg.ReserveReply
	}
	// The parser requires JSON. Ask the provider to constrain syntax instead
	// of relying on prompt wording alone; schema and finding evidence are
	// still validated locally after the response arrives.
	resp, err := r.completer.CompleteMessages(ctx, parts.Messages(), llm.Options{
		MaxTokens:   maxReplyTokens,
		Temperature: r.cfg.Temperature,
		JSONMode:    true,
	})
	if err != nil {
		return llm.Response{}, fmt.Errorf("LLM request failed: %w", err)
	}
	return resp, nil
}

// parse decodes the model reply and redacts every model-authored string.
func (r *Reviewer) parse(resp llm.Response) (*types.ReviewResult, error) {
	result, err := r.parser.ParseReviewResponse(resp.Text)
	if err != nil {
		var parseErr *prompt.ParseError
		if errors.As(err, &parseErr) {
			parseErr.FinishReason = resp.FinishReason
		}
		return nil, fmt.Errorf("parse failed: %w", err)
	}
	// Model output is untrusted input: a model may echo a secret from the
	// diff back in a finding. Every string of the parsed result that can
	// reach a sink (report, stdout, cache, evidence) is redacted here, at
	// the boundary where it enters the process, and deliberately BEFORE
	// enforceRuleCitations appends the trusted rule-citation suffix from
	// the embedded catalog -- redacting after would also rewrite the
	// catalog's own "...-secret: <title>" spelling and change the
	// published output format for a secret-free review (AUR-432).
	redactReviewResult(r.filter, result)
	return result, nil
}
