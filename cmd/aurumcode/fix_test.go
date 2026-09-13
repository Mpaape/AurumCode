package main

import (
	"bytes"
	"encoding/json"
	"fmt"
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

// writeWorkingTreeFile writes a file with exactly lineCount lines under
// dir/relPath, where line lineCount's text is exactly matchLine -- enough
// real working-tree context for AC-003's validation (git apply --check, or
// the no-git fallback) to accept a suggestion anchored at that line.
func writeWorkingTreeFile(t *testing.T, dir, relPath string, lineCount int, matchLine string) {
	t.Helper()
	var sb strings.Builder
	for i := 1; i < lineCount; i++ {
		fmt.Fprintf(&sb, "// filler line %d\n", i)
	}
	sb.WriteString(matchLine + "\n")
	full := filepath.Join(dir, relPath)
	if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(sb.String()), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestRunFixBuildsPatchFromSuggestionsArray(t *testing.T) {
	dir := t.TempDir()
	writeWorkingTreeFile(t, dir, "store.go", 42, "store.Save(order)")
	defer chdir(t, dir)()

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
	dir := t.TempDir()
	writeWorkingTreeFile(t, dir, "q.go", 7, "x := 1")
	defer chdir(t, dir)()

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
