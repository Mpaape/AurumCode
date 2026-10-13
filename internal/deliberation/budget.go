package deliberation

import "sync"

// Budget is the deliberation ceiling of one review when more than one
// session draws from it (one per analyst): the rounds that ask for a tool
// and the cost tokens are summed over every session, so selecting analysts
// never multiplies what a single pass may spend. Each session still answers
// once; a session that starts with no tool round left is asked for its
// answer at once, with the evidence the earlier sessions gathered.
type Budget struct {
	mu            sync.Mutex
	maxToolRounds int
	maxCostTokens int
	toolRounds    int
	costTokens    int
}

// NewBudget is the review's budget under limits: MaxRounds less the answer
// round a single pass keeps for itself is the number of rounds that may ask
// for a tool, and MaxCostTokens is the deliberation cost the review may add
// beyond its base prompts.
func NewBudget(limits Limits) *Budget {
	return &Budget{maxToolRounds: limits.MaxRounds - 1, maxCostTokens: limits.MaxCostTokens}
}

// ToolRoundsLeft is how many rounds may still ask for a tool.
func (b *Budget) ToolRoundsLeft() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.maxToolRounds - b.toolRounds
}

// Spent is what the sessions booked so far: rounds that asked for a tool
// and cost tokens.
func (b *Budget) Spent() (toolRounds, costTokens int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.toolRounds, b.costTokens
}

// useToolRound books one round that asked for a tool.
func (b *Budget) useToolRound() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.toolRounds++
}

// addCost books n cost tokens and reports the total and whether it is past
// the ceiling.
func (b *Budget) addCost(n int) (total int, exceeded bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.costTokens += n
	return b.costTokens, b.costTokens > b.maxCostTokens
}
