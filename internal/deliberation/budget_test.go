package deliberation

// AUR-616: the sessions of one review (one per analyst) draw their tool
// rounds and cost from one Budget, and the merged transcript names each
// analyst's share.

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Mpaape/AurumCode/internal/llm"
)

// budgetCaller answers each round from a script, repeating the last entry,
// and records the tools each round offered.
type budgetCaller struct {
	rounds  []llm.ToolResponse
	calls   int
	offered [][]string
	seen    [][]llm.Message
}

func (c *budgetCaller) CompleteWithTools(_ context.Context, msgs []llm.Message, tools []llm.ToolSpec, _ llm.Options) (llm.ToolResponse, error) {
	c.seen = append(c.seen, append([]llm.Message(nil), msgs...))
	c.offered = append(c.offered, specNames(tools))
	i := c.calls
	if i >= len(c.rounds) {
		i = len(c.rounds) - 1
	}
	c.calls++
	return c.rounds[i], nil
}

func askThenAnswer(text string) []llm.ToolResponse {
	return []llm.ToolResponse{
		{Response: llm.Response{TokensIn: 10, TokensOut: 2}, ToolCalls: []llm.ToolCall{call("c1", "scan", `{"path":"a.go"}`)}},
		answer(text, 20, 5),
	}
}

func sharedLimits() Limits {
	return Limits{MaxRounds: 2, MaxCostTokens: 1000, PerToolTimeout: time.Second}
}

// AC-002: with max_rounds 2 the review has one tool round. The first
// session spends it; the second is offered no tool and asked for its
// answer at once, which it gives.
func TestAUR616SessionsShareTheBudgetRounds(t *testing.T) {
	budget := NewBudget(sharedLimits())
	tool := &countingTool{name: "scan"}
	first := &budgetCaller{rounds: askThenAnswer(`{"summary":"a"}`)}
	out, err := Session{Caller: first, Tools: []Tool{tool}, Limits: sharedLimits(), Shared: budget}.Run(context.Background(), llm.SystemUserMessages("s", "u"))
	if err != nil || out.Transcript.Rounds != 2 || tool.runs != 1 || out.Answer.Text != `{"summary":"a"}` {
		t.Fatalf("first session: err=%v rounds=%d runs=%d answer=%q", err, out.Transcript.Rounds, tool.runs, out.Answer.Text)
	}
	if left := budget.ToolRoundsLeft(); left != 0 {
		t.Fatalf("tool rounds left after the first session = %d, want 0", left)
	}
	second := &budgetCaller{rounds: []llm.ToolResponse{answer(`{"summary":"b"}`, 20, 5)}}
	out, err = Session{Caller: second, Tools: []Tool{tool}, Limits: sharedLimits(), Shared: budget}.Run(context.Background(), llm.SystemUserMessages("s", "u"))
	if err != nil || out.Transcript.Rounds != 1 || tool.runs != 1 || out.Answer.Text != `{"summary":"b"}` {
		t.Fatalf("second session: err=%v transcript=%+v runs=%d", err, out.Transcript, tool.runs)
	}
	if len(second.offered) != 1 || len(second.offered[0]) != 0 {
		t.Fatalf("the second session was offered tools after the budget was spent: %v", second.offered)
	}
	if msgs := second.seen[0]; msgs[len(msgs)-1].Content != finalRoundInstruction {
		t.Fatalf("the second session was not asked for its answer at once: %+v", msgs[len(msgs)-1])
	}
	if len(out.Transcript.NotRequested) != 1 || out.Transcript.NotRequested[0] != "scan" {
		t.Fatalf("the tool not offered must count as not requested: %+v", out.Transcript)
	}
}

