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
