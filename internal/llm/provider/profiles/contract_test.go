package profiles

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/llm"
	"github.com/Mpaape/AurumCode/internal/llm/provider/litellm"
)

// dialectContract is what a provider's endpoint expects, written down here
// from its public documentation and independently of profiles.yml: a
// catalog entry that drifts from it fails, and a catalog entry with no
// contract here fails too (no provider is "supported" untested).
type dialectContract struct {
	path       string
	query      string
	authHeader string // header carrying the key; "" = no credential
	authValue  string // its exact value for key "k-123"
	tokenField string
	jsonMode   string // response_format.type sent for a JSON-mode call; "" = none
	schemaMode string // response_format.type sent when a schema is given; "" = none
	defaultURL string // base URL with the test environment below
}

var contracts = map[string]dialectContract{
	"openai-compatible": {"/chat/completions", "", "Authorization", "Bearer k-123", "max_tokens", "json_object", "json_schema", ""},
	"openai":            {"/chat/completions", "", "Authorization", "Bearer k-123", "max_completion_tokens", "json_object", "json_schema", "https://api.openai.com/v1"},
	"azure-openai":      {"/chat/completions", "api-version=2024-10-21", "api-key", "k-123", "max_completion_tokens", "json_object", "json_schema", "https://res1.openai.azure.com/openai/deployments/dep1"},
	"anthropic":         {"/chat/completions", "", "Authorization", "Bearer k-123", "max_tokens", "", "", "https://api.anthropic.com/v1"},
	"google":            {"/chat/completions", "", "Authorization", "Bearer k-123", "max_tokens", "json_object", "json_object", "https://generativelanguage.googleapis.com/v1beta/openai"},
	"bedrock":           {"/chat/completions", "", "Authorization", "Bearer k-123", "max_tokens", "json_object", "json_object", "https://bedrock-runtime.us-east-1.amazonaws.com/openai/v1"},
	"litellm":           {"/chat/completions", "", "Authorization", "Bearer k-123", "max_tokens", "json_object", "json_schema", "http://localhost:4000"},
	"openrouter":        {"/chat/completions", "", "Authorization", "Bearer k-123", "max_tokens", "json_object", "json_object", "https://openrouter.ai/api/v1"},
	"opencode":          {"/chat/completions", "", "Authorization", "Bearer k-123", "max_tokens", "json_object", "json_object", "https://opencode.ai/zen/v1"},
	"ollama":            {"/chat/completions", "", "", "", "max_tokens", "json_object", "json_object", "http://localhost:11434/v1"},
}

func env(vars map[string]string) func(string) string {
	return func(k string) string { return vars[k] }
}

var templateEnv = map[string]string{
	"LLM_API_KEY":             "k-123",
	"AZURE_OPENAI_RESOURCE":   "res1",
	"AZURE_OPENAI_DEPLOYMENT": "dep1",
	"AWS_REGION":              "us-east-1",
}

type seen struct {
	path, query string
	header      http.Header
	body        map[string]json.RawMessage
}

func TestEveryProfileSpeaksItsDocumentedDialect(t *testing.T) {
	catalog, err := Embedded()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range catalog.Names() {
		want, ok := contracts[name]
		if !ok {
			t.Fatalf("profile %q has no contract test", name)
		}
		t.Run(name, func(t *testing.T) { checkDialect(t, catalog, name, want) })
	}
	for name := range contracts {
		if _, ok := catalog[name]; !ok {
			t.Fatalf("contract %q has no profile in the catalog", name)
		}
	}
}

