package docgen

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

func (m *mockProvider) Name() string                            { return "mock" }
func (m *mockProvider) Tokens(input string) (int, error)        { return len(input) / 4, nil }

func TestGenerate(t *testing.T) {
	mock := &mockProvider{
		response: "# API Documentation\n\nThis module provides authentication services.\n\n## Usage\n\nImport the package and call `Authenticate()`.",
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

	doc, err := generator.Generate(context.Background(), diff)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	if !strings.Contains(doc, "API Documentation") {
		t.Error("documentation should contain 'API Documentation'")
	}

	if !strings.Contains(doc, "authentication") {
		t.Error("documentation should mention authentication")
	}
}

func TestGenerate_EmptyDiff(t *testing.T) {
	mock := &mockProvider{
		response: "# Documentation",
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
			name: "multiple languages - python dominant",
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
			name: "mixed with unknown",
			diff: &types.Diff{
				Files: []types.DiffFile{
					{Path: "main.go"},
					{Path: "README"},
				},
			},
			expected: "go",
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
		response: "# Documentation\n\nGenerated docs.",
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
		response: "# Multi-file Documentation\n\nCovers multiple modules.",
	}

	tracker := cost.NewTracker(100.0, 1000.0, map[string]cost.PriceMap{})
	orchestrator := llm.NewOrchestrator(mock, []llm.Provider{}, tracker)
	generator := NewGenerator(orchestrator)

	diff := &types.Diff{
		Files: []types.DiffFile{
			{Path: "auth.go"},
			{Path: "handler.go"},
			{Path: "middleware.go"},
		},
	}

	doc, err := generator.Generate(context.Background(), diff)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	if !strings.Contains(doc, "Multi-file") {
		t.Error("documentation should mention multi-file")
	}
}
