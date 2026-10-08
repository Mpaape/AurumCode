package profiles

import (
	"fmt"
	"strings"
)

// MaxFallbacks is how many fallback slots the environment is read for:
// LLM_FALLBACK_1_* through LLM_FALLBACK_5_*.
const MaxFallbacks = 5

// FallbackPrefix starts every variable of a fallback slot.
const FallbackPrefix = "LLM_FALLBACK_"

// Slot variable suffixes. A slot is empty when all four are empty.
const (
	slotProvider = "PROVIDER"
	slotBaseURL  = "BASE_URL"
	slotAPIKey   = "API_KEY"
	slotModel    = "MODEL"
)

// Fallback is one resolved fallback slot: the endpoint and the model it
// serves ("" lets the endpoint choose).
type Fallback struct {
	Slot     int
	Endpoint Endpoint
	Model    string
}

// FallbacksFromEnv resolves the fallback slots in order. Each slot is its
// own provider: LLM_FALLBACK_<n>_PROVIDER names a catalog profile
// (openai-compatible when empty), LLM_FALLBACK_<n>_BASE_URL and
// LLM_FALLBACK_<n>_API_KEY take the places of LLM_BASE_URL and LLM_API_KEY
// for that slot only, and LLM_FALLBACK_<n>_MODEL is its model -- the
// primary's LLM_MODEL is never inherited, since another provider rarely
// serves the same model id. Like the primary, these are operator settings,
// never read from the repository under review. A slot that is set but
// unusable is an error naming the slot: a fallback that cannot work would
// otherwise be discovered only during the outage it was meant to cover.
func FallbacksFromEnv(getenv func(string) string) ([]Fallback, error) {
	var slots []int
	for n := 1; n <= MaxFallbacks; n++ {
		if slotSet(n, getenv) {
			slots = append(slots, n)
		}
	}
	if len(slots) == 0 {
		return nil, nil
	}
	catalog, err := catalogFromEnv(getenv)
	if err != nil {
		return nil, err
	}
	fallbacks := make([]Fallback, 0, len(slots))
	for _, n := range slots {
		name := strings.TrimSpace(getenv(slotVar(n, slotProvider)))
		if name == "" {
			name = "openai-compatible"
		}
		profile, err := catalog.Lookup(name)
		if err != nil {
			return nil, fmt.Errorf("%s%d: %w", FallbackPrefix, n, err)
		}
		endpoint, err := catalog.Resolve(name, slotEnv(n, profile.KeyEnv, getenv))
		if err != nil {
			// The catalog names the primary's variables; for a slot the
			// operator must set the slot's own.
			return nil, fmt.Errorf("%s%d: %s", FallbackPrefix, n, slotMessage(n, err))
		}
		fallbacks = append(fallbacks, Fallback{Slot: n, Endpoint: endpoint, Model: strings.TrimSpace(getenv(slotVar(n, slotModel)))})
	}
	return fallbacks, nil
}

func slotVar(n int, suffix string) string {
	return fmt.Sprintf("%s%d_%s", FallbackPrefix, n, suffix)
}

func slotSet(n int, getenv func(string) string) bool {
	for _, suffix := range []string{slotProvider, slotBaseURL, slotAPIKey, slotModel} {
		if strings.TrimSpace(getenv(slotVar(n, suffix))) != "" {
			return true
		}
	}
	return false
}

// slotEnv reads the slot's URL and key in place of the primary's. Every
// key variable of the profile (LLM_API_KEY and the provider's native ones,
// such as OPENAI_API_KEY) reads the slot's own LLM_FALLBACK_<n>_API_KEY: a
// native variable in the environment belongs to the primary and must never
// be sent to a fallback's endpoint. Any other variable (a URL placeholder
// such as AWS_REGION) is read as is.
func slotEnv(n int, keyEnv []string, getenv func(string) string) func(string) string {
	return func(name string) string {
		if name == EnvBaseURL {
			return getenv(slotVar(n, slotBaseURL))
		}
		for _, k := range keyEnv {
			if name == k {
				return getenv(slotVar(n, slotAPIKey))
			}
		}
		return getenv(name)
	}
}

// slotMessage rewrites a resolution error so it names the slot's own
// variables: the catalog names the primary's.
func slotMessage(n int, err error) string {
	msg := err.Error()
	if i := strings.Index(msg, "set one of "); i >= 0 {
		msg = msg[:i] + "set " + slotVar(n, slotAPIKey)
	}
	return strings.ReplaceAll(msg, EnvBaseURL, slotVar(n, slotBaseURL))
}
