package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/pkg/types"
)

func writeFixInput(t *testing.T, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "suggestions.json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRunFixBuildsPatchFromSuggestionsArray(t *testing.T) {
	suggestions := []types.ReviewSuggestion{
		{File: "store.go", Line: 42, CurrentCode: "store.Save(order)", ProposedCode: "if err := store.Save(order); err != nil {\n\treturn err\n}"},
	}
	data, err := json.Marshal(suggestions)
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := runFix([]string{"--file", writeFixInput(t, data)}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr.String())
	}
	patch := stdout.String()
	for _, want := range []string{"--- a/store.go", "+++ b/store.go", "-store.Save(order)", "+if err := store.Save(order); err != nil {"} {
		if !strings.Contains(patch, want) {
			t.Fatalf("patch missing %q:\n%s", want, patch)
		}
	}
}

func TestRunFixAcceptsReviewResponseShape(t *testing.T) {
	response := `{"verdict":"approve","suggestions":[{"file":"q.go","line":7,"current_code":"x := 1","proposed_code":"x := 2"}]}`
	var stdout, stderr bytes.Buffer
	if code := runFix([]string{"--file", writeFixInput(t, []byte(response))}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr.String())
	}
	if patch := stdout.String(); !strings.Contains(patch, "-x := 1") || !strings.Contains(patch, "+x := 2") {
		t.Fatalf("unexpected patch: %q", patch)
	}
}

func TestRunFixSkipsUnsafeSuggestions(t *testing.T) {
	suggestions := []types.ReviewSuggestion{
		{File: "../escape.go", Line: 1, CurrentCode: "a", ProposedCode: "b"},
		{File: "ok.go", Line: 1, CurrentCode: "", ProposedCode: "b"},
		{File: "same.go", Line: 1, CurrentCode: "x", ProposedCode: "x"},
	}
	data, err := json.Marshal(suggestions)
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := runFix([]string{"--file", writeFixInput(t, data)}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr.String())
	}
	if strings.TrimSpace(stdout.String()) != "" {
		t.Fatalf("expected empty patch for all-unsafe suggestions, got %q", stdout.String())
	}
}

func TestRunFixRejectsInvalidJSON(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := runFix([]string{"--file", writeFixInput(t, []byte(`{"suggestions": `))}, &stdout, &stderr); code == 0 {
		t.Fatal("expected non-zero exit for invalid JSON")
	}
	if !strings.Contains(stderr.String(), "parsing suggestions") {
		t.Fatalf("unexpected stderr: %q", stderr.String())
	}
}
