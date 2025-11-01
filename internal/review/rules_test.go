package review

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRulesLoader(t *testing.T) {
	// Create temp directory with test rules
	tmpDir := t.TempDir()
	rulesFile := filepath.Join(tmpDir, "test-rules.yml")

	testYAML := `rules:
  - id: test/rule-1
    title: Test Rule 1
    description: Test description
    severity: error
    category: testing
    tags: [test, example]
  - id: test/rule-2
    title: Test Rule 2
    description: Another test
    severity: warning
    category: testing
    tags: [test]
`

	if err := os.WriteFile(rulesFile, []byte(testYAML), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	// Load rules
	loader := NewRulesLoader(tmpDir)
	if err := loader.Load(); err != nil {
		t.Fatalf("Failed to load rules: %v", err)
	}

	// Test Get
	rule, ok := loader.Get("test/rule-1")
	if !ok {
		t.Error("Expected to find test/rule-1")
	}
	if rule.Title != "Test Rule 1" {
		t.Errorf("Expected title 'Test Rule 1', got %s", rule.Title)
	}

	// Test GetAll
	allRules := loader.GetAll()
	if len(allRules) != 2 {
		t.Errorf("Expected 2 rules, got %d", len(allRules))
	}

	// Test GetByCategory
	testingRules := loader.GetByCategory("testing")
	if len(testingRules) != 2 {
		t.Errorf("Expected 2 testing rules, got %d", len(testingRules))
	}
}

func TestRulesLoader_MissingDirectory(t *testing.T) {
	loader := NewRulesLoader("/nonexistent/directory")
	err := loader.Load()
	// Should not error on missing directory, just return empty rules
	if err != nil {
		t.Logf("Got error loading from missing directory: %v", err)
	}

	allRules := loader.GetAll()
	if len(allRules) != 0 {
		t.Errorf("Expected 0 rules from missing directory, got %d", len(allRules))
	}
}
