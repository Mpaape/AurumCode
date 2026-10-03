package deliberation

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/Mpaape/AurumCode/internal/llm"
)

// scriptedCaller answers each round from a script, repeating the last entry.
type scriptedCaller struct {
	rounds []llm.ToolResponse
	calls  int
	seen   [][]llm.Message
}

func (c *scriptedCaller) CompleteWithTools(_ context.Context, msgs []llm.Message, _ []llm.ToolSpec, _ llm.Options) (llm.ToolResponse, error) {
	c.seen = append(c.seen, append([]llm.Message(nil), msgs...))
	i := c.calls
	if i >= len(c.rounds) {
		i = len(c.rounds) - 1
	}
	c.calls++
	return c.rounds[i], nil
}

type pathArgs struct {
	Path string `json:"path"`
}

// countingTool records how often it ran.
type countingTool struct {
	name  string
	runs  int
	delay time.Duration
	err   error
}

func (t *countingTool) Spec() llm.ToolSpec {
	return llm.ToolSpec{Name: t.name, Description: "test tool", Parameters: llm.SchemaOf(pathArgs{})}
}

func (t *countingTool) Run(ctx context.Context, _ json.RawMessage) (Result, error) {
	t.runs++
	if t.delay > 0 {
		select {
		case <-time.After(t.delay):
		case <-ctx.Done():
		}
	}
	if t.err != nil {
		return Result{}, t.err
	}
	return Result{Content: "resultado de " + t.name, Summary: "1 item", Digest: "d1"}, nil
}

func call(id, name, args string) llm.ToolCall {
	return llm.ToolCall{ID: id, Name: name, Arguments: json.RawMessage(args)}
}

func limits() Limits {
	return Limits{MaxRounds: 3, MaxCostTokens: 1000, PerToolTimeout: time.Second}
}

func answer(text string, in, out int) llm.ToolResponse {
	return llm.ToolResponse{Response: llm.Response{Text: text, TokensIn: in, TokensOut: out}}
}

func TestAUR580ToolResultGoesBackAndAnswerEnds(t *testing.T) {
	tool := &countingTool{name: "scan"}
	caller := &scriptedCaller{rounds: []llm.ToolResponse{
		{Response: llm.Response{TokensIn: 10, TokensOut: 2}, ToolCalls: []llm.ToolCall{call("c1", "scan", `{"path":"a.go"}`)}},
		answer(`{"summary":"ok"}`, 20, 5),
	}}
	out, err := Session{Caller: caller, Tools: []Tool{tool}, Limits: limits()}.Run(context.Background(), llm.SystemUserMessages("s", "u"))
	if err != nil {
		t.Fatal(err)
	}
	if out.Answer.Text != `{"summary":"ok"}` || tool.runs != 1 {
		t.Fatalf("answer=%q runs=%d", out.Answer.Text, tool.runs)
	}
	second := caller.seen[1]
	if last := second[len(second)-1]; last.Role != llm.RoleTool || last.ToolCallID != "c1" || !strings.Contains(last.Content, "resultado de scan") {
		t.Fatalf("the tool result must go back as the tool message of c1: %+v", last)
	}
	tr := out.Transcript
	if tr.Rounds != 2 || tr.TokensIn != 30 || tr.TokensOut != 7 || tr.Outcome != OutcomeAnswered {
		t.Fatalf("transcript = %+v", tr)
	}
	if len(tr.Requested) != 1 || tr.Requested[0] != "scan" || len(tr.NotRequested) != 0 {
		t.Fatalf("decision = requested %v not requested %v", tr.Requested, tr.NotRequested)
	}
}

// AC-002: exceeding MaxRounds is a typed limit error with no answer, even
// when the last round carried text beside its tool calls.
func TestAUR580MaxRoundsIsALimitErrorWithoutAnswer(t *testing.T) {
	tool := &countingTool{name: "scan"}
	caller := &scriptedCaller{rounds: []llm.ToolResponse{
		{Response: llm.Response{Text: `{"summary":"parcial"}`, TokensIn: 1, TokensOut: 1}, ToolCalls: []llm.ToolCall{call("c", "scan", `{"path":"a.go"}`)}},
	}}
	out, err := Session{Caller: caller, Tools: []Tool{tool}, Limits: Limits{MaxRounds: 2, MaxCostTokens: 1000, PerToolTimeout: time.Second}}.Run(context.Background(), nil)
	var limit *LimitError
	if !errors.As(err, &limit) || limit.Limit != LimitMaxRounds || !errors.Is(err, ErrLimit) {
		t.Fatalf("err = %v, want a max_rounds LimitError", err)
	}
	if out.Answer.Text != "" {
		t.Fatalf("a deliberation over its rounds returned an answer: %q", out.Answer.Text)
	}
	if caller.calls != 2 || out.Transcript.Outcome != "deliberation_limit:max_rounds" {
		t.Fatalf("calls=%d outcome=%q", caller.calls, out.Transcript.Outcome)
	}
}

