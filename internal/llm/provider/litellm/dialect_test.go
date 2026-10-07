package litellm

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/llm"
)

// dialectRequest is what a fake endpoint saw.
type dialectRequest struct {
	path, query, body string
	header            http.Header
}

func dialectServer(t *testing.T, status int, reply string) (*httptest.Server, *dialectRequest) {
	t.Helper()
	got := &dialectRequest{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		got.path, got.query, got.body, got.header = r.URL.Path, r.URL.RawQuery, string(b), r.Header.Clone()
		w.WriteHeader(status)
		_, _ = w.Write([]byte(reply))
	}))
	t.Cleanup(srv.Close)
	return srv, got
}

const okReply = `{"model":"m","choices":[{"message":{"role":"assistant","content":"{\"issues\":[]}"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`

// The request the provider sent before provider profiles existed, captured
// as literal bytes: without a profile nothing on the wire may change.
const (
	legacyCompleteBody = `{"model":"m","messages":[{"role":"system","content":"sys"},{"role":"user","content":"hi"}],"max_tokens":100,"response_format":{"type":"json_object"}}`
	legacyToolsBody    = `{"model":"m","messages":[{"role":"user","content":"hi"}],"max_tokens":100,"response_format":{"type":"json_schema","json_schema":{"name":"answer","schema":{"type":"object"}}}}`
)

func TestDefaultDialectRequestIsUnchanged(t *testing.T) {
	srv, got := dialectServer(t, http.StatusOK, okReply)
	p := NewProvider("k-123", srv.URL, "m")
	if _, err := p.Complete("hi", llm.Options{System: "sys", MaxTokens: 100, JSONMode: true}); err != nil {
		t.Fatal(err)
	}
	if got.body != legacyCompleteBody || got.path != "/chat/completions" || got.query != "" {
		t.Fatalf("Complete request changed:\npath=%s query=%s\nbody=%s", got.path, got.query, got.body)
	}
	if got.header.Get("Authorization") != "Bearer k-123" || got.header.Get("Content-Type") != "application/json" {
		t.Fatalf("headers changed: %v", got.header)
	}
	opts := llm.Options{MaxTokens: 100, JSONMode: true, ResponseSchema: json.RawMessage(`{"type":"object"}`), ResponseSchemaName: "answer"}
	if _, err := p.CompleteWithTools([]llm.Message{{Role: llm.RoleUser, Content: "hi"}}, nil, opts); err != nil {
		t.Fatal(err)
	}
	if got.body != legacyToolsBody {
		t.Fatalf("tool request changed:\n%s", got.body)
	}
	if p.Name() != "litellm" {
		t.Fatalf("name changed: %s", p.Name())
	}
}

func TestDialectSendsOnlyItsTokenFieldAndAuth(t *testing.T) {
	srv, got := dialectServer(t, http.StatusOK, okReply)
	d := Dialect{Name: "azure-openai", Auth: AuthHeaderKey, AuthHeader: "api-key", Path: "/chat/completions",
		Query: map[string]string{"api-version": "2024-10-21"}, TokenField: TokenFieldMaxCompletionTokens, StructuredOutput: StructuredJSONSchema}
	p := NewProviderWithDialect(d, "k-123", srv.URL, "m")
	if _, err := p.Complete("hi", llm.Options{MaxTokens: 100, JSONMode: true}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got.body, `"max_tokens"`) || !strings.Contains(got.body, `"max_completion_tokens":100`) {
		t.Fatalf("token field: %s", got.body)
	}
	if got.header.Get("Authorization") != "" || got.header.Get("api-key") != "k-123" || got.query != "api-version=2024-10-21" {
		t.Fatalf("auth/query: auth=%q api-key=%q query=%q", got.header.Get("Authorization"), got.header.Get("api-key"), got.query)
	}
}

func TestDialectWithoutStructuredOutputSendsNoResponseFormat(t *testing.T) {
	srv, got := dialectServer(t, http.StatusOK, okReply)
	d := DefaultDialect()
	d.Name, d.StructuredOutput, d.Auth = "anthropic", StructuredNone, AuthNone
	p := NewProviderWithDialect(d, "", srv.URL, "m")
	if _, err := p.Complete("hi", llm.Options{JSONMode: true}); err != nil {
		t.Fatal(err)
	}
	opts := llm.Options{JSONMode: true, ResponseSchema: json.RawMessage(`{"type":"object"}`)}
	if _, err := p.CompleteWithTools([]llm.Message{{Role: llm.RoleUser, Content: "hi"}}, nil, opts); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got.body, "response_format") || got.header.Get("Authorization") != "" {
		t.Fatalf("none dialect sent response_format or auth: %s %v", got.body, got.header)
	}
}

// Each known error envelope is summarised to the provider's own message,
// the key it echoed is redacted and the provider is named.
func TestProviderErrorIsSummarisedRedactedAndNamed(t *testing.T) {
	key := "sk-" + "proj-" + "AbCdEf0123456789AbCdEf0123456789"
	envelopes := map[string]string{
		"openai":    `{"error":{"message":"Incorrect API key provided: ` + key + `","type":"invalid_request_error"}}`,
		"anthropic": `{"type":"error","error":{"type":"authentication_error","message":"invalid x-api-key ` + key + `"}}`,
		"google":    `[{"error":{"code":400,"message":"API key not valid ` + key + `","status":"INVALID_ARGUMENT"}}]`,
		"bedrock":   `{"message":"The security token included in the request is invalid: ` + key + `"}`,
	}
	for name, body := range envelopes {
		srv, _ := dialectServer(t, http.StatusUnauthorized, body)
		d := DefaultDialect()
		d.Name = name
		_, err := NewProviderWithDialect(d, key, srv.URL, "m").Complete("hi", llm.Options{})
		if err == nil {
			t.Fatalf("%s: no error", name)
		}
		msg := err.Error()
		if strings.Contains(msg, key) || strings.Contains(msg, "AbCdEf0123456789") {
			t.Fatalf("%s: key leaked: %s", name, msg)
		}
		if !strings.HasPrefix(msg, name+" API error (status 401): ") || strings.Contains(msg, `{"`) {
			t.Fatalf("%s: not a named summary: %s", name, msg)
		}
	}
}
