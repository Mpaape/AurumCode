package integration

// Integration program for card AUR-502, selector IntegrationAUR502.
//
// Exercises the config -> team-file -> multi-profile seam on a real
// filesystem: .aurumcode/config.yml lists review.profiles, .aurumcode/
// profiles.yml declares team profiles, and the two are read and resolved into
// a multi-agent review whose profiles declare themselves and whose findings
// merge with attribution. A malformed config list fails closed.

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/reviewprofile"
)

func IntegrationAUR502(t *testing.T) {
	t.Run("ConfigProfilesAndTeamFileResolve", testAUR502ConfigAndTeam)
	t.Run("EmptyConfigEntryFailsClosed", testAUR502EmptyConfigEntry)
	t.Run("DuplicateConfigEntryFailsClosed", testAUR502DuplicateConfigEntry)
	t.Run("UnknownConfiguredProfileFailsClosed", testAUR502UnknownConfigured)
}

func writeRepo(t *testing.T, configDoc, teamDoc string) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, ".aurumcode")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if configDoc != "" {
		if err := os.WriteFile(filepath.Join(dir, "config.yml"), []byte(configDoc), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if teamDoc != "" {
		if err := os.WriteFile(filepath.Join(dir, "profiles.yml"), []byte(teamDoc), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func testAUR502ConfigAndTeam(t *testing.T) {
	team := "profiles:\n  - name: release\n    emphasis: upstream compatibility\n    families: [quality]\n    instructions: Confira o contrato.\n"
	root := writeRepo(t, "review:\n  profiles: [solid, release]\n", team)
	cfg, err := config.Load(root)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	names, err := cfg.ReviewProfiles()
	if err != nil {
		t.Fatalf("ReviewProfiles: %v", err)
	}
	loadedTeam, err := reviewprofile.LoadTeamFile(root)
	if err != nil {
		t.Fatalf("LoadTeamFile: %v", err)
	}
	res, err := reviewprofile.ResolveAll(reviewprofile.Selection{Names: names}, loadedTeam)
	if err != nil {
		t.Fatalf("ResolveAll: %v", err)
	}
	if !res.Applied || len(res.Profiles) != 2 || !strings.Contains(res.Declared, "release") {
		t.Fatalf("resolved = %+v", res)
	}
	merged := reviewprofile.MergeFindings([]reviewprofile.Finding{
		{Profile: "solid", RuleID: "q", File: "a", Line: 1, Message: "m"},
		{Profile: "release", RuleID: "q", File: "a", Line: 1, Message: "m"},
		{Profile: "release", RuleID: "p", File: "b", Line: 2, Message: "n"},
	})
	if len(merged) != 2 || merged[0].Profile != "solid" || merged[1].Profile != "release" {
		t.Fatalf("merged = %+v", merged)
	}
}

func testAUR502EmptyConfigEntry(t *testing.T) {
	root := writeRepo(t, "review:\n  profiles: [\"solid\", \"\"]\n", "")
	if _, err := config.Load(root); err == nil {
		t.Fatal("empty review.profiles entry was accepted")
	}
}

func testAUR502DuplicateConfigEntry(t *testing.T) {
	root := writeRepo(t, "review:\n  profiles: [solid, SOLID]\n", "")
	if _, err := config.Load(root); err == nil {
		t.Fatal("duplicate review.profiles entry was accepted")
	}
}

func testAUR502UnknownConfigured(t *testing.T) {
	root := writeRepo(t, "review:\n  profiles: [desconhecido]\n", "")
	cfg, err := config.Load(root)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	names, _ := cfg.ReviewProfiles()
	team, err := reviewprofile.LoadTeamFile(root)
	if err != nil {
		t.Fatalf("LoadTeamFile: %v", err)
	}
	_, err = reviewprofile.ResolveAll(reviewprofile.Selection{Names: names}, team)
	if !errors.Is(err, reviewprofile.ErrUnknownProfile) || !strings.Contains(err.Error(), "desconhecido") {
		t.Fatalf("unknown configured profile error = %v", err)
	}
}
