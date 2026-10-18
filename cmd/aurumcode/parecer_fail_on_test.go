package main

// The parecer agrees with the exit under an explicit --fail-on. The policy
// gate counts the model's findings only when they cite a policy skill, so an
// error that cites a built-in catalog rule passes the gate while --fail-on
// error catches it and exits 3: the parecer must then read blocked, list the
// finding as blocking, and still say the policy gate itself passed.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/scanner"
)

const parecerFailOnResponse = `{"summary":"A mudança tem um número mágico.","issues":[{"file":"app.go","line":4,"severity":"error","rule_id":"quality/magic-numbers","message":"Número mágico sem nome.","impact":"Leitura difícil.","evidence":"A linha adicionada usa o literal direto.","verification":"Dê nome à constante."}]}`

func TestParecerFollowsFailOnOverAPassingGate(t *testing.T) {
	aur579Repo(t)
	writeRepoConfig(t, "gate:\n  fail_on: [error]\n  inconclusive: warn\n  triage:\n    analysis: none\n    sast: none\n  sources: [skills]\n")
	fixture := filepath.Join(t.TempDir(), "fixture.json")
	if err := os.WriteFile(fixture, []byte(parecerFailOnResponse), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AURUMCODE_LLM_FIXTURE", fixture)
	code, out, errOut := aur579Review(t, scanner.Command(nil), "--fail-on", "error")
	if code != exitFindings {
		t.Fatalf("exit=%d, want %d (--fail-on error catches the model's error)\nstdout=%s\nstderr=%s", code, exitFindings, out, errOut)
	}
	for _, passed := range []string{"Gate passou", "Aprovado", "Gate passed", "Approved"} {
		if strings.Contains(out, passed) {
			t.Fatalf("parecer reads %q on a run that exits on --fail-on:\n%s", passed, out)
		}
	}
	if !strings.Contains(out, "policy gate: passed") && !strings.Contains(out, "gate da política: passou") {
		t.Fatalf("the policy gate itself passed and the parecer must say so:\n%s", out)
	}
	if !strings.Contains(out, "app.go:4") {
		t.Fatalf("the caught finding is missing from the parecer:\n%s", out)
	}
}
