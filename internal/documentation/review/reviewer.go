// Package review creates an editorial review of the documentation site.
//
// Extractors remain the source of truth for API facts. This package only asks
// the model to evaluate clarity, navigation and missing explanations in the
// rendered documentation corpus; it never rewrites generated API pages.
package review

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Mpaape/AurumCode/internal/documentation/site"
	"github.com/Mpaape/AurumCode/internal/llm"
)

const (
	defaultPromptPath = ".aurumcode/prompts/documentation/docs-review.md"
	maxCorpusBytes    = 80000
	maxPageBytes      = 9000
	maxSkillBytes     = 12000
)

//go:embed templates/docs-review.md
var embeddedPrompt string

var (
	ErrNoLLMProvider = errors.New("no LLM provider configured for documentation review")
	ErrEmptyResponse = errors.New("documentation review returned an empty response")
)

// Reviewer performs one bounded, repository-local documentation review.
type Reviewer struct {
	orchestrator *llm.Orchestrator
}

// NewReviewer creates a reviewer. A nil orchestrator is valid for automatic
// mode, where deterministic generation remains available.
func NewReviewer(orchestrator *llm.Orchestrator) *Reviewer {
	return &Reviewer{orchestrator: orchestrator}
}

// GenerateOptions identifies the source material and destination of the
// editorial report. SiteDir is the Jekyll root; GeneratedDir is the API output
// inside it (or the legacy output directory for local runs).
type GenerateOptions struct {
	ProjectDir   string
	SiteDir      string
	GeneratedDir string
	OutputPath   string
	Language     string
}

// Generate writes a Jekyll page containing the model's review. The report is
// separate from generated API pages so editorial prose is never mistaken for
// an extracted symbol page.
func (r *Reviewer) Generate(ctx context.Context, opts GenerateOptions) (string, error) {
	if r.orchestrator == nil {
		return "", ErrNoLLMProvider
	}

	readme, err := readFile(filepath.Join(opts.ProjectDir, "README.md"), maxPageBytes)
	if err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("read README.md: %w", err)
	}
	corpus, err := collectCorpus(opts.SiteDir, opts.GeneratedDir)
	if err != nil {
		return "", fmt.Errorf("collect documentation corpus: %w", err)
	}
	skills, err := collectSkills(opts.ProjectDir)
	if err != nil {
		return "", fmt.Errorf("collect documentation skills: %w", err)
	}
	promptTemplate, err := loadPrompt(opts.ProjectDir)
	if err != nil {
		return "", err
	}
	language := strings.TrimSpace(opts.Language)
	if language == "" {
		language = "pt-BR"
	}

	prompt := strings.NewReplacer(
		"{{README_CONTENT}}", site.Redact(readme),
		"{{SITE_CONTENT}}", corpus,
		"{{SKILLS_CONTENT}}", skills,
		"{{LANGUAGE}}", language,
	).Replace(promptTemplate)

	llmOpts := llm.DefaultOptions()
	llmOpts.Temperature = 0.1
	llmOpts.MaxTokens = 2200
	llmOpts.System = "You are a precise technical documentation editor. Use only evidence in the supplied repository material."
	response, err := r.orchestrator.Complete(ctx, prompt, llmOpts)
	if err != nil {
		return "", fmt.Errorf("documentation review LLM call failed: %w", err)
	}
	if strings.TrimSpace(response.Text) == "" {
		return "", ErrEmptyResponse
	}

	content := renderReport(response.Text, language)
	if opts.OutputPath != "" {
		if err := os.MkdirAll(filepath.Dir(opts.OutputPath), 0o755); err != nil {
			return "", fmt.Errorf("create review directory: %w", err)
		}
		if err := os.WriteFile(opts.OutputPath, []byte(content), 0o644); err != nil {
			return "", fmt.Errorf("write documentation review: %w", err)
		}
	}
	return content, nil
}

