package deliberation

import (
	"context"
	"encoding/json"
	"time"

	"github.com/Mpaape/AurumCode/internal/llm"
)

// Result is what one tool run returns: Content goes back to the model as
// the tool message, Summary is the one-line record kept in the transcript,
// and Digest identifies the full result (the caller's cache keys include
// it, so a different tool result never reuses a verdict).
type Result struct {
	Content string
	Summary string
	Digest  string
}

// Tool is one capability the model may ask for. Spec's Parameters is the
// JSON Schema its arguments must satisfy; Run is only ever called with
// arguments that passed it. A tool never writes and never reaches the
// network beyond what the product already does.
type Tool interface {
	Spec() llm.ToolSpec
	Run(ctx context.Context, args json.RawMessage) (Result, error)
}

// Caller is the model side of one round: llm.Orchestrator implements it,
// reserving and committing the cost of every round.
type Caller interface {
	CompleteWithTools(ctx context.Context, messages []llm.Message, tools []llm.ToolSpec, opts llm.Options) (llm.ToolResponse, error)
}

// Limits bound one deliberation. Every field must be positive; Validate
// says which one is not.
type Limits struct {
	// MaxRounds is how many model calls one deliberation may make, the
	// final answer included.
	MaxRounds int
	// MaxCostTokens bounds the tokens (input plus output, as the provider
	// reports them) summed over every round. It is counted in tokens so it
	// holds without a price table; a USD ceiling (--limite) is enforced
	// per round by the orchestrator's reservation as well.
	MaxCostTokens int
	// PerToolTimeout bounds one tool run.
	PerToolTimeout time.Duration
}
