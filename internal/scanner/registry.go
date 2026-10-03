package scanner

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
)

var (
	registryMu sync.RWMutex
	registry   = map[string]Engine{}
)

// Register adds an engine compiled into the binary. It panics on an empty
// or duplicate name: both are programming errors of the engine list.
func Register(e Engine) {
	name := Normalize(e.Name())
	if name == "" {
		panic("scanner: engine without a name")
	}
	registryMu.Lock()
	defer registryMu.Unlock()
	if _, dup := registry[name]; dup {
		panic(fmt.Sprintf("scanner: engine %q registered twice", name))
	}
	registry[name] = e
}

// Unregister removes an engine; tests that register a fake engine use it to
// leave the registry as they found it.
func Unregister(name string) {
	registryMu.Lock()
	defer registryMu.Unlock()
	delete(registry, Normalize(name))
}

// Lookup returns the registered engine named name.
func Lookup(name string) (Engine, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	e, ok := registry[Normalize(name)]
	return e, ok
}

// Names lists the registered engines, sorted.
func Names() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	out := make([]string, 0, len(registry))
	for name := range registry {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// NamesIn lists the registered engines of category, sorted.
func NamesIn(category string) []string {
	var out []string
	for _, name := range Names() {
		if e, _ := Lookup(name); Normalize(e.Category) == Normalize(category) {
			out = append(out, name)
		}
	}
	return out
}

// Categories lists the categories of the registered engines, sorted and
// without repetition.
func Categories() []string {
	seen := map[string]bool{}
	var out []string
	for _, name := range Names() {
		e, _ := Lookup(name)
		if c := Normalize(e.Category); c != "" && !seen[c] {
			seen[c] = true
			out = append(out, c)
		}
	}
	sort.Strings(out)
	return out
}

// Answers reports whether a gate.sources/gate.triage entry names this
// engine, by its name, its category or its typed origin.
func (e Engine) Answers(source string) bool {
	s := Normalize(source)
	return s != "" && (s == Normalize(e.Name()) || s == Normalize(e.Category) || s == Normalize(e.TypedOrigin()))
}

// KnownSource reports whether source names a registered engine or category.
func KnownSource(source string) bool {
	for _, name := range Names() {
		if e, _ := Lookup(name); e.Answers(source) {
			return true
		}
	}
	return false
}

// ErrNotRegistered is the error of an engine the binary does not hold.
var ErrNotRegistered = errors.New("scanner: engine not registered")

// Unregistered stands for an engine name the registry does not hold: it
// never runs and always fails, so a caller reads it as inconclusive.
func Unregistered(name string) Engine { return Engine{Scanner: unregistered(Normalize(name))} }

type unregistered string

func (u unregistered) Name() string { return string(u) }
func (u unregistered) Run(context.Context, Request) (Report, error) {
	return Report{}, ErrNotRegistered
}
