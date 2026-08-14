// Package integration holds AUR-456's Integration-layer proof: the two
// claims the Unit layer takes on faith (tests/unit/AUR-456.go) actually have
// lastro in the source this repository ships, read directly -- never by
// re-deriving them from the deleted paths themselves, since those paths no
// longer exist to compare against.
//
//  1. .aurumcode/prompts/documentation/welcome-page.md is not merely
//     present (Unit layer): internal/documentation/welcome/generator.go's
//     defaultPromptPath constant names that EXACT path, so the surviving
//     file really is read by the generator, not a coincidentally-named
//     leftover.
//  2. The live rules catalog internal/review/rules/security.yml -- not
//     .aurumcode/rules/security.yml, which this card deletes -- carries the
//     singular id security/hardcoded-secret with a real pattern: key. This
//     is the exact divergence the card's Achados section and MUT-001 name:
//     the dead copy this card removes still spelled the id
//     security/hardcoded-secrets (plural) with no pattern: at all. Checking
//     the live file's fingerprint directly (not merely "the dead file is
//     gone") is what makes MUT-001 fail for being STALE, not just PRESENT.
//
// The CHANGELOG-style false-capability claims in _api/index.md
// (review_pipeline.go/docs_pipeline.go/qa_pipeline.go under
// internal/pipeline/) are not re-verified here: internal/pipeline is
// outside this card's declared read_paths, and _api/ itself is deleted by
// this same card, so by GREEN there is nothing left to cross-check -- the
// absence proof in tests/unit/AUR-456.go already covers it. That inventory
// was taken once, by direct inspection, during this card's build; see
// docs/specs/AUR-456.md.
//
// Scope, same as every sibling card in this office: the sandbox has no
// network and this card never executes `aurumcode` or `regenerate-docs`;
// every check here is a static read of source files already materialized by
// paths/read_paths.
//
// Not named "_test.go" on purpose, same technique as tests/unit/AUR-456.go.
//
// Selector naming note: see tests/unit/AUR-456.go's header. This file uses
// IntegrationAUR456 instead of the card text's IntegrationAUR445, which
// already exists in this package's tests/integration/AUR-445.go and would
// collide.
package integration

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// aur456IntegrationRoot mirrors tests/unit/AUR-456.go's root resolution,
// duplicated rather than imported because these test-support files are not
// wired into a shared internal package (matching every sibling card).
func aur456IntegrationRoot(t *testing.T) string {
	t.Helper()
	if root := os.Getenv("AURUMCODE_ROOT"); root != "" {
		return root
	}
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("AUR-456/AC-001/infrastructure: getwd: %v", err)
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("AUR-456/AC-001/infrastructure: no go.mod above the working directory and AURUMCODE_ROOT is unset")
		}
		dir = parent
	}
}

func aur456ReadFile(t *testing.T, root, rel string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatalf("AUR-456/AC-001/infrastructure: %s unreadable: %v", rel, err)
	}
	return string(raw)
}

// IntegrationAUR456 is AUR-456's Integration-layer selector.
func IntegrationAUR456(t *testing.T) {
	root := aur456IntegrationRoot(t)

	// 1. generator.go really names the surviving override path.
	generator := aur456ReadFile(t, root, filepath.Join("internal", "documentation", "welcome", "generator.go"))
	wantConst := `defaultPromptPath = ".aurumcode/prompts/documentation/welcome-page.md"`
	if !strings.Contains(generator, wantConst) {
		t.Fatalf("AUR-456/AC-001/behavior-missing: internal/documentation/welcome/generator.go no longer declares %s", wantConst)
	}

	// 2. The live catalog carries the singular, patterned id -- the exact
	// divergence from the dead .aurumcode/rules/security.yml copy this card
	// deletes. Both directions matter: the plural, patternless id must be
	// absent, and the singular, patterned id must be present, so a live
	// catalog that regressed to the dead shape is caught here too.
	liveRules := aur456ReadFile(t, root, filepath.Join("internal", "review", "rules", "security.yml"))
	if regexp.MustCompile(`(?m)^\s*-\s*id:\s*security/hardcoded-secrets\s*$`).MatchString(liveRules) {
		t.Fatalf("AUR-456/AC-001/behavior-missing: internal/review/rules/security.yml regressed to the dead plural id security/hardcoded-secrets")
	}
	idPattern := regexp.MustCompile(`(?m)^\s*-\s*id:\s*security/hardcoded-secret\s*$`)
	idLoc := idPattern.FindStringIndex(liveRules)
	if idLoc == nil {
		t.Fatalf("AUR-456/AC-001/behavior-missing: internal/review/rules/security.yml does not declare the live singular id security/hardcoded-secret")
	}
	// A pattern: key must appear in the same rule block: between this id
	// and either the next "- id:" line or end of file.
	rest := liveRules[idLoc[1]:]
	if nextID := regexp.MustCompile(`(?m)^\s*-\s*id:`).FindStringIndex(rest); nextID != nil {
		rest = rest[:nextID[0]]
	}
	if !regexp.MustCompile(`(?m)^\s*pattern:\s*\S`).MatchString(rest) {
		t.Fatalf("AUR-456/AC-001/behavior-missing: internal/review/rules/security.yml's security/hardcoded-secret rule carries no pattern: key")
	}

	t.Logf("AUR-456/AC-001/integration pass welcome_override=bound live_rule=security/hardcoded-secret")
}
