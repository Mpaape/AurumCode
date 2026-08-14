// Package unit holds AUR-448's Unit-layer proof, at the internal/review
// package API level: GenerateReview names how many findings the rule gate
// (AUR-434) discarded and why -- a missing rule_id, an unknown one, or both
// -- in result.Metadata["discard_warning"], and that string is empty
// exactly when nothing was discarded (the byte-identical happy path
// cmd/aurumcode's stderr guard depends on).
//
// This card's TDD proof section names the unit selector TestAUR435; that
// collides with the function tests/unit/AUR-435.go already declares in
// this same package (a different card). See docs/specs/AUR-448.md's "A
// note on this card's own TDD-proof identifiers" for why this file
// declares TestAUR448 instead, following every sibling card's own
// numeral-matches-file convention (TestAUR434, TestAUR443, ...).
//
// This file is not named "_test.go" on purpose, mirroring every sibling
// card in this office (see tests/unit/AUR-434.go's own note):
// tests/acceptance/AUR-448.sh stages a private writable copy of the module
// and writes a tiny bridge "_test.go" file that calls TestAUR448, so the
// assertions below run inside the sandboxed acceptance instead of being
// swept into an unrelated top-level `go test ./...`.
package unit

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/analyzer"
	"github.com/Mpaape/AurumCode/internal/llm"
	"github.com/Mpaape/AurumCode/internal/review"
)

// aur448Root resolves the repository root. The acceptance harness sets
// AURUMCODE_ROOT to the staged materialization root (see
// tests/acceptance/AUR-448.sh); running the bridge directly from a full
// checkout works too, climbing two directories from tests/unit back to the
// repository root.
func aur448Root() string {
	if r := os.Getenv("AURUMCODE_ROOT"); r != "" {
		return r
	}
	return filepath.Join("..", "..")
}

// aur448AllGroundedResponse cites a real catalog rule for its only finding
// -- the happy path this card must leave byte-identical: zero discards.
const aur448AllGroundedResponse = `{
  "issues": [
    {
      "file": "config/demo-tokens.txt",
      "line": 3,
      "severity": "error",
      "rule_id": "security/hardcoded-secret",
      "message": "A credential-shaped value was committed in plain text."
    }
  ],
  "summary": "All-grounded fixture for AUR-448."
}`

// aur448MixedResponse mixes one grounded finding with one that cites no
// rule_id and one that cites a rule_id the embedded catalog does not
// recognize -- the same shape AUR-434's own fixtures use, reused here
// because this card's outcome is layered strictly on top of AUR-434's
// gate, never a replacement for it.
const aur448MixedResponse = `{
  "issues": [
    {
      "file": "config/demo-tokens.txt",
      "line": 3,
      "severity": "error",
      "rule_id": "security/hardcoded-secret",
      "message": "grounded"
    },
    {
      "file": "config/demo-tokens.txt",
      "line": 4,
      "severity": "error",
      "message": "no rule_id at all"
    },
    {
      "file": "config/demo-tokens.txt",
      "line": 5,
      "severity": "warning",
      "rule_id": "security/definitely-not-a-rule",
      "message": "unknown rule_id"
    }
  ],
  "summary": "Mixed fixture for AUR-448."
}`

// aur448MissingOnlyResponse discards for exactly one reason (no rule_id at
// all), so the rendered warning must not mention "unknown rule_id".
const aur448MissingOnlyResponse = `{
  "issues": [
    {
      "file": "config/demo-tokens.txt",
      "line": 4,
      "severity": "error",
      "message": "no rule_id at all"
    }
  ],
  "summary": "Missing-only fixture for AUR-448."
}`

// aur448UnknownOnlyResponse discards for exactly the other reason (an
// unrecognized rule_id), so the rendered warning must not mention "no
// rule_id".
const aur448UnknownOnlyResponse = `{
  "issues": [
    {
      "file": "config/demo-tokens.txt",
      "line": 5,
      "severity": "warning",
      "rule_id": "security/definitely-not-a-rule",
      "message": "unknown rule_id"
    }
  ],
  "summary": "Unknown-only fixture for AUR-448."
}`

func aur448Diff(t *testing.T) *analyzer.Repo {
	t.Helper()
	repoPath := filepath.Join(aur448Root(), "tests/fixtures/repos/git-demo/repo.git")
	repo, err := analyzer.OpenRepo(repoPath)
	if err != nil {
		t.Fatalf("OpenRepo(%s): %v", repoPath, err)
	}
	return repo
}

func aur448Generate(t *testing.T, response string) *review.Reviewer {
	t.Helper()
	orch := llm.NewOrchestrator(&review.FakeProvider{Response: response}, nil, nil)
	return review.NewReviewer(orch, review.DefaultConfig())
}

