package config

import (
	"strings"
	"testing"
	"time"
)

func TestAUR580DeliberationParsesWithDefaultsAndRefusesBadValues(t *testing.T) {
	cfg, err := Parse([]byte("deliberation:\n  enabled: true\n  max_rounds: 2\n"), "c.yml")
	if err != nil {
		t.Fatal(err)
	}
	rounds, cost, perTool := cfg.Deliberation.EffectiveLimits()
	if !cfg.Deliberation.Active() || rounds != 2 || cost != 0 || perTool != DefaultDeliberationToolTimeoutSecond*time.Second {
		t.Fatalf("limits = %d %d %s", rounds, cost, perTool)
	}
	for body, want := range map[string]string{
		"deliberation:\n  max_rounds: -1\n": "deliberation.max_rounds",
		"deliberation:\n  max_turns: 4\n":   "max_turns",
		"deliberation:\n  enabled: maybe\n": "maybe",
	} {
		if _, err := Parse([]byte(body), "c.yml"); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: err = %v, want it to name %s", body, err, want)
		}
	}
	var absent *DeliberationConfig
	if absent.Active() {
		t.Fatal("an absent section must not offer tools")
	}
}

func TestAUR580PolicyDeliberationDecidesAlone(t *testing.T) {
	repo, _ := Parse([]byte("deliberation:\n  enabled: true\n  max_rounds: 9\n"), "repo.yml")
	policy, _ := Parse([]byte("deliberation:\n  enabled: true\n  max_rounds: 2\n"), "policy.yml")
	eff, warnings := ApplyCentralPolicy(repo, policy)
	if eff.Deliberation.MaxRounds != 2 {
		t.Fatalf("max_rounds = %d, want the policy's", eff.Deliberation.MaxRounds)
	}
	found := false
	for _, w := range warnings {
		found = found || strings.Contains(w.Reason, "deliberation do config do repositório foi ignorado")
	}
	if !found {
		t.Fatalf("no named warning: %+v", warnings)
	}
	silent, _ := Parse([]byte("gate:\n  fail_on: [error]\n"), "policy.yml")
	if eff, _ := ApplyCentralPolicy(repo, silent); eff.Deliberation.MaxRounds != 9 {
		t.Fatal("a policy silent on deliberation must leave the repository's")
	}
}

// max_cost_tokens is optional: an absent section, an absent key and an
// explicit 0 all mean no ceiling. A positive value is an explicit ceiling,
// handed on as written; a negative one is refused by name.
func TestMaxCostTokensIsOptionalAndZeroMeansNoCeiling(t *testing.T) {
	var absent *DeliberationConfig
	if rounds, cost, perTool := absent.EffectiveLimits(); cost != 0 || rounds != DefaultDeliberationMaxRounds || perTool != DefaultDeliberationToolTimeoutSecond*time.Second {
		t.Fatalf("absent section: limits = %d %d %s, want the guard defaults and no cost ceiling", rounds, cost, perTool)
	}
	for name, body := range map[string]string{
		"key absent":    "deliberation:\n  enabled: true\n",
		"explicit zero": "deliberation:\n  enabled: true\n  max_cost_tokens: 0\n",
	} {
		cfg, err := Parse([]byte(body), "c.yml")
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if _, cost, _ := cfg.Deliberation.EffectiveLimits(); cost != 0 {
			t.Errorf("%s: cost = %d, want 0 (no ceiling)", name, cost)
		}
	}
	cfg, err := Parse([]byte("deliberation:\n  enabled: true\n  max_cost_tokens: 12345\n"), "c.yml")
	if err != nil {
		t.Fatal(err)
	}
	if _, cost, _ := cfg.Deliberation.EffectiveLimits(); cost != 12345 {
		t.Fatalf("cost = %d, want the explicit ceiling 12345", cost)
	}
	if _, err := Parse([]byte("deliberation:\n  max_cost_tokens: -1\n"), "c.yml"); err == nil || !strings.Contains(err.Error(), "deliberation.max_cost_tokens") {
		t.Fatalf("err = %v, want a negative max_cost_tokens refused by name", err)
	}
}
