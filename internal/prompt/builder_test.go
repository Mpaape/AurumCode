package prompt

import (
	"aurumcode/internal/analyzer"
	"aurumcode/pkg/types"
	"strings"
	"testing"
)

func TestBuildReviewPrompt(t *testing.T) {
	builder := NewPromptBuilder()

	diff := &types.Diff{
		Files: []types.DiffFile{
			{
				Path: "main.go",
				Hunks: []types.DiffHunk{
					{
						Lines: []string{
							"+func main() {",
							"+\tprintln(\"hello\")",
							"+}",
						},
					},
				},
			},
		},
	}

	metrics := &analyzer.DiffMetrics{
		TotalFiles:        1,
		LinesAdded:        3,
		LinesDeleted:      0,
		TestFiles:         0,
		ConfigFiles:       0,
		LanguageBreakdown: map[string]int{"go": 1},
	}

	prompt := builder.BuildReviewPrompt(diff, metrics)

	// Check that prompt contains key elements
	expectedElements := []string{
		"code reviewer",
		"Total files: 1",
		"Lines added: 3",
		"File: main.go",
		"Language: go",
		"Code Quality",
		"Security",
		"Performance",
		"JSON",
	}

	for _, element := range expectedElements {
		if !strings.Contains(prompt, element) {
			t.Errorf("prompt missing expected element: %s", element)
		}
	}
}

func TestBuildDocumentationPrompt(t *testing.T) {
	builder := NewPromptBuilder()

	diff := &types.Diff{
		Files: []types.DiffFile{
			{
				Path: "api.go",
				Hunks: []types.DiffHunk{
					{
						Lines: []string{
							"+func NewAPI() *API {",
							"+\treturn &API{}",
							"+}",
						},
					},
				},
			},
		},
	}

	prompt := builder.BuildDocumentationPrompt(diff, "go")

	expectedElements := []string{
		"technical documentation",
		"Language: go",
		"API Documentation",
		"Usage Examples",
		"Configuration",
		"Breaking Changes",
		"Markdown",
	}

	for _, element := range expectedElements {
		if !strings.Contains(prompt, element) {
			t.Errorf("documentation prompt missing: %s", element)
		}
	}
}

func TestBuildTestPrompt(t *testing.T) {
	builder := NewPromptBuilder()

	diff := &types.Diff{
		Files: []types.DiffFile{
			{
				Path: "service.go",
				Hunks: []types.DiffHunk{
					{
						Lines: []string{
							"+func Process(data string) error {",
							"+\treturn nil",
							"+}",
						},
					},
				},
			},
			{
				Path: "service_test.go", // Should be skipped
				Hunks: []types.DiffHunk{
					{
						Lines: []string{"+func TestService(t *testing.T) {}"},
					},
				},
			},
		},
	}

	prompt := builder.BuildTestPrompt(diff, "go")

	// Should include service.go but not service_test.go
	if !strings.Contains(prompt, "service.go") {
		t.Error("test prompt should include service.go")
	}

	if strings.Contains(prompt, "service_test.go") {
		t.Error("test prompt should skip test files")
	}

	expectedElements := []string{
		"test engineer",
		"Language: go",
		"Happy Path",
		"Edge Cases",
		"Error Handling",
		"Integration",
	}

	for _, element := range expectedElements {
		if !strings.Contains(prompt, element) {
			t.Errorf("test prompt missing: %s", element)
		}
	}
}

func TestBuildSummaryPrompt(t *testing.T) {
	builder := NewPromptBuilder()

	diff := &types.Diff{
		Files: []types.DiffFile{
			{Path: "main.go"},
			{Path: "handler.go"},
		},
	}

	metrics := &analyzer.DiffMetrics{
		TotalFiles:        2,
		LinesAdded:        50,
		LinesDeleted:      10,
		LanguageBreakdown: map[string]int{"go": 2},
	}

	prompt := builder.BuildSummaryPrompt(diff, metrics)

	expectedElements := []string{
		"Summarize",
		"Files changed: 2",
		"Lines: +50 -10",
		"main.go",
		"handler.go",
	}

	for _, element := range expectedElements {
		if !strings.Contains(prompt, element) {
			t.Errorf("summary prompt missing: %s", element)
		}
	}
}

func TestTruncatePrompt(t *testing.T) {
	builder := NewPromptBuilder()

	tests := []struct {
		name      string
		prompt    string
		maxTokens int
		truncated bool
	}{
		{
			name:      "short prompt",
			prompt:    "Short prompt",
			maxTokens: 100,
			truncated: false,
		},
		{
			name:      "long prompt",
			prompt:    strings.Repeat("a", 10000),
			maxTokens: 100,
			truncated: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := builder.TruncatePrompt(test.prompt, test.maxTokens)

			if test.truncated {
				if len(result) >= len(test.prompt) {
					t.Error("expected prompt to be truncated")
				}
				if !strings.Contains(result, "truncated") {
					t.Error("truncated prompt should indicate truncation")
				}
			} else {
				if result != test.prompt {
					t.Error("short prompt should not be modified")
				}
			}
		})
	}
}

func TestGetLanguageList(t *testing.T) {
	builder := NewPromptBuilder()

	metrics := &analyzer.DiffMetrics{
		LanguageBreakdown: map[string]int{
			"go":         3,
			"javascript": 2,
			"python":     1,
		},
	}

	languages := builder.getLanguageList(metrics)

	if len(languages) != 3 {
		t.Errorf("expected 3 languages, got %d", len(languages))
	}

	// Check all languages are present
	languageMap := make(map[string]bool)
	for _, lang := range languages {
		languageMap[lang] = true
	}

	expectedLangs := []string{"go", "javascript", "python"}
	for _, lang := range expectedLangs {
		if !languageMap[lang] {
			t.Errorf("missing language: %s", lang)
		}
	}
}

func TestBuildReviewPrompt_EmptyDiff(t *testing.T) {
	builder := NewPromptBuilder()

	diff := &types.Diff{
		Files: []types.DiffFile{},
	}

	metrics := &analyzer.DiffMetrics{
		TotalFiles:        0,
		LanguageBreakdown: map[string]int{},
	}

	prompt := builder.BuildReviewPrompt(diff, metrics)

	// Should still contain instructions
	if !strings.Contains(prompt, "code reviewer") {
		t.Error("empty diff should still generate valid prompt")
	}
}

func TestBuildReviewPrompt_MultipleLanguages(t *testing.T) {
	builder := NewPromptBuilder()

	diff := &types.Diff{
		Files: []types.DiffFile{
			{Path: "main.go"},
			{Path: "script.py"},
			{Path: "app.js"},
		},
	}

	metrics := &analyzer.DiffMetrics{
		TotalFiles: 3,
		LanguageBreakdown: map[string]int{
			"go":         1,
			"python":     1,
			"javascript": 1,
		},
	}

	prompt := builder.BuildReviewPrompt(diff, metrics)

	// Should list all languages
	for lang := range metrics.LanguageBreakdown {
		if !strings.Contains(prompt, lang) {
			t.Errorf("prompt should mention language: %s", lang)
		}
	}
}
