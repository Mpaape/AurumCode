package deliberation

import (
	"context"
	"errors"
	"testing"

	"github.com/Mpaape/AurumCode/internal/llm"
)

// The ceiling is the deliberation's own cost: a base prompt far above it,
// answered without any tool, never reaches it.
func TestAUR595LargeBasePromptWithoutToolsIsNotALimit(t *testing.T) {
	caller := &scriptedCaller{rounds: []llm.ToolResponse{answer(`{"summary":"ok"}`, 65918, 91)}}
	lim := Limits{MaxRounds: 3, MaxCostTokens: 60000, PerToolTimeout: limits().PerToolTimeout}
	out, err := Session{Caller: caller, Tools: []Tool{&countingTool{name: "scan"}}, Limits: lim}.Run(context.Background(), llm.SystemUserMessages("s", "u"))
	if err != nil {
		t.Fatalf("a base prompt above the ceiling stopped a deliberation that asked for nothing: %v", err)
	}
	tr := out.Transcript
	if out.Answer.Text != `{"summary":"ok"}` || tr.BaseTokens != 65918 || tr.CostTokens != 91 || tr.Outcome != OutcomeAnswered {
		t.Fatalf("answer=%q transcript=%+v", out.Answer.Text, tr)
	}
}

// What the tool rounds add beyond the base prompt still exhausts the
// ceiling, however large the base prompt.
func TestAUR595ToolRoundsBeyondTheBaseStillExceedTheCeiling(t *testing.T) {
	tool := &countingTool{name: "scan"}
	caller := &scriptedCaller{rounds: []llm.ToolResponse{
		{Response: llm.Response{TokensIn: 65918, TokensOut: 50}, ToolCalls: []llm.ToolCall{call("c1", "scan", `{"path":"a.go"}`)}},
		{Response: llm.Response{TokensIn: 70000, TokensOut: 50}, ToolCalls: []llm.ToolCall{call("c2", "scan", `{"path":"b.go"}`)}},
		answer(`{}`, 75000, 50),
	}}
	lim := Limits{MaxRounds: 5, MaxCostTokens: 8000, PerToolTimeout: limits().PerToolTimeout}
	out, err := Session{Caller: caller, Tools: []Tool{tool}, Limits: lim}.Run(context.Background(), nil)
	var limit *LimitError
	if !errors.As(err, &limit) || limit.Limit != LimitMaxCostTokens || out.Answer.Text != "" {
		t.Fatalf("err=%v answer=%q, want max_cost_tokens from the tool rounds", err, out.Answer.Text)
	}
	// 50 + (70000-65918)+50 + (75000-65918)+50 = 13314 > 8000, first crossed at round 3.
	if tr := out.Transcript; tr.Rounds != 3 || tr.BaseTokens != 65918 || tr.CostTokens != 13314 || tr.Outcome != "deliberation_limit:max_cost_tokens" {
		t.Fatalf("transcript=%+v", tr)
	}
}
