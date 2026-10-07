package tools

import (
	"fmt"
	"sync"

	"github.com/Mpaape/AurumCode/internal/deliberation"
)

// LimitMaxReadBytes names the repository tools' byte ceiling, as the
// configuration spells it (deliberation.max_read_bytes).
const LimitMaxReadBytes = "max_read_bytes"

// LimitMaxCacheBytes names the ceiling of file bytes the repository tools
// keep in memory over one review (deliberation.max_cache_bytes).
const LimitMaxCacheBytes = "max_cache_bytes"

// defaultMaxReadBytes applies when a budget is built without a ceiling.
const defaultMaxReadBytes = 256 * 1024

// ByteBudget bounds the bytes the repository tools return to the model over
// one review. Once a result would cross it, nothing more is returned and
// the review is partial: the caller turns Exhausted into a deliberation
// limit, so the gate is inconclusive as the policy configures it.
type ByteBudget struct {
	mu        sync.Mutex
	max, used int
	exhausted bool
	// limit and detail name what exhausted the budget.
	limit, detail string
}

// NewByteBudget is a budget of max bytes (a non-positive max takes the
// default).
func NewByteBudget(max int) *ByteBudget {
	if max <= 0 {
		max = defaultMaxReadBytes
	}
	return &ByteBudget{max: max}
}

// Charge books n bytes, or refuses them and marks the budget exhausted.
func (b *ByteBudget) Charge(n int) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.exhausted {
		return fmt.Errorf("teto de %s atingido; a revisão fica parcial", b.limit)
	}
	if b.used+n > b.max {
		b.exhausted, b.limit = true, LimitMaxReadBytes
		b.detail = fmt.Sprintf("%d de %d bytes devolvidos pelas ferramentas do repositório", b.used, b.max)
		return fmt.Errorf("teto de %s atingido (%d de %d bytes já devolvidos); a revisão fica parcial", LimitMaxReadBytes, b.used, b.max)
	}
	b.used += n
	return nil
}

// Exhausted reports a budget that refused a result.
func (b *ByteBudget) Exhausted() bool {
	if b == nil {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.exhausted
}

// Exhaust marks the budget exhausted by another ceiling of the same
// review (the file cache's): nothing more is returned and the review is
// partial, exactly as for max_read_bytes.
func (b *ByteBudget) Exhaust(limit, detail string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.exhausted {
		b.exhausted, b.limit, b.detail = true, limit, detail
	}
}

// Limit names the ceiling that exhausted the budget ("" while it is not).
func (b *ByteBudget) Limit() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.limit
}

// Detail says how the budget was spent, for the limit error.
func (b *ByteBudget) Detail() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.detail != "" {
		return b.detail
	}
	return fmt.Sprintf("%d de %d bytes devolvidos pelas ferramentas do repositório", b.used, b.max)
}

// budgeted is a tool that charges a ByteBudget.
type budgeted interface{ byteBudget() *ByteBudget }

func (t *ReadFileTool) byteBudget() *ByteBudget { return t.rev.Budget() }
func (t *SearchTool) byteBudget() *ByteBudget   { return t.rev.Budget() }
func (t *SymbolTool) byteBudget() *ByteBudget   { return t.rev.Budget() }
func (t *DiffFileTool) byteBudget() *ByteBudget { return t.budget }

// PartialLimit is review.Deliberation's Partial hook for offers: the
// byte ceiling of their repository tools as a deliberation limit, once it
// refused a result; nil when no offer charges a budget.
func PartialLimit(offers []Offer) func() *deliberation.LimitError {
	var budgets []*ByteBudget
	for _, o := range offers {
		if b, ok := o.Tool.(budgeted); ok {
			budgets = append(budgets, b.byteBudget())
		}
	}
	if len(budgets) == 0 {
		return nil
	}
	return func() *deliberation.LimitError {
		for _, b := range budgets {
			if b.Exhausted() {
				return &deliberation.LimitError{Limit: b.Limit(), Detail: b.Detail()}
			}
		}
		return nil
	}
}
