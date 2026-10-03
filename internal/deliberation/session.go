package deliberation

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Mpaape/AurumCode/internal/llm"
)

// maxToolMessageBytes caps what one tool result sends back to the model;
// the full result stays with the tool (a scanner's findings reach the gate
// whatever the cap).
const maxToolMessageBytes = 8192

// truncationNote ends a tool message cut by maxToolMessageBytes.
const truncationNote = "\n[resultado truncado pelo limite da mensagem de ferramenta]"

// Session is one bounded deliberation.
type Session struct {
	Caller  Caller
	Tools   []Tool
	Limits  Limits
	Options llm.Options
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
// returns a *LimitError and no answer.
func (s Session) Run(ctx context.Context, messages []llm.Message) (Outcome, error) {
	if err := s.Limits.Validate(); err != nil {
		return Outcome{}, err
	}
	tools, specs := s.index()
	out := Outcome{Transcript: Transcript{Offered: specNames(specs), Calls: []Call{}}}
	conversation := append([]llm.Message(nil), messages...)
	// last is the latest round's reply. Text that came beside tool calls is
	// never an answer: only a round without calls ends the conversation.
	var last llm.ToolResponse
	for round := 1; ; round++ {
		if round > s.Limits.MaxRounds {
			return s.stop(out, &LimitError{Limit: LimitMaxRounds, Detail: fmt.Sprintf("%d rodadas sem resposta final", s.Limits.MaxRounds)})
		}
		resp, err := s.Caller.CompleteWithTools(ctx, conversation, specs, s.Options)
		if err != nil {
			return s.stop(out, err)
		}
		last = resp
		out.Transcript.Rounds = round
		out.Transcript.TokensIn += resp.TokensIn
		out.Transcript.TokensOut += resp.TokensOut
		if used := out.Transcript.TokensIn + out.Transcript.TokensOut; used > s.Limits.MaxCostTokens {
			return s.stop(out, &LimitError{Limit: LimitMaxCostTokens, Detail: fmt.Sprintf("%d tokens usados, teto %d", used, s.Limits.MaxCostTokens)})
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

// stop ends a deliberation with err: the transcript keeps what happened,
// the answer stays empty.
func (s Session) stop(out Outcome, err error) (Outcome, error) {
	out.Answer = llm.Response{}
	out.Transcript.Outcome = err.Error()
	var limit *LimitError
	if errors.As(err, &limit) {
		out.Transcript.Outcome = limit.Reason()
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
