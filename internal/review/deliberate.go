package review

import (
	"context"
	"fmt"

	"github.com/Mpaape/AurumCode/internal/deliberation"
	"github.com/Mpaape/AurumCode/internal/llm"
	"github.com/Mpaape/AurumCode/internal/prompt"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// Deliberation lets the model ask for tools before it answers. Caller is
// the tool-capable model side (llm.Orchestrator); Tools are the offered
// tools, whose manifest the caller also puts in ReviewContext.Tools.
type Deliberation struct {
	Caller deliberation.Caller
	Tools  []deliberation.Tool
	Limits deliberation.Limits
}

// reviewAnswerSchemaName names the answer schema sent to the provider.
const reviewAnswerSchemaName = "aurumcode_review"

// SetDeliberation enables deliberation for the next reviews; nil disables
// it and the review is the single call it always was.
func (r *Reviewer) SetDeliberation(d *Deliberation) {
	r.deliberation = d
}

// Transcript is the last deliberation's record, nil when none ran.
func (r *Reviewer) Transcript() *deliberation.Transcript {
	return r.transcript
}

// answer asks the model: one call without deliberation, a bounded tool
// conversation with it. A deliberation stopped by a limit returns the
// error and no answer, so no partial verdict can reach a sink.
func (r *Reviewer) answer(ctx context.Context, parts prompt.PromptParts) (llm.Response, error) {
	d := r.deliberation
	if d == nil || len(d.Tools) == 0 {
		return r.complete(ctx, parts)
	}
	session := deliberation.Session{
		Caller: d.Caller,
		Tools:  d.Tools,
		Limits: d.Limits,
		Options: llm.Options{
			MaxTokens:          r.replyCap(),
			Temperature:        r.cfg.Temperature,
			JSONMode:           true,
			ResponseSchema:     llm.SchemaOf(types.ReviewResult{}),
			ResponseSchemaName: reviewAnswerSchemaName,
		},
		Redact: r.filter.Redact,
	}
	out, err := session.Run(ctx, parts.Messages())
	r.transcript = &out.Transcript
	if err != nil {
		return llm.Response{}, fmt.Errorf("LLM deliberation failed: %w", err)
	}
	return out.Answer, nil
}

// replyCap is the reply cap derived from MaxTokens, 0 when uncapped.
func (r *Reviewer) replyCap() int {
	if r.cfg.MaxTokens > r.cfg.ReserveReply {
		return r.cfg.MaxTokens - r.cfg.ReserveReply
	}
	return 0
}
