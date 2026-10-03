package litellm

import (
	"encoding/json"
	"testing"

	"github.com/Mpaape/AurumCode/internal/llm"
)

// A tool round with a response schema sends response_format json_schema;
// without one it keeps the bare JSON object mode.
func TestAUR580ToolRoundSendsTheJSONSchema(t *testing.T) {
	reply := `{"model":"m","choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"{}"}}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`
	schema := llm.SchemaOf(struct {
		Summary string `json:"summary"`
	}{})
	for _, tc := range []struct {
		opts llm.Options
		want string
	}{
		{llm.Options{JSONMode: true, ResponseSchema: schema, ResponseSchemaName: "review"}, "json_schema"},
		{llm.Options{JSONMode: true}, "json_object"},
	} {
		var body []byte
		srv := captureServer(t, reply, &body, nil)
		if _, err := NewProvider("k", srv.URL, "m").CompleteWithTools([]llm.Message{{Role: llm.RoleUser, Content: "x"}}, nil, tc.opts); err != nil {
			t.Fatal(err)
		}
		srv.Close()
		var req struct {
			ResponseFormat struct {
				Type       string `json:"type"`
				JSONSchema *struct {
					Name   string          `json:"name"`
					Schema json.RawMessage `json:"schema"`
				} `json:"json_schema"`
			} `json:"response_format"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatal(err)
		}
		if req.ResponseFormat.Type != tc.want {
			t.Fatalf("response_format.type = %q, want %q", req.ResponseFormat.Type, tc.want)
		}
		if tc.want == "json_schema" && (req.ResponseFormat.JSONSchema == nil || req.ResponseFormat.JSONSchema.Name != "review" || string(req.ResponseFormat.JSONSchema.Schema) != string(schema)) {
			t.Fatalf("json_schema = %+v", req.ResponseFormat.JSONSchema)
		}
	}
}
