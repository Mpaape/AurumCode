package review

import (
	"aurumcode/internal/analyzer"
	"aurumcode/internal/llm"
	"aurumcode/pkg/types"
	"context"
	"os"
	"path/filepath"
	"testing"
)

// StubOrchestrator for testing
type StubOrchestrator struct {
	response string
	tokens   int
}

func (s *StubOrchestrator) Complete(ctx context.Context, prompt string, opts llm.Options) (*llm.Response, error) {
	return &llm.Response{
		Text:      s.response,
		TokensIn:  s.tokens / 2,
		TokensOut: s.tokens / 2,
		Model:     "test-model",
	}, nil
}

func TestNewReviewer(t *testing.T) {
	// Create temp directories
	tmpDir := t.TempDir()
	rulesDir := filepath.Join(tmpDir, "rules")
	if err := os.MkdirAll(rulesDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create test rule file
	ruleContent := `rules:
  - id: test/rule
    title: Test Rule
    description: Test
    severity: warning
    category: test
    tags: [test]
`
	if err := os.WriteFile(filepath.Join(rulesDir, "test.yml"), []byte(ruleContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create ISO config
	isoConfig := filepath.Join(tmpDir, "iso.yml")
	isoContent := `weights:
  functionality: 0.15
  reliability: 0.15
  usability: 0.10
  efficiency: 0.12
  maintainability: 0.18
  portability: 0.08
  security: 0.17
  compatibility: 0.05
thresholds:
  excellent: 90
  good: 75
  acceptable: 60
  poor: 40
  critical: 0
static_signals:
  complexity_increase: -5
  complexity_decrease: 3
  todo_comments: -2
  fixme_comments: -3
  code_smells: -4
  test_coverage_increase: 5
  test_coverage_decrease: -10
  missing_docs: -2
  added_docs: 3
`
	if err := os.WriteFile(isoConfig, []byte(isoContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create reviewer
	stubOrch := &StubOrchestrator{
		response: `{"issues": [], "iso_scores": {"functionality": 8, "reliability": 7, "usability": 9, "efficiency": 8, "maintainability": 7, "portability": 9, "security": 6, "compatibility": 8}, "summary": "Test"}`,
		tokens:   100,
	}

	reviewer, err := NewReviewer(stubOrch, Config{
		RulesDir:      rulesDir,
		ISOConfigPath: isoConfig,
		MaxTokens:     4000,
		Temperature:   0.3,
	})

	if err != nil {
		t.Fatalf("Failed to create reviewer: %v", err)
	}

	if reviewer == nil {
		t.Fatal("Expected reviewer to be created")
	}

	// Test GetRules
	rules := reviewer.GetRules()
	if len(rules) != 1 {
		t.Errorf("Expected 1 rule, got %d", len(rules))
	}
}

func TestGenerateReview(t *testing.T) {
	// Setup similar to TestNewReviewer
	tmpDir := t.TempDir()
	rulesDir := filepath.Join(tmpDir, "rules")
	if err := os.MkdirAll(rulesDir, 0755); err != nil {
		t.Fatal(err)
	}

	ruleContent := `rules:
  - id: quality/test
    title: Test Quality
    description: Test quality rule
    severity: warning
    category: quality
    tags: [test]
`
	if err := os.WriteFile(filepath.Join(rulesDir, "quality.yml"), []byte(ruleContent), 0644); err != nil {
		t.Fatal(err)
	}

	isoConfig := filepath.Join(tmpDir, "iso.yml")
	isoContent := `weights:
  functionality: 0.15
  reliability: 0.15
  usability: 0.10
  efficiency: 0.12
  maintainability: 0.18
  portability: 0.08
  security: 0.17
  compatibility: 0.05
thresholds:
  excellent: 90
  good: 75
  acceptable: 60
  poor: 40
  critical: 0
static_signals:
  complexity_increase: -5
  complexity_decrease: 3
  todo_comments: -2
  fixme_comments: -3
  code_smells: -4
  test_coverage_increase: 5
  test_coverage_decrease: -10
  missing_docs: -2
  added_docs: 3
`
	if err := os.WriteFile(isoConfig, []byte(isoContent), 0644); err != nil {
		t.Fatal(err)
	}

	stubOrch := &StubOrchestrator{
		response: `{
			"issues": [
				{
					"file": "test.go",
					"line": 10,
					"severity": "warning",
					"rule_id": "quality/test",
					"message": "Test issue",
					"suggestion": "Fix it"
				}
			],
			"iso_scores": {
				"functionality": 85,
				"reliability": 80,
				"usability": 90,
				"efficiency": 85,
				"maintainability": 75,
				"portability": 90,
				"security": 70,
				"compatibility": 85
			},
			"summary": "Good code quality"
		}`,
		tokens: 200,
	}

	reviewer, err := NewReviewer(stubOrch, Config{
		RulesDir:      rulesDir,
		ISOConfigPath: isoConfig,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Create test diff
	diff := &types.Diff{
		Files: []types.DiffFile{
			{
				Path: "test.go",
				Hunks: []types.DiffHunk{
					{
						Lines: []string{"+func test() {", "+}"},
					},
				},
			},
		},
	}

	// Generate review
	result, err := reviewer.GenerateReview(context.Background(), diff)
	if err != nil {
		t.Fatalf("GenerateReview failed: %v", err)
	}

	// Verify result
	if len(result.Issues) != 1 {
		t.Errorf("Expected 1 issue, got %d", len(result.Issues))
	}

	if result.Cost.Tokens != 200 {
		t.Errorf("Expected 200 tokens, got %d", result.Cost.Tokens)
	}

	if result.ISOScores.Functionality == 0 {
		t.Error("Expected ISO scores to be populated")
	}
}
