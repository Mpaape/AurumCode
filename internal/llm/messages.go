package llm

import "strings"

// MessageCompleter is the capability of a provider that accepts a
// conversation as separate role messages (system, user) instead of one
// flattened prompt. A provider without it receives FlattenMessages(...)
// through Complete, byte for byte what a single-string caller sent before
// messages existed.
type MessageCompleter interface {
	CompleteMessages(messages []Message, opts Options) (Response, error)
}

// messageSeparator joins message contents when a conversation is flattened
// for a provider that only takes one prompt string.
const messageSeparator = "\n\n"

// FlattenMessages joins every message's content, in order, with a blank
// line between them. It is the single definition of the flattened form:
// the orchestrator's token estimate and the fallback Complete call both use
// it, so the budget is checked against exactly the bytes that are sent.
func FlattenMessages(messages []Message) string {
	parts := make([]string, 0, len(messages))
	for _, m := range messages {
		parts = append(parts, m.Content)
	}
	return strings.Join(parts, messageSeparator)
}

// SystemUserMessages builds the two-message conversation a single-shot
// review sends: trusted instructions as the system message, the reviewed
// material as the user message.
func SystemUserMessages(system, user string) []Message {
	return []Message{{Role: RoleSystem, Content: system}, {Role: RoleUser, Content: user}}
}

// Unwrapper is implemented by a provider decorator that forwards requests
// UNCHANGED to the provider it wraps, so a capability of the wrapped
// provider (ModelResolver, ToolCaller, MessageCompleter) can be found
// through it. A decorator that alters the request (injects text, swaps the
// model) must not implement it: unwrapping would let a caller reach the
// inner provider and skip that alteration.
type Unwrapper interface {
	Unwrap() Provider
}

// maxUnwrapDepth bounds the decorator walk so a cyclic Unwrap chain cannot
// loop forever.
const maxUnwrapDepth = 32

// As finds the first provider in the decorator chain starting at p that
// implements capability T, following Unwrapper links. It replaces bare
// type assertions, which a decorator silently defeats.
func As[T any](p Provider) (T, bool) {
	for depth := 0; p != nil && depth < maxUnwrapDepth; depth++ {
		if capability, ok := p.(T); ok {
			return capability, true
		}
		u, ok := p.(Unwrapper)
		if !ok {
			break
		}
		p = u.Unwrap()
	}
	var zero T
	return zero, false
}
