package main

// AUR-608: the model decides over the deterministic evidence by default
// (gate.triage absent is model), only with a justification, only for a
// declared gate and never under a central policy; without the model's
// answer the evidence counts in full and the run says so. Driven through
// the real `aurumcode review --base` command over AUR-579's fixture
// repository (analysis E1 and, with SAST, semgrep E2 on app.go:4).

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/gate"
	"github.com/Mpaape/AurumCode/internal/prompt"
	"github.com/Mpaape/AurumCode/internal/review/session"
	"github.com/Mpaape/AurumCode/internal/scanner"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// aur608Justified disputes both offered items with a justification.
const aur608Justified = `[
 {"evidence_id":"E1","status":"disputed","justification":"o valor e um exemplo de teste, nao uma credencial","priority":"low"},
 {"evidence_id":"E2","status":"disputed","justification":"mesmo trecho de E1, valor de exemplo","priority":"low"}
]`

// aur608Unjustified disputes both offered items without any reasoning.
const aur608Unjustified = `[
 {"evidence_id":"E1","status":"disputed","justification":"","priority":"low"},
 {"evidence_id":"E2","status":"disputed","justification":"   ","priority":"low"}
]`

// The demotion lines AC-001 expects, one per triaged source.
const (
	aur608AnalysisDemoted = "gate.triage (analysis: model): app.go:4 analysis/hardcoded-secret contestado pelo modelo deixou de contar"
	aur608SASTDemoted     = "gate.triage (sast: model): app.go:4 semgrep:generic.secrets.hardcoded contestado pelo modelo deixou de contar"
)

// The fail-closed notice of AC-004, in pt-BR and in the default en-US.
const (
	aur608NoticePT = "gate.triage: a triagem pelo modelo não ocorreu (quality_skipped); a evidência determinística contou integralmente e o bloqueio foi mantido"
	aur608NoticeEN = "gate.triage: the model's triage did not run (quality_skipped); the deterministic evidence counted in full and the block was kept"
)

