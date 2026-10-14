package redaction

import (
	"encoding/json"
	"strings"
	"testing"
)

// The incident of the 2.1.0 batch: a model justification quoting Go source
// with a secret-shaped assignment inside escaped quotes. Redacting the
// serialized document whole swallowed the `\` of `\"` and broke the SARIF
// and the audit record.
func TestRedactJSONKeepsTheDocumentValid(t *testing.T) {
	f := NewFilter()
	doc := map[string]any{
		"justification": "O teste define `aur622Canary = \"pass" + "word=canario-aur622-7c1d\"`, tratado como segredo.",
		"pass" + "word": "um-valor-qualquer",
		"nested":        []any{map[string]any{"text": "tok" + "en=abc123def456"}},
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if json.Valid([]byte(f.Redact(string(data)))) {
		t.Fatal("the fixture must reproduce the broken whole-document redaction")
	}
	out := f.RedactJSON(data)
	if !json.Valid(out) {
		t.Fatalf("RedactJSON returned invalid JSON:\n%s", out)
	}
	for _, leaked := range []string{"canario-aur622-7c1d", "um-valor-qualquer", "abc123def456"} {
		if strings.Contains(string(out), leaked) {
			t.Fatalf("%q leaked:\n%s", leaked, out)
		}
	}
}

// A document the whole-document pass keeps valid is returned exactly as
// Redact returns it.
func TestRedactJSONMatchesRedactWhenValid(t *testing.T) {
	f := NewFilter()
	data := []byte(`{"msg": "no secret here", "url": "https://example.com/a?token=x"}`)
	if got, want := string(f.RedactJSON(data)), f.Redact(string(data)); got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}