// AC-002: a model that still asks for a tool after the review's tool rounds
// are spent exceeds max_rounds; no answer is returned.
func TestAUR616AskingAfterTheBudgetIsSpentIsARoundsLimit(t *testing.T) {
	budget := NewBudget(sharedLimits())
	tool := &countingTool{name: "scan"}
	first := &budgetCaller{rounds: askThenAnswer(`{"summary":"a"}`)}
	if _, err := (Session{Caller: first, Tools: []Tool{tool}, Limits: sharedLimits(), Shared: budget}).Run(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	insistent := &budgetCaller{rounds: []llm.ToolResponse{{Response: llm.Response{TokensIn: 10, TokensOut: 2}, ToolCalls: []llm.ToolCall{call("c9", "scan", `{"path":"a.go"}`)}}}}
	out, err := Session{Caller: insistent, Tools: []Tool{tool}, Limits: sharedLimits(), Shared: budget}.Run(context.Background(), nil)
	var limit *LimitError
	if !errors.As(err, &limit) || limit.Limit != LimitMaxRounds || !strings.Contains(limit.Detail, "somadas na revisao") {
		t.Fatalf("err = %v, want a max_rounds LimitError of the review", err)
	}
	if out.Answer.Text != "" || insistent.calls != 1 || tool.runs != 1 || out.Transcript.Outcome != "deliberation_limit:max_rounds" {
		t.Fatalf("answer=%q calls=%d runs=%d outcome=%q", out.Answer.Text, insistent.calls, tool.runs, out.Transcript.Outcome)
	}
}

// AC-002: the cost of every session is summed against max_cost_tokens:
// two sessions that each fit the ceiling alone exceed it together.
func TestAUR616SessionsShareTheCostCeiling(t *testing.T) {
	limits := Limits{MaxRounds: 3, MaxCostTokens: 100, PerToolTimeout: time.Second}
	run := func(budget *Budget) error {
		_, err := Session{Caller: &budgetCaller{rounds: []llm.ToolResponse{answer(`{}`, 500, 60)}}, Limits: limits, Shared: budget}.Run(context.Background(), nil)
		return err
	}
	if err := run(nil); err != nil {
		t.Fatalf("a session alone within the ceiling: %v", err)
	}
	if err := run(nil); err != nil {
		t.Fatalf("a second session alone within the ceiling: %v", err)
	}
	budget := NewBudget(limits)
	if err := run(budget); err != nil {
		t.Fatalf("the first session of the budget: %v", err)
	}
	err := run(budget)
	var limit *LimitError
	if !errors.As(err, &limit) || limit.Limit != LimitMaxCostTokens || !strings.Contains(limit.Detail, "120 tokens de deliberacao somados na revisao, teto 100") {
		t.Fatalf("err = %v, want the summed cost over the ceiling", err)
	}
	if rounds, cost := budget.Spent(); rounds != 0 || cost != 120 {
		t.Fatalf("spent = %d rounds, %d tokens; want 0 and 120", rounds, cost)
	}
}

// The merged transcript of a review's passes: sums, one list of tools
// requested by some pass, the rest not requested, each call and one entry
// per attributed analyst.
func TestAUR616MergeAttributesCallsToAnalysts(t *testing.T) {
	if Merge() != nil || Merge(nil, nil) != nil {
		t.Fatal("merging nothing must be nil")
	}
	a := &Transcript{Offered: []string{"scan", "read"}, Requested: []string{"scan"}, NotRequested: []string{"read"}, Rounds: 2, BaseTokens: 100, CostTokens: 30, Calls: []Call{{Round: 1, Tool: "scan", Status: StatusExecuted}}, Outcome: OutcomeAnswered}
	a.AttributeTo("seguranca")
	b := &Transcript{Offered: []string{"scan", "read"}, Requested: []string{}, NotRequested: []string{"scan", "read"}, Rounds: 1, BaseTokens: 90, CostTokens: 5, Calls: []Call{}, Outcome: OutcomeAnswered}
	b.AttributeTo("solid")
	m := Merge(a, nil, b)
	if m.Rounds != 3 || m.BaseTokens != 190 || m.CostTokens != 35 || m.Outcome != OutcomeAnswered {
		t.Fatalf("sums = %+v", m)
	}
	if strings.Join(m.Requested, ",") != "scan" || strings.Join(m.NotRequested, ",") != "read" || strings.Join(m.Offered, ",") != "scan,read" {
		t.Fatalf("decision = requested %v, not requested %v, offered %v", m.Requested, m.NotRequested, m.Offered)
	}
	if len(m.Calls) != 1 || m.Calls[0].Analyst != "seguranca" {
		t.Fatalf("calls = %+v, want the call attributed to seguranca", m.Calls)
	}
	if len(m.Analysts) != 2 || m.Analysts[0].Analyst != "seguranca" || m.Analysts[0].Rounds != 2 || m.Analysts[0].CostTokens != 30 || m.Analysts[1].Analyst != "solid" || m.Analysts[1].Rounds != 1 || len(m.Analysts[1].Requested) != 0 {
		t.Fatalf("analysts = %+v", m.Analysts)
	}
}
