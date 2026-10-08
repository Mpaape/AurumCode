package config

import (
	"strings"
	"testing"
)

// The dependencies section parses, defaults and refuses what would weaken
// the check.
func TestDependenciesSectionParse(t *testing.T) {
	cfg, err := Parse([]byte("dependencies:\n  fail_on: [critical, high]\n  preexisting: block\n  licenses_denied: [AGPL-3.0-only]\n"), "")
	if err != nil {
		t.Fatal(err)
	}
	d := cfg.Dependencies
	if !d.Declared() || d.EffectivePreexisting() != PreexistingBlock || d.EffectiveOSVURL() != DefaultOSVURL {
		t.Fatalf("section = %+v", d)
	}
	if !d.Fails("CRITICAL") || !d.Fails("high") || d.Fails("moderate") || !d.Fails("") {
		t.Fatalf("fail_on [critical, high] read wrong: %+v", d.FailOn)
	}
	high := &DependenciesConfig{FailOn: []string{"high"}}
	if !high.Fails("critical") || high.Fails("medium") {
		t.Fatal("fail_on [high] must fail a critical advisory and alert a medium one")
	}
	if (&DependenciesConfig{FailOn: []string{"critical"}}).Fails("high") {
		t.Fatal("fail_on [critical] must not fail a high advisory")
	}
	if mode, _ := cfg.InconclusiveMode(); mode != InconclusiveBlock {
		t.Fatalf("declared section inconclusive mode = %q, want block", mode)
	}
	for _, bad := range []string{
		"dependencies:\n  fail_on: [severe]\n",
		"dependencies:\n  preexisting: ignore\n",
		"dependencies:\n  osv_url: ftp://mirror\n",
		"dependencies:\n  licenses_denied: [\"MIT OR AGPL-3.0-only\"]\n",
		"dependencies:\n  max_source_age_hours: 0\n",
		"dependencies:\n  fial_on: [high]\n",
	} {
		if _, err := Parse([]byte(bad), ""); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
	empty, err := Parse([]byte("gate:\n  fail_on: [high]\n"), "")
	if err != nil || empty.Dependencies.Declared() {
		t.Fatalf("absent section must stay undeclared: %+v %v", empty.Dependencies, err)
	}
}

// A policy that declares dependencies decides alone.
func TestDependenciesGovernedByPolicy(t *testing.T) {
	repo := &Config{Dependencies: &DependenciesConfig{Preexisting: PreexistingWarn}}
	central := &Config{Dependencies: &DependenciesConfig{Preexisting: PreexistingBlock}}
	effective := &Config{}
	warnings := governDependencies(effective, repo, central)
	if effective.Dependencies != central.Dependencies || len(warnings) != 1 || !strings.Contains(warnings[0].Reason, "dependencies") {
		t.Fatalf("effective = %+v warnings = %v", effective.Dependencies, warnings)
	}
}

// The advisory and registry sources must be https (http only on loopback),
// and the scanner a command name from PATH or an absolute path: a relative
// path would resolve inside the checkout the pull request controls.
func TestDependenciesSourcesAndScannerValidated(t *testing.T) {
	for _, bad := range []string{
		"dependencies:\n  osv_url: http://mirror.example\n",
		"dependencies:\n  deps_dev_url: http://10.0.0.5/\n",
		"dependencies:\n  scanner: ./bin/osv-scanner\n",
		"dependencies:\n  scanner: tools/osv-scanner\n",
		"dependencies:\n  scanner: ..\n",
	} {
		if _, err := Parse([]byte(bad), ""); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
	for _, good := range []string{
		"dependencies:\n  osv_url: https://osv.mirror.example\n",
		"dependencies:\n  osv_url: http://127.0.0.1:8080\n",
		"dependencies:\n  deps_dev_url: http://localhost:9000\n",
		"dependencies:\n  scanner: osv-scanner\n",
		"dependencies:\n  scanner: /usr/local/bin/osv-scanner\n",
	} {
		if _, err := Parse([]byte(good), ""); err != nil {
			t.Errorf("refused %q: %v", good, err)
		}
	}
}

// A declared fail_on with an unreadable level fails closed even beside a
// valid one: doubt never lets a finding pass (Validate refuses the config;
// this guards a value that bypassed it).
func TestDependenciesFailsClosedOnUnreadableLevel(t *testing.T) {
	mixed := &DependenciesConfig{FailOn: []string{"critical", "hgih"}}
	if !mixed.Fails("low") {
		t.Fatal("an unreadable fail_on level let a low advisory pass")
	}
	only := &DependenciesConfig{FailOn: []string{"???"}}
	if !only.Fails("medium") {
		t.Fatal("a fail_on with no readable level let an advisory pass")
	}
}
