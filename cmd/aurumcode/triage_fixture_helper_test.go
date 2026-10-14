package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/prompt"
)

// The model decides every deterministic finding (owner, 2026-10-14). Tests
// that assert a deterministic finding blocks run with an offline model that
// confirms it: confirmTriage makes the fixture at path answer the dedicated
// triage by confirming every id it could be asked about. Ids the triage did
// not offer are refused by the engine, so the list may be generous.
func confirmTriage(t *testing.T, path string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var parts []string
	for i := 1; i <= 60; i++ {
		for _, prefix := range []string{"E", "T"} {
			parts = append(parts, fmt.Sprintf(`{"evidence_id":"%s%d","status":"confirmed","justification":"achado real no codigo do teste"}`, prefix, i))
		}
	}
	confirm := json.RawMessage(`{"prompt_contains":"` + prompt.TriageMarker + `","response":{"evidence_assessments":[` + strings.Join(parts, ",") + `]}}`)
	if !json.Valid(data) {
		return // a fixture that is not JSON answers verbatim, as before
	}
	var top map[string]json.RawMessage
	env := map[string]json.RawMessage{"default": data}
	if json.Unmarshal(data, &top) == nil && top["aurumcode_fixture"] != nil {
		env = map[string]json.RawMessage{}
		if err := json.Unmarshal(top["aurumcode_fixture"], &env); err != nil {
			t.Fatal(err)
		}
	}
	var cases []json.RawMessage
	if env["cases"] != nil {
		if err := json.Unmarshal(env["cases"], &cases); err != nil {
			t.Fatal(err)
		}
	}
	casesJSON, err := json.Marshal(append([]json.RawMessage{confirm}, cases...))
	if err != nil {
		t.Fatal(err)
	}
	env["cases"] = casesJSON
	wrapped, err := json.Marshal(map[string]any{"aurumcode_fixture": env})
	if err != nil {
		t.Fatal(err)
	}
	body := string(wrapped)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}
