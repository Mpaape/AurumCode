package llm

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

var errDown = errors.New("503 service unavailable")

// AC-001: the primary fails, the next provider answers the same request
// and the switch is announced with both names.
func TestAUR605PrimaryFailsFallbackAnswers(t *testing.T) {
	primary := &mockProvider{name: "primario", err: errDown}
	reserva := &mockProvider{name: "reserva", response: Response{Text: "ok"}}
	var notes []string
	chain := NewFallback(primary, []Provider{reserva}, func(failed string, err error, next string) {
		notes = append(notes, failed+"|"+err.Error()+"|"+next)
	})

	resp, err := NewOrchestrator(chain, nil, nil).Complete(context.Background(), "p", DefaultOptions())
	if err != nil {
		t.Fatalf("fallback should answer: %v", err)
	}
	if resp.Text != "ok" || primary.callCount != 1 || reserva.callCount != 1 {
		t.Fatalf("resp=%q primary=%d reserva=%d", resp.Text, primary.callCount, reserva.callCount)
	}
	if len(notes) != 1 || notes[0] != "primario|503 service unavailable|reserva" {
		t.Fatalf("switch not announced: %v", notes)
	}
	if chain.Name() != "primario" {
		t.Fatalf("chain name = %q, want the primary's", chain.Name())
	}
}

// AC-001: a healthy primary is the only one called.
func TestAUR605HealthyPrimaryAloneIsCalled(t *testing.T) {
	primary := &mockProvider{name: "primario", response: Response{Text: "ok"}}
	reserva := &mockProvider{name: "reserva", response: Response{Text: "nao"}}
	resp, err := NewFallback(primary, []Provider{reserva}, nil).Complete("p", DefaultOptions())
	if err != nil || resp.Text != "ok" || reserva.callCount != 0 {
		t.Fatalf("resp=%q err=%v reserva=%d", resp.Text, err, reserva.callCount)
	}
}

// AC-002: when every provider fails the request fails, naming each one.
func TestAUR605AllFailNamesEveryProvider(t *testing.T) {
	a := &mockProvider{name: "a", err: errDown}
	b := &mockProvider{name: "b", err: errors.New("401 unauthorized")}
	c := &mockProvider{name: "c", err: errors.New("timeout")}
	_, err := NewFallback(a, []Provider{b, c}, nil).Complete("p", DefaultOptions())
	if !errors.Is(err, ErrAllProvidersFailed) || !errors.Is(err, errDown) {
		t.Fatalf("err = %v, want ErrAllProvidersFailed wrapping each failure", err)
	}
	for _, want := range []string{"3 tried", "a: 503", "b: 401", "c: timeout"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not name %q", err, want)
		}
	}
}

// AC-003: without fallbacks nothing changes: the primary itself is used.
func TestAUR605NoFallbackIsThePrimaryItself(t *testing.T) {
	primary := &mockProvider{name: "primario"}
	if got := NewFallback(primary, nil, nil); got != Provider(primary) {
		t.Fatalf("single provider wrapped: %T", got)
	}
	if got := NewFallback(primary, []Provider{nil}, nil); got != Provider(primary) {
		t.Fatalf("nil fallback wrapped: %T", got)
	}
}

// AC-004: a tool conversation only goes to members that call tools, and a
// chain without any tool caller does not advertise the capability.
func TestAUR605ToolRoundsSkipMembersWithoutTools(t *testing.T) {
	plain := &mockProvider{name: "sem-ferramentas", response: Response{Text: "x"}}
	failing := &failingToolProvider{toolProvider{mockProvider: mockProvider{name: "ferramentas-fora"}}}
	tools := &toolProvider{mockProvider: mockProvider{name: "ferramentas"}, rounds: []ToolResponse{
		{Response: Response{Text: "{}"}, ToolCalls: []ToolCall{{ID: "c", Name: "t", Arguments: json.RawMessage(`{}`)}}},
	}}
	chain := NewFallback(failing, []Provider{plain, tools}, nil)
	caller, ok := AsToolCaller(chain)
	if !ok {
		t.Fatal("chain with a tool caller must call tools")
	}
	resp, err := caller.CompleteWithTools(nil, nil, DefaultOptions())
	if err != nil || len(resp.ToolCalls) != 1 || plain.callCount != 0 {
		t.Fatalf("resp=%+v err=%v plain=%d", resp, err, plain.callCount)
	}
	if _, ok := AsToolCaller(NewFallback(plain, []Provider{&mockProvider{name: "outro"}}, nil)); ok {
		t.Fatal("chain without tool callers advertises tools")
	}
}

// slowFailing fails only after its delay, like a client timeout.
type slowFailing struct {
	mockProvider
	delay time.Duration
}

func (s *slowFailing) Complete(string, Options) (Response, error) {
	time.Sleep(s.delay)
	return Response{}, errors.New("client timeout")
}

// AC-005: the orchestrator bounds a chain by one timeout per member. A
// primary that spends most of one timeout before failing still leaves the
// fallback time to answer.
func TestAUR605ChainTimeoutLeavesTimeForTheFallback(t *testing.T) {
	t.Setenv("AURUMCODE_LLM_TIMEOUT_SECONDS", "1")
	primary := &slowFailing{mockProvider: mockProvider{name: "lento"}, delay: 800 * time.Millisecond}
	reserva := &slowFailing{mockProvider: mockProvider{name: "reserva-lenta"}, delay: 0}
	answer := &mockProvider{name: "reserva", response: Response{Text: "ok"}}
	slowAnswer := &delayed{inner: answer, delay: 500 * time.Millisecond}
	chain := NewFallback(primary, []Provider{reserva, slowAnswer}, nil)
	resp, err := NewOrchestrator(chain, nil, nil).Complete(context.Background(), "p", DefaultOptions())
	if err != nil || resp.Text != "ok" {
		t.Fatalf("fallback cut by a single timeout: resp=%q err=%v", resp.Text, err)
	}
}

// delayed answers like inner after delay.
type delayed struct {
	inner Provider
	delay time.Duration
}

func (d *delayed) Complete(p string, o Options) (Response, error) {
	time.Sleep(d.delay)
	return d.inner.Complete(p, o)
}
func (d *delayed) Tokens(s string) (int, error) { return d.inner.Tokens(s) }
func (d *delayed) Name() string                 { return d.inner.Name() }

// AC-009: a run answered by a fallback is marked, so the review cache never
// stores it under the primary's identity; the primary's URL stays visible.
func TestAUR605FallbackAnswerIsMarkedAndURLIsThePrimarys(t *testing.T) {
	primary := &urlProvider{mockProvider: mockProvider{name: "primario", response: Response{Text: "ok"}}, url: "https://a.example"}
	reserva := &mockProvider{name: "reserva", response: Response{Text: "ok"}}
	chain := NewFallback(primary, []Provider{reserva}, nil)
	marked, _ := chain.(interface{ FellBack() bool })
	if _, err := chain.Complete("p", DefaultOptions()); err != nil || marked.FellBack() {
		t.Fatalf("healthy primary marked as fallback: err=%v", err)
	}
	primary.err = errDown
	if _, err := chain.Complete("p", DefaultOptions()); err != nil || !marked.FellBack() {
		t.Fatalf("fallback answer not marked: err=%v", err)
	}
	if u, ok := As[interface{ BaseURL() string }](chain); !ok || u.BaseURL() != "https://a.example" {
		t.Fatal("chain hides the primary's base URL")
	}
}

type urlProvider struct {
	mockProvider
	url string
}

func (u *urlProvider) BaseURL() string { return u.url }