func loadPrompt(projectDir string) (string, error) {
	path := filepath.Join(projectDir, filepath.FromSlash(defaultPromptPath))
	if data, err := os.ReadFile(path); err == nil {
		return string(data), nil
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("read documentation review prompt %s: %w", defaultPromptPath, err)
	}
	return embeddedPrompt, nil
}

func collectCorpus(siteDir, generatedDir string) (string, error) {
	if strings.TrimSpace(siteDir) == "" {
		siteDir = generatedDir
	}
	root, err := filepath.Abs(siteDir)
	if err != nil {
		return "", err
	}

	var paths []string
	err = filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			if path != root && skipCorpusDir(info.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.EqualFold(filepath.Ext(path), ".md") || info.Name() == "_config.yml" {
			return nil
		}
		paths = append(paths, path)
		return nil
	})
	if os.IsNotExist(err) {
		return "(nenhuma página Markdown encontrada)\n", nil
	}
	if err != nil {
		return "", err
	}
	sort.Strings(paths)

	var b strings.Builder
	used := 0
	for _, path := range paths {
		content, readErr := readFile(path, maxPageBytes)
		if readErr != nil {
			return "", readErr
		}
		content = site.Redact(content)
		remaining := maxCorpusBytes - used
		if remaining <= 0 {
			break
		}
		if len(content) > remaining {
			content = content[:remaining] + "\n...[corpus truncated]"
		}
		rel, _ := filepath.Rel(root, path)
		b.WriteString("\n--- ")
		b.WriteString(filepath.ToSlash(rel))
		b.WriteString(" ---\n")
		b.WriteString(content)
		b.WriteByte('\n')
		used += len(content)
	}
	if b.Len() == 0 {
		return "(nenhuma página Markdown encontrada)\n", nil
	}
	return b.String(), nil
}

func collectSkills(projectDir string) (string, error) {
	dir := filepath.Join(projectDir, ".aurumcode", "skills", "documentation")
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return "(nenhuma skill adicional configurada)\n", nil
	}
	if err != nil {
		return "", err
	}
	var names []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".md") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	var b strings.Builder
	for _, name := range names {
		content, err := readFile(filepath.Join(dir, name), maxSkillBytes)
		if err != nil {
			return "", err
		}
		b.WriteString("\n--- skill: ")
		b.WriteString(name)
		b.WriteString(" ---\n")
		b.WriteString(site.Redact(content))
		b.WriteByte('\n')
	}
	if b.Len() == 0 {
		return "(nenhuma skill adicional configurada)\n", nil
	}
	return b.String(), nil
}

func readFile(path string, maxBytes int) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if len(data) > maxBytes {
		data = append(data[:maxBytes], []byte("\n...[truncated]")...)
	}
	return string(data), nil
}

func skipCorpusDir(name string) bool {
	switch name {
	case ".git", "vendor", "node_modules", "_site", ".jekyll-cache", ".sass-cache", "prompts", "rules", "skills", "reviews":
		return true
	default:
		return false
	}
}

func renderReport(raw, language string) string {
	content := stripFrontMatter(strings.TrimSpace(site.Redact(raw)))
	content = strings.ReplaceAll(content, "{{", "{ {")
	content = strings.ReplaceAll(content, "}}", "} }")
	if content == "" {
		content = "## Resultado\n\nA revisão não produziu conteúdo editorial."
	}
	if !strings.HasPrefix(content, "#") {
		content = "## Resultado\n\n" + content
	}
	return fmt.Sprintf("---\nlayout: default\ntitle: Revisão da documentação\nnav_order: 3\npermalink: /reviews/docs-review/\nlanguage: %s\n---\n\n> Esta revisão editorial é regenerada a cada build a partir do README e das páginas publicadas. Ela não substitui a documentação extraída do código.\n\n%s\n", yamlScalar(language), content)
}

func stripFrontMatter(content string) string {
	if !strings.HasPrefix(content, "---\n") {
		return content
	}
	if end := strings.Index(content[4:], "\n---\n"); end >= 0 {
		return strings.TrimSpace(content[4+end+5:])
	}
	return content
}

func yamlScalar(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "\"", "\\\"")
	return `"` + value + `"`
}
