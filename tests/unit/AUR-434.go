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

// aur434Root resolves the repository root. The acceptance harness sets
// AURUMCODE_ROOT to the staged materialization root (see
// tests/acceptance/AUR-434.sh); running the bridge directly from a full
// checkout works too, climbing two directories from tests/unit back to the
// repository root.
func aur434Root() string {
	if r := os.Getenv("AURUMCODE_ROOT"); r != "" {
		return r
	}
	return filepath.Join("..", "..")
}

// aur434MixedResponse is a deterministic model response with exactly one
// finding that cites a real rule from the embedded catalog and two
// ungrounded findings: one with no rule_id at all and one citing a rule id
// that does not exist in the catalog. AUR-434's outcome is that only the
// first reaches the user.
const aur434MixedResponse = `{
  "issues": [
    {
      "file": "config/demo-tokens.txt",
      "line": 3,
      "severity": "error",
      "rule_id": "security/hardcoded-secret",
      "message": "A credential-shaped value was committed in plain text."
    },
    {
      "file": "config/demo-tokens.txt",
      "line": 4,
      "severity": "error",
      "message": "UNGROUNDED-NO-RULE planted finding without any rule id."
    },
    {
      "file": "config/demo-tokens.txt",
      "line": 5,
      "severity": "warning",
      "rule_id": "security/definitely-not-a-rule",
      "message": "UNGROUNDED-BAD-RULE planted finding citing a nonexistent rule."
    }
  ],
  "summary": "Mixed fixture for AUR-434."
}`

const aur434AllUngroundedResponse = `{
  "issues": [
    {
      "file": "config/demo-tokens.txt",
      "line": 4,
      "severity": "error",
      "message": "UNGROUNDED-NO-RULE planted finding without any rule id."
    },
    {
      "file": "config/demo-tokens.txt",
      "line": 5,
      "severity": "warning",
      "rule_id": "security/definitely-not-a-rule",
      "message": "UNGROUNDED-BAD-RULE planted finding citing a nonexistent rule."
    }
  ],
  "summary": "Only ungrounded findings."
}`

func aur434Diff(t *testing.T) *analyzer.Repo {
	t.Helper()
	repoPath := filepath.Join(aur434Root(), "tests/fixtures/repos/git-demo/repo.git")
	repo, err := analyzer.OpenRepo(repoPath)
	if err != nil {
		t.Fatalf("OpenRepo(%s): %v", repoPath, err)
	}
	return repo
}

func aur434Review(t *testing.T, response string) *review.Reviewer {
	t.Helper()
	orch := llm.NewOrchestrator(&review.FakeProvider{Response: response}, nil, nil)
	return review.NewReviewer(orch, review.DefaultConfig())
}

// TestAUR434 proves, at the package API level, the two halves of AUR-434's
// outcome: every issue the reviewer reports cites a rule from the embedded
// project review standard, and an issue that cannot cite such a rule never
// reaches the caller.
func TestAUR434(t *testing.T) {
	t.Run("embedded catalog loads and fails closed on unknown ids", func(t *testing.T) {
		loader := review.NewRulesLoader()
		if err := loader.Load(); err != nil {
			t.Fatalf("Load() of the embedded catalog must succeed, got: %v", err)
		}
		all := loader.GetAll()
		if len(all) == 0 {
			t.Fatal("embedded catalog must not be empty")
		}
		rule, ok := loader.Get("security/hardcoded-secret")
		if !ok {
			t.Fatal("expected security/hardcoded-secret in the embedded catalog")
		}
		if rule.Title == "" || rule.Severity == "" || rule.Category != "security" {
			t.Fatalf("rule metadata incomplete: %+v", rule)
		}
		if _, ok := loader.Get(""); ok {
			t.Fatal("an empty rule id must never resolve to a rule")
		}
		if _, ok := loader.Get("security/definitely-not-a-rule"); ok {
			t.Fatal("an unknown rule id must never resolve to a rule")
		}
	})

	t.Run("only findings citing a real rule reach the caller", func(t *testing.T) {
		repo := aur434Diff(t)
		diff, _, err := repo.Diff("HEAD~1", "HEAD")
		if err != nil {
			t.Fatalf("Diff: %v", err)
		}

		result, err := aur434Review(t, aur434MixedResponse).GenerateReview(context.Background(), diff)
		if err != nil {
			t.Fatalf("GenerateReview: %v", err)
		}
		if len(result.Issues) != 1 {
			t.Fatalf("expected exactly 1 grounded issue to survive, got %d: %+v", len(result.Issues), result.Issues)
		}
		issue := result.Issues[0]
		if issue.RuleID != "security/hardcoded-secret" {
			t.Fatalf("surviving issue cites the wrong rule: %+v", issue)
		}
		if !strings.Contains(issue.Message, "(rule security/hardcoded-secret: Hardcoded Secrets)") {
			t.Fatalf("surviving issue must cite its sustaining rule in the message, got: %q", issue.Message)
		}
		for _, marker := range []string{"UNGROUNDED-NO-RULE", "UNGROUNDED-BAD-RULE"} {
			for _, got := range result.Issues {
				if strings.Contains(got.Message, marker) {
					t.Fatalf("ungrounded finding %s reached the caller: %+v", marker, got)
				}
			}
		}
		if got := result.Metadata["issues_rejected_without_rule"]; got != "2" {
			t.Fatalf("expected issues_rejected_without_rule=2, got %q", got)
		}
	})

	t.Run("a review with only ungrounded findings reports none", func(t *testing.T) {
		repo := aur434Diff(t)
		diff, _, err := repo.Diff("HEAD~1", "HEAD")
		if err != nil {
			t.Fatalf("Diff: %v", err)
		}

		result, err := aur434Review(t, aur434AllUngroundedResponse).GenerateReview(context.Background(), diff)
		if err != nil {
			t.Fatalf("GenerateReview must succeed even when every finding is rejected: %v", err)
		}
		if len(result.Issues) != 0 {
			t.Fatalf("expected no surviving issues, got %+v", result.Issues)
		}
		if got := result.Metadata["issues_rejected_without_rule"]; got != "2" {
			t.Fatalf("expected issues_rejected_without_rule=2, got %q", got)
		}
	})
}
