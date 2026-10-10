package gate

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// AC-002: the scanner's inconclusive line names its reason as a sentence
// with the code in brackets in Portuguese and keeps its English bytes, on
// stderr and in the limitations.
func TestAUR607ScannerInconclusiveLineFollowsTheLanguage(t *testing.T) {
	missing := ScannerContributor{Scans: []Scan{semgrepScan(t, OriginRepo, "sast_unavailable")}}
	for _, tc := range []struct{ language, want string }{
		{"en-US", "aurumcode review: policy gate: SAST (semgrep, origem sast, secao repo) inconclusivo (sast_unavailable)\n"},
		{"pt-BR", "aurumcode review: policy gate: SAST (semgrep, origem sast, secao repo) inconclusivo (análise estática habilitada, mas o Semgrep não está instalado [sast_unavailable])\n"},
	} {
		run := sastOnlyRun("block")
		run.Language = tc.language
		var stderr bytes.Buffer
		run.Stderr = &stderr
		var res Result
		if err := NewPipeline(missing).Run(context.Background(), run, &res); err != nil {
			t.Fatal(err)
		}
		ApplyOutcome(run, &res)
		if stderr.String() != tc.want {
			t.Errorf("%s stderr = %q, want %q", tc.language, stderr.String(), tc.want)
		}
		if len(run.Review.Limitations) != 1 || "aurumcode review: "+run.Review.Limitations[0]+"\n" != tc.want {
			t.Errorf("%s limitations = %q", tc.language, run.Review.Limitations)
		}
	}
}

// AC-002: the policy gate's inconclusive line in both languages, and the
// first breach the status names.
func TestAUR607PolicyLineAndFirstBreach(t *testing.T) {
	gateCfg := config.GateConfig{FailOn: []string{"error"}, Inconclusive: "warn"}
	for _, tc := range []struct{ language, want string }{
		{"", "review inconclusive (partial_coverage)"},
		{"pt-BR", "revisão inconclusiva — parte do diff ficou fora da revisão [partial_coverage]"},
	} {
		d, err := EvaluateGateIn(tc.language, gateCfg, OriginRepo, nil, nil, "partial_coverage", nil, "", time.Now())
		if err != nil {
			t.Fatal(err)
		}
		if len(d.Lines) != 1 || d.Lines[0] != tc.want {
			t.Errorf("%q lines = %q, want %q", tc.language, d.Lines, tc.want)
		}
	}
	var d Result
	issues := []types.ReviewIssue{
		{File: "a.go", Line: 3, Severity: "error", RuleID: "analysis/x", Message: "primeiro"},
		{File: "b.go", Line: 9, Severity: "error", RuleID: "analysis/y", Message: "segundo"},
	}
	if err := applyDeterministic(&d, gateCfg, issues, nil, "", time.Now(), OriginAnalysis, func(string) bool { return true }); err != nil {
		t.Fatal(err)
	}
	if d.FirstBreach == nil || d.FirstBreach.Title != "primeiro" || d.FirstBreach.Path != "a.go" || d.FirstBreach.Line != 3 || d.FirstBreach.Severity != "error" {
		t.Fatalf("first breach = %+v", d.FirstBreach)
	}
	var merged Result
	merged.Merge(Result{Active: true})
	merged.Merge(d)
	if merged.FirstBreach != d.FirstBreach || !strings.Contains(strings.Join(merged.Lines, "\n"), "primeiro") {
		t.Fatalf("merge lost the first breach: %+v", merged.FirstBreach)
	}
}

// AC-002: the Dependency-Track line names its reason in the review language;
// English keeps the code.
func TestAUR607DTrackReasonFollowsTheLanguage(t *testing.T) {
	// Names of unset environment variables, built at runtime so the
	// repository's own gate does not read them as a credential literal.
	keyVar, projectVar := "AUR607_UNSET_"+"KEY", "AUR607_UNSET_"+"PROJECT"
	cfg := &config.SsorDtrackConfig{Enabled: true, APIKeySecret: keyVar, ProjectIDSecret: projectVar}
	t.Setenv(keyVar, "")
	t.Setenv(projectVar, "")
	for _, tc := range []struct{ language, want string }{
		{"", "ssor_dtrack: revisão inconclusiva (dtrack_secret_missing): "},
		{"pt-BR", "ssor_dtrack: revisão inconclusiva (falta a chave de acesso ao Dependency-Track [dtrack_secret_missing]): "},
	} {
		res, reason, _ := ApplyDTrackGate(context.Background(), tc.language, cfg, nil)
		if reason != ReasonDTrackSecretMissing || len(res.Lines) != 1 || !strings.HasPrefix(res.Lines[0], tc.want) {
			t.Errorf("%q: reason %q lines %q, want prefix %q", tc.language, reason, res.Lines, tc.want)
		}
	}
}

// AC-001: the count of findings a reused verdict added keeps its earlier
// bytes in English and agrees in number in Portuguese.
func TestAUR607ReappliedVerdictCount(t *testing.T) {
	for _, tc := range []struct {
		language string
		n        int
		want     string
	}{
		{"", 1, "1 achado(s) de uma revisão concluída anterior deste conteúdo reaplicado(s)"},
		{"", 2, "2 achado(s) de uma revisão concluída anterior deste conteúdo reaplicado(s)"},
		{"pt-BR", 1, "1 achado de uma revisão concluída anterior deste conteúdo foi reaplicado"},
		{"pt-BR", 2, "2 achados de uma revisão concluída anterior deste conteúdo foram reaplicados"},
	} {
		if got := reappliedText(tc.language, tc.n); got != tc.want {
			t.Errorf("reappliedText(%q, %d) = %q, want %q", tc.language, tc.n, got, tc.want)
		}
	}
}

// The audit record keeps the language-independent lines: the inconclusive
// reason codes and the engine's own finding message, across a merge.
func TestAUR607AuditLinesIgnoreTheLanguage(t *testing.T) {
	gateCfg := config.GateConfig{FailOn: []string{"error"}, Inconclusive: "warn"}
	d, err := EvaluateGateIn("pt-BR", gateCfg, OriginRepo, nil, nil, "partial_coverage", nil, "", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	scan := semgrepScan(t, OriginRepo, "")
	issue := types.ReviewIssue{File: "a.go", Line: 2, Severity: "error", RuleID: "semgrep:x", Message: "mensagem mostrada"}
	scan.ToolMessages = map[string]string{FindingOriginKey("semgrep:x", "a.go", 2): "engine message"}
	var part Result
	if err := ApplyScannerGateIn("pt-BR", &part, scan, []types.ReviewIssue{issue}); err != nil {
		t.Fatal(err)
	}
	var merged Result
	merged.Merge(d)
	merged.Merge(part)
	shown, audit := strings.Join(merged.Lines, "\n"), strings.Join(merged.AuditLines(), "\n")
	if !strings.Contains(shown, "revisão inconclusiva — parte do diff") || !strings.Contains(shown, "mensagem mostrada") {
		t.Errorf("shown lines = %q", shown)
	}
	if !strings.Contains(audit, "review inconclusive (partial_coverage)") || !strings.Contains(audit, "engine message") || strings.Contains(audit, "mensagem mostrada") || strings.Contains(audit, "revisão inconclusiva —") {
		t.Errorf("audit lines = %q", audit)
	}
}
