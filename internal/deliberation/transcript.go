package deliberation

import "github.com/Mpaape/AurumCode/internal/llm"

// Call statuses recorded in the transcript.
const (
	StatusExecuted = "executed"
	StatusRefused  = "refused"
	StatusFailed   = "failed"
)

// Outcomes of a deliberation recorded in the transcript.
const (
	OutcomeAnswered = "answered"
)

// Transcript is the audit record of one deliberation: what was offered,
// what the model asked for and did not, each call with its redacted
// arguments, duration and summarized result, and how it ended.
type Transcript struct {
	Offered      []string `json:"offered"`
	Requested    []string `json:"requested"`
	NotRequested []string `json:"not_requested"`
	Rounds       int      `json:"rounds"`
	TokensIn     int      `json:"tokens_in"`
	TokensOut    int      `json:"tokens_out"`
	// BaseTokens is the base prompt as the provider measured it: the input
	// tokens of the first round, before any tool result existed. The prompt
	// budget already bounds it.
	BaseTokens int `json:"base_tokens"`
	// CostTokens is what the deliberation added beyond the base prompt, the
	// figure max_cost_tokens bounds: every round's output plus each round's
	// input above BaseTokens (the tool calls and results carried forward).
	CostTokens int    `json:"cost_tokens"`
	Calls      []Call `json:"calls"`
	Outcome    string `json:"outcome"`
	// Limit names the exceeded limit, empty when none was.
	Limit string `json:"limit,omitempty"`
	// Undecided says why the model never decided about the tools it was
	// offered (it never answered); NotRequested is then empty, because
	// nothing was declined.
	Undecided string `json:"undecided,omitempty"`
}

// MarkUndecided records that the model never finished deciding: no offered
// tool counts as declined.
func (t *Transcript) MarkUndecided(reason string) {
	t.Undecided = reason
	t.NotRequested = []string{}
}

// Call is one tool call the model made.
type Call struct {
	Round      int    `json:"round"`
	ID         string `json:"id"`
	Tool       string `json:"tool"`
	Arguments  string `json:"arguments"`
	Status     string `json:"status"`
	DurationMS int64  `json:"duration_ms"`
	Result     string `json:"result"`
	Digest     string `json:"digest,omitempty"`
}

// ResultDigests is every executed call's result digest, in call order: the
// part of the conversation a cache key must include.
func (t Transcript) ResultDigests() []string {
	var out []string
	for _, c := range t.Calls {
		if c.Status == StatusExecuted {
			out = append(out, c.Tool+"="+c.Digest)
		}
	}
	return out
}

// decide fills Requested and NotRequested from the calls.
func (t *Transcript) decide() {
	asked := map[string]bool{}
	t.Requested = []string{}
	for _, c := range t.Calls {
		if !asked[c.Tool] {
			asked[c.Tool] = true
			t.Requested = append(t.Requested, c.Tool)
		}
	}
	t.NotRequested = []string{}
	for _, name := range t.Offered {
		if !asked[name] {
			t.NotRequested = append(t.NotRequested, name)
		}
	}
}

// count adds one round's provider-reported tokens. The first round's input
// is the base prompt; a later round's input counts only what it carries
// beyond it, so a large base prompt alone never exhausts the ceiling.
func (t *Transcript) count(resp llm.ToolResponse) {
	t.Rounds++
	t.TokensIn += resp.TokensIn
	t.TokensOut += resp.TokensOut
	if t.Rounds == 1 {
		t.BaseTokens = resp.TokensIn
	} else if grown := resp.TokensIn - t.BaseTokens; grown > 0 {
		t.CostTokens += grown
	}
	t.CostTokens += resp.TokensOut
}
