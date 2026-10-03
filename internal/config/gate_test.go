package config

import "testing"

// TestAUR519GateConfigThreshold proves the fail_on list normalizes through
// the same severity vocabulary --fail-on uses, picking the lowest rank
// among the declared entries as the gate's threshold (AC-001), and accepts
// the card's own documented "critical" alias.
func TestAUR519GateConfigThreshold(t *testing.T) {
	g := GateConfig{FailOn: []string{"critical", "medium"}}
	rank, name, ok, err := g.Threshold()
	if err != nil {
		t.Fatalf("Threshold() error = %v", err)
	}
	if !ok || rank != GateSeverityWarning || name != "warning" {
		t.Fatalf("Threshold() = rank %v name %q ok %v, want warning/true (the lower of critical,medium)", rank, name, ok)
	}

	if _, _, _, err := (GateConfig{FailOn: []string{"bogus"}}).Threshold(); err == nil {
		t.Fatal("Threshold() with an unknown severity should error")
	}
}

// TestAUR519GateConfigInconclusive proves gate.inconclusive accepts the
// documented block/warn values (and their Portuguese aliases), defaults to
// "" (no opt-in) when absent, and rejects anything else.
func TestAUR519GateConfigInconclusive(t *testing.T) {
	cases := map[string]string{"": "fallback", "block": "block", "bloquear": "block", "warn": "warn", "alertar": "warn"}
	for in, want := range cases {
		got, err := (GateConfig{Inconclusive: in}).resolveInconclusive("fallback")
		if err != nil {
			t.Fatalf("InconclusiveMode(%q) error = %v", in, err)
		}
		if got != want {
			t.Fatalf("InconclusiveMode(%q) = %q, want %q", in, got, want)
		}
	}
	if _, err := (GateConfig{Inconclusive: "maybe"}).InconclusiveMode(); err == nil {
		t.Fatal("InconclusiveMode(\"maybe\") should error")
	}
}

// TestAUR519ApplyCentralPolicyGate proves AC-005's precedence mirror: under
// a central policy, only the policy's own Gate applies, and a repository's
// own Gate is dropped with a named warning instead of being merged with
// or silently overriding the policy's.
func TestAUR519ApplyCentralPolicyGate(t *testing.T) {
	repo := &Config{Gate: GateConfig{FailOn: []string{"info"}, Inconclusive: "warn"}}
	central := &Config{Gate: GateConfig{FailOn: []string{"critical"}, Inconclusive: "block"}}

	effective, warnings := ApplyCentralPolicy(repo, central)
	if len(effective.Gate.FailOn) != 1 || effective.Gate.FailOn[0] != "critical" {
		t.Fatalf("effective.Gate = %+v, want the policy's own gate", effective.Gate)
	}
	found := false
	for _, w := range warnings {
		if w.Reason == "gate do config do repositório foi ignorado: a política central decide sozinha" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a warning naming the dropped repo gate, got %+v", warnings)
	}

	// Without a policy, the repo's own gate is its explicit opt-in (AC-005)
	// and is returned completely unchanged.
	effective, warnings = ApplyCentralPolicy(repo, nil)
	if warnings != nil {
		t.Fatalf("no policy: warnings = %+v, want nil", warnings)
	}
	if len(effective.Gate.FailOn) != 1 || effective.Gate.FailOn[0] != "info" {
		t.Fatalf("no policy: effective.Gate = %+v, want the repo's own gate unchanged", effective.Gate)
	}
}

// TestAUR575DeclaredGateDefaultsToBlock: a gate declared with fail_on and no
// gate.inconclusive blocks; no gate at all resolves to "" (nothing gated).
func TestAUR575DeclaredGateDefaultsToBlock(t *testing.T) {
	if got, err := (GateConfig{FailOn: []string{"error"}}).InconclusiveMode(); err != nil || got != InconclusiveBlock {
		t.Fatalf("declared gate without inconclusive = %q, %v; want block", got, err)
	}
	if got, err := (GateConfig{}).InconclusiveMode(); err != nil || got != "" {
		t.Fatalf("no gate = %q, %v; want empty", got, err)
	}
}

// gate.triage is validated strictly at parse time: a source outside
// gate.sources' vocabulary or a value other than model/none is an error,
// and the default (absent) demotes nothing.
func TestGateTriageParse(t *testing.T) {
	cfg, err := Parse([]byte("gate:\n  fail_on: [high]\n  triage:\n    analysis: model\n    sast: none\n"), "test")
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Gate.TriageByModel(GateSourceAnalysis) || cfg.Gate.TriageByModel(GateSourceSAST) || cfg.Gate.TriageByModel(GateSourceSkills) {
		t.Fatalf("triage = %v", cfg.Gate.Triage)
	}
	for _, bad := range []string{
		"gate:\n  triage:\n    dtrack: model\n",
		"gate:\n  triage:\n    analysis: always\n",
		"gate:\n  triage: model\n",
		"gate:\n  triagem:\n    analysis: model\n",
	} {
		if _, err := Parse([]byte(bad), "test"); err == nil {
			t.Errorf("Parse accepted %q", bad)
		}
	}
	plain, err := Parse([]byte("gate:\n  fail_on: [high]\n"), "test")
	if err != nil {
		t.Fatal(err)
	}
	if plain.Gate.TriageByModel(GateSourceAnalysis) {
		t.Fatal("the default must be none")
	}
}
