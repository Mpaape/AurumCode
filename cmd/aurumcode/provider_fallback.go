// Provider fallback at the command seam: the LLM_FALLBACK_<n>_* slots the
// operator configured are put behind the primary provider, in order, and
// every switch is announced on the redacted stderr.
package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Mpaape/AurumCode/internal/llm"
	"github.com/Mpaape/AurumCode/internal/llm/provider/litellm"
	"github.com/Mpaape/AurumCode/internal/llm/provider/profiles"
)

// fallbackNoticeSink receives the "provider failed, trying the next" line.
// run points it at the redacted stderr writer before any command runs; the
// default discards, so a test that builds a provider writes nowhere.
var fallbackNoticeSink io.Writer = io.Discard

// withFallbacks puts the configured fallback slots behind primary. With no
// slot set it returns primary and mechanism unchanged. A slot that is set
// but unusable fails the selection, as an unusable LLM_PROVIDER does.
func withFallbacks(primary llm.Provider, mechanism string) (llm.Provider, string, error) {
	slots, err := profiles.FallbacksFromEnv(os.Getenv)
	if err != nil {
		return nil, "", err
	}
	if len(slots) == 0 {
		return primary, mechanism, nil
	}
	fallbacks := make([]llm.Provider, 0, len(slots))
	names := make([]string, 0, len(slots))
	for _, s := range slots {
		slot := fmt.Sprintf("%s%d", profiles.FallbackPrefix, s.Slot)
		fallbacks = append(fallbacks, slotProvider{
			Provider: litellm.NewProviderWithDialect(s.Endpoint.Dialect, s.Endpoint.APIKey, s.Endpoint.BaseURL, s.Model),
			name:     slot + " (" + s.Endpoint.Profile + ")",
		})
		names = append(names, fmt.Sprintf("%s %s endpoint %s", slot, s.Endpoint.Profile, redactedEndpoint(s.Endpoint.BaseURL)))
	}
	chain := llm.NewFallback(primary, fallbacks, announceFallback)
	return chain, mechanism + ", fallbacks: " + strings.Join(names, ", "), nil
}

// announceFallback tells the operator the primary (or an earlier
// fallback) failed and which provider answers next. The error text goes
// through the same redacted writer as every other stderr line.
func announceFallback(failed string, err error, next string) {
	fmt.Fprintf(fallbackNoticeSink, "aurumcode: provider %s failed (%v); trying fallback %s\n", failed, err, next)
}

// slotProvider names a fallback by its slot, so the stderr line and the
// final error say which configured fallback failed or answered. It only
// renames: every request is forwarded unchanged, so it unwraps.
type slotProvider struct {
	llm.Provider
	name string
}

func (s slotProvider) Name() string         { return s.name }
func (s slotProvider) Unwrap() llm.Provider { return s.Provider }

// answeredByFallback reports whether a fallback, not the primary, answered
// at least one request of this run through provider.
func answeredByFallback(provider llm.Provider) bool {
	if f, ok := llm.As[interface{ FellBack() bool }](provider); ok {
		return f.FellBack()
	}
	return false
}
