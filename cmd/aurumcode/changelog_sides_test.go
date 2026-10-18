package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/changelog"
	"github.com/Mpaape/AurumCode/internal/config"
)

// A config.yml that still declares the old size keys keeps loading, and the
// size of an entry never fails the check.
func TestChangelogOldSizeKeysLoadAndNeverFail(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	yml := "changelog_check:\n  mode: required\n  max_entry_lines: 1\n  max_line_length: 5\n  max_release_lines: 1\n"
	if err := os.WriteFile(path, []byte(yml), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.LoadPath(path)
	if err != nil {
		t.Fatalf("config with the old size keys refused: %v", err)
	}
	req, err := changelogRequirement(cfg.ChangelogCheck)
	if err != nil {
		t.Fatalf("requirement: %v", err)
	}
	old := "# Changelog\n\n## Unreleased\n\n"
	entry := "- " + strings.Repeat("mudança visível ao usuário ", 40) + "\n- Segunda linha com mudança visível.\n"
	if got := req.Verify(changelog.Change{State: changelog.FileChanged, Old: old, New: old + entry}); !got.OK {
		t.Fatalf("long entry under old size keys: verdict %+v", got)
	}
}
