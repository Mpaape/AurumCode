package deliberation

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
	Calls        []Call   `json:"calls"`
	Outcome      string   `json:"outcome"`
	// Limit names the exceeded limit, empty when none was.
	Limit string `json:"limit,omitempty"`
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
