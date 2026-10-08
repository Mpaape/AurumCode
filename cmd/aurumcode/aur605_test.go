package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/llm"
)

// fakeEndpoint answers Chat Completions with text, or fails with status.
func fakeEndpoint(t *testing.T, status int, text string, seenKey *string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if seenKey != nil {
			*seenKey = strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		}
		if status != http.StatusOK {
			http.Error(w, `{"error":{"message":"fora do ar"}}`, status)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model":   "modelo-falso",
			"choices": []map[string]any{{"index": 0, "message": map[string]any{"role": "assistant", "content": text}, "finish_reason": "stop"}},
			"usage":   map[string]int{"prompt_tokens": 1, "completion_tokens": 1},
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

// AC-001 at the command seam: the operator's LLM_FALLBACK_1_* slot answers
// when the primary endpoint is down, with its own key, and the switch is
// written to stderr.
func TestAUR605ProviderFromEnvFallsBackToTheSlot(t *testing.T) {
	var reservaKey string
	primary := fakeEndpoint(t, http.StatusServiceUnavailable, "", nil)
	reserva := fakeEndpoint(t, http.StatusOK, "resposta da reserva", &reservaKey)
	t.Setenv("AURUMCODE_LLM_FIXTURE", "")
	t.Setenv("LLM_PROVIDER", "")
	t.Setenv("LLM_API_KEY", "chave-primaria")
	t.Setenv("LLM_BASE_URL", primary.URL)
	t.Setenv("LLM_FALLBACK_1_BASE_URL", reserva.URL)
	t.Setenv("LLM_FALLBACK_1_API_KEY", "chave-reserva")
	var sink bytes.Buffer
	old := fallbackNoticeSink
	fallbackNoticeSink = &sink
	t.Cleanup(func() { fallbackNoticeSink = old })

	p, mechanism, err := providerFromEnv("fixture", "modelo")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(mechanism, "LLM_FALLBACK_1 openai-compatible endpoint") {
		t.Fatalf("selection note does not name the fallback: %q", mechanism)
	}
	resp, err := p.Complete("revise", llm.DefaultOptions())
	if err != nil || resp.Text != "resposta da reserva" {
		t.Fatalf("resp=%q err=%v", resp.Text, err)
	}
	if reservaKey != "chave-reserva" {
		t.Fatalf("fallback received key %q, want its own", reservaKey)
	}
	if !strings.Contains(sink.String(), "failed") || !strings.Contains(sink.String(), "trying fallback") {
		t.Fatalf("switch not announced: %q", sink.String())
	}
}

// AC-007 at the command seam: a slot set but unusable fails the selection.
func TestAUR605UnusableSlotFailsTheSelection(t *testing.T) {
	t.Setenv("AURUMCODE_LLM_FIXTURE", "")
	t.Setenv("LLM_PROVIDER", "")
	t.Setenv("LLM_API_KEY", "k")
	t.Setenv("LLM_BASE_URL", "http://127.0.0.1:1")
	t.Setenv("LLM_FALLBACK_1_BASE_URL", "http://127.0.0.1:2")
	if _, _, err := providerFromEnv("fixture", ""); err == nil || !strings.Contains(err.Error(), "LLM_FALLBACK_1") {
		t.Fatalf("err = %v, want the slot named", err)
	}
}

// AC-009 at the command seam: with a fallback configured, the cache key
// still carries the primary profile's resolved URL (AUR-513).
func TestAUR605CacheKeyKeepsThePrimaryURL(t *testing.T) {
	t.Setenv("AURUMCODE_LLM_FIXTURE", "")
	t.Setenv("LLM_BASE_URL", "")
	t.Setenv("LLM_PROVIDER", "azure-openai")
	t.Setenv("AZURE_OPENAI_RESOURCE", "recurso-a")
	t.Setenv("AZURE_OPENAI_DEPLOYMENT", "dep")
	t.Setenv("LLM_API_KEY", "k")
	t.Setenv("LLM_FALLBACK_1_BASE_URL", "http://127.0.0.1:2")
	t.Setenv("LLM_FALLBACK_1_API_KEY", "k2")
	p, _, err := providerFromEnv("fixture", "m")
	if err != nil {
		t.Fatal(err)
	}
	if key := modelCacheKey(p); !strings.Contains(key, ":baseurl:https://recurso-a.openai.azure.com/") {
		t.Fatalf("cache key lost the primary URL: %q", key)
	}
}
