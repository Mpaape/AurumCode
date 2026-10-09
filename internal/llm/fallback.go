package llm

import (
	"fmt"
	"strings"
	"sync/atomic"
	"time"
)

// FallbackNotice is called when one provider of a Fallback chain failed and
// the next one is about to be tried. It receives the failed provider's
// name, its error and the next provider's name, so the operator sees the
// switch: a silent switch would hide an outage of the primary.
type FallbackNotice func(failed string, err error, next string)

// Fallback is a provider that tries an ordered list of providers, the
// primary first, and answers with the first one that succeeds. It is a
// provider itself, so every decorator and orchestrator in front of it
// (profile prefix, fixed cost key, the tool-round loop) keeps working
// unchanged: the switch happens below them, on the same request.
//
// Only an error moves to the next provider. A response that arrived but is
// unusable (outside the schema) is the reviewer's to judge, never a reason
// to ask another model the same question until one agrees.
type Fallback struct {
	providers []Provider
	head      Provider
	notice    FallbackNotice
	fellBack  *atomic.Bool
}

// NewFallback returns primary alone when there is no usable fallback, so a
// single configured provider behaves exactly as before. With fallbacks it
// returns a *Fallback, or a *ToolFallback when at least one member can
// call tools (a chain that cannot must not advertise the capability).
func NewFallback(primary Provider, fallbacks []Provider, notice FallbackNotice) Provider {
	members := make([]Provider, 0, len(fallbacks)+1)
	for _, p := range append([]Provider{primary}, fallbacks...) {
		if p != nil {
			members = append(members, p)
		}
	}
	if len(members) == 0 {
		return nil
	}
	if len(members) == 1 {
		return members[0]
	}
	f := &Fallback{providers: members, head: members[0], notice: notice, fellBack: &atomic.Bool{}}
	for _, p := range members {
		if _, ok := AsToolCaller(p); ok {
			return &ToolFallback{Fallback: f}
		}
	}
	return f
}

// primary is the provider the operator chose first; in a tool round's
// subset it may not be the first member tried.
func (f *Fallback) primary() Provider { return f.head }

// Members returns the providers' names in the order they are tried.
func (f *Fallback) Members() []string {
	names := make([]string, len(f.providers))
	for i, p := range f.providers {
		names[i] = p.Name()
	}
	return names
}

// FellBack reports whether any request so far was answered by a member
// other than the primary. A caller that stores answers under the primary's
// identity (the review cache) must not store them then: a later run with a
// healthy primary would be served another model's review as the primary's.
func (f *Fallback) FellBack() bool { return f.fellBack.Load() }

// BaseURL is the primary's endpoint URL when the primary reports one, so
// an identity built from the URL (the review cache key) keeps telling two
// endpoints apart with or without fallbacks.
func (f *Fallback) BaseURL() string {
	if r, ok := As[interface{ BaseURL() string }](f.providers[0]); ok {
		return r.BaseURL()
	}
	return ""
}

// Name is the primary's name: cost keys, caches and the selection note
// keep naming the provider the operator chose first.
func (f *Fallback) Name() string { return f.providers[0].Name() }

// Tokens counts with the primary, the provider the estimate is made for.
func (f *Fallback) Tokens(input string) (int, error) { return f.providers[0].Tokens(input) }

// ResolveModel implements ModelResolver with the primary's model, so the
// budget ceiling is checked against the key the operator priced.
func (f *Fallback) ResolveModel(opts Options) string {
	if r, ok := As[ModelResolver](f.providers[0]); ok {
		return r.ResolveModel(opts)
	}
	return opts.ModelKey
}

// CallTimeout implements TimeoutScaler: each member is bounded by its own
// client timeout, so the whole chain may take one timeout per member. A
// single outer timeout would expire while the first fallback was still
// answering, and the fallback would never be heard.
func (f *Fallback) CallTimeout(each time.Duration) time.Duration {
	return each * time.Duration(len(f.providers))
}

// Complete tries each member in order with the same prompt.
func (f *Fallback) Complete(prompt string, opts Options) (Response, error) {
	return try(f, func(p Provider) (Response, error) { return p.Complete(prompt, opts) })
}

// CompleteMessages tries each member in order; a member without separate
// messages receives the flattened conversation, as the orchestrator does.
func (f *Fallback) CompleteMessages(messages []Message, opts Options) (Response, error) {
	return try(f, func(p Provider) (Response, error) {
		if mc, ok := As[MessageCompleter](p); ok {
			return mc.CompleteMessages(messages, opts)
		}
		return p.Complete(FlattenMessages(messages), opts)
	})
}

// ToolFallback is a Fallback whose chain has at least one tool caller. A
// tool conversation only ever goes to members that call tools: sending it
// to one that cannot would silently drop the tools the model was offered.
type ToolFallback struct {
	*Fallback
}

// CompleteWithTools tries, in order, only the members that call tools.
func (t *ToolFallback) CompleteWithTools(messages []Message, tools []ToolSpec, opts Options) (ToolResponse, error) {
	callers := &Fallback{head: t.head, notice: t.notice, fellBack: t.fellBack}
	for _, p := range t.providers {
		if _, ok := AsToolCaller(p); ok {
			callers.providers = append(callers.providers, p)
		}
	}
	return try(callers, func(p Provider) (ToolResponse, error) {
		caller, _ := AsToolCaller(p)
		return caller.CompleteWithTools(messages, tools, opts)
	})
}

// try calls each member in order until one answers. Every failure is kept
// and named, so when all of them fail the error says what each one did.
func try[T any](f *Fallback, call func(Provider) (T, error)) (T, error) {
	var failures []string
	var errs []error
	for i, p := range f.providers {
		resp, err := call(p)
		if err == nil {
			if p != f.primary() {
				f.fellBack.Store(true)
			}
			return resp, nil
		}
		failures = append(failures, fmt.Sprintf("%s: %v", p.Name(), err))
		errs = append(errs, err)
		if i+1 < len(f.providers) && f.notice != nil {
			f.notice(p.Name(), err, f.providers[i+1].Name())
		}
	}
	var zero T
	return zero, &fallbackError{
		msg:  fmt.Sprintf("%v (%d tried: %s)", ErrAllProvidersFailed, len(f.providers), strings.Join(failures, "; ")),
		errs: append([]error{ErrAllProvidersFailed}, errs...),
	}
}

// fallbackError names every member's failure once and still answers
// errors.Is for ErrAllProvidersFailed and for each member's own error.
type fallbackError struct {
	msg  string
	errs []error
}

func (e *fallbackError) Error() string   { return e.msg }
func (e *fallbackError) Unwrap() []error { return e.errs }

// TimeoutScaler is implemented by a provider that makes more than one
// bounded call per request (a Fallback chain): given the timeout of one
// call, it returns the bound for the whole request.
type TimeoutScaler interface {
	CallTimeout(each time.Duration) time.Duration
}

// callTimeout is the bound the orchestrator puts on one request to p.
func callTimeout(p Provider) time.Duration {
	if s, ok := As[TimeoutScaler](p); ok {
		return s.CallTimeout(ProviderTimeout())
	}
	return ProviderTimeout()
}
