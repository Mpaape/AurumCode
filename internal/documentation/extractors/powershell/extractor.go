package powershell

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Mpaape/AurumCode/internal/documentation/extractors"
	"github.com/Mpaape/AurumCode/internal/documentation/site"
)

// PowerShellExtractor extracts documentation from PowerShell scripts
type PowerShellExtractor struct {
	runner site.CommandRunner
}

// NewPowerShellExtractor creates a new PowerShell documentation extractor
func NewPowerShellExtractor(runner site.CommandRunner) *PowerShellExtractor {
	return &PowerShellExtractor{
		runner: runner,
	}
}

// Extract generates documentation from PowerShell scripts
func (p *PowerShellExtractor) Extract(ctx context.Context, req *extractors.ExtractRequest) (*extractors.ExtractResult, error) {
	if req.Language != extractors.LanguagePowerShell {
		return nil, fmt.Errorf("invalid language: expected %s, got %s", extractors.LanguagePowerShell, req.Language)
	}

	if _, err := os.Stat(req.SourceDir); err != nil {
		return nil, fmt.Errorf("invalid source directory: %w", err)
	}

	if err := os.MkdirAll(req.OutputDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	// Find PowerShell scripts
	scripts, err := p.findPowerShellScripts(req.SourceDir)
	if err != nil {
		return nil, fmt.Errorf("failed to find PowerShell scripts: %w", err)
	}

	if len(scripts) == 0 {
		return &extractors.ExtractResult{
			Language: extractors.LanguagePowerShell,
			Files:    []string{},
			Stats:    extractors.ExtractionStats{},
		}, nil
	}

	// Extract documentation from each script
	result := &extractors.ExtractResult{
		Language: extractors.LanguagePowerShell,
		Files:    []string{},
		Stats:    extractors.ExtractionStats{},
	}

	for _, script := range scripts {
		outputPath := filepath.Join(req.OutputDir, filepath.Base(script)+".md")
		if err := p.extractScriptDocs(script, outputPath); err != nil {
			result.Errors = append(result.Errors, err)
			continue
		}
		result.Files = append(result.Files, outputPath)
		result.Stats.FilesProcessed++
		result.Stats.DocsGenerated++
	}

	return result, nil
}

// Validate checks if PowerShell is available
func (p *PowerShellExtractor) Validate(ctx context.Context) error {
	_, err := p.runner.Run(ctx, "pwsh", []string{"-Version"}, ".", nil)
	if err != nil {
		return fmt.Errorf("pwsh not found: please install from https://aka.ms/powershell")
	}
	return nil
}

// Language returns the language this extractor handles
func (p *PowerShellExtractor) Language() extractors.Language {
	return extractors.LanguagePowerShell
}

func (p *PowerShellExtractor) findPowerShellScripts(rootDir string) ([]string, error) {
	scripts := []string{}
	err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext == ".ps1" || ext == ".psm1" {
			scripts = append(scripts, path)
		}
		return nil
	})
	return scripts, err
}

func (p *PowerShellExtractor) extractScriptDocs(scriptPath, outputPath string) error {
	content, err := os.ReadFile(scriptPath)
	if err != nil {
		return err
	}

	var doc strings.Builder
	doc.WriteString(fmt.Sprintf("# %s\n\n", filepath.Base(scriptPath)))

	lines := strings.Split(string(content), "\n")
	inComment := false
	inBlockComment := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Handle block comments
		if strings.HasPrefix(trimmed, "<#") {
			inBlockComment = true
			doc.WriteString("## Documentation\n\n")
			continue
		}
		if strings.HasSuffix(trimmed, "#>") {
			inBlockComment = false
			doc.WriteString("\n")
			continue
		}
		if inBlockComment {
			doc.WriteString(trimmed)
			doc.WriteString("\n")
			continue
		}

		// Handle single line comments
		if strings.HasPrefix(trimmed, "#") {
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
