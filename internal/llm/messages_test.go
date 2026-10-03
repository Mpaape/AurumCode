package llm

import (
	"context"
	"testing"
)

type promptOnlyProvider struct{ got string }

func (p *promptOnlyProvider) Complete(prompt string, _ Options) (Response, error) {
	p.got = prompt
	return Response{Text: "ok"}, nil
}
func (p *promptOnlyProvider) Tokens(input string) (int, error) { return len(input) / 4, nil }
func (p *promptOnlyProvider) Name() string                     { return "prompt-only" }

type messageProvider struct {
	promptOnlyProvider
	messages []Message
}

func (p *messageProvider) CompleteMessages(messages []Message, _ Options) (Response, error) {
	p.messages = messages
	return Response{Text: "ok"}, nil
}

type passThrough struct{ inner Provider }

func (p passThrough) Complete(prompt string, opts Options) (Response, error) {
	return p.inner.Complete(prompt, opts)
}
func (p passThrough) Tokens(input string) (int, error) { return p.inner.Tokens(input) }
func (p passThrough) Name() string                     { return p.inner.Name() }
func (p passThrough) Unwrap() Provider                 { return p.inner }

func TestCompleteMessagesUsesCapabilityOrFlattens(t *testing.T) {
	msgs := SystemUserMessages("system text", "user text")

	plain := &promptOnlyProvider{}
	if _, err := NewOrchestrator(plain, nil, nil).CompleteMessages(context.Background(), msgs, Options{}); err != nil {
		t.Fatal(err)
	}
	if plain.got != "system text\n\nuser text" {
		t.Fatalf("a provider without the capability got %q, want the flattened system+user", plain.got)
	}

	capable := &messageProvider{}
	if _, err := NewOrchestrator(passThrough{inner: capable}, nil, nil).CompleteMessages(context.Background(), msgs, Options{}); err != nil {
		t.Fatal(err)
	}
	if len(capable.messages) != 2 || capable.messages[0].Role != RoleSystem || capable.messages[1].Role != RoleUser {
		t.Fatalf("the capable provider behind a pass-through decorator got %+v", capable.messages)
	}
}
