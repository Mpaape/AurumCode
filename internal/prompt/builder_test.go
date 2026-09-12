package prompt

import (
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/analyzer"
	"github.com/Mpaape/AurumCode/pkg/types"
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
		"code review",
		"Total files: 1",
		"Lines added: 3",
		"File: main.go",
		"Language: go",
		"Correção",
		"Segurança",
		"Performance",
		"JSON",
	}

	for _, element := range expectedElements {
		if !strings.Contains(prompt, element) {
			t.Errorf("prompt missing expected element: %s", element)
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