func TestAUR580MaxCostTokensIsALimitError(t *testing.T) {
	caller := &scriptedCaller{rounds: []llm.ToolResponse{answer(`{}`, 900, 200)}}
	out, err := Session{Caller: caller, Limits: limits()}.Run(context.Background(), nil)
	var limit *LimitError
	if !errors.As(err, &limit) || limit.Limit != LimitMaxCostTokens || out.Answer.Text != "" {
		t.Fatalf("err=%v answer=%q, want max_cost_tokens and no answer", err, out.Answer.Text)
	}
}

func TestAUR580PerToolTimeoutIsALimitError(t *testing.T) {
	tool := &countingTool{name: "slow", delay: time.Second}
	caller := &scriptedCaller{rounds: []llm.ToolResponse{{ToolCalls: []llm.ToolCall{call("c", "slow", `{"path":"a"}`)}}, answer(`{}`, 1, 1)}}
	_, err := Session{Caller: caller, Tools: []Tool{tool}, Limits: Limits{MaxRounds: 3, MaxCostTokens: 100, PerToolTimeout: 20 * time.Millisecond}}.Run(context.Background(), nil)
	var limit *LimitError
	if !errors.As(err, &limit) || limit.Limit != LimitPerToolTimeout {
		t.Fatalf("err=%v, want per-tool timeout", err)
	}
}

// AC-004: invalid arguments and unknown tools are refused before anything
// runs; the model is told why and the transcript records the refusal.
func TestAUR580InvalidArgumentsRefusedBeforeRun(t *testing.T) {
	for name, args := range map[string]string{
		"wrong type":    `{"path":5}`,
		"missing":       `{}`,
		"unknown field": `{"path":"a","cmd":"rm"}`,
		"not an object": `["a"]`,
	} {
		t.Run(name, func(t *testing.T) {
			tool := &countingTool{name: "scan"}
			caller := &scriptedCaller{rounds: []llm.ToolResponse{
				{ToolCalls: []llm.ToolCall{call("c", "scan", args), call("d", "nope", `{}`)}},
				answer(`{}`, 1, 1),
			}}
			out, err := Session{Caller: caller, Tools: []Tool{tool}, Limits: limits()}.Run(context.Background(), nil)
			if err != nil {
				t.Fatal(err)
			}
			if tool.runs != 0 {
				t.Fatalf("a call with %s arguments ran the tool", name)
			}
			for _, c := range out.Transcript.Calls {
				if c.Status != StatusRefused {
					t.Fatalf("call %+v was not refused", c)
				}
			}
			msgs := caller.seen[1]
			if !strings.Contains(msgs[len(msgs)-2].Content, "recusada antes de executar") {
				t.Fatalf("the model was not told about the refusal: %+v", msgs[len(msgs)-2])
			}
		})
	}
}

// AC-005: the transcript carries redacted arguments, the duration and the
// summarized result of each call.
func TestAUR580TranscriptRedactsArguments(t *testing.T) {
	tool := &countingTool{name: "scan"}
	caller := &scriptedCaller{rounds: []llm.ToolResponse{{ToolCalls: []llm.ToolCall{call("c", "scan", `{"path":"tok-SECRET-1"}`)}}, answer(`{}`, 1, 1)}}
	ticks := []time.Time{time.Unix(0, 0), time.Unix(0, int64(7*time.Millisecond))}
	clock := func() time.Time { t0 := ticks[0]; ticks = ticks[1:]; return t0 }
	redact := func(s string) string { return strings.ReplaceAll(s, "SECRET", "[REDACTED]") }
	out, err := Session{Caller: caller, Tools: []Tool{tool, &countingTool{name: "ctx"}}, Limits: limits(), Redact: redact, Clock: clock}.Run(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	c := out.Transcript.Calls[0]
	if strings.Contains(c.Arguments, "SECRET") || !strings.Contains(c.Arguments, "[REDACTED]") {
		t.Fatalf("arguments not redacted: %q", c.Arguments)
	}
	if c.DurationMS != 7 || c.Status != StatusExecuted || c.Result != "1 item" || c.Digest != "d1" {
		t.Fatalf("call record = %+v", c)
	}
	if len(out.Transcript.NotRequested) != 1 || out.Transcript.NotRequested[0] != "ctx" {
		t.Fatalf("not requested = %v", out.Transcript.NotRequested)
	}
}

func TestAUR580LimitsMustBePositive(t *testing.T) {
	if err := (Limits{MaxRounds: 0, MaxCostTokens: 1, PerToolTimeout: time.Second}).Validate(); err == nil || !strings.Contains(err.Error(), LimitMaxRounds) {
		t.Fatalf("err = %v", err)
	}
}

func TestAUR580ToolMessageIsCappedOnARuneBoundary(t *testing.T) {
	long := strings.Repeat("é", maxToolMessageBytes)
	got := capContent(long)
	if len(got) > maxToolMessageBytes || !strings.HasSuffix(got, truncationNote) || !utf8.ValidString(got) {
		t.Fatalf("capped message: %d bytes, valid=%v", len(got), utf8.ValidString(got))
	}
}