func checkDialect(t *testing.T, catalog Catalog, name string, want dialectContract) {
	if want.defaultURL != "" {
		ep, err := catalog.Resolve(name, env(templateEnv))
		if err != nil || ep.BaseURL != want.defaultURL {
			t.Fatalf("default URL = %q, %v; want %q", ep.BaseURL, err, want.defaultURL)
		}
	}
	var got seen
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		got = seen{path: r.URL.Path, query: r.URL.RawQuery, header: r.Header.Clone()}
		_ = json.Unmarshal(raw, &got.body)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"{\"issues\":[]}"}}],"usage":{}}`))
	}))
	defer srv.Close()
	vars := map[string]string{"LLM_BASE_URL": srv.URL}
	for k, v := range templateEnv {
		vars[k] = v
	}
	ep, err := catalog.Resolve(name, env(vars))
	if err != nil {
		t.Fatal(err)
	}
	p := litellm.NewProviderWithDialect(ep.Dialect, ep.APIKey, ep.BaseURL, "m")
	resp, err := p.Complete("hi", llm.Options{MaxTokens: 64, JSONMode: true})
	if err != nil || resp.Text != `{"issues":[]}` {
		t.Fatalf("review call did not conclude: %q %v", resp.Text, err)
	}
	if got.path != want.path || got.query != want.query {
		t.Fatalf("path/query = %s?%s", got.path, got.query)
	}
	checkAuth(t, got.header, want)
	checkTokens(t, got.body, want.tokenField)
	if f := formatType(got.body); f != want.jsonMode {
		t.Fatalf("JSON-mode response_format = %q, want %q", f, want.jsonMode)
	}
	opts := llm.Options{MaxTokens: 64, JSONMode: true, ResponseSchema: json.RawMessage(`{"type":"object"}`)}
	if _, err := p.CompleteWithTools([]llm.Message{{Role: llm.RoleUser, Content: "hi"}}, nil, opts); err != nil {
		t.Fatal(err)
	}
	checkAuth(t, got.header, want)
	checkTokens(t, got.body, want.tokenField)
	if f := formatType(got.body); f != want.schemaMode {
		t.Fatalf("schema response_format = %q, want %q", f, want.schemaMode)
	}
}

// checkAuth: the key travels only where the dialect puts it. A named
// header never comes with an Authorization header as well.
func checkAuth(t *testing.T, h http.Header, want dialectContract) {
	t.Helper()
	if want.authHeader == "" {
		if h.Get("Authorization") != "" || h.Get("api-key") != "" {
			t.Fatalf("credential sent to a keyless dialect: %v", h)
		}
		return
	}
	if h.Get(want.authHeader) != want.authValue {
		t.Fatalf("%s = %q, want %q", want.authHeader, h.Get(want.authHeader), want.authValue)
	}
	if want.authHeader != "Authorization" && h.Get("Authorization") != "" {
		t.Fatalf("Authorization sent together with %s", want.authHeader)
	}
}

// checkTokens: exactly the dialect's cap field, never the other one.
func checkTokens(t *testing.T, body map[string]json.RawMessage, field string) {
	t.Helper()
	other := "max_tokens"
	if field == other {
		other = "max_completion_tokens"
	}
	if string(body[field]) != "64" {
		t.Fatalf("%s = %s, want 64", field, body[field])
	}
	if _, sent := body[other]; sent {
		t.Fatalf("%s sent besides %s", other, field)
	}
}

func formatType(body map[string]json.RawMessage) string {
	var rf struct {
		Type string `json:"type"`
	}
	if raw, ok := body["response_format"]; ok {
		_ = json.Unmarshal(raw, &rf)
	}
	return rf.Type
}

func TestUnknownProfileListsTheValidOnes(t *testing.T) {
	_, selected, err := FromEnv(env(map[string]string{"LLM_PROVIDER": "nope"}))
	if !selected || err == nil {
		t.Fatalf("selected=%v err=%v", selected, err)
	}
	for _, name := range []string{"openai", "azure-openai", "anthropic", "google", "bedrock", "litellm", "openrouter", "opencode", "ollama", "openai-compatible"} {
		if !strings.Contains(err.Error(), name) {
			t.Fatalf("error does not list %q: %v", name, err)
		}
	}
}

func TestUnsetProviderKeepsTheLegacySelection(t *testing.T) {
	if _, selected, err := FromEnv(env(nil)); selected || err != nil {
		t.Fatalf("selected=%v err=%v", selected, err)
	}
}

func TestMissingKeyOrPlaceholderFailsClosed(t *testing.T) {
	catalog, _ := Embedded()
	if _, err := catalog.Resolve("openai", env(nil)); err == nil || !strings.Contains(err.Error(), "LLM_API_KEY") {
		t.Fatalf("missing key: %v", err)
	}
	if _, err := catalog.Resolve("bedrock", env(map[string]string{"LLM_API_KEY": "k"})); err == nil || !strings.Contains(err.Error(), "AWS_REGION") {
		t.Fatalf("missing region: %v", err)
	}
	if _, err := catalog.Resolve("openai-compatible", env(map[string]string{"LLM_API_KEY": "k"})); err == nil || !strings.Contains(err.Error(), "LLM_BASE_URL") {
		t.Fatalf("missing base URL: %v", err)
	}
	if ep, err := catalog.Resolve("ollama", env(nil)); err != nil || ep.APIKey != "" {
		t.Fatalf("keyless profile: %v", err)
	}
}

func TestOperatorFileAddsAndReplacesProfiles(t *testing.T) {
	catalog, _ := Embedded()
	merged, err := catalog.Merge([]byte("profiles:\n  minha-gateway:\n    base_url: http://localhost:9000/v1\n    key_env: [GW_KEY]\n    auth: header\n    auth_header: x-gw-key\n    path: /chat/completions\n    token_field: max_tokens\n    structured_output: none\n"), "ops.yml")
	if err != nil {
		t.Fatal(err)
	}
	ep, err := merged.Resolve("minha-gateway", env(map[string]string{"GW_KEY": "g"}))
	if err != nil || ep.Dialect.AuthHeader != "x-gw-key" || ep.BaseURL != "http://localhost:9000/v1" {
		t.Fatalf("%+v %v", ep, err)
	}
	if _, err := catalog.Merge([]byte("profiles:\n  x:\n    auth: magic\n    path: /c\n    token_field: max_tokens\n    structured_output: none\n"), "bad.yml"); err == nil {
		t.Fatal("invalid auth accepted")
	}
}
