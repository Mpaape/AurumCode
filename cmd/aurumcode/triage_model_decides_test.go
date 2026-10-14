package main

// The model always decides the deterministic evidence (owner's rule,
// 2026-10-14): a finding the review left unassessed goes to a dedicated
// triage call; disputed with a justification it stops counting, confirmed
// it blocks; when the model gives no usable answer the finding waits for
// it -- an inconclusive run, never "N achados reprovam o gate". Driven
// through the real `aurumcode review --base` over AUR-579's fixture
// repository (analysis E1 on app.go:4).

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/prompt"
	"github.com/Mpaape/AurumCode/internal/review/session"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// triageFixture answers the review without assessing anything and the
// dedicated triage with triageReply.
func triageFixture(t *testing.T, triageReply string) string {
	t.Helper()
	body := `{"aurumcode_fixture":{"cases":[{"prompt_contains":"` + prompt.TriageMarker + `","response":` + triageReply + `}],` +
		`"default":{"verdict":"approve","strengths":[],"issues":[],"suggestions":[],"ci_analysis":[],"test_plan":[],"limitations":[],"summary":"revisou sem avaliar a evidencia"}}}`
	path := filepath.Join(t.TempDir(), "fixture.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

const triageGate = "review:\n  language: pt-BR\ngate:\n  fail_on: [high]\n"

// (b) The review is silent on E1; the dedicated triage decides it.
func TestUnassessedEvidenceGoesToTheDedicatedTriage(t *testing.T) {
	for _, tc := range []struct {
		name, reply string
		want        int
		demoted     bool
	}{
		{"disputed", `{"evidence_assessments":[{"evidence_id":"E1","status":"disputed","justification":"valor sintetico de fixture de teste"}]}`, 0, true},
		{"confirmed", `{"evidence_assessments":[{"evidence_id":"E1","status":"confirmed","justification":"credencial real"}]}`, exitFindings, false},
	} {
		aur579Repo(t)
		aur608Config(t, triageGate)
		capture := filepath.Join(t.TempDir(), "prompt.txt")
		t.Setenv("AURUMCODE_PROMPT_CAPTURE", capture)
		t.Setenv("AURUMCODE_LLM_FIXTURE", triageFixture(t, tc.reply))
		code, out, errOut := aur579Review(t, nil)
		if code != tc.want {
			t.Fatalf("%s: exit=%d, want %d\nstderr=%s\nout=%s", tc.name, code, tc.want, errOut, out)
		}
		demoted := strings.Contains(errOut, aur608AnalysisDemoted)
		if demoted != tc.demoted {
			t.Errorf("%s: demotion line present=%v, want %v:\n%s", tc.name, demoted, tc.demoted, errOut)
		}
		sent, err := os.ReadFile(capture + ".triagem")
		if err != nil || !strings.Contains(string(sent), "regra: analysis/hardcoded-secret") || !strings.Contains(string(sent), "local: app.go:4") {
			t.Errorf("%s: the triage prompt must name the rule and place (err %v):\n%s", tc.name, err, sent)
		}
		if review, err := os.ReadFile(capture); err != nil || strings.Contains(string(review), prompt.TriageMarker) {
			t.Errorf("%s: the review's own capture must stay the review prompt (err %v)", tc.name, err)
		}
	}
}

// (c) A dedicated triage that gives no usable answer leaves E1 awaiting
// the model: inconclusive, failing, and never a finding that fails the gate.
func TestModelWithoutAnswerLeavesEvidenceAwaiting(t *testing.T) {
	for _, tc := range []struct {
		name, fixture string
		reason        string
	}{
		{"triage not answered", "", "[model_parse_failure]"},
		{"triage silent twice", `{"evidence_assessments":[]}`, "[triage_unanswered]"},
	} {
		aur579Repo(t)
		aur608Config(t, triageGate)
		fixture := triageFixture(t, `"nao e json"`)
		if tc.fixture != "" {
			fixture = triageFixture(t, tc.fixture)
		}
		t.Setenv("AURUMCODE_LLM_FIXTURE", fixture)
		code, out, errOut := aur579Review(t, nil)
		if code == 0 || code == exitFindings {
			t.Fatalf("%s: exit=%d, want an inconclusive failure (never a pass, never findings)\nstderr=%s", tc.name, code, errOut)
		}
		all := errOut + out
		if !strings.Contains(all, "achados determinísticos aguardando a avaliação do modelo: 1") || !strings.Contains(all, tc.reason) {
			t.Errorf("%s: the run must say the finding awaits the model (%s):\n%s", tc.name, tc.reason, all)
		}
		for _, banned := range []string{"reprovam o gate", "policy gate: analysis/hardcoded-secret", "deixou de contar"} {
			if strings.Contains(all, banned) {
				t.Errorf("%s: %q must not appear: the static finding does not fail the gate without the model:\n%s", tc.name, banned, all)
			}
		}
	}
}

// (c) The review's provider failing on a pull request, in both
// gate.inconclusive modes: the security finding at the threshold waits for
// the model; the gate is inconclusive and fails, and no line counts it as a
// finding that fails the gate.
func TestProviderFailureLeavesEvidenceAwaiting(t *testing.T) {
	for _, cfg := range []string{blockGate, warnGate} {
		c := sessionCase{name: "provider failure", cfg: cfg, model: modelProviderFailed, security: []types.ReviewIssue{securityBreach}}
		code, lines := runSessionCase(t, session.PullRequest, c)
		all := strings.Join(lines, "\n")
		if code != exitQualityNotReviewed {
			t.Errorf("%q: exit=%d, want the inconclusive %d", cfg, code, exitQualityNotReviewed)
		}
		if !strings.Contains(all, "1 deterministic finding(s) awaiting the model's assessment") || !strings.Contains(all, "provider_failure") {
			t.Errorf("%q: the gate must say the finding awaits the model:\n%s", cfg, all)
		}
		if strings.Contains(all, fmt.Sprintf("%s -", securityBreach.RuleID)) {
			t.Errorf("%q: the awaiting finding was published as a gate failure:\n%s", cfg, all)
		}
	}
}

// An explicit --fail-on respects the triage: the finding the model disputed
// does not count toward it.
func TestFailOnRespectsTheTriage(t *testing.T) {
	aur579Repo(t)
	aur608Config(t, triageGate)
	t.Setenv("AURUMCODE_LLM_FIXTURE", triageFixture(t, `{"evidence_assessments":[{"evidence_id":"E1","status":"disputed","justification":"valor sintetico de fixture de teste"}]}`))
	if code, _, errOut := aur579Review(t, nil, "--fail-on", "warning"); code != 0 {
		t.Fatalf("the disputed finding must not count toward --fail-on: exit=%d\n%s", code, errOut)
	}
}
