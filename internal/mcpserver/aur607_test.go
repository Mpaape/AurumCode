package mcpserver

import (
	"strings"
	"testing"
)

// AC-001: the next step written for the human follows the review language;
// English keeps its earlier bytes; an inconclusive answer in Portuguese says
// what to configure.
func TestAUR607NextStepFollowsTheLanguage(t *testing.T) {
	en := map[Decision]string{
		DecisionPass:         "The gate passes. Commit or push.",
		DecisionFail:         "Fix every blocking finding (see suggestion, or call aurum_explain with its id), then call aurum_gate again. Never disable or weaken a rule to pass.",
		DecisionInconclusive: "The review did not conclude, so this is not a pass. Read reason and fix the cause (configure the model provider, retry), then call aurum_gate again.",
	}
	for d, want := range en {
		for _, reason := range []string{"", "quality_skipped", "sast_unavailable"} {
			if got := nextStep("en-US", d, reason); got != want {
				t.Errorf("en %s/%q next = %q, want %q", d, reason, got, want)
			}
		}
	}
	if got := nextStep("en-US", DecisionInconclusive, "quality_skipped"); got != en[DecisionInconclusive] {
		t.Errorf("en inconclusive = %q", got)
	}
	if got := nextStep("", DecisionInconclusive, reasonEmptyChange); !strings.HasPrefix(got, "Nothing changed between base and HEAD") {
		t.Errorf("en empty change = %q", got)
	}
	provider := answerFor(SessionOutcome{Language: "pt-BR", InconclusiveReason: "quality_skipped"}).Next
	if !strings.HasPrefix(provider, "A revisão não concluiu, então isto não é aprovação.") || !strings.Contains(provider, "LLM_API_KEY e LLM_BASE_URL no ambiente que inicia o aurumcode mcp") {
		t.Errorf("pt-BR provider next step = %q", provider)
	}
	scanner := nextStep("pt-BR", DecisionInconclusive, "sast_unavailable,partial_coverage")
	if !strings.Contains(scanner, "Instale o scanner que faltou") || strings.Contains(scanner, "LLM_API_KEY") {
		t.Errorf("pt-BR scanner next step = %q", scanner)
	}
	for _, d := range []Decision{DecisionPass, DecisionFail} {
		if got := nextStep("pt-BR", d, ""); got == en[d] || got == "" {
			t.Errorf("pt-BR %s next step is not Portuguese: %q", d, got)
		}
	}
}

// AC-001: aurum_explain's how_to_fix follows the latest answer's language.
func TestAUR607HowToFixFollowsTheLanguage(t *testing.T) {
	s := New(Options{})
	f := Finding{ID: "0123456789ab", File: "app.go", Line: 6, RuleID: "security/hardcoded-secret", Origin: "security"}
	for _, tc := range []struct{ language, want string }{
		{"en-US", "Rule security/hardcoded-secret (security) at app.go:6. Apply the suggestion or remove the cause in the code, then call aurum_gate again; an exception or a disabled rule is a policy decision for a human, not a fix."},
		{"pt-BR", "Regra security/hardcoded-secret (security) em app.go:6. Aplique a sugestão ou remova a causa no código e chame aurum_gate de novo; exceção ou regra desligada é decisão de política para um humano, não correção."},
	} {
		s.remember(gateAnswer{Findings: []Finding{f}}, nil, tc.language)
		out, _, rerr := s.explain([]byte(`{"finding_id":"0123456789ab"}`))
		if rerr != nil {
			t.Fatal(rerr)
		}
		if got := out.(map[string]any)["how_to_fix"]; got != tc.want {
			t.Errorf("%s how_to_fix = %q, want %q", tc.language, got, tc.want)
		}
	}
}
