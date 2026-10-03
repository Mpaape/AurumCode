package deliberation

import (
	"errors"
	"fmt"
)

// ReasonLimit is the inconclusive motive of a deliberation that exceeded
// one of its limits.
const ReasonLimit = "deliberation_limit"

// The names of the limits, as the configuration spells them.
const (
	LimitMaxRounds      = "max_rounds"
	LimitMaxCostTokens  = "max_cost_tokens"
	LimitPerToolTimeout = "per_tool_timeout_seconds"
)

// ErrLimit marks every LimitError.
var ErrLimit = errors.New(ReasonLimit)

// LimitError is a deliberation stopped by one of its limits. The caller
// treats it as an inconclusive review: no part of the conversation is a
// verdict.
type LimitError struct {
	Limit  string
	Detail string
}

func (e *LimitError) Error() string {
	return fmt.Sprintf("%s: %s (%s)", ReasonLimit, e.Limit, e.Detail)
}

func (e *LimitError) Unwrap() error { return ErrLimit }

// Reason is the motive a caller records: deliberation_limit:<limit>.
func (e *LimitError) Reason() string { return ReasonLimit + ":" + e.Limit }

// Validate refuses a non-positive limit, naming it.
func (l Limits) Validate() error {
	switch {
	case l.MaxRounds <= 0:
		return fmt.Errorf("%s must be positive (got %d)", LimitMaxRounds, l.MaxRounds)
	case l.MaxCostTokens <= 0:
		return fmt.Errorf("%s must be positive (got %d)", LimitMaxCostTokens, l.MaxCostTokens)
	case l.PerToolTimeout <= 0:
		return fmt.Errorf("%s must be positive (got %s)", LimitPerToolTimeout, l.PerToolTimeout)
	}
	return nil
}
