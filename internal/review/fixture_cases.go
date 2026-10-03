package review

import (
	"encoding/json"
	"strings"
)

// FixtureEnvelopeKey is the reserved top-level key of an offline fixture
// whose answer depends on the prompt it receives. A fixture without it is
// returned verbatim, exactly as before.
const FixtureEnvelopeKey = "aurumcode_fixture"

// FixtureCase is one conditional answer of an offline fixture: the first
// case whose PromptContains occurs in the received prompt answers.
type FixtureCase struct {
	PromptContains string
	Response       string
}

// fixtureEnvelope is the on-disk shape of a conditional fixture.
type fixtureEnvelope struct {
	Cases []struct {
		PromptContains string          `json:"prompt_contains"`
		Response       json.RawMessage `json:"response"`
	} `json:"cases"`
	Default json.RawMessage `json:"default"`
}

// NewFixtureProvider builds the offline provider for a fixture file's
// content. A content carrying FixtureEnvelopeKey answers by case; any
// other content (including content that is not JSON) is the one fixed
// answer it always was.
func NewFixtureProvider(content, name, capturePath string) *FakeProvider {
	p := &FakeProvider{Response: content, NameStr: name, CapturePath: capturePath}
	var top map[string]json.RawMessage
	if json.Unmarshal([]byte(content), &top) != nil {
		return p
	}
	raw, ok := top[FixtureEnvelopeKey]
	if !ok {
		return p
	}
	var env fixtureEnvelope
	if json.Unmarshal(raw, &env) != nil {
		return p
	}
	p.Response = string(env.Default)
	for _, c := range env.Cases {
		p.Cases = append(p.Cases, FixtureCase{PromptContains: c.PromptContains, Response: string(c.Response)})
	}
	return p
}

// answerFor picks the fixture answer for prompt.
func (f *FakeProvider) answerFor(prompt string) string {
	for _, c := range f.Cases {
		if c.PromptContains != "" && strings.Contains(prompt, c.PromptContains) {
			return c.Response
		}
	}
	return f.Response
}
