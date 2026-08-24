// AUR-460 unit selector. DISTINCT ASSERTION: the RAW bytes the litellm
// provider puts on the wire, in isolation from the rest of the review
// pipeline, never contain a "temperature" key -- no matter what value
// llm.Options.Temperature carries when Complete is called. This is the
// card's one required proof (the JSON body sent to the provider must not
// contain the key at all): tests/integration/AUR-460.go instead proves the
// composition through internal/review.Reviewer end-to-end against a
// gateway fixture that 400s on ANY explicit temperature, and
// tests/e2e/AUR-460.sh exercises the real compiled binary the same way.
//
// Measured achado (2026-08-14, real gateway): `--modelo gpt-5.6-luna`
// returned "Unsupported value: 'temperature' does not support 0.3 with
// this model. Only the default (1) value is supported." Sending 0 instead
// of 0.3 is ALSO rejected by that model family, so the only value proven
// to work across the whole gateway fleet is the key's absence -- which is
// why this test checks for the substring "temperature" in the raw
// serialized body, not for a particular numeric value.
package unit

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

// TestAUR460 is the proof: for a spread of Options.Temperature values --
// the pre-AUR-460 fixed default (0.3), the zero value, and a caller who
// explicitly asks for something else -- the JSON body the litellm provider
// sends never carries a "temperature" key.
func TestAUR460(t *testing.T) {
	cases := []struct {
		name string
		opts llm.Options
	}{
		{name: "the old fixed default (0.3)", opts: llm.Options{Temperature: 0.3, MaxTokens: 100}},
		{name: "zero value (also rejected by the measured gateway)", opts: llm.Options{Temperature: 0, MaxTokens: 100}},
		{name: "a caller-chosen non-default value", opts: llm.Options{Temperature: 0.9, MaxTokens: 100}},
		{name: "llm.DefaultOptions()", opts: llm.DefaultOptions()},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var rawBody []byte
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var err error
				rawBody, err = io.ReadAll(r.Body)
				if err != nil {
					t.Fatalf("reading request body: %v", err)
				}

				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"id":      "test",
					"object":  "chat.completion",
					"created": 0,
					"model":   "test-model",
					"choices": []map[string]interface{}{
						{"index": 0, "message": map[string]string{"role": "assistant", "content": "ok"}, "finish_reason": "stop"},
					},
					"usage": map[string]int{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
				})
			}))
			defer server.Close()

			provider := litellm.NewProvider("test-key", server.URL, "gpt-5.6-luna")
			if _, err := provider.Complete("hello", tc.opts); err != nil {
				t.Fatalf("Complete failed: %v", err)
			}

			if rawBody == nil {
				t.Fatal("the provider never reached the test server")
			}
			body := string(rawBody)
			if strings.Contains(body, `"temperature"`) {
				t.Fatalf("the JSON sent to the provider must never contain a \"temperature\" key; got:\n%s", body)
			}
			// Sanity: the rest of the request still made it across, so this
			// is proving omission, not a broken request that sent nothing.
			if !strings.Contains(body, `"model":"gpt-5.6-luna"`) {
				t.Fatalf("request body lost unrelated fields; got:\n%s", body)
			}
		})
	}
}
