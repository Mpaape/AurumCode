package welcome

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aurumcode/internal/llm"
)

// MockProvider implements llm.Provider for testing
type MockProvider struct {
	response     string
	err          error
	tokens       int
	tokenErr     error
	callCount    int
	lastPrompt   string
	lastOptions  llm.Options
}

func (m *MockProvider) Complete(prompt string, opts llm.Options) (llm.Response, error) {
	m.callCount++
	m.lastPrompt = prompt
	m.lastOptions = opts

	if m.err != nil {
		return llm.Response{}, m.err
	}

	return llm.Response{
		Text:      m.response,
		TokensIn:  100,
		TokensOut: 200,
		Model:     "mock-model",
	}, nil
}

func (m *MockProvider) Tokens(input string) (int, error) {
	if m.tokenErr != nil {
		return 0, m.tokenErr
	}
	return m.tokens, nil
}

func (m *MockProvider) Name() string {
	return "mock-provider"
}

func TestNewGenerator(t *testing.T) {
	mockProvider := &MockProvider{}
	orch := llm.NewOrchestrator(mockProvider, nil, nil)
	gen := NewGenerator(orch)

	if gen == nil {
		t.Fatal("NewGenerator returned nil")
	}

	if gen.orchestrator == nil {
		t.Error("Generator orchestrator is nil")
	}

	if gen.promptPath != defaultPromptPath {
		t.Errorf("Prompt path = %q, want %q", gen.promptPath, defaultPromptPath)
	}
}

func TestNewGeneratorWithPrompt(t *testing.T) {
	mockProvider := &MockProvider{}
	orch := llm.NewOrchestrator(mockProvider, nil, nil)
	customPath := "custom/prompt.md"
	gen := NewGeneratorWithPrompt(orch, customPath)

	if gen.promptPath != customPath {
		t.Errorf("Prompt path = %q, want %q", gen.promptPath, customPath)
	}
}

func TestGenerate_Success(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test README
	readmeContent := "# Test Project\n\nThis is a test project.\n\n## Features\n- Feature 1\n- Feature 2"
	readmePath := filepath.Join(tmpDir, "README.md")
	if err := os.WriteFile(readmePath, []byte(readmeContent), 0644); err != nil {
		t.Fatalf("Failed to create README: %v", err)
	}

	// Create test prompt template
	promptTemplate := `Transform this README:\n\n{{README_CONTENT}}`
	promptPath := filepath.Join(tmpDir, "prompt.md")
	if err := os.WriteFile(promptPath, []byte(promptTemplate), 0644); err != nil {
		t.Fatalf("Failed to create prompt: %v", err)
	}

	// Setup mock provider
	mockProvider := &MockProvider{
		response: "# Welcome\n\nGenerated welcome page content.",
	}
	orch := llm.NewOrchestrator(mockProvider, nil, nil)
	gen := NewGeneratorWithPrompt(orch, promptPath)

	// Generate
	opts := GenerateOptions{
		ReadmePath: readmePath,
		ProjectDir: "",
		Title:      "Test Home",
		NavOrder:   1,
	}

	content, err := gen.Generate(context.Background(), opts)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	// Verify front matter
	if !strings.HasPrefix(content, "---\n") {
		t.Error("Content should start with front matter")
	}

	if !strings.Contains(content, "layout: default") {
		t.Error("Content should contain layout")
	}

	if !strings.Contains(content, "title: Test Home") {
		t.Error("Content should contain title")
	}

	if !strings.Contains(content, "nav_order: 1") {
		t.Error("Content should contain nav_order")
	}

	// Verify content
	if !strings.Contains(content, "Welcome") {
		t.Error("Content should contain generated text")
	}

	// Verify mock was called
	if mockProvider.callCount != 1 {
		t.Errorf("Provider called %d times, want 1", mockProvider.callCount)
	}

	// Verify prompt contained README content
	if !strings.Contains(mockProvider.lastPrompt, readmeContent) {
		t.Error("Prompt should contain README content")
	}
}

func TestGenerate_WithOutput(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test README
	readmeContent := "# Test\n\nContent"
	readmePath := filepath.Join(tmpDir, "README.md")
	if err := os.WriteFile(readmePath, []byte(readmeContent), 0644); err != nil {
		t.Fatalf("Failed to create README: %v", err)
	}

	// Create prompt
	promptTemplate := `{{README_CONTENT}}`
	promptPath := filepath.Join(tmpDir, "prompt.md")
	if err := os.WriteFile(promptPath, []byte(promptTemplate), 0644); err != nil {
		t.Fatalf("Failed to create prompt: %v", err)
	}

	// Setup mock
	mockProvider := &MockProvider{
		response: "Generated content",
	}
	orch := llm.NewOrchestrator(mockProvider, nil, nil)
	gen := NewGeneratorWithPrompt(orch, promptPath)

	// Generate with output
	outputPath := filepath.Join(tmpDir, "docs", "index.md")
	opts := GenerateOptions{
		ReadmePath: readmePath,
		OutputPath: outputPath,
	}

	_, err := gen.Generate(context.Background(), opts)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	// Verify output file exists
	if _, err := os.Stat(outputPath); err != nil {
		t.Errorf("Output file not created: %v", err)
	}

	// Read and verify content
	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("Failed to read output: %v", err)
	}

	if !strings.HasPrefix(string(content), "---\n") {
		t.Error("Output should have front matter")
	}
}

