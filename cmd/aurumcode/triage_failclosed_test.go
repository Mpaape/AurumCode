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
	aur608NoticePT = "gate.triage: achados determinísticos aguardando a avaliação do modelo: 1 — o modelo não deu uma resposta utilizável (revisão por modelo não executada (sem provedor configurado) [quality_skipped]); sem a decisão do modelo não há veredito"
	aur608NoticeEN = "gate.triage: 1 deterministic finding(s) awaiting the model's assessment — the model gave no usable answer (quality_skipped); without the model's decision there is no verdict"
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

// AC-004 (owner, 2026-10-14): without the model, with evidence and a
// declared gate, the finding waits for the model (inconclusive) and a line (pt-BR under review.language pt-BR, en-US by default)
// says the triage did not happen, in stderr and in the report.
func TestAUR608WithoutModelKeepsDeterministicBlock(t *testing.T) {
	for _, tc := range []struct{ lang, notice string }{
		{"review:\n  language: pt-BR\n", aur608NoticePT},
		{"", aur608NoticeEN},
	} {
		aur579Repo(t)
		aur608Config(t, tc.lang+"gate:\n  fail_on: [high]\n")
		code, out, errOut := aur579Review(t, nil)
		if code == 0 || code == exitFindings {
			t.Fatalf("without the model the deterministic finding must wait for it (inconclusive): exit=%d\nstderr=%s", code, errOut)
		}
		if !strings.Contains(errOut, tc.notice) {
			t.Errorf("stderr must say the finding awaits the model (%q):\n%s", tc.notice, errOut)
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
// and the fail-closed notice join the review's limitations; the parecer
// renders the notice as a sentence and leaves the refuted finding out. The
// notice needs triageable evidence and a model that did not answer.
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
	body := formatPublishedReviewBody(s.result, &types.Diff{}, "pt-BR", false, "")
	// A finding the model refuted leaves the parecer; the line stays on
	// stderr and in the review's limitations for the audit.
	if strings.Contains(body, "gate.triage") || !strings.Contains(strings.Join(s.result.Limitations, "\n"), "gate.triage (analysis: model): app.go:4 analysis/hardcoded-secret contestado pelo modelo deixou de contar") {
		t.Errorf("the refuted finding must leave the parecer and stay in the limitations:\n%s", body)
	}

	// Without the model (none configured, or one that failed) the finding
	// waits for it: one line, inconclusive and failing, never a breach.
	for _, model := range []modelOutcome{modelSkipped, modelProviderFailed} {
		s, _ = newState(model)
		if tr := s.triage(); !tr.Awaiting || !tr.BySource[config.GateSourceAnalysis] {
			t.Fatalf("model %v: the analysis evidence must await the model: %+v", model, tr)
		}
		res := &gateDecision{}
		s.settleAwaiting([]gate.Demotion{{Source: config.GateSourceAnalysis, Issue: finding}}, res)
		if !res.Inconclusive || !res.Fail || res.Breach || len(res.BlockingFindings) != 0 || !strings.Contains(strings.Join(res.Lines, "\n"), "aguardando a avaliação do modelo: 1") {
			t.Errorf("model %v: the awaiting finding must leave the gate inconclusive, not breached: %+v", model, res)
		}
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
		if s.triage().BySource[config.GateSourceAnalysis] {
			t.Errorf("%s: no triage could happen, the evidence counts", name)
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
	sample := "hunter2-" + "correct-horse"
	s.diff = &types.Diff{Files: []types.DiffFile{{Path: "app.go", Hunks: []types.DiffHunk{{NewStart: 4, Lines: []string{"+\tdbPassword := \"" + sample + "\""}}}}}}
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
	// The owner's rule (2026-10-14): the model decides. Without a usable
	// answer the finding waits for it -- inconclusive, failing -- and never
	// fails the gate as a static error.
	notice := "gate.triage: achados determinísticos aguardando a avaliação do modelo: 1 — o modelo não deu uma resposta utilizável (a resposta do modelo só foi aproveitada como texto livre [degraded_parse]); sem a decisão do modelo não há veredito"
	if !strings.Contains(errOut, notice) || !strings.Contains(strings.Join(s.result.Limitations, "\n"), notice) {
		t.Errorf("stderr and the parecer must say the finding awaits the model (%q):\nstderr=%s\nlimitations=%v", notice, errOut, s.result.Limitations)
	}
	if strings.Contains(errOut, "policy gate: analysis/hardcoded-secret") || s.gateRes == nil || len(s.gateRes.BlockingFindings) != 0 || !s.gateRes.Inconclusive {
		t.Errorf("the awaiting finding must not block as a static error:\n%s", errOut)
	}
}
