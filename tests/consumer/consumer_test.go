package consumer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func scenarios(t *testing.T) []Scenario {
	t.Helper()
	s, err := LoadScenarios("cenarios.json")
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func byID(t *testing.T, all []Scenario, id string) Scenario {
	t.Helper()
	for _, s := range all {
		if s.ID == id {
			return s
		}
	}
	t.Fatalf("cenário %s ausente", id)
	return Scenario{}
}

// conforming builds the evidence a correct product would record for s. It
// is synthetic and lives only in memory: it proves the verifier, never the
// product.
func conforming(s Scenario) Evidence {
	e := Evidence{Scenario: s.ID, ToolSHA: strings.Repeat("a", 40), Repo: "exemplo/consumidor", SHA: "abcdef1234567", RunID: 42, RunURL: "https://github.example.test/run/42", Measured: true}
	fill := func(x Expectation, e *Evidence) {
		e.Conclusion, e.Language, e.Mode = x.Conclusion, x.Language, x.Mode
		e.Statuses = map[string]string{}
		for k, v := range x.Statuses {
			e.Statuses[k] = v
		}
		if x.InlineSuggestion {
			e.Inline = 1
		}
		e.Comments = 1
		e.Text = strings.Join(x.Contains, " ")
	}
	fill(s.Expect, &e)
	if s.AfterFix != nil {
		after := e
		after.AfterFix = nil
		fill(*s.AfterFix, &after)
		e.AfterFix = &after
	}
	return e
}

// AC-001..AC-004: the scenario table covers both installations pinned by
// SHA, both publication modes, inline suggestion, every failure the card
// names, fork, changelog, repeated round and the fix that clears the block.
func TestAUR512ScenarioTableCoversTheContract(t *testing.T) {
	all := scenarios(t)
	acs := map[string]bool{}
	workflows := map[string]bool{}
	var modes, inline, rounds, fix, fork bool
	for _, s := range all {
		acs[s.AC] = true
		workflows[s.Workflow] = true
		modes = modes || s.Expect.Mode == "review"
		inline = inline || s.Expect.InlineSuggestion
		rounds = rounds || (s.Rounds > 1 && s.Expect.MaxComments == 1)
		fix = fix || (s.Fix != "" && s.AfterFix != nil && s.AfterFix.Conclusion == "success")
		fork = fork || (s.AC == "AC-003" && s.Manual)
		if _, err := os.Stat(filepath.Join("fixtures", s.Fixture)); err != nil {
			t.Errorf("%s: fixture %s ausente", s.ID, s.Fixture)
		}
	}
	for _, ac := range []string{"AC-001", "AC-002", "AC-003", "AC-004"} {
		if !acs[ac] {
			t.Errorf("nenhum cenário cobre %s", ac)
		}
	}
	if !modes || !inline || !rounds || !fix || !fork {
		t.Errorf("cobertura incompleta: review=%v inline=%v rodadas=%v correcao=%v fork=%v", modes, inline, rounds, fix, fork)
	}
	for _, id := range []string{"modelo-ausente", "modelo-inconclusivo", "sem-permissao", "ci-falhando", "changelog-ausente", "changelog-valido", "acao-direta"} {
		byID(t, all, id)
	}
	for wf := range workflows {
		data, err := os.ReadFile(filepath.Join("workflows", wf))
		if err != nil {
			t.Fatalf("workflow %s: %v", wf, err)
		}
		text := string(data)
		if !strings.Contains(text, "@0000000000000000000000000000000000000000") || strings.Contains(text, "AurumCode/.github/workflows/review.yml@main") || strings.Contains(text, "Mpaape/AurumCode@main") {
			t.Errorf("workflow %s não está pinado pelo SHA sob teste", wf)
		}
		if strings.Contains(text, "pull_request_target") {
			t.Errorf("workflow %s usa pull_request_target: código de fork com token privilegiado", wf)
		}
	}
}

// AC-004 and MUT-001: a product whose changelog gate (or quality gate) was
// removed lets a negative scenario pass; the QA must reject that evidence.
func TestAUR512RegressedProductFailsQA(t *testing.T) {
	all := scenarios(t)
	evidence := map[string]Evidence{}
	for _, s := range all {
		if !s.Manual {
			evidence[s.ID] = conforming(s)
		}
	}
	if results, ok := Report(all, evidence); !ok {
		t.Fatalf("a conforming run must pass the QA: %+v", results)
	}
	regressed := conforming(byID(t, all, "changelog-ausente"))
	regressed.Conclusion = "success"
	regressed.Statuses["changelog / Changelog obrigatório"] = "success"
	evidence["changelog-ausente"] = regressed
	if _, ok := Report(all, evidence); ok {
		t.Fatal("a PR without changelog entry that passed must fail the QA")
	}
	quality := conforming(byID(t, all, "comments-inline"))
	quality.Conclusion = "success"
	quality.Statuses["aurumcode/policy-gate"] = "success"
	if r := Verify(byID(t, all, "comments-inline"), quality, true); r.State != Failed {
		t.Fatalf("a SQL injection that passed the gate must fail the QA: %+v", r)
	}
	repeated := conforming(byID(t, all, "rodada-repetida"))
	repeated.Comments = 2
	if r := Verify(byID(t, all, "rodada-repetida"), repeated, true); r.State != Failed {
		t.Fatalf("a repeated round that doubled the comments must fail: %+v", r)
	}
	stuck := conforming(byID(t, all, "comments-inline"))
	stuck.AfterFix.Conclusion = "failure"
	if r := Verify(byID(t, all, "comments-inline"), stuck, true); r.State != Failed {
		t.Fatalf("a fix that did not clear the block must fail: %+v", r)
	}
}

// AC-005: evidence names repo, SHA and run; an infrastructure failure is
// "nao_medido" with its limitation, never a pass and never silent.
func TestAUR512EvidenceIdentifiesTheRunAndInfraIsNotMeasured(t *testing.T) {
	all := scenarios(t)
	s := byID(t, all, "comments-inline")
	anonymous := conforming(s)
	anonymous.Repo, anonymous.SHA, anonymous.RunID = "", "", 0
	if r := Verify(s, anonymous, true); r.State != Failed || len(r.Reasons) < 3 {
		t.Fatalf("evidence without repo, SHA and run: %+v", r)
	}
	billing := Evidence{Scenario: s.ID, Measured: false, Limitations: []string{"Actions sem minutos: billing"}}
	if r := Verify(s, billing, true); r.State != NotMeasured || r.Reasons[0] != "Actions sem minutos: billing" {
		t.Fatalf("infrastructure failure: %+v", r)
	}
	silent := Evidence{Scenario: s.ID, Measured: false}
	if r := Verify(s, silent, true); r.State != Failed {
		t.Fatalf("not measured without a limitation must fail: %+v", r)
	}
	if r := Verify(s, Evidence{}, false); r.State != Failed {
		t.Fatalf("missing evidence must fail: %+v", r)
	}
	fork := byID(t, all, "pr-de-fork")
	if r := Verify(fork, Evidence{}, false); r.State != NotMeasured {
		t.Fatalf("manual scenario without evidence is not measured: %+v", r)
	}
	// A release needs every non-manual scenario measured and passed.
	all2 := map[string]Evidence{}
	for _, sc := range all {
		if !sc.Manual {
			all2[sc.ID] = Evidence{Scenario: sc.ID, Measured: false, Limitations: []string{"billing"}}
		}
	}
	if _, ok := Report(all, all2); ok {
		t.Fatal("zero measured scenarios must not pass the QA")
	}
	dupDir := t.TempDir()
	for _, name := range []string{"a.json", "b.json"} {
		if err := os.WriteFile(filepath.Join(dupDir, name), []byte(`{"cenario":"comments-inline"}`), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := LoadEvidence(dupDir); err == nil {
		t.Fatal("two evidence files for the same scenario must be refused")
	}
	emptyDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(emptyDir, "x.json"), []byte(`{"medido":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadEvidence(emptyDir); err == nil {
		t.Fatal("evidence without a scenario must be refused")
	}
	if dir := os.Getenv("AURUMCODE_QA_EVIDENCIA"); dir != "" {
		evidence, err := LoadEvidence(dir)
		if err != nil {
			t.Fatal(err)
		}
		results, ok := Report(all, evidence)
		for _, r := range results {
			t.Logf("%s: %s %v", r.Scenario, r.State, r.Reasons)
		}
		if !ok {
			t.Fatal("a evidência gravada pelo run.sh reprova o QA do consumidor")
		}
	}
}