func TestGenerate_ReadmeNotFound(t *testing.T) {
	tmpDir := t.TempDir()

	mockProvider := &MockProvider{}
	orch := llm.NewOrchestrator(mockProvider, nil, nil)
	gen := NewGenerator(orch)

	opts := GenerateOptions{
		ReadmePath: filepath.Join(tmpDir, "nonexistent.md"),
	}

	_, err := gen.Generate(context.Background(), opts)
	if err == nil {
		t.Error("Expected error for missing README")
	}
}

func TestGenerate_PromptTemplateNotFound(t *testing.T) {
	tmpDir := t.TempDir()

	// Create README but not prompt
	readmePath := filepath.Join(tmpDir, "README.md")
	if err := os.WriteFile(readmePath, []byte("# Test"), 0644); err != nil {
		t.Fatalf("Failed to create README: %v", err)
	}

	mockProvider := &MockProvider{}
	orch := llm.NewOrchestrator(mockProvider, nil, nil)
	gen := NewGeneratorWithPrompt(orch, filepath.Join(tmpDir, "missing.md"))

	opts := GenerateOptions{
		ReadmePath: readmePath,
	}

	_, err := gen.Generate(context.Background(), opts)
	if err == nil {
		t.Error("Expected error for missing prompt template")
	}
}

func TestGenerate_MissingPlaceholder(t *testing.T) {
	tmpDir := t.TempDir()

	// Create README
	readmePath := filepath.Join(tmpDir, "README.md")
	if err := os.WriteFile(readmePath, []byte("# Test"), 0644); err != nil {
		t.Fatalf("Failed to create README: %v", err)
	}

	// Create prompt without placeholder
	promptPath := filepath.Join(tmpDir, "prompt.md")
	if err := os.WriteFile(promptPath, []byte("No placeholder here"), 0644); err != nil {
		t.Fatalf("Failed to create prompt: %v", err)
	}

	mockProvider := &MockProvider{}
	orch := llm.NewOrchestrator(mockProvider, nil, nil)
	gen := NewGeneratorWithPrompt(orch, promptPath)

	opts := GenerateOptions{
		ReadmePath: readmePath,
	}

	_, err := gen.Generate(context.Background(), opts)
	if err == nil {
		t.Error("Expected error for missing placeholder")
	}

	if !strings.Contains(err.Error(), "placeholder") {
		t.Errorf("Error should mention placeholder, got: %v", err)
	}
}

func TestStripFrontMatter(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name: "with front matter",
			input: `---
title: Test
---

# Content`,
			want: "# Content",
		},
		{
			name:  "without front matter",
			input: "# Just Content",
			want:  "# Just Content",
		},
		{
			name: "multiple delimiters",
			input: `---
title: Test
layout: default
---

# Content with ---
More content`,
			want: "# Content with ---\nMore content",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripFrontMatter(tt.input)
			if got != tt.want {
				t.Errorf("stripFrontMatter() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAddFrontMatter(t *testing.T) {
	gen := &Generator{}

	tests := []struct {
		name    string
		content string
		opts    GenerateOptions
		checks  []string
	}{
		{
			name:    "default options",
			content: "# Content",
			opts:    GenerateOptions{},
			checks: []string{
				"---",
				"layout: default",
				"title: Home",
				"nav_order: 1",
				"# Content",
			},
		},
		{
			name:    "custom title and nav order",
			content: "# Content",
			opts: GenerateOptions{
				Title:    "Custom Title",
				NavOrder: 5,
			},
			checks: []string{
				"title: Custom Title",
				"nav_order: 5",
			},
		},
		{
			name: "strips existing front matter",
			content: `---
old: value
---

# Content`,
			opts: GenerateOptions{},
			checks: []string{
				"title: Home",
				"# Content",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := gen.addFrontMatter(tt.content, tt.opts)

			for _, check := range tt.checks {
				if !strings.Contains(result, check) {
					t.Errorf("Result should contain %q\nGot:\n%s", check, result)
				}
			}
		})
	}
}
