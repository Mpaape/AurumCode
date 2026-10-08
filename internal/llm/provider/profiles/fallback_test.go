package profiles

import (
	"strings"
	"testing"
)

func envOf(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

// AC-006: each slot resolves through the catalog with its own URL, key and
// model; the primary's key and model never leak into a fallback.
func TestAUR605SlotsResolveInOrderWithTheirOwnKey(t *testing.T) {
	env := envOf(map[string]string{
		"LLM_API_KEY":             "chave-do-primario",
		"LLM_BASE_URL":            "http://primario",
		"LLM_MODEL":               "modelo-do-primario",
		"LLM_FALLBACK_1_BASE_URL": "http://reserva-um/",
		"LLM_FALLBACK_1_API_KEY":  "chave-um",
		"LLM_FALLBACK_1_MODEL":    "modelo-um",
		"LLM_FALLBACK_3_PROVIDER": "anthropic",
		"LLM_FALLBACK_3_API_KEY":  "chave-tres",
		"LLM_FALLBACK_2_PROVIDER": "",
		"ANTHROPIC_API_KEY":       "nao-usada",
		"LLM_FALLBACK_9_PROVIDER": "openai",
	})
	got, err := FallbacksFromEnv(env)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("slots = %+v, want 1 and 3", got)
	}
	um, tres := got[0], got[1]
	if um.Slot != 1 || um.Endpoint.Profile != "openai-compatible" || um.Endpoint.BaseURL != "http://reserva-um" || um.Endpoint.APIKey != "chave-um" || um.Model != "modelo-um" {
		t.Fatalf("slot 1 = %+v", um)
	}
	if tres.Slot != 3 || tres.Endpoint.Profile != "anthropic" || tres.Endpoint.BaseURL != "https://api.anthropic.com/v1" || tres.Endpoint.APIKey != "chave-tres" || tres.Model != "" {
		t.Fatalf("slot 3 = %+v", tres)
	}
}

// AC-007: a slot that is set but cannot work fails naming the slot, and the
// primary's key is never the fallback's key.
func TestAUR605UnusableSlotFailsNamingIt(t *testing.T) {
	cases := map[string]map[string]string{
		"sem chave":           {"LLM_API_KEY": "chave-do-primario", "LLM_FALLBACK_2_BASE_URL": "http://reserva"},
		"sem url":             {"LLM_FALLBACK_2_API_KEY": "k"},
		"perfil desconhecido": {"LLM_FALLBACK_2_PROVIDER": "nao-existe", "LLM_FALLBACK_2_API_KEY": "k"},
	}
	for name, vars := range cases {
		_, err := FallbacksFromEnv(envOf(vars))
		if err == nil || !strings.Contains(err.Error(), "LLM_FALLBACK_2") {
			t.Fatalf("%s: err = %v, want an error naming LLM_FALLBACK_2", name, err)
		}
	}
	_, err := FallbacksFromEnv(envOf(cases["sem chave"]))
	if !strings.Contains(err.Error(), "LLM_FALLBACK_2_API_KEY") || strings.Contains(err.Error(), " LLM_API_KEY") {
		t.Fatalf("missing key error names the primary's variable: %v", err)
	}
}

// AC-003: no slot set, no fallback and no error.
func TestAUR605NoSlotNoFallback(t *testing.T) {
	got, err := FallbacksFromEnv(envOf(map[string]string{"LLM_API_KEY": "k", "LLM_BASE_URL": "http://x"}))
	if err != nil || got != nil {
		t.Fatalf("got %+v, %v", got, err)
	}
}

// AC-007: a native key variable of the profile (OPENAI_API_KEY) belongs to
// the primary and is never read for a slot.
func TestAUR605NativeKeyNeverReachesASlot(t *testing.T) {
	env := envOf(map[string]string{
		"LLM_PROVIDER":            "openai",
		"OPENAI_API_KEY":          "chave-nativa-do-primario",
		"LLM_FALLBACK_1_PROVIDER": "openai",
		"LLM_FALLBACK_1_BASE_URL": "https://outro.example/v1",
	})
	if got, err := FallbacksFromEnv(env); err == nil || !strings.Contains(err.Error(), "LLM_FALLBACK_1_API_KEY") {
		t.Fatalf("slot without its own key resolved: %+v, %v", got, err)
	}
	env = envOf(map[string]string{"OPENAI_API_KEY": "chave-nativa-do-primario", "LLM_FALLBACK_1_PROVIDER": "openai", "LLM_FALLBACK_1_API_KEY": "chave-da-reserva"})
	got, err := FallbacksFromEnv(env)
	if err != nil || got[0].Endpoint.APIKey != "chave-da-reserva" {
		t.Fatalf("slot key = %+v, %v", got, err)
	}
}
