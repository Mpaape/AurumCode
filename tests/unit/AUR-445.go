// Package unit holds AUR-445's Unit-layer proof: none of the five families
// of false statement the 2026-08-13 audit found
// (.board/cards/ready/AUR-445.md, "Achados medidos") still appear in the six
// root documentation files this card owns, and the true replacement text is
// present where the audit demanded it.
//
// The five families, each already false before this card's fix:
//   - "No LICENSE file is present" (README.md, ACTION_USAGE.md, CHANGELOG.md)
//   - "No code-review or test-generation output" (CHANGELOG.md)
//   - "Every knob is an environment variable" as an unqualified,
//     whole-repository claim (CHANGELOG.md, docs/getting-started.md)
//   - `gomarkdoc` declared required for Go (README.md, SETUP_GUIDE.md,
//     ACTION_USAGE.md, docs/getting-started.md, RUN_DOCS_PIPELINE.md)
//   - "the only command in this repository" (RUN_DOCS_PIPELINE.md)
//
// This layer is pure text: it normalizes each file (strip backticks,
// lowercase, collapse all whitespace including newlines to single spaces)
// and substring-matches the same-normalized banned phrase and the
// same-normalized required anchor. It does not read any other package's
// source; that cross-check is the Integration layer
// (tests/integration/AUR-445.go).
//
// Not named "_test.go" on purpose, mirroring every sibling card in this
// office (AUR-402..AUR-411, AUR-422, AUR-424, AUR-428, AUR-440):
// tests/acceptance/AUR-445.sh stages a private writable copy of the module
// and writes a tiny bridge "_test.go" file that calls TestAUR445, so these
// assertions run inside the sandboxed acceptance instead of being swept into
// an unrelated top-level `go test ./...`.
//
// Selector naming note: the card's own "TDD proof" section names the
// selectors TestAUR428 / IntegrationAUR428 / E2EAUR428 -- residue from the
// AUR-428 template this card was cloned from. Those symbols already exist in
// tests/unit/AUR-428.go and tests/integration/AUR-428.go, in these same
// packages; redeclaring them here would collide and break `go build ./...`.
// This file uses TestAUR445 instead, matching this card's own ID and the
// naming pattern every other card in the office follows. Recorded in
// docs/specs/AUR-445.md.
package unit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// aur445RepoRoot resolves the repository root: the acceptance script exports
// AURUMCODE_ROOT pointing at its staged copy; a developer running the bridge
// manually gets a walk-up from the working directory to the nearest go.mod.
func aur445RepoRoot(t *testing.T) string {
	t.Helper()
	if root := os.Getenv("AURUMCODE_ROOT"); root != "" {
		return root
	}
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("AUR-445/AC-001/infrastructure: getwd: %v", err)
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("AUR-445/AC-001/infrastructure: no go.mod above the working directory and AURUMCODE_ROOT is unset")
		}
		dir = parent
	}
}

// aur445Normalize is the one normalization every layer of this card uses so
// a banned phrase cannot survive by being re-wrapped across a markdown line
// break or re-spelled with different backtick placement. It strips
// backticks, lowercases (ASCII, sufficient for the English/Portuguese ASCII
// prose in these files), and collapses every run of whitespace -- including
// newlines -- to a single space.
func aur445Normalize(s string) string {
	s = strings.ReplaceAll(s, "`", "")
	s = strings.ToLower(s)
	fields := strings.Fields(s)
	return strings.Join(fields, " ")
}

// aur445Docs is the exact, closed set of root documentation files this card
// owns. It is enumerated literally -- never discovered by glob -- because
// docs/specs/AUR-445.md and tests/acceptance/AUR-445.sh legitimately quote
// the banned phrases (to document what was fixed and to drive MUT-001), and
// a glob or repo-wide scan would fail on this card's own artifacts.
var aur445Docs = []string{
	"README.md",
	"CHANGELOG.md",
	"ACTION_USAGE.md",
	"SETUP_GUIDE.md",
	"RUN_DOCS_PIPELINE.md",
	filepath.Join("docs", "getting-started.md"),
}

// aur445BannedPhrase names one of the five audited false-statement families
// and its normalized text (see aur445Normalize). files is the subset of
// aur445Docs the audit named for that family; empty means "check all six".
type aur445BannedPhrase struct {
	label      string
	normalized string
	files      []string
}