// TestAUR448 proves, at the internal/review package API level, both halves
// of this card's outcome: the discard-warning metadata is empty exactly
// when nothing was discarded, and non-empty with the right count and
// reason(s) when something was.
func TestAUR448(t *testing.T) {
	t.Run("no discard: discard_warning is empty", func(t *testing.T) {
		repo := aur448Diff(t)
		diff, _, err := repo.Diff("HEAD~1", "HEAD")
		if err != nil {
			t.Fatalf("Diff: %v", err)
		}
		result, err := aur448Generate(t, aur448AllGroundedResponse).GenerateReview(context.Background(), diff)
		if err != nil {
			t.Fatalf("GenerateReview: %v", err)
		}
		if len(result.Issues) != 1 {
			t.Fatalf("expected 1 surviving issue, got %d: %+v", len(result.Issues), result.Issues)
		}
		if got := result.Metadata["issues_rejected_without_rule"]; got != "0" {
			t.Fatalf("expected issues_rejected_without_rule=0, got %q", got)
		}
		if got := result.Metadata["discard_warning"]; got != "" {
			t.Fatalf("expected an empty discard_warning on the happy path, got %q", got)
		}
	})

	t.Run("mixed discard: names how many and why, both reasons", func(t *testing.T) {
		repo := aur448Diff(t)
		diff, _, err := repo.Diff("HEAD~1", "HEAD")
		if err != nil {
			t.Fatalf("Diff: %v", err)
		}
		result, err := aur448Generate(t, aur448MixedResponse).GenerateReview(context.Background(), diff)
		if err != nil {
			t.Fatalf("GenerateReview: %v", err)
		}
		if got := result.Metadata["issues_rejected_without_rule"]; got != "2" {
			t.Fatalf("expected issues_rejected_without_rule=2, got %q", got)
		}
		warning := result.Metadata["discard_warning"]
		if warning == "" {
			t.Fatal("expected a non-empty discard_warning when 2 findings were discarded")
		}
		for _, want := range []string{"2 finding(s) discarded", "1 with no rule_id", "1 citing an unknown rule_id", "security/definitely-not-a-rule"} {
			if !strings.Contains(warning, want) {
				t.Errorf("expected %q in discard_warning, got %q", want, warning)
			}
		}
	})

	t.Run("missing-only discard: warning never mentions unknown rule_id", func(t *testing.T) {
		repo := aur448Diff(t)
		diff, _, err := repo.Diff("HEAD~1", "HEAD")
		if err != nil {
			t.Fatalf("Diff: %v", err)
		}
		result, err := aur448Generate(t, aur448MissingOnlyResponse).GenerateReview(context.Background(), diff)
		if err != nil {
			t.Fatalf("GenerateReview: %v", err)
		}
		warning := result.Metadata["discard_warning"]
		if !strings.Contains(warning, "1 finding(s) discarded: 1 with no rule_id") {
			t.Fatalf("unexpected discard_warning: %q", warning)
		}
		if strings.Contains(warning, "unknown rule_id") {
			t.Fatalf("missing-only discard must not mention unknown rule_id, got %q", warning)
		}
	})

	t.Run("unknown-only discard: warning never mentions no rule_id", func(t *testing.T) {
		repo := aur448Diff(t)
		diff, _, err := repo.Diff("HEAD~1", "HEAD")
		if err != nil {
			t.Fatalf("Diff: %v", err)
		}
		result, err := aur448Generate(t, aur448UnknownOnlyResponse).GenerateReview(context.Background(), diff)
		if err != nil {
			t.Fatalf("GenerateReview: %v", err)
		}
		warning := result.Metadata["discard_warning"]
		if !strings.Contains(warning, "1 finding(s) discarded: 1 citing an unknown rule_id (security/definitely-not-a-rule)") {
			t.Fatalf("unexpected discard_warning: %q", warning)
		}
		if strings.Contains(warning, "with no rule_id") {
			t.Fatalf("unknown-only discard must not mention a missing rule_id, got %q", warning)
		}
	})

	t.Run("determinism: same input, same discard_warning", func(t *testing.T) {
		repo := aur448Diff(t)
		diff, _, err := repo.Diff("HEAD~1", "HEAD")
		if err != nil {
			t.Fatalf("Diff: %v", err)
		}
		r1, err := aur448Generate(t, aur448MixedResponse).GenerateReview(context.Background(), diff)
		if err != nil {
			t.Fatalf("GenerateReview: %v", err)
		}
		r2, err := aur448Generate(t, aur448MixedResponse).GenerateReview(context.Background(), diff)
		if err != nil {
			t.Fatalf("GenerateReview: %v", err)
		}
		if r1.Metadata["discard_warning"] != r2.Metadata["discard_warning"] {
			t.Fatalf("discard_warning is not deterministic:\n1: %q\n2: %q", r1.Metadata["discard_warning"], r2.Metadata["discard_warning"])
		}
	})
}
