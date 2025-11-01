package testgen

import (
	"aurumcode/internal/llm"
	"aurumcode/internal/llm/cost"
	"aurumcode/pkg/types"
	"context"
	"errors"
	"strings"
	"testing"
)

type mockProvider struct {
	response string
	err      error
}

func (m *mockProvider) Complete(prompt string, opts llm.Options) (llm.Response, error) {
	if m.err != nil {
		return llm.Response{}, m.err
	}
	return llm.Response{
		Text:      m.response,
		TokensIn:  100,
		TokensOut: 200,
		Model:     "test",
	}, nil
}

func (m *mockProvider) Name() string                     { return "mock" }
func (m *mockProvider) Tokens(input string) (int, error) { return len(input) / 4, nil }

func TestGenerate(t *testing.T) {
	mock := &mockProvider{
		response: "```go\nfunc TestAuthenticate(t *testing.T) {\n\tresult := Authenticate(\"token\")\n\tif result != nil {\n\t\tt.Error(\"expected nil error\")\n\t}\n}\n```",
	}

	tracker := cost.NewTracker(100.0, 1000.0, map[string]cost.PriceMap{})
	orchestrator := llm.NewOrchestrator(mock, []llm.Provider{}, tracker)
	generator := NewGenerator(orchestrator)

	diff := &types.Diff{
		Files: []types.DiffFile{
			{
				Path: "auth.go",
				Hunks: []types.DiffHunk{
					{
						Lines: []string{
							"+func Authenticate(token string) error {",
							"+\treturn nil",
							"+}",
						},
					},
				},
			},
		},
	}

	tests, err := generator.Generate(context.Background(), diff)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	if !strings.Contains(tests, "TestAuthenticate") {
		t.Error("generated tests should contain TestAuthenticate")
	}

	if !strings.Contains(tests, "testing.T") {
		t.Error("generated tests should use testing.T")
	}
}

func TestGenerate_EmptyDiff(t *testing.T) {
	mock := &mockProvider{
		response: "```go\nfunc TestExample(t *testing.T) {}\n```",
	}

	tracker := cost.NewTracker(100.0, 1000.0, map[string]cost.PriceMap{})
	orchestrator := llm.NewOrchestrator(mock, []llm.Provider{}, tracker)
	generator := NewGenerator(orchestrator)

	diff := &types.Diff{
		Files: []types.DiffFile{},
	}

	_, err := generator.Generate(context.Background(), diff)
	if err == nil {
		t.Fatal("expected error for empty diff")
	}

	if !strings.Contains(err.Error(), "no files") {
		t.Errorf("expected 'no files' error, got: %v", err)
	}
}

func TestGenerate_UnknownLanguage(t *testing.T) {
	mock := &mockProvider{
		response: "```\ntest code\n```",
	}

	tracker := cost.NewTracker(100.0, 1000.0, map[string]cost.PriceMap{})
	orchestrator := llm.NewOrchestrator(mock, []llm.Provider{}, tracker)
	generator := NewGenerator(orchestrator)

	diff := &types.Diff{
		Files: []types.DiffFile{
			{Path: "README"},
			{Path: "LICENSE"},
		},
	}

	_, err := generator.Generate(context.Background(), diff)
	if err == nil {
		t.Fatal("expected error for unknown language")
	}

	if !strings.Contains(err.Error(), "unable to detect language") {
		t.Errorf("expected 'unable to detect language' error, got: %v", err)
	}
}

func TestGenerate_LLMFailure(t *testing.T) {
	mock := &mockProvider{
		err: errors.New("LLM timeout"),
	}

	tracker := cost.NewTracker(100.0, 1000.0, map[string]cost.PriceMap{})
	orchestrator := llm.NewOrchestrator(mock, []llm.Provider{}, tracker)
	generator := NewGenerator(orchestrator)

	diff := &types.Diff{
		Files: []types.DiffFile{{Path: "test.go"}},
	}

	_, err := generator.Generate(context.Background(), diff)
	if err == nil {
		t.Fatal("expected error for LLM failure")
	}

	if !strings.Contains(err.Error(), "LLM request failed") {
		t.Errorf("expected 'LLM request failed' error, got: %v", err)
	}
}

func TestGenerate_ParseFailure(t *testing.T) {
	mock := &mockProvider{
		response: "", // Empty response causes parse failure
	}

	tracker := cost.NewTracker(100.0, 1000.0, map[string]cost.PriceMap{})
	orchestrator := llm.NewOrchestrator(mock, []llm.Provider{}, tracker)
	generator := NewGenerator(orchestrator)

	diff := &types.Diff{
		Files: []types.DiffFile{{Path: "test.go"}},
	}

	_, err := generator.Generate(context.Background(), diff)
	if err == nil {
		t.Fatal("expected error for parse failure")
	}

	if !strings.Contains(err.Error(), "parse failed") {
		t.Errorf("expected 'parse failed' error, got: %v", err)
	}
}

