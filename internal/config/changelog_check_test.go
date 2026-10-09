package config

import (
	"strings"
	"testing"
)

// AUR-509: changelog_check parses, refuses an unknown mode and is decided by
// a central policy that declares it.
func TestAUR509ChangelogCheckSection(t *testing.T) {
	cfg, err := Parse([]byte("changelog_check:\n  mode: required\n  section: Unreleased\n"), "t")
	if err != nil || !cfg.ChangelogCheck.Required() {
		t.Fatalf("required mode: cfg=%+v err=%v", cfg, err)
	}
	if _, err := Parse([]byte("changelog_check:\n  mode: talvez\n"), "t"); err == nil || !strings.Contains(err.Error(), "changelog_check.mode") {
		t.Errorf("unknown mode accepted: %v", err)
	}
	if _, err := Parse([]byte("changelog_check:\n  mode: required\n  file: ../fora.md\n"), "t"); err == nil {
		t.Error("file outside the repository accepted")
	}
	var absent *ChangelogCheckConfig
	if absent.Required() {
		t.Error("absent section must be off")
	}
	repo := &Config{ChangelogCheck: &ChangelogCheckConfig{Mode: "off"}}
	central := &Config{ChangelogCheck: &ChangelogCheckConfig{Mode: "required"}}
	effective := &Config{}
	if w := mergeChangelogCheck(effective, repo, central); len(w) != 1 || !effective.ChangelogCheck.Required() {
		t.Errorf("policy must decide alone: warnings=%v effective=%+v", w, effective.ChangelogCheck)
	}
}

// AUR-610: changelog_check.bots only lowers the mode for a bot (absent =
// suggest), never raises it, never touches a person, and an unknown value
// is refused naming the key.
func TestAUR610BotsNeverRaiseTheMode(t *testing.T) {
	cases := []struct {
		mode, bots string
		want       ChangelogMode
	}{
		{"required", "", ChangelogSuggest},
		{"required", "suggest", ChangelogSuggest},
		{"required", "off", ChangelogOff},
		{"required", "required", ChangelogRequired},
		{"suggest", "required", ChangelogSuggest},
		{"off", "required", ChangelogOff},
		{"suggest", "off", ChangelogOff},
		{"obrigatorio", "sugerir", ChangelogSuggest},
	}
	for _, c := range cases {
		cfg := &ChangelogCheckConfig{Mode: c.mode, Bots: c.bots}
		if got := cfg.EffectiveModeFor(true); got != c.want {
			t.Errorf("mode %q bots %q: bot gets %q, want %q", c.mode, c.bots, got, c.want)
		}
		if got := cfg.EffectiveModeFor(false); got != cfg.EffectiveMode() {
			t.Errorf("mode %q bots %q: a person gets %q, want the declared %q", c.mode, c.bots, got, cfg.EffectiveMode())
		}
	}
	var absent *ChangelogCheckConfig
	if absent.EffectiveModeFor(true) != ChangelogOff || absent.EffectiveModeFor(false) != ChangelogOff {
		t.Error("absent section must be off for everyone")
	}
}

// AUR-610: an unknown bots value is refused at parse time and, should it
// reach the mode, lowers nothing.
func TestAUR610UnknownBotsRefused(t *testing.T) {
	_, err := Parse([]byte("changelog_check:\n  mode: required\n  bots: talvez\n"), "t")
	if err == nil || !strings.Contains(err.Error(), "changelog_check.bots") {
		t.Fatalf("unknown bots accepted or unnamed: %v", err)
	}
	cfg, err := Parse([]byte("changelog_check:\n  mode: required\n  bots: off\n"), "t")
	if err != nil || cfg.ChangelogCheck.EffectiveModeFor(true) != ChangelogOff {
		t.Fatalf("bots off: cfg=%+v err=%v", cfg, err)
	}
	if got := (&ChangelogCheckConfig{Mode: "required", Bots: "talvez"}).EffectiveModeFor(true); got != ChangelogRequired {
		t.Fatalf("an unknown bots lowered the mode to %q", got)
	}
}
