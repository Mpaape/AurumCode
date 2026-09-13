package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/pkg/types"
)

// TestAUR489FixApplies is AUR-489's AC-003 on the success side: the patch
// runFix prints is applied FOR REAL with git apply on a temporary repository
// and the file content is compared afterwards -- not a strings.Contains over
// the patch text, which is what let a wrong hunk pass before this card.
func TestAUR489FixApplies(t *testing.T) {
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not on PATH; the no-git fallback is covered by TestAUR489FixRejectsStale")
	}
	dir := t.TempDir()
	writeWorkingTreeFile(t, dir, "store.go", 5, "store.Save(order)")
	for _, args := range [][]string{{"init", "-q"}, {"add", "."}, {"-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "-m", "base"}} {
		cmd := exec.Command(gitPath, args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	defer chdir(t, dir)()

	data, err := json.Marshal([]types.ReviewSuggestion{{File: "store.go", Line: 5, CurrentCode: "store.Save(order)", ProposedCode: "if err := store.Save(order); err != nil {\n\treturn err\n}"}})
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := runFix([]string{"--file", writeFixInput(t, data)}, &stdout, &stderr); code != 0 {
		t.Fatalf("AUR-489/AC-003: exit=%d stderr=%s", code, stderr.String())
	}
	apply := exec.Command(gitPath, "apply", "--unidiff-zero")
	apply.Dir = dir
	apply.Stdin = strings.NewReader(stdout.String())
	if out, err := apply.CombinedOutput(); err != nil {
		t.Fatalf("AUR-489/AC-003: the printed patch does not apply: %v\n%s\n%s", err, out, stdout.String())
	}
	got, err := os.ReadFile(filepath.Join(dir, "store.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "if err := store.Save(order); err != nil {") || strings.Contains(string(got), "\nstore.Save(order)\n") {
		t.Fatalf("AUR-489/AC-003: file after apply is wrong:\n%s", got)
	}
}

// TestAUR489FixRejectsStale is AC-003 on the failure side: a suggestion whose
// current_code exists nowhere in the named file must exit 1 and name the file
// and line, instead of printing a patch nobody can apply.
func TestAUR489FixRejectsStale(t *testing.T) {
	dir := t.TempDir()
	writeWorkingTreeFile(t, dir, "app.go", 4, "}")
	defer chdir(t, dir)()

	data, err := json.Marshal([]types.ReviewSuggestion{{File: "app.go", Line: 3, CurrentCode: "return err\nfoo", ProposedCode: "return nil"}})
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := runFix([]string{"--file", writeFixInput(t, data)}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("AUR-489/AC-003: stale suggestion produced exit=%d with patch:\n%s", code, stdout.String())
	}
	if !strings.Contains(stderr.String(), "app.go") {
		t.Fatalf("AUR-489/AC-003: stderr does not name the file: %q", stderr.String())
	}
	if strings.TrimSpace(stdout.String()) != "" {
		t.Fatalf("AUR-489/AC-003: a rejected fix still printed a patch:\n%s", stdout.String())
	}
}