func TestDetectPrimaryLanguage(t *testing.T) {
	mock := &mockProvider{}
	tracker := cost.NewTracker(100.0, 1000.0, map[string]cost.PriceMap{})
	orchestrator := llm.NewOrchestrator(mock, []llm.Provider{}, tracker)
	generator := NewGenerator(orchestrator)

	tests := []struct {
		name     string
		diff     *types.Diff
		expected string
	}{
		{
			name: "single language",
			diff: &types.Diff{
				Files: []types.DiffFile{
					{Path: "main.go"},
					{Path: "handler.go"},
				},
			},
			expected: "go",
		},
		{
			name: "excludes test files",
			diff: &types.Diff{
				Files: []types.DiffFile{
					{Path: "main.go"},
					{Path: "handler.go"},
					{Path: "main_test.go"}, // Should be excluded
				},
			},
			expected: "go",
		},
		{
			name: "multiple languages - go dominant",
			diff: &types.Diff{
				Files: []types.DiffFile{
					{Path: "main.go"},
					{Path: "handler.go"},
					{Path: "utils.go"},
					{Path: "script.py"},
				},
			},
			expected: "go",
		},
		{
			name: "only test files",
			diff: &types.Diff{
				Files: []types.DiffFile{
					{Path: "main_test.go"},
					{Path: "handler_test.go"},
				},
			},
			expected: "unknown",
		},
		{
			name: "unknown files",
			diff: &types.Diff{
				Files: []types.DiffFile{
					{Path: "README"},
					{Path: "LICENSE"},
				},
			},
			expected: "unknown",
		},
		{
			name: "python dominant",
			diff: &types.Diff{
				Files: []types.DiffFile{
					{Path: "main.py"},
					{Path: "utils.py"},
					{Path: "config.py"},
					{Path: "index.js"},
				},
			},
			expected: "python",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generator.detectPrimaryLanguage(tt.diff)
			if result != tt.expected {
				t.Errorf("expected language %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestGenerate_WithContext(t *testing.T) {
	mock := &mockProvider{
		response: "```go\nfunc TestExample(t *testing.T) {}\n```",
	}

	tracker := cost.NewTracker(100.0, 1000.0, map[string]cost.PriceMap{})
	orchestrator := llm.NewOrchestrator(mock, []llm.Provider{}, tracker)
	generator := NewGenerator(orchestrator)

	diff := &types.Diff{
		Files: []types.DiffFile{{Path: "test.go"}},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := generator.Generate(ctx, diff)
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestGenerate_MultipleFiles(t *testing.T) {
	mock := &mockProvider{
		response: "```go\nfunc TestAuth(t *testing.T) {}\nfunc TestHandler(t *testing.T) {}\n```",
	}

	tracker := cost.NewTracker(100.0, 1000.0, map[string]cost.PriceMap{})
	orchestrator := llm.NewOrchestrator(mock, []llm.Provider{}, tracker)
	generator := NewGenerator(orchestrator)

	diff := &types.Diff{
		Files: []types.DiffFile{
			{Path: "auth.go"},
			{Path: "handler.go"},
		},
	}

	tests, err := generator.Generate(context.Background(), diff)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	if !strings.Contains(tests, "TestAuth") || !strings.Contains(tests, "TestHandler") {
		t.Error("generated tests should contain multiple test functions")
	}
}

func TestGenerate_JavaScript(t *testing.T) {
	mock := &mockProvider{
		response: "```javascript\ndescribe('Authentication', () => {\n  it('should authenticate valid token', () => {\n    expect(authenticate('token')).toBe(true);\n  });\n});\n```",
	}

	tracker := cost.NewTracker(100.0, 1000.0, map[string]cost.PriceMap{})
	orchestrator := llm.NewOrchestrator(mock, []llm.Provider{}, tracker)
	generator := NewGenerator(orchestrator)

	diff := &types.Diff{
		Files: []types.DiffFile{
			{Path: "auth.js"},
		},
	}

	tests, err := generator.Generate(context.Background(), diff)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	if !strings.Contains(tests, "describe") {
		t.Error("JavaScript tests should contain 'describe'")
	}
}

func TestGenerate_Python(t *testing.T) {
	mock := &mockProvider{
		response: "```python\ndef test_authenticate():\n    result = authenticate('token')\n    assert result is not None\n```",
	}

	tracker := cost.NewTracker(100.0, 1000.0, map[string]cost.PriceMap{})
	orchestrator := llm.NewOrchestrator(mock, []llm.Provider{}, tracker)
	generator := NewGenerator(orchestrator)

	diff := &types.Diff{
		Files: []types.DiffFile{
			{Path: "auth.py"},
		},
	}

	tests, err := generator.Generate(context.Background(), diff)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	if !strings.Contains(tests, "test_authenticate") {
		t.Error("Python tests should contain test function")
	}

	if !strings.Contains(tests, "assert") {
		t.Error("Python tests should use assert")
	}
}
