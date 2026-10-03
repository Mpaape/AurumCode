package main

import (
	"context"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/config"
	igate "github.com/Mpaape/AurumCode/internal/gate"
	"github.com/Mpaape/AurumCode/internal/scanner"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// TestAUR575SASTMissingWithoutInconclusiveBlocks: quality_gates.sast enabled,
// the scanner could not run, no gate.inconclusive written: the review exits
// non-zero and the gate line says the run was blocked as inconclusive. With
// "inconclusive: warn" written, the run keeps the alert-only exit 0.
func TestAUR575SASTMissingWithoutInconclusiveBlocks(t *testing.T) {
	cleanFixture(t, "quality_gates:\n  sast:\n    enabled: true\n")
	aur572CleanModel(t)
	setSemgrepPATH(t, semgrepFake(t, "boom: semgrep crashed", true, ""))

	var out, errOut strings.Builder
	code := runReview([]string{"--base", "HEAD~1"}, &out, &errOut, redaction.NewFilter())
	if code == 0 {
		t.Fatalf("exit=0 with SAST enabled and inconclusive; stdout=%s stderr=%s", out.String(), errOut.String())
	}
	if !strings.Contains(errOut.String(), "inconclusivo (sast_execution_error)") {
		t.Fatalf("gate line does not name the inconclusive SAST: %s", errOut.String())
	}
}

// TestAUR575SASTMissingPublishesBlockedStatus: the same decision on the --pr
// path publishes the gate status as failure, "inconclusivo (bloqueio)";
// with "warn" written it stays success with the alert.
func TestAUR575SASTMissingPublishesBlockedStatus(t *testing.T) {
	for _, tc := range []struct {
		mode, state, word string
	}{{"", "failure", "revisão inconclusiva (bloqueio)"}, {"warn", "success", "revisão inconclusiva (alerta)"}} {
		cfg := &config.Config{}
		cfg.QualityGates.Sast = &config.SastConfig{Enabled: true}
		cfg.Gate.Inconclusive = tc.mode
		run := &igate.Run{Cfg: cfg, Review: &types.ReviewResult{}}
		var res igate.Result
		engine, _ := scanner.Lookup("semgrep")
		missing := igate.ScannerContributor{Scans: []igate.Scan{{Config: *cfg.QualityGates.Sast.AsScanner(), Engine: engine, Section: gateOriginRepo, Reason: "sast_unavailable"}}}
		if err := igate.NewPipeline(missing).Run(context.Background(), run, &res); err != nil {
			t.Fatal(err)
		}
		got := publishStatusForTest(t, res)
		if got.State != tc.state || !strings.HasPrefix(got.Description, "inconclusivo:") || !strings.Contains(got.Description, tc.word) {
			t.Fatalf("mode %q: status %+v, want %s naming %q", tc.mode, got, tc.state, tc.word)
		}
	}
}

func TestAUR575SASTMissingWithWrittenWarnAlerts(t *testing.T) {
	cleanFixture(t, "gate:\n  inconclusive: warn\nquality_gates:\n  sast:\n    enabled: true\n")
	aur572CleanModel(t)
	setSemgrepPATH(t, semgrepFake(t, "boom: semgrep crashed", true, ""))

	var out, errOut strings.Builder
	if code := runReview([]string{"--base", "HEAD~1"}, &out, &errOut, redaction.NewFilter()); code != 0 {
		t.Fatalf("exit=%d with inconclusive: warn written; stderr=%s", code, errOut.String())
	}
}

// TestAUR575InvalidConfigRefusedBeforeModel: an unsupported SAST engine, a
// flag-like rule pack and a misspelled gate key are configuration errors:
// the review stops at load, names the key, and no review is produced.
func TestAUR575InvalidConfigRefusedBeforeModel(t *testing.T) {
	for key, doc := range map[string]string{
		"quality_gates.sast.engine":     "quality_gates:\n  sast:\n    enabled: true\n    engine: gitleaks\n",
		"quality_gates.sast.rule_packs": "quality_gates:\n  sast:\n    enabled: true\n    rule_packs: [\"--dangerous\"]\n",
		"fial_on":                       "gate:\n  fial_on: [error]\n",
	} {
		cleanFixture(t, doc)
		aur572CleanModel(t)
		var out, errOut strings.Builder
		code := runReview([]string{"--base", "HEAD~1"}, &out, &errOut, redaction.NewFilter())
		// 1 is the product's load-error code, the same a refused central
		// policy already exits with.
		if code != 1 || !strings.Contains(errOut.String(), key) {
			t.Fatalf("%s: exit=%d stderr=%q, want exit 1 and a load error naming the key", key, code, errOut.String())
		}
		if strings.Contains(out.String(), "Code Review Summary") {
			t.Fatalf("%s: a review was produced from an invalid configuration:\n%s", key, out.String())
		}
	}
}
