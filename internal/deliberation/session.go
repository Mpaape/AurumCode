package deliberation

import (
	"context"
	"errors"
	"time"

	"github.com/Mpaape/AurumCode/internal/llm"
)

// Session is one bounded deliberation.
type Session struct {
	Caller  Caller
	Tools   []Tool
	Limits  Limits
	Options llm.Options
	// Shared, when set, is the review's Budget this session draws from
	// beside the review's other sessions (one per analyst): tool rounds and
	// cost are then summed over the sessions instead of bounded by Limits
	// alone. nil keeps Limits for this session alone.
	Shared *Budget
	// Redact is applied to every argument recorded in the transcript and to
	// every tool result before it is sent to the model. nil keeps the text.
	Redact func(string) string
	// Clock measures each call's duration; nil uses time.Now.
	Clock func() time.Time
}

// Outcome is a finished deliberation: the model's final answer and the
// transcript. On error Answer is the zero value and Transcript records how
// far the conversation went.
type Outcome struct {
	Answer     llm.Response
	Transcript Transcript
}

// Run converses until the model answers without asking for a tool, or a
// limit is exceeded. Exceeding MaxRounds, MaxCostTokens or PerToolTimeout
// returns a *LimitError and no answer. MaxCostTokens bounds the
// deliberation's own cost (Transcript.CostTokens), never the base prompt; a
// MaxCostTokens of 0 sets no ceiling.
// With Shared set, the rounds that ask for a tool and the cost are the
// review's (ledger.go): the session answers when the budget is spent.
func (s Session) Run(ctx context.Context, messages []llm.Message) (Outcome, error) {
	if err := s.Limits.Validate(); err != nil {
		return Outcome{}, err
	}
	tools, specs := s.index()
	// Offered lists only what some round actually offered: a session of a
	// review's Budget may start after the tool rounds are spent.
	out := Outcome{Transcript: Transcript{Offered: []string{}, Calls: []Call{}}}
	conversation := append([]llm.Message(nil), messages...)
	ledger := s.ledger()
	// last is the latest round's reply. Text that came beside tool calls is
	// never an answer: only a round without calls ends the conversation.
	var last llm.ToolResponse
	askedWithoutTools := false
	for round := 1; ; round++ {
		if limit := ledger.exceeded(round, askedWithoutTools); limit != nil {
			return s.stop(out, limit)
		}
		roundSpecs := specs
		if !ledger.offersTools(round) {
			// A round that offers no tool (the last allowed one, or any
			// round after the review's tool rounds are spent) asks for the
			// review with the evidence already gathered: a model that keeps
			// asking for context would otherwise spend every round and
			// publish nothing. A tool call here still ends at the limit
			// above.
			roundSpecs = nil
			conversation = append(conversation, llm.Message{Role: llm.RoleUser, Content: finalRoundInstruction})
		}
		out.Transcript.recordOffer(roundSpecs, specs, ledger)
		resp, err := s.Caller.CompleteWithTools(ctx, conversation, roundSpecs, s.Options)
		if err != nil {
			return s.stop(out, err)
		}
		last = resp
		out.Transcript.count(resp)
		if limit := ledger.charge(&out.Transcript, roundSpecs != nil && len(resp.ToolCalls) > 0); limit != nil {
			return s.stop(out, limit)
		}
		if len(resp.ToolCalls) > 0 && roundSpecs == nil {
			// Still asking with no tool offered: the next iteration is past
			// the rounds and stops at the limit above.
			askedWithoutTools = true
			continue
		}
		if len(resp.ToolCalls) == 0 {
			out.Answer = last.Response
			out.Transcript.Outcome = OutcomeAnswered
			out.Transcript.decide()
			return out, nil
		}
		conversation = append(conversation, llm.Message{Role: llm.RoleAssistant, Content: resp.Text, ToolCalls: resp.ToolCalls})
		for _, call := range resp.ToolCalls {
			msg, record, err := s.execute(ctx, round, tools, call)
			out.Transcript.Calls = append(out.Transcript.Calls, record)
			if err != nil {
				return s.stop(out, err)
			}
			conversation = append(conversation, msg)
		}
	}
}

// finalRoundInstruction is what the last allowed round tells the model:
// no further tool is available, answer now in the requested format.
const finalRoundInstruction = "No more tools are available in this review: this is the last round. Answer now with the final review in the requested format, using only the evidence already gathered; state any limitation it leaves."

// recordOffer records what this round offered: the session's tools the
// first time a round offers them, or, while no round has, why none did.
func (t *Transcript) recordOffer(roundSpecs, specs []llm.ToolSpec, l ledger) {
	switch {
	case len(t.Offered) > 0 || len(specs) == 0:
	case roundSpecs != nil:
		t.Offered, t.NotOffered = specNames(specs), ""
	case t.NotOffered == "":
		t.NotOffered = l.noToolsReason()
	}
}

// stop ends a deliberation with err: the transcript keeps what happened,
// the answer stays empty.
func (s Session) stop(out Outcome, err error) (Outcome, error) {
	out.Answer = llm.Response{}
	out.Transcript.Outcome = err.Error()
	var limit *LimitError
	if errors.As(err, &limit) {
		out.Transcript.Outcome = limit.Reason()
		out.Transcript.Limit = limit.Limit
	}
	out.Transcript.decide()
	return out, err
}

// index maps the offered tools by name and lists their specs.
func (s Session) index() (map[string]Tool, []llm.ToolSpec) {
	byName := make(map[string]Tool, len(s.Tools))
	specs := make([]llm.ToolSpec, 0, len(s.Tools))
	for _, t := range s.Tools {
		spec := t.Spec()
		byName[spec.Name] = t
		specs = append(specs, spec)
	}
	return byName, specs
}

func specNames(specs []llm.ToolSpec) []string {
	names := make([]string, 0, len(specs))
	for _, s := range specs {
		names = append(names, s.Name)
	}
	return names
}

func (s Session) redact(text string) string {
	if s.Redact == nil {
		return text
	}
	return s.Redact(text)
}

func (s Session) now() time.Time {
	if s.Clock == nil {
		return time.Now()
	}
	return s.Clock()
}
