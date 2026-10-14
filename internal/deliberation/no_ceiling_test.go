package deliberation

// max_cost_tokens is optional (issue #179): 0 sets no token ceiling, so the
// rounds and the tool timeout alone bound a deliberation, and the cost is
// still counted and recorded. A positive value is an explicit ceiling that
// stops the deliberation with the same error as always.

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Mpaape/AurumCode/internal/llm"
)

// hugeCostRounds is a tool round and a final answer that cost far more than
// any ceiling someone would write: a base prompt of 2,000,000 tokens and
// 8,300,000 more from the deliberation (500,000 + 7,000,000 + 800,000).
func hugeCostRounds() []llm.ToolResponse {
	return []llm.ToolResponse{
		{Response: llm.Response{TokensIn: 2_000_000, TokensOut: 500_000}, ToolCalls: []llm.ToolCall{call("c1", "scan", `{"path":"a.go"}`)}},
		answer(`{"summary":"ok"}`, 9_000_000, 800_000),
	}
}

func TestMaxCostTokensZeroSetsNoCeiling(t *testing.T) {
	tool := &countingTool{name: "scan"}
	lim := Limits{MaxRounds: 3, MaxCostTokens: 0, PerToolTimeout: time.Second}
	out, err := Session{Caller: &scriptedCaller{rounds: hugeCostRounds()}, Tools: []Tool{tool}, Limits: lim}.Run(context.Background(), llm.SystemUserMessages("s", "u"))
	if err != nil {
		t.Fatalf("a deliberation with no cost ceiling stopped: %v", err)
	}
	tr := out.Transcript
	if out.Answer.Text != `{"summary":"ok"}` || tr.Outcome != OutcomeAnswered || tr.Limit != "" || tool.runs != 1 {
		t.Fatalf("answer=%q runs=%d transcript=%+v, want an answered deliberation with no limit", out.Answer.Text, tool.runs, tr)
	}
	if tr.BaseTokens != 2_000_000 || tr.CostTokens != 8_300_000 {
		t.Fatalf("base=%d cost=%d, want the cost still counted and recorded", tr.BaseTokens, tr.CostTokens)
	}
}

func TestMaxCostTokensPositiveStillExceedsWithTheSameError(t *testing.T) {
	tool := &countingTool{name: "scan"}
	lim := Limits{MaxRounds: 3, MaxCostTokens: 1_000_000, PerToolTimeout: time.Second}
	out, err := Session{Caller: &scriptedCaller{rounds: hugeCostRounds()}, Tools: []Tool{tool}, Limits: lim}.Run(context.Background(), llm.SystemUserMessages("s", "u"))
	var limit *LimitError
	if !errors.As(err, &limit) || limit.Limit != LimitMaxCostTokens || out.Answer.Text != "" {
		t.Fatalf("err=%v answer=%q, want max_cost_tokens and no answer", err, out.Answer.Text)
	}
	const detail = "8300000 tokens de deliberacao alem do prompt base de 2000000, teto 1000000"
	if limit.Detail != detail || err.Error() != "deliberation_limit: max_cost_tokens ("+detail+")" {
		t.Fatalf("detail=%q error=%q, want the message unchanged", limit.Detail, err.Error())
	}
	if tr := out.Transcript; tr.Outcome != "deliberation_limit:max_cost_tokens" || tr.Limit != LimitMaxCostTokens || tr.CostTokens != 8_300_000 {
		t.Fatalf("transcript = %+v", tr)
	}
}

func TestLimitsValidateAcceptsNoCostCeilingAndRefusesANegativeOne(t *testing.T) {
	if err := (Limits{MaxRounds: 1, MaxCostTokens: 0, PerToolTimeout: time.Second}).Validate(); err != nil {
		t.Fatalf("a zero cost ceiling must mean none, got %v", err)
	}
	err := (Limits{MaxRounds: 1, MaxCostTokens: -1, PerToolTimeout: time.Second}).Validate()
	if err == nil || !strings.Contains(err.Error(), LimitMaxCostTokens) {
		t.Fatalf("err = %v, want a negative max_cost_tokens refused by name", err)
	}
}

// A review's shared budget with no cost ceiling never reports the cost as
// exceeded, however much the sessions spend, and still sums what they spend.
func TestSharedBudgetWithoutCostCeilingNeverReportsItExceeded(t *testing.T) {
	budget := NewBudget(Limits{MaxRounds: 3, MaxCostTokens: 0, PerToolTimeout: time.Second})
	if total, exceeded := budget.addCost(1_000_000_000); exceeded || total != 1_000_000_000 {
		t.Fatalf("addCost = %d, %v; want the total and no excess", total, exceeded)
	}
	if total, exceeded := budget.addCost(1_000_000_000); exceeded || total != 2_000_000_000 {
		t.Fatalf("addCost = %d, %v; want the sum and no excess", total, exceeded)
	}
	// An explicit ceiling on the same arithmetic does report it.
	capped := NewBudget(Limits{MaxRounds: 3, MaxCostTokens: 100, PerToolTimeout: time.Second})
	if _, exceeded := capped.addCost(101); !exceeded {
		t.Fatal("a ceiling of 100 must report 101 tokens as exceeded")
	}
}

// Two sessions of one review, each costing more than any sane ceiling, both
// answer when the budget has none; the budget still records their sum.
func TestSessionsSharingABudgetWithoutCostCeilingBothAnswer(t *testing.T) {
	lim := Limits{MaxRounds: 3, MaxCostTokens: 0, PerToolTimeout: time.Second}
	budget := NewBudget(lim)
	for i := 1; i <= 2; i++ {
		caller := &budgetCaller{rounds: []llm.ToolResponse{answer(`{"summary":"ok"}`, 500_000, 600_000)}}
		out, err := Session{Caller: caller, Limits: lim, Shared: budget}.Run(context.Background(), nil)
		if err != nil || out.Answer.Text != `{"summary":"ok"}` || out.Transcript.Limit != "" {
			t.Fatalf("session %d: err=%v answer=%q transcript=%+v", i, err, out.Answer.Text, out.Transcript)
		}
	}
	if rounds, cost := budget.Spent(); rounds != 0 || cost != 1_200_000 {
		t.Fatalf("spent = %d rounds, %d tokens; want 0 and 1200000", rounds, cost)
	}
}
