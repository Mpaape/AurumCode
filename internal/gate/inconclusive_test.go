package gate

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/scanner"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// sastOnlyRun is a configuration with quality_gates.sast enabled, no gate
// section and, unless mode is written, no gate.inconclusive.
func sastOnlyRun(mode string) *Run {
	cfg := &config.Config{}
	cfg.QualityGates.Sast = &config.SastConfig{Enabled: true}
	cfg.Gate.Inconclusive = mode
	return &Run{Cfg: cfg, Review: &types.ReviewResult{}}
}

// TestAUR575SASTMissingBlocksWithoutInconclusive: the scanner is enabled,
// its binary is missing, gate.inconclusive is absent: the gate fails. Only
// a written "warn" keeps the alert-only behavior.
func TestAUR575SASTMissingBlocksWithoutInconclusive(t *testing.T) {
	missing := ScannerContributor{Scans: []Scan{semgrepScan(t, OriginRepo, "sast_unavailable")}}
	for _, tc := range []struct {
		mode     string
		wantFail bool
	}{{"", true}, {"block", true}, {"warn", false}} {
		var res Result
		if err := NewPipeline(missing).Run(context.Background(), sastOnlyRun(tc.mode), &res); err != nil {
			t.Fatal(err)
		}
		if !res.Active || !res.Inconclusive || res.Fail != tc.wantFail {
			t.Fatalf("mode %q: got %+v, want Active Inconclusive Fail=%v", tc.mode, res, tc.wantFail)
		}
	}
}

// TestAUR575OneRuleForEveryInconclusiveSource: a contributor that returns a
// plain error ends exactly like the missing SAST binary, because the mode is
// applied in one place of the pipeline, not by each source.
func TestAUR575OneRuleForEveryInconclusiveSource(t *testing.T) {
	boom := stubContributor{name: "boom", fn: func(*Run, *Result) error { return errors.New("exploded") }}
	missing := ScannerContributor{Scans: []Scan{semgrepScan(t, OriginRepo, "sast_unavailable")}}
	for _, mode := range []string{"", "block", "warn"} {
		var viaError, viaSAST Result
		if err := NewPipeline(boom).Run(context.Background(), sastOnlyRun(mode), &viaError); err != nil {
			t.Fatal(err)
		}
		if err := NewPipeline(missing).Run(context.Background(), sastOnlyRun(mode), &viaSAST); err != nil {
			t.Fatal(err)
		}
		if viaError.Fail != viaSAST.Fail || viaError.Inconclusive != viaSAST.Inconclusive || viaError.Active != viaSAST.Active {
			t.Fatalf("mode %q: contributor error %+v and missing SAST %+v must decide the same", mode, viaError, viaSAST)
		}
	}
}

// TestAUR575UnknownSeverityCountsAsError: a deterministic finding whose
// severity the normalizer does not recognize counts as error, in the SAST
// source and in the shared deterministic loop (analysis, security pass).
func TestAUR575UnknownSeverityCountsAsError(t *testing.T) {
	odd := types.ReviewIssue{RuleID: "analysis/odd", File: "a.go", Line: 1, Severity: "CRITICAL!", Message: "m"}

	var sast Result
	if err := ApplyScannerGate(&sast, semgrepScan(t, OriginRepo, ""), []types.ReviewIssue{odd}); err != nil {
		t.Fatal(err)
	}
	if !sast.Fail || !sast.Breach || len(sast.BlockingFindings) != 1 {
		t.Fatalf("SAST: unknown severity must count as error, got %+v", sast)
	}

	gate := config.GateConfig{FailOn: []string{"error"}}
	var analysis Result
	if err := ApplyAnalysisGate(&analysis, gate, []types.ReviewIssue{odd}, nil, "", time.Now()); err != nil {
		t.Fatal(err)
	}
	if !analysis.Fail || !analysis.Breach || len(analysis.BlockingFindings) != 1 {
		t.Fatalf("analysis: unknown severity must count as error, got %+v", analysis)
	}

	var security Result
	if err := ApplySecurityGate(&security, gate, []types.ReviewIssue{odd}, nil, "", time.Now()); err != nil {
		t.Fatal(err)
	}
	if !security.Fail || len(security.BlockingFindings) != 1 {
		t.Fatalf("security pass: unknown severity must count as error, got %+v", security)
	}
}

// semgrepScan is the semgrep engine's scan as quality_gates.sast declares it.
func semgrepScan(t *testing.T, section, reason string) Scan {
	t.Helper()
	engine, ok := scanner.Lookup("semgrep")
	if !ok {
		t.Fatal("semgrep engine not registered")
	}
	return Scan{Config: *(&config.SastConfig{Enabled: true}).AsScanner(), Engine: engine, Section: section, Reason: reason}
}
