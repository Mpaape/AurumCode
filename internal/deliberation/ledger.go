package deliberation

import "fmt"

// ledger decides, round by round, whether a session may still offer tools
// and when it crossed its rounds or its cost. A session alone keeps its
// Limits for itself; a session of a review's Budget draws from it.
type ledger interface {
	// exceeded is the limit a new round would cross, nil when none;
	// askedWithoutTools reports a previous round that asked for a tool when
	// none was offered.
	exceeded(round int, askedWithoutTools bool) *LimitError
	// offersTools reports whether round may offer tools.
	offersTools(round int) bool
	// charge books the round the transcript just counted and reports the
	// explicit cost ceiling it crossed (none is crossed without a ceiling);
	// toolRound is a round that asked for a tool.
	charge(t *Transcript, toolRound bool) *LimitError
	// noToolsReason is why a round offers no tool before any round of the
	// session offered one (Transcript.NotOffered).
	noToolsReason() string
}

// ownLedger is the single session's own limits: MaxRounds calls, the
// final answer included, with no tool offered on the last allowed round,
// and the cost ceiling MaxCostTokens when it is positive (0 sets none).
type ownLedger struct{ limits Limits }

func (l ownLedger) exceeded(round int, _ bool) *LimitError {
	if round > l.limits.MaxRounds {
		return &LimitError{Limit: LimitMaxRounds, Detail: fmt.Sprintf("%d rodadas sem resposta final", l.limits.MaxRounds)}
	}
	return nil
}

func (l ownLedger) offersTools(round int) bool {
	return !(round == l.limits.MaxRounds && round > 1)
}

func (l ownLedger) charge(t *Transcript, _ bool) *LimitError {
	if l.limits.MaxCostTokens > 0 && t.CostTokens > l.limits.MaxCostTokens {
		return &LimitError{Limit: LimitMaxCostTokens, Detail: fmt.Sprintf("%d tokens de deliberacao alem do prompt base de %d, teto %d", t.CostTokens, t.BaseTokens, l.limits.MaxCostTokens)}
	}
	return nil
}

// noToolsReason is empty: a session alone always offers its tools on its
// first round.
func (l ownLedger) noToolsReason() string { return "" }

// sharedLedger draws the session's tool rounds and cost from the review's
// Budget: a round may offer tools while the budget has a tool round left,
// a session whose model still asks for a tool after the budget is spent
// exceeds max_rounds, and the cost of every session is summed against
// max_cost_tokens when the budget has a ceiling (Budget.addCost).
type sharedLedger struct {
	budget *Budget
	// charged is the transcript's cost already booked in the budget.
	charged int
}

func (l *sharedLedger) exceeded(_ int, askedWithoutTools bool) *LimitError {
	if !askedWithoutTools {
		return nil
	}
	rounds, _ := l.budget.Spent()
	return &LimitError{Limit: LimitMaxRounds, Detail: fmt.Sprintf("%d rodadas com ferramenta somadas na revisao, teto %d", rounds, l.budget.maxToolRounds)}
}

func (l *sharedLedger) offersTools(int) bool {
	return l.budget.ToolRoundsLeft() > 0
}

func (l *sharedLedger) charge(t *Transcript, toolRound bool) *LimitError {
	if toolRound {
		l.budget.useToolRound()
	}
	total, exceeded := l.budget.addCost(t.CostTokens - l.charged)
	l.charged = t.CostTokens
	if !exceeded {
		return nil
	}
	return &LimitError{Limit: LimitMaxCostTokens, Detail: fmt.Sprintf("%d tokens de deliberacao somados na revisao, teto %d", total, l.budget.maxCostTokens)}
}

func (l *sharedLedger) noToolsReason() string {
	if l.budget.ToolRounds() <= 0 {
		return NotOfferedNoToolRound
	}
	return NotOfferedRoundsSpent
}

// ledger is the session's own limits, or the review's budget when set.
func (s Session) ledger() ledger {
	if s.Shared != nil {
		return &sharedLedger{budget: s.Shared}
	}
	return ownLedger{limits: s.Limits}
}
