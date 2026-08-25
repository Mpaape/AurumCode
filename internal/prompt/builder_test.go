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
	if !strings.Contains(prompt, "code review") {
		t.Error("empty diff should still generate valid prompt")
	}
}

func TestReviewChangeScopeRejectsOperationalPraise(t *testing.T) {
	operational := &types.Diff{Files: []types.DiffFile{
		{Path: ".github/workflows/review.yml", Hunks: []types.DiffHunk{{Lines: []string{"+permissions:", "+  contents: read"}}}},
		{Path: "README.md", Hunks: []types.DiffHunk{{Lines: []string{"+document the setup"}}}},
	}}
	if HasSubstantiveCodeChange(operational) {
		t.Fatal("configuration and documentation changes were classified as substantive code")
	}
	if scope := ReviewChangeScope(operational); !strings.Contains(scope, "leave strengths empty") {
		t.Fatalf("operational scope = %q, want explicit no-praise instruction", scope)
	}

	code := &types.Diff{Files: []types.DiffFile{{
		Path:  "internal/review.go",
		Hunks: []types.DiffHunk{{Lines: []string{"+return err"}}},
	}}}
	if !HasSubstantiveCodeChange(code) {
		t.Fatal("source code change was not classified as substantive")
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

// TestBuildPrompt_SourcedFromAurumcodePromptsReviewMD proves the wiring
// AUR-430's card requires: the "review" schema kind's System prompt is
// built from the content copied from .aurumcode/prompts/review.md (see
// templates/review.md and docs/specs/AUR-430.md), not a generic inline
// string. It checks for text that only exists in that richer prompt
// (the structured report sections and the ISO/IEC 25010 section), so a
// regression back to the old one-line system message would fail this test.
func TestBuildPrompt_SourcedFromAurumcodePromptsReviewMD(t *testing.T) {
	builder := NewPromptBuilder()

	diff := &types.Diff{
		Files: []types.DiffFile{
			{
				Path: "main.go",
				Hunks: []types.DiffHunk{
					{Lines: []string{"+func main() {}"}},
				},
			},
		},
	}
	metrics := &analyzer.DiffMetrics{TotalFiles: 1, LanguageBreakdown: map[string]int{"go": 1}}

	parts, err := builder.BuildPrompt(diff, metrics, BuildOptions{
		MaxTokens:    4000,
		SchemaKind:   "review",
		Role:         "reviewer",
		ReserveReply: 1000,
	})
	if err != nil {
		t.Fatalf("BuildPrompt failed: %v", err)
	}

	for _, marker := range []string{
		"Response format",
		"CI context",
		"ISO/IEC 25010",
	} {
		if !strings.Contains(parts.System, marker) {
			t.Errorf("expected System prompt sourced from .aurumcode/prompts/review.md to contain %q, got:\n%s", marker, parts.System)
		}
	}

	for _, marker := range []string{
		"Trate sintaxe de workflow como configuração, não como credencial",
		"contents: read",
		"secrets.NAME",
		"valor de credencial efetivamente",
		"Leia o diff inteiro antes de formar uma conclusão",
		"foi introduzido pela mudança",
		"Use `suggestions` com parcimônia",
		"`suggestions: []` é um resultado correto",
		"Não use `suggestions` para preferência pessoal",
		"quando a melhor opção depender de contexto ausente",
		"Só use esse tipo quando",
		"adaptação manual",
		"issues: []",
	} {
		if !strings.Contains(parts.System, marker) {
			t.Errorf("expected the review prompt to distinguish workflow references from committed credentials; missing %q", marker)
		}
	}

	if !strings.Contains(parts.User, "func main") {
		t.Errorf("expected User prompt to carry the actual diff content, got:\n%s", parts.User)
	}
}

func TestBuildPromptUsesConfiguredReviewLanguage(t *testing.T) {
	builder := NewPromptBuilder()
	diff := &types.Diff{Files: []types.DiffFile{{
		Path:  "main.go",
		Hunks: []types.DiffHunk{{Lines: []string{"+func main() {}"}}},
	}}}
	metrics := &analyzer.DiffMetrics{TotalFiles: 1, LanguageBreakdown: map[string]int{"go": 1}}
	parts, err := builder.BuildPrompt(diff, metrics, BuildOptions{
		MaxTokens:    4000,
		SchemaKind:   "review",
		Role:         "reviewer",
		ReserveReply: 1000,
		Language:     "pt-BR",
	})
	if err != nil {
		t.Fatalf("BuildPrompt: %v", err)
	}
	if !strings.Contains(parts.System, "pt-BR") || !strings.Contains(parts.System, "Escreva todo texto") {
		t.Fatalf("configured review language was not included in the prompt:\n%s", parts.System)
	}
}
