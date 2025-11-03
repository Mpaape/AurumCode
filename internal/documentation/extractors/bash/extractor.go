package bash

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Mpaape/AurumCode/internal/documentation/extractors"
	"github.com/Mpaape/AurumCode/internal/documentation/site"
)

// BashExtractor extracts documentation from Bash scripts
type BashExtractor struct {
	runner site.CommandRunner
}

// NewBashExtractor creates a new Bash documentation extractor
func NewBashExtractor(runner site.CommandRunner) *BashExtractor {
	return &BashExtractor{
		runner: runner,
	}
}

// Extract generates documentation from Bash scripts
func (b *BashExtractor) Extract(ctx context.Context, req *extractors.ExtractRequest) (*extractors.ExtractResult, error) {
	if req.Language != extractors.LanguageBash {
		return nil, fmt.Errorf("invalid language: expected %s, got %s", extractors.LanguageBash, req.Language)
	}

	if _, err := os.Stat(req.SourceDir); err != nil {
		return nil, fmt.Errorf("invalid source directory: %w", err)
	}

	if err := os.MkdirAll(req.OutputDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	// Find Bash scripts
	scripts, err := b.findBashScripts(req.SourceDir)
	if err != nil {
		return nil, fmt.Errorf("failed to find Bash scripts: %w", err)
	}

	if len(scripts) == 0 {
		return &extractors.ExtractResult{
			Language: extractors.LanguageBash,
			Files:    []string{},
			Stats:    extractors.ExtractionStats{},
		}, nil
	}

	// Extract documentation from each script
	result := &extractors.ExtractResult{
		Language: extractors.LanguageBash,
		Files:    []string{},
		Stats:    extractors.ExtractionStats{},
	}

	for _, script := range scripts {
		outputPath := filepath.Join(req.OutputDir, filepath.Base(script)+".md")
		if err := b.extractScriptDocs(script, outputPath); err != nil {
			result.Errors = append(result.Errors, err)
			continue
		}
		result.Files = append(result.Files, outputPath)
		result.Stats.FilesProcessed++
		result.Stats.DocsGenerated++
	}

	return result, nil
}

// Validate checks if Bash is available
func (b *BashExtractor) Validate(ctx context.Context) error {
	_, err := b.runner.Run(ctx, "bash", []string{"--version"}, ".", nil)
	if err != nil {
		return fmt.Errorf("bash not found")
	}
	return nil
}

// Language returns the language this extractor handles
func (b *BashExtractor) Language() extractors.Language {
	return extractors.LanguageBash
}

func (b *BashExtractor) findBashScripts(rootDir string) ([]string, error) {
	scripts := []string{}
	err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		ext := filepath.Ext(path)
		if ext == ".sh" || ext == ".bash" {
			scripts = append(scripts, path)
		}
		return nil
	})
	return scripts, err
}

func (b *BashExtractor) extractScriptDocs(scriptPath, outputPath string) error {
	content, err := os.ReadFile(scriptPath)
	if err != nil {
		return err
	}

	var doc strings.Builder
	doc.WriteString(fmt.Sprintf("# %s\n\n", filepath.Base(scriptPath)))

	lines := strings.Split(string(content), "\n")
	inComment := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") && !strings.HasPrefix(trimmed, "#!/") {
			if !inComment {
				doc.WriteString("## Documentation\n\n")
				inComment = true
			}
			comment := strings.TrimPrefix(trimmed, "#")
			doc.WriteString(strings.TrimSpace(comment))
			doc.WriteString("\n")
		} else if inComment && trimmed != "" {
			inComment = false
			doc.WriteString("\n")
		}
	}

	return os.WriteFile(outputPath, []byte(doc.String()), 0644)
}
