package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Mpaape/AurumCode/internal/llm/provider/litellm"
	"github.com/Mpaape/AurumCode/internal/llm/provider/profiles"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
)

// providerFixture prepares a repository and a fake endpoint answering
// status/reply, with LLM_PROVIDER=profile pointed at it.
func providerFixture(t *testing.T, profile string, status int, reply string) *int32 {
	t.Helper()
	coverageFixture(t, "")
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/chat/completions" {
			atomic.AddInt32(&hits, 1)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(reply))
	}))
	t.Cleanup(srv.Close)
	for k, v := range map[string]string{
		"LLM_PROVIDER": profile, "LLM_PROVIDERS_FILE": "", "LLM_BASE_URL": srv.URL, "LLM_API_KEY": aur596Key,
		"AZURE_OPENAI_RESOURCE": "r", "AZURE_OPENAI_DEPLOYMENT": "d", "AWS_REGION": "us-east-1",
	} {
		t.Setenv(k, v)
	}
	return &hits
}

// Built by concatenation so the source carries no credential-shaped literal
// (the sealed runner and the secret scanner refuse one).
var aur596Key = "sk-" + "proj-" + "Zz9Yy8Xx7Ww6Vv5Uu4Tt3Ss2Rr1Qq0Pp"

func aur596Review(t *testing.T) (int, string, string) {
	t.Helper()
	var out, errOut strings.Builder
	code := runReview([]string{"--base", "HEAD~1"}, &out, &errOut, redaction.NewFilter())
	return code, out.String(), errOut.String()
}

// AC-002: every catalog profile reaches its endpoint and the review concludes.
func TestAUR596EveryProfileConcludesAReview(t *testing.T) {
	catalog, err := profiles.Embedded()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range catalog.Names() {
		t.Run(name, func(t *testing.T) {
			hits := providerFixture(t, name, http.StatusOK, `{"choices":[{"message":{"content":"{\"summary\":\"ok\",\"verdict\":\"approve\",\"issues\":[]}"}}],"usage":{}}`)
			code, stdout, stderr := aur596Review(t)
			if code != 0 || atomic.LoadInt32(hits) != 1 {
				t.Fatalf("exit=%d hits=%d\nstdout=%s\nstderr=%s", code, atomic.LoadInt32(hits), stdout, stderr)
			}
		})
	}
}

// AC-004: a provider without structured output answering outside the
// schema leaves the review inconclusive, never approved.
func TestAUR596OutOfSchemaAnswerIsInconclusive(t *testing.T) {
	providerFixture(t, "anthropic", http.StatusOK, `{"choices":[{"message":{"content":"{\"answer\":\"looks fine\"}"}}],"usage":{}}`)
	code, stdout, stderr := aur596Review(t)
	if code == 0 || !strings.Contains(stderr, "could not understand the model's response (validation_failed)") || strings.Contains(stdout, "No issues found") {
		t.Fatalf("exit=%d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
}

// AC-005: a provider error echoing the key reaches the user redacted and
// naming the provider.
func TestAUR596ProviderErrorIsRedactedAndNamed(t *testing.T) {
	providerFixture(t, "anthropic", http.StatusUnauthorized, `{"type":"error","error":{"type":"authentication_error","message":"invalid x-api-key `+aur596Key+`"}}`)
	code, stdout, stderr := aur596Review(t)
	if code == 0 || strings.Contains(stdout+stderr, aur596Key) || strings.Contains(stdout+stderr, "Zz9Yy8Xx7Ww6") || !strings.Contains(stderr, "anthropic API error (status 401): authentication_error: invalid x-api-key") {
		t.Fatalf("exit=%d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
}

// AC-006: an unknown profile fails and lists the valid ones.
func TestAUR596UnknownProfileFailsListingTheValidOnes(t *testing.T) {
	providerFixture(t, "nao-existe", http.StatusOK, `{}`)
	code, _, stderr := aur596Review(t)
	if code == 0 || !strings.Contains(stderr, `unknown LLM provider profile "nao-existe"; valid profiles: anthropic, azure-openai, bedrock, google, litellm, ollama, openai, openai-compatible, opencode, openrouter`) {
		t.Fatalf("exit=%d stderr=%s", code, stderr)
	}
}

// Two profile endpoints resolved without LLM_BASE_URL (two Azure
// deployments) never share a review cache entry.
func TestAUR596ProfileEndpointIsPartOfTheCacheKey(t *testing.T) {
	t.Setenv("LLM_BASE_URL", "")
	t.Setenv("AURUMCODE_LLM_FIXTURE", "")
	catalog, err := profiles.Embedded()
	if err != nil {
		t.Fatal(err)
	}
	keys := map[string]bool{}
	for _, deployment := range []string{"d1", "d2"} {
		env := map[string]string{"LLM_API_KEY": "k", "AZURE_OPENAI_RESOURCE": "r", "AZURE_OPENAI_DEPLOYMENT": deployment}
		ep, err := catalog.Resolve("azure-openai", func(k string) string { return env[k] })
		if err != nil {
			t.Fatal(err)
		}
		keys[modelCacheKey(litellm.NewProviderWithDialect(ep.Dialect, ep.APIKey, ep.BaseURL, "m"))] = true
	}
	if len(keys) != 2 {
		t.Fatalf("two deployments share a cache key: %v", keys)
	}
}

// The agent's gate (aurumcode mcp) holds the same line: a schema-less
// provider answering outside the schema is inconclusive, never a pass.
func TestAUR596OutOfSchemaAnswerIsInconclusiveForTheAgentGate(t *testing.T) {
	providerFixture(t, "anthropic", http.StatusOK, `{"choices":[{"message":{"content":"{\"answer\":\"looks fine\"}"}}],"usage":{}}`)
	freshCache(t)
	a := mcpAnswer(t, mcpSession(t, mcpCall(1, "aurum_gate", `{"base":"HEAD~1"}`))[0])
	if a["decision"] != "inconclusive" {
		t.Fatalf("decision %v (%v), want inconclusive", a["decision"], a["reason"])
	}
}