var aur445Banned = []aur445BannedPhrase{
	{
		label:      "no-license-file",
		normalized: aur445Normalize("No `LICENSE` file is present"),
		files:      []string{"README.md", "ACTION_USAGE.md", "CHANGELOG.md"},
	},
	{
		label:      "no-code-review-output",
		normalized: aur445Normalize("No code-review or test-generation output"),
		files:      []string{"CHANGELOG.md"},
	},
	{
		label:      "every-knob-is-env-var",
		normalized: aur445Normalize("Every knob is an environment variable"),
		files:      []string{"CHANGELOG.md", filepath.Join("docs", "getting-started.md")},
	},
	{
		label:      "gomarkdoc-mentioned",
		normalized: "gomarkdoc",
		files:      nil, // all six: Go needs it nowhere in the root docs, since AUR-424
	},
	{
		label:      "only-command-in-repository",
		normalized: aur445Normalize("the only command in this repository"),
		files:      []string{"RUN_DOCS_PIPELINE.md"},
	},
}

// aur445RequiredAnchor names a true statement that must survive, normalized,
// in at least one of the listed files -- the positive half of the proof:
// fixing a lie is not the same as saying nothing.
type aur445RequiredAnchor struct {
	label      string
	normalized string
	files      []string
}

var aur445Required = []aur445RequiredAnchor{
	{
		label:      "license-is-mit",
		normalized: aur445Normalize("[MIT](LICENSE)"),
		files:      []string{"README.md", "ACTION_USAGE.md", "CHANGELOG.md"},
	},
	{
		label:      "review-command-documented",
		normalized: aur445Normalize("aurumcode review"),
		files:      []string{"CHANGELOG.md"},
	},
	{
		label:      "go-stdlib-mechanism-documented",
		normalized: aur445Normalize("go/parser"),
		files:      []string{"README.md", "ACTION_USAGE.md", "SETUP_GUIDE.md", "RUN_DOCS_PIPELINE.md", filepath.Join("docs", "getting-started.md")},
	},
	{
		label:      "second-binary-documented",
		normalized: aur445Normalize("cmd/aurumcode"),
		files:      []string{"RUN_DOCS_PIPELINE.md"},
	},
	{
		label:      "security-pass-scope-honest",
		normalized: aur445Normalize("2 of the 8 rules"),
		files:      []string{"CHANGELOG.md"},
	},
}

// TestAUR445 is AUR-445's Unit-layer selector.
//
// Before the fix this failed with AUR-445/AC-001/behavior-missing for every
// banned family still present in its named files: the audit's whole point
// was that these files lied. A MUT-001 run that reintroduces "No LICENSE
// file is present" into README.md, ACTION_USAGE.md or CHANGELOG.md fails
// here with the AUR-445/AC-001/MUT-001 label.
func TestAUR445(t *testing.T) {
	root := aur445RepoRoot(t)

	normalizedContent := make(map[string]string, len(aur445Docs))
	for _, rel := range aur445Docs {
		raw, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatalf("AUR-445/AC-001/behavior-missing: %s unreadable: %v", rel, err)
		}
		if len(raw) == 0 {
			t.Fatalf("AUR-445/AC-001/behavior-missing: %s is empty", rel)
		}
		normalizedContent[rel] = aur445Normalize(string(raw))
	}

	for _, banned := range aur445Banned {
		targets := banned.files
		if targets == nil {
			targets = aur445Docs
		}
		for _, rel := range targets {
			content, ok := normalizedContent[rel]
			if !ok {
				t.Fatalf("AUR-445/AC-001/infrastructure: %s not loaded", rel)
			}
			if strings.Contains(content, banned.normalized) {
				// The banned LICENSE-absent phrase reads identically whether
				// it is this card's own pre-fix RED or a deliberately
				// reintroduced MUT-001 mutation -- content alone cannot
				// distinguish them. Only the run can: the acceptance's
				// MUT-001 case exports AUR445_MUTATION=MUT-001 when, and
				// only when, it is the one that injected the sentence. Any
				// other run -- including the untouched pre-fix tree -- must
				// read as behavior-missing, never as a false MUT-001 report.
				label := "behavior-missing"
				if banned.label == "no-license-file" && os.Getenv("AUR445_MUTATION") == "MUT-001" {
					label = "MUT-001"
				}
				t.Fatalf("AUR-445/AC-001/%s: %s still contains the banned %q family (%s)", label, rel, banned.normalized, banned.label)
			}
		}
	}

	for _, req := range aur445Required {
		found := false
		for _, rel := range req.files {
			content, ok := normalizedContent[rel]
			if !ok {
				t.Fatalf("AUR-445/AC-001/infrastructure: %s not loaded", rel)
			}
			if strings.Contains(content, req.normalized) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("AUR-445/AC-001/behavior-missing: none of %v carries the required %q anchor (%s)", req.files, req.normalized, req.label)
		}
	}

	t.Logf("AUR-445/AC-001/unit pass files=%d banned_families=%d required_anchors=%d", len(aur445Docs), len(aur445Banned), len(aur445Required))
}