// aur608Config writes the fixture repository's own configuration.
func aur608Config(t *testing.T, cfg string) {
	t.Helper()
	if err := os.MkdirAll(".aurumcode", 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(".aurumcode", "config.yml"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
}

// aur608SASTGate is a declared gate over SAST and the embedded analysis.
const aur608SASTGate = "review:\n  language: pt-BR\nquality_gates:\n  sast:\n    enabled: true\ngate:\n  fail_on: [high]\n"

// AC-001: with no gate.triage key, a declared gate and a justified dispute,
// the disputed evidence of every source stops counting and both stderr and
// the report name the source that was triaged.
func TestAUR608DefaultTriageDemotesJustifiedDispute(t *testing.T) {
	aur579Repo(t)
	semgrep := aur579SAST(t, "")
	aur608Config(t, aur608SASTGate)
	t.Setenv("AURUMCODE_LLM_FIXTURE", aur579Fixture(t, aur608Justified))
	code, out, errOut := aur579Review(t, semgrep)
	if code != 0 {
		t.Fatalf("no gate.triage key: the justified dispute must demote and the gate pass: exit=%d\nstderr=%s\nout=%s", code, errOut, out)
	}
	for _, want := range []string{aur608AnalysisDemoted, aur608SASTDemoted} {
		if !strings.Contains(errOut, "aurumcode review: "+want) {
			t.Errorf("stderr must name the triaged source (%q):\n%s", want, errOut)
		}
	}
	if strings.Contains(errOut, "a triagem pelo modelo não ocorreu") {
		t.Errorf("the model answered: no fail-closed notice:\n%s", errOut)
	}
}

// AC-002: an explicit none keeps that source's evidence counting.
func TestAUR608ExplicitNoneKeepsEvidenceCounting(t *testing.T) {
	aur579Repo(t)
	semgrep := aur579SAST(t, "")
	aur608Config(t, aur608SASTGate+"  triage:\n    analysis: none\n    semgrep: none\n")
	t.Setenv("AURUMCODE_LLM_FIXTURE", aur579Fixture(t, aur608Justified))
	code, out, errOut := aur579Review(t, semgrep)
	if code != exitFindings {
		t.Fatalf("triage none: the disputed evidence must keep counting: exit=%d\nstderr=%s", code, errOut)
	}
	if strings.Contains(errOut, "gate.triage") || strings.Contains(out, "gate.triage") {
		t.Errorf("triage none: nothing is demoted or announced:\nstderr=%s\nout=%s", errOut, out)
	}
}

// AC-003: a dispute with an empty or blank justification weighs as
// needs_context and keeps counting.
func TestAUR608DisputeWithoutJustificationStillCounts(t *testing.T) {
	aur579Repo(t)
	semgrep := aur579SAST(t, "")
	aur608Config(t, aur608SASTGate)
	t.Setenv("AURUMCODE_LLM_FIXTURE", aur579Fixture(t, aur608Unjustified))
	code, _, errOut := aur579Review(t, semgrep)
	if code != exitFindings {
		t.Fatalf("a dispute without a justification must not demote: exit=%d\nstderr=%s", code, errOut)
	}
	if strings.Contains(errOut, "contestado pelo modelo deixou de contar") {
		t.Errorf("nothing may be demoted without reasoning:\n%s", errOut)
	}
}

// AC-004: without the model, with evidence and a declared gate, the block
// is kept and a line (pt-BR under review.language pt-BR, en-US by default)
// says the triage did not happen, in stderr and in the report.
func TestAUR608WithoutModelKeepsDeterministicBlock(t *testing.T) {
	for _, tc := range []struct{ lang, notice string }{
		{"review:\n  language: pt-BR\n", aur608NoticePT},
		{"", aur608NoticeEN},
	} {
		aur579Repo(t)
		aur608Config(t, tc.lang+"gate:\n  fail_on: [high]\n")
		code, out, errOut := aur579Review(t, nil)
		if code != exitFindings {
			t.Fatalf("without the model the deterministic finding must keep blocking: exit=%d\nstderr=%s", code, errOut)
		}
		if !strings.Contains(errOut, "aurumcode review: "+tc.notice) {
			t.Errorf("stderr must say the triage did not happen (%q):\n%s", tc.notice, errOut)
		}
		if strings.Contains(out, "contestado pelo modelo deixou de contar") {
			t.Errorf("without the model nothing may be demoted:\n%s", out)
		}
	}
}

// AC-005: without a declared gate nothing is triaged or announced, even
// where a scanner withholds approval on its own.
func TestAUR608TriageSilentWithoutDeclaredGate(t *testing.T) {
	aur579Repo(t)
	semgrep := aur579SAST(t, "")
	t.Setenv("AURUMCODE_LLM_FIXTURE", aur579Fixture(t, aur608Justified))
	code, out, errOut := aur579Review(t, semgrep)
	if code != exitFindings {
		t.Fatalf("no gate: the scanner's own decision is unchanged by the dispute: exit=%d\nstderr=%s", code, errOut)
	}
	if strings.Contains(errOut, "gate.triage") || strings.Contains(out, "gate.triage") {
		t.Errorf("no gate: nothing may be demoted or announced:\nstderr=%s\nout=%s", errOut, out)
	}
	aur579Repo(t)
	_, out, errOut = aur579Review(t, nil)
	if strings.Contains(errOut, "gate.triage") || strings.Contains(out, "gate.triage") {
		t.Errorf("no gate, no model: nothing may be announced:\nstderr=%s\nout=%s", errOut, out)
	}
}

// AC-006: under a central policy the result is the one before AUR-608: the
// justified dispute does not demote (it becomes a proposed exception), and
// without the model nothing about the triage is announced.
func TestAUR608CentralPolicyUnchanged(t *testing.T) {
	aur579Repo(t)
	policy := policyFixture(t, "gate:\n  fail_on: [high]\n")
	t.Setenv("AURUMCODE_LLM_FIXTURE", aur579Fixture(t, aur608Justified))
	code, out, errOut := aur579Review(t, nil, "--politica", policy)
	if code != exitFindings {
		t.Fatalf("policy: the disputed finding must still fail the gate: exit=%d\nstderr=%s", code, errOut)
	}
	if strings.Contains(errOut, "gate.triage") {
		t.Errorf("policy: nothing may be demoted under a central policy:\n%s", errOut)
	}
	if !strings.Contains(out, `rule: "analysis/hardcoded-secret"`) {
		t.Errorf("policy: the dispute must still become a proposed exception:\n%s", out)
	}
	aur579Repo(t)
	code, out, errOut = aur579Review(t, nil, "--politica", policy)
	if code != exitFindings {
		t.Fatalf("policy, no model: the finding must keep failing the gate: exit=%d\nstderr=%s", code, errOut)
	}
	if strings.Contains(errOut, "gate.triage") || strings.Contains(out, "gate.triage") {
		t.Errorf("policy, no model: nothing about the triage may be announced:\nstderr=%s\nout=%s", errOut, out)
	}
}

// AC-001 and AC-004, parecer half: the demotion line (naming the source)
// and the fail-closed notice join the review's limitations, which the
// published parecer renders; the notice needs triageable evidence and a
// model that did not answer.
func TestAUR608TriageLinesReachTheParecer(t *testing.T) {
	finding := types.ReviewIssue{File: "app.go", Line: 4, Severity: "error", RuleID: "analysis/hardcoded-secret", Origin: gateOriginAnalysis}
	newState := func(model modelOutcome) (*reviewState, *strings.Builder) {
		var errOut strings.Builder
		return &reviewState{
			stderr: &errOut, result: &types.ReviewResult{}, model: model, reviewLanguage: "pt-BR",
			cfg:            &config.Config{Gate: config.GateConfig{FailOn: []string{"high"}}},
			analysisIssues: []types.ReviewIssue{finding},
		}, &errOut
	}

	s, _ := newState(modelReviewed)
	s.reportTriage([]gate.Demotion{{Source: config.GateSourceAnalysis, Issue: finding}})
	s.reportTriageNotRun(&gateDecision{})
	body := formatPublishedReviewBody(s.result, &types.Diff{}, "pt-BR", false, "")
	if !strings.Contains(body, "gate.triage (analysis: model): app.go:4 analysis/hardcoded-secret contestado pelo modelo deixou de contar") {
		t.Errorf("the parecer must name the triaged source:\n%s", body)
	}
	if strings.Contains(body, "a triagem pelo modelo não ocorreu") {
		t.Errorf("the model answered: no notice:\n%s", body)
	}

	s, errOut := newState(modelSkipped)
	s.reportTriageNotRun(&gateDecision{Fail: true})
	body = formatPublishedReviewBody(s.result, &types.Diff{}, "pt-BR", false, "")
	if !strings.Contains(body, aur608NoticePT) || !strings.Contains(errOut.String(), aur608NoticePT) {
		t.Errorf("the parecer and stderr must say the triage did not happen:\n%s\nstderr=%s", body, errOut.String())
	}

	s, _ = newState(modelProviderFailed)
	s.reportTriageNotRun(&gateDecision{})
	if got := strings.Join(s.result.Limitations, "\n"); !strings.Contains(got, "(provider_failure); a evidência determinística contou integralmente") || strings.Contains(got, "bloqueio") {
		t.Errorf("a passing gate states the evidence counted, never a kept block: %q", got)
	}

	for name, configure := range map[string]func(*reviewState){
		"no declared gate": func(s *reviewState) { s.cfg = &config.Config{} },
		"central policy":   func(s *reviewState) { s.centralCfg = &config.Config{} },
		"analysis opted out": func(s *reviewState) {
			s.cfg.Gate.Triage = map[string]string{config.GateSourceAnalysis: config.TriageNone}
		},
	} {
		s, _ = newState(modelSkipped)
		configure(s)
		s.reportTriageNotRun(&gateDecision{Fail: true})
		if len(s.result.Limitations) != 0 {
			t.Errorf("%s: no triage could happen, nothing to announce: %v", name, s.result.Limitations)
		}
	}
}

// AC-004, degraded half: a consolidated answer the parser degraded (one
// profile or batch out of several, profiles.go and batch_merge.go) never
// demotes, even when a healthy pass disputed the evidence with a
// justification. Under gate.inconclusive: warn the finding still fails the
// gate, and the run says the triage did not happen. Driven through the
// session's real gate and exit steps.
func TestAUR608DegradedModelNeverDemotes(t *testing.T) {
	cfg, err := config.Parse([]byte("review:\n  language: pt-BR\ngate:\n  fail_on: [high]\n  inconclusive: warn\n"), "case.yml")
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr strings.Builder
	s := newReviewState(session.LocalDiff, reviewIO{stdout: &stdout, stderr: &stderr, filter: redaction.NewFilter(),
		deps: reviewDeps{scanners: scanner.Executor{Command: missingSemgrep}, env: &reviewEnv{}}})
	s.cfg, s.model, s.reviewLanguage = cfg, modelReviewed, "pt-BR"
	s.repoIdentity, s.repoIdentityKnown = "owner/repo", true
	secret := "hunter2-" + "correct-horse"
	s.diff = &types.Diff{Files: []types.DiffFile{{Path: "app.go", Hunks: []types.DiffHunk{{NewStart: 4, Lines: []string{"+\tdbPassword := \"" + secret + "\""}}}}}}
	s.analysisIssues = staticAnalysisIssues(s.diff, s.reviewLanguage)
	if len(s.analysisIssues) == 0 {
		t.Fatal("fixture: the embedded analysis must report the credential")
	}
	s.analysisIssues[0].Assessment = &types.EvidenceAssessment{EvidenceID: "E1", Status: types.AssessmentDisputed, Justification: "valor de exemplo de teste, nao uma credencial"}
	s.result = &types.ReviewResult{Metadata: map[string]string{prompt.ParseModeKey: prompt.ParseModeDegraded}}
	s.result.Issues = append(s.result.Issues, s.analysisIssues...)
	defer s.flush()
	if code, done := s.runGate(); done {
		t.Fatalf("the gate ended the run (exit %d):\n%s", code, stderr.String())
	}
	code := s.decideExit(publishOutcome{})
	errOut := stderr.String()
	if code == 0 {
		t.Fatalf("a degraded answer demoted the disputed finding and the run passed (fail open):\n%s", errOut)
	}
	if strings.Contains(errOut, "deixou de contar") {
		t.Errorf("a degraded answer must never demote:\n%s", errOut)
	}
	notice := "gate.triage: a triagem pelo modelo não ocorreu (degraded_parse); a evidência determinística contou integralmente e o bloqueio foi mantido"
	if !strings.Contains(errOut, notice) || !strings.Contains(strings.Join(s.result.Limitations, "\n"), notice) {
		t.Errorf("stderr and the parecer must say the triage did not happen (%q):\nstderr=%s\nlimitations=%v", notice, errOut, s.result.Limitations)
	}
}
