package deliberation

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Mpaape/AurumCode/internal/llm"
)

// specCaller records the tools offered on every round and replays rounds.
type specCaller struct {
	scriptedCaller
	offered [][]llm.ToolSpec
}

func (c *specCaller) CompleteWithTools(ctx context.Context, msgs []llm.Message, specs []llm.ToolSpec, opts llm.Options) (llm.ToolResponse, error) {
	c.offered = append(c.offered, specs)
	return c.scriptedCaller.CompleteWithTools(ctx, msgs, specs, opts)
}

// The last allowed round offers no tool and asks for the review: a model
// that kept asking for context still delivers an answer instead of
// spending every round and publishing nothing.
func TestFinalRoundOffersNoToolAndAnswers(t *testing.T) {
	tool := &countingTool{name: "scan"}
	caller := &specCaller{scriptedCaller: scriptedCaller{rounds: []llm.ToolResponse{
		{Response: llm.Response{TokensIn: 1, TokensOut: 1}, ToolCalls: []llm.ToolCall{call("a", "scan", `{"path":"a.go"}`)}},
		{Response: llm.Response{TokensIn: 1, TokensOut: 1}, ToolCalls: []llm.ToolCall{call("b", "scan", `{"path":"b.go"}`)}},
		{Response: llm.Response{Text: `{"summary":"final"}`, TokensIn: 1, TokensOut: 1}},
	}}}
	out, err := Session{Caller: caller, Tools: []Tool{tool}, Limits: Limits{MaxRounds: 3, MaxCostTokens: 1000, PerToolTimeout: time.Second}}.Run(context.Background(), nil)
	if err != nil {
		t.Fatalf("err = %v, want an answer on the last round", err)
	}
	if out.Answer.Text != `{"summary":"final"}` || out.Transcript.Outcome != OutcomeAnswered {
		t.Fatalf("answer=%q outcome=%q", out.Answer.Text, out.Transcript.Outcome)
	}
	if len(caller.offered) != 3 || len(caller.offered[0]) != 1 || len(caller.offered[1]) != 1 || len(caller.offered[2]) != 0 {
		t.Fatalf("tools offered per round = %v, want tools on 1 and 2, none on 3", caller.offered)
	}
	last := caller.seen[2]
	if !strings.Contains(last[len(last)-1].Content, "last round") {
		t.Fatalf("the last round did not tell the model to answer now: %+v", last[len(last)-1])
	}
}
