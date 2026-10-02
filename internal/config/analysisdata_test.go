package config

import (
	"strings"
	"testing"
)

func TestAUR533AnalysisDataSectionParsesAndDefaults(t *testing.T) {
	cfg, err := Parse([]byte("language: pt\n"), "")
	if err != nil || cfg.AnalysisData.Declared() {
		t.Fatalf("undeclared section must stay nil: %+v %v", cfg.AnalysisData, err)
	}
	cfg, err = Parse([]byte("analysis_data: {}\n"), "")
	if err != nil || !cfg.AnalysisData.Declared() || cfg.AnalysisData.EffectiveMaxAgeDays() != 7 || cfg.AnalysisData.EffectiveRepository() != DefaultAnalysisDataRepository {
		t.Fatalf("declared-empty must default: %+v %v", cfg.AnalysisData, err)
	}
	cfg, err = Parse([]byte("analysis_data:\n  max_age_days: 30\n  repository: o/r\n"), "")
	if err != nil || cfg.AnalysisData.EffectiveMaxAgeDays() != 30 || cfg.AnalysisData.EffectiveRepository() != "o/r" {
		t.Fatalf("explicit values: %+v %v", cfg.AnalysisData, err)
	}
	for _, bad := range []string{"max_age_days: -1", "max_age_days: 9999", "repository: x"} {
		if _, err := Parse([]byte("analysis_data:\n  "+bad+"\n"), ""); err == nil {
			t.Fatalf("accepted %q", bad)
		}
	}
}

func TestAUR533AnalysisDataCentralPolicyPrecedence(t *testing.T) {
	repo, _ := Parse([]byte("analysis_data:\n  max_age_days: 300\n"), "")
	silent, _ := Parse([]byte("language: pt\n"), "")
	eff, warns := ApplyCentralPolicy(repo, silent)
	if eff.AnalysisData == nil || eff.AnalysisData.MaxAgeDays != 300 || len(warns) != 0 {
		t.Fatalf("silent policy must keep the repo section: %+v %v", eff.AnalysisData, warns)
	}
	strict, err := Parse([]byte("analysis_data:\n  max_age_days: 3\n"), "")
	if err != nil {
		t.Fatal(err)
	}
	eff, warns = ApplyCentralPolicy(repo, strict)
	if eff.AnalysisData.MaxAgeDays != 3 {
		t.Fatalf("policy must win: %+v", eff.AnalysisData)
	}
	found := false
	for _, w := range warns {
		found = found || strings.Contains(w.Reason, "analysis_data")
	}
	if !found {
		t.Fatalf("overridden repo section must warn: %v", warns)
	}
	// No section anywhere: nothing declared.
	eff, _ = ApplyCentralPolicy(silent, silent)
	if eff.AnalysisData != nil {
		t.Fatal("nothing declared must stay nil")
	}
}

func TestAUR533CentralPolicyFileAcceptsAnalysisData(t *testing.T) {
	dir := t.TempDir()
	writePolicyFile(t, dir, DefaultConfigPath, "analysis_data:\n  max_age_days: 2\n")
	cfg, err := LoadCentralPolicy(dir)
	if err != nil || cfg.AnalysisData == nil || cfg.AnalysisData.MaxAgeDays != 2 {
		t.Fatalf("strict policy decode must accept analysis_data: %+v %v", cfg, err)
	}
}
