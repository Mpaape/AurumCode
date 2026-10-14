package review

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Mpaape/AurumCode/internal/llm"
	"github.com/Mpaape/AurumCode/internal/prompt"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// The model always decides the deterministic evidence (gate.triage: model).
// A review may answer without assessing some of it -- a long prompt, a
// reply that skipped the evidence section -- and an unassessed finding
// would then count as a static error. The dedicated triage asks the model
// about exactly those findings, in a small prompt of its own (rule, place,
// the code around it), and its answer passes the same validation as the
// review's assessments. A prompt the provider refuses as too long, or a
// reply cut short, is split in halves; nothing bounds how many findings are
// decided.

// TriageCaller is the provider the review already uses (llm.Orchestrator).
type TriageCaller interface {
	Complete(ctx context.Context, prompt string, opts llm.Options) (llm.Response, error)
}

// EvidenceTriager asks the model to decide evidence items.
type EvidenceTriager struct {
	Caller TriageCaller
	// Language is the language of the justifications.
	Language string
	// Redact filters the prompt and the model's text; nil leaves them.
	Redact func(string) string
}

// triageAnswer is the triage's JSON reply.
type triageAnswer struct {
	EvidenceAssessments []types.EvidenceAssessment `json:"evidence_assessments"`
}

// triageSchemaName names the answer schema sent to the provider.
const triageSchemaName = "aurumcode_evidence_triage"

// Assess returns the model's assessments of items: only of the ids offered,
// with a known status (weighAssessments). An error means the model did not
// answer (provider failure, a refusal that no split resolves, a reply that
// is not the JSON asked for): the caller must treat the items as awaiting
// the model, never as decided.
func (t *EvidenceTriager) Assess(ctx context.Context, items []prompt.EvidenceItem) ([]types.EvidenceAssessment, error) {
	if len(items) == 0 {
		return nil, nil
	}
	text, err := prompt.BuildTriagePrompt(prompt.TriageInput{Language: t.Language, Items: items})
	if err != nil {
		return nil, err
	}
	resp, err := t.Caller.Complete(ctx, t.redact(text), llm.Options{
		JSONMode:           true,
		ResponseSchema:     llm.SchemaOf(triageAnswer{}),
		ResponseSchemaName: triageSchemaName,
	})
	_, tooLong := llm.AsPromptTooLong(err)
	if (tooLong || (err == nil && resp.FinishReason == "length")) && len(items) > 1 {
		return t.assessHalves(ctx, items)
	}
	if err != nil {
		return nil, fmt.Errorf("evidence triage: %w", err)
	}
	return t.parse(resp.Text, items)
}

// assessHalves decides items in two smaller prompts.
func (t *EvidenceTriager) assessHalves(ctx context.Context, items []prompt.EvidenceItem) ([]types.EvidenceAssessment, error) {
	half := len(items) / 2
	first, err := t.Assess(ctx, items[:half])
	if err != nil {
		return nil, err
	}
	second, err := t.Assess(ctx, items[half:])
	if err != nil {
		return nil, err
	}
	return append(first, second...), nil
}

// parse reads the reply (one JSON object; a surrounding fence or prose is
// tolerated) and keeps the valid assessments of the offered ids.
func (t *EvidenceTriager) parse(text string, items []prompt.EvidenceItem) ([]types.EvidenceAssessment, error) {
	start, end := strings.Index(text, "{"), strings.LastIndex(text, "}")
	if start < 0 || end < start {
		return nil, fmt.Errorf("evidence triage: the reply holds no JSON object")
	}
	var answer triageAnswer
	if err := json.Unmarshal([]byte(text[start:end+1]), &answer); err != nil {
		return nil, fmt.Errorf("evidence triage: the reply is not the JSON asked for: %v", err)
	}
	offered := make([]string, 0, len(items))
	for _, item := range items {
		offered = append(offered, item.ID)
	}
	result := &types.ReviewResult{EvidenceAssessments: answer.EvidenceAssessments}
	weighAssessments(result, offered)
	for i := range result.EvidenceAssessments {
		a := &result.EvidenceAssessments[i]
		a.Justification, a.Suggestion = t.redact(a.Justification), t.redact(a.Suggestion)
	}
	return result.EvidenceAssessments, nil
}

func (t *EvidenceTriager) redact(text string) string {
	if t.Redact == nil {
		return text
	}
	return t.Redact(text)
}
