package llm

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"testing"

	"github.com/Mpaape/AurumCode/internal/llm/cost"
)

// toolProvider is a ToolCaller that answers each round with fixed usage
// and resolves to a priced model.
type toolProvider struct {
	mockProvider
	rounds   []ToolResponse
	toolCall int
}

func (p *toolProvider) ResolveModel(Options) string { return "priced" }

func (p *toolProvider) CompleteWithTools(_ []Message, _ []ToolSpec, _ Options) (ToolResponse, error) {
	r := p.rounds[p.toolCall%len(p.rounds)]
	p.toolCall++
	return r, nil
}

var pricedModel = map[string]cost.PriceMap{"priced": {InputPer1K: 1, OutputPer1K: 2}}

// AC-003: every round is reserved and committed, so the tracker holds the
// sum of the rounds' actual cost.
func TestAUR580EveryToolRoundIsReservedAndCommitted(t *testing.T) {
	p := &toolProvider{mockProvider: mockProvider{name: "tools"}, rounds: []ToolResponse{
		{Response: Response{TokensIn: 1000, TokensOut: 0}, ToolCalls: []ToolCall{{ID: "c", Name: "t", Arguments: json.RawMessage(`{}`)}}},
		{Response: Response{TokensIn: 500, TokensOut: 1000, Text: "{}"}},
	}}
	tracker := cost.NewTracker(100, 1000, pricedModel)
	o := NewOrchestrator(p, nil, tracker)
	for i := 0; i < 2; i++ {
		if _, err := o.CompleteWithTools(context.Background(), SystemUserMessages("s", "u"), nil, Options{}); err != nil {
			t.Fatal(err)
		}
	}
	perRun, _ := tracker.Remaining()
	spent := 100 - perRun
	want := (1000.0/1000)*1 + (500.0/1000)*1 + (1000.0/1000)*2
	if math.Abs(spent-want) > 1e-9 {
		t.Fatalf("tracker committed %.4f USD over two rounds, want the sum of the rounds %.4f", spent, want)
	}
}

// AC-003: the ceiling is checked before every round, not once per
// conversation.
func TestAUR580RoundOverTheCeilingIsRefusedBeforeTheCall(t *testing.T) {
	p := &toolProvider{mockProvider: mockProvider{name: "tools", tokenCount: 10}, rounds: []ToolResponse{{Response: Response{TokensIn: 10, TokensOut: 10, Text: "{}"}}}}
	o := NewOrchestrator(p, nil, cost.NewTracker(0.0001, 1, pricedModel))
	_, err := o.CompleteWithTools(context.Background(), SystemUserMessages("s", "u"), nil, Options{MaxTokens: 1000})
	if !errors.Is(err, ErrBudgetExceeded) || p.toolCall != 0 {
		t.Fatalf("err=%v calls=%d, want the round refused before any call", err, p.toolCall)
	}
}

// AC-003: a provider without ToolCaller never receives a tool round, not
// even as a fallback.
func TestAUR580NoFallbackToAProviderWithoutTools(t *testing.T) {
	plain := &mockProvider{name: "plain", response: Response{Text: "{}"}}
	o := NewOrchestrator(plain, nil, nil)
	if o.SupportsTools() {
		t.Fatal("a chain without ToolCaller must not support tools")
	}
	if _, err := o.CompleteWithTools(context.Background(), nil, nil, Options{}); !errors.Is(err, ErrToolsUnsupported) {
		t.Fatalf("err = %v, want ErrToolsUnsupported", err)
	}
	failing := &failingToolProvider{toolProvider: toolProvider{mockProvider: mockProvider{name: "tools"}}}
	o = NewOrchestrator(failing, []Provider{plain}, nil)
	if _, err := o.CompleteWithTools(context.Background(), nil, nil, Options{}); !errors.Is(err, ErrAllProvidersFailed) {
		t.Fatalf("err = %v, want the tool provider's failure", err)
	}
	if plain.callCount != 0 {
		t.Fatalf("the provider without tools was called %d time(s) as a fallback", plain.callCount)
	}
}

type failingToolProvider struct{ toolProvider }

func (p *failingToolProvider) CompleteWithTools([]Message, []ToolSpec, Options) (ToolResponse, error) {
	return ToolResponse{}, errors.New("down")
}

// The answer schema is derived from the Go struct it is decoded into.
func TestAUR580SchemaOfStruct(t *testing.T) {
	type inner struct {
		Name string `json:"name" desc:"o nome"`
	}
	type sample struct {
		Items []inner          `json:"items"`
		Count int              `json:"count,omitempty"`
		Meta  map[string]bool  `json:"meta,omitempty"`
		Skip  string           `json:"-"`
		Raw   *json.RawMessage `json:"raw,omitempty"`
	}
	var got map[string]any
	if err := json.Unmarshal(SchemaOf(sample{}), &got); err != nil {
		t.Fatal(err)
	}
	props := got["properties"].(map[string]any)
	if _, ok := props["Skip"]; ok || props["count"].(map[string]any)["type"] != "integer" {
		t.Fatalf("properties = %v", props)
	}
	items := props["items"].(map[string]any)["items"].(map[string]any)
	if items["properties"].(map[string]any)["name"].(map[string]any)["description"] != "o nome" {
		t.Fatalf("nested schema = %v", items)
	}
	if req := got["required"].([]any); len(req) != 1 || req[0] != "items" || got["additionalProperties"] != false {
		t.Fatalf("required=%v additional=%v", req, got["additionalProperties"])
	}
}
