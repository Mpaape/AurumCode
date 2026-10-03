package review

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/Mpaape/AurumCode/internal/llm"
)

// FixtureToolCase is one tool call an offline fixture makes, emulating a
// model that decides from what the prompt shows: it asks for Tool when the
// tool is offered, the prompt contains PromptContains (when set) and the
// change summary counts more than LinesAbove changed lines (when set). A
// case asks once per conversation unless EveryRound is set.
type FixtureToolCase struct {
	PromptContains string          `json:"prompt_contains"`
	LinesAbove     int             `json:"lines_above"`
	Tool           string          `json:"tool"`
	Arguments      json.RawMessage `json:"arguments"`
	EveryRound     bool            `json:"every_round"`
}

// ToolFixtureProvider is the offline provider of a fixture that declares
// tool calls: a FakeProvider that also implements llm.ToolCaller. A
// fixture without tool calls never builds one, so it is never offered
// tools.
type ToolFixtureProvider struct {
	*FakeProvider
	ToolCases []FixtureToolCase
}

// fixtureToolEnvelope is the tool part of a conditional fixture.
type fixtureToolEnvelope struct {
	ToolCalls []FixtureToolCase `json:"tool_calls"`
}

// NewOfflineProvider is the provider of an offline fixture file: a
// ToolFixtureProvider when the fixture declares tool calls, the plain
// fixture provider otherwise.
func NewOfflineProvider(content, name, capturePath string) llm.Provider {
	base := NewFixtureProvider(content, name, capturePath)
	var top map[string]json.RawMessage
	if json.Unmarshal([]byte(content), &top) != nil {
		return base
	}
	var env fixtureToolEnvelope
	if raw, ok := top[FixtureEnvelopeKey]; !ok || json.Unmarshal(raw, &env) != nil || len(env.ToolCalls) == 0 {
		return base
	}
	return &ToolFixtureProvider{FakeProvider: base, ToolCases: env.ToolCalls}
}

// CompleteWithTools implements llm.ToolCaller.
func (f *ToolFixtureProvider) CompleteWithTools(messages []llm.Message, tools []llm.ToolSpec, opts llm.Options) (llm.ToolResponse, error) {
	text := llm.FlattenMessages(messages)
	calls := f.callsFor(messages, tools, text)
	resp, err := f.Complete(text, opts)
	if err != nil {
		return llm.ToolResponse{}, err
	}
	// Like a real model, the fixture may write text beside its tool calls;
	// that text is the answer it would give now, never a final answer.
	return llm.ToolResponse{Response: resp, ToolCalls: calls}, nil
}

// callsFor is the tool calls the fixture makes in this round.
func (f *ToolFixtureProvider) callsFor(messages []llm.Message, tools []llm.ToolSpec, text string) []llm.ToolCall {
	offered := map[string]bool{}
	for _, t := range tools {
		offered[t.Name] = true
	}
	called := map[string]bool{}
	round := 1
	for _, m := range messages {
		if m.Role == llm.RoleAssistant && len(m.ToolCalls) > 0 {
			round++
			for _, c := range m.ToolCalls {
				called[c.Name] = true
			}
		}
	}
	var calls []llm.ToolCall
	for _, c := range f.ToolCases {
		if !offered[c.Tool] || (called[c.Tool] && !c.EveryRound) || !c.matches(text) {
			continue
		}
		args := c.Arguments
		if len(args) == 0 {
			args = json.RawMessage(`{}`)
		}
		calls = append(calls, llm.ToolCall{ID: fmt.Sprintf("call-%d-%s", round, c.Tool), Name: c.Tool, Arguments: args})
	}
	return calls
}

// changeSummaryLine reads one count of the prompt's change summary.
var changeSummaryLine = regexp.MustCompile(`(?m)^- Lines (?:added|deleted): (\d+)$`)

// matches applies the case's conditions to the prompt.
func (c FixtureToolCase) matches(text string) bool {
	if c.PromptContains != "" && !strings.Contains(text, c.PromptContains) {
		return false
	}
	if c.LinesAbove > 0 {
		changed := 0
		for _, m := range changeSummaryLine.FindAllStringSubmatch(text, -1) {
			n, _ := strconv.Atoi(m[1])
			changed += n
		}
		return changed > c.LinesAbove
	}
	return true
}
