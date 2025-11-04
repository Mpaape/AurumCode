package pipeline

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/Mpaape/AurumCode/internal/documentation/extractors"
	"github.com/Mpaape/AurumCode/internal/documentation/incremental"
	"github.com/Mpaape/AurumCode/internal/documentation/normalizer"
	"github.com/Mpaape/AurumCode/internal/documentation/site"
	"github.com/Mpaape/AurumCode/internal/documentation/welcome"
	"github.com/Mpaape/AurumCode/internal/llm"
)

// ExtractorPipelineConfig configures the documentation extraction pipeline
type ExtractorPipelineConfig struct {
	SourceDir       string   // Root directory of source code
	OutputDir       string   // Output directory for documentation
	DocsDir         string   // Jekyll docs directory (e.g., "docs/")
	Languages       []string // Languages to extract (empty = all)
	Incremental     bool     // Enable incremental mode
	GenerateWelcome bool     // Generate LLM-powered welcome page
	ValidateJekyll  bool     // Validate Jekyll site after generation
	DeployGHPages   bool     // Deploy to gh-pages branch
}

// ExtractorPipeline orchestrates complete documentation extraction and site generation
type ExtractorPipeline struct {
	config         *ExtractorPipelineConfig
	registry       *extractors.Registry
	runner         site.CommandRunner
	incrementalMgr *incremental.Manager
	normalizer     *normalizer.Normalizer
	welcomeGen     *welcome.Generator
	llmOrch        *llm.Orchestrator
}

// NewExtractorPipeline creates a new documentation extraction pipeline
func NewExtractorPipeline(
	config *ExtractorPipelineConfig,
	runner site.CommandRunner,
	llmOrch *llm.Orchestrator,
) *ExtractorPipeline {
	registry := extractors.NewRegistry()

	// Register all extractors (assuming they're already registered in init())
	// This would be done in the main package or via extractors.RegisterAll()

	return &ExtractorPipeline{
		config:         config,
		registry:       registry,
		runner:         runner,
		incrementalMgr: incremental.NewManager(runner, config.SourceDir),
		normalizer:     normalizer.NewNormalizer(config.DocsDir),
		welcomeGen:     welcome.NewGenerator(llmOrch),
		llmOrch:        llmOrch,
	}
}

// RegisterExtractor registers a language extractor with the pipeline registry.
func (p *ExtractorPipeline) RegisterExtractor(extractor extractors.Extractor) error {
	return p.registry.Register(extractor)
}

// Run executes the complete documentation pipeline
func (p *ExtractorPipeline) Run(ctx context.Context) error {
	log.Printf("[Pipeline] Starting documentation extraction pipeline")
	log.Printf("[Pipeline] Source: %s, Output: %s", p.config.SourceDir, p.config.OutputDir)

	// Step 1: Determine what needs to be extracted
	filesToProcess, err := p.determineFilesToProcess(ctx)
	if err != nil {
		return fmt.Errorf("failed to determine files to process: %w", err)
	}

	if len(filesToProcess) == 0 {
		log.Printf("[Pipeline] No files to process")
		return nil
	}

	log.Printf("[Pipeline] Found %d files to process", len(filesToProcess))

	// Step 2: Extract documentation for each language
	stats, errors := p.extractDocumentation(ctx, filesToProcess)

	// Log statistics
	log.Printf("[Pipeline] Extraction complete: %d files processed, %d docs generated",
		stats.FilesProcessed, stats.DocsGenerated)

	if len(errors) > 0 {
		log.Printf("[Pipeline] %d extraction errors occurred", len(errors))
		for _, err := range errors {
			log.Printf("[Pipeline] Error: %v", err)
		}
	}

	// Step 3: Normalize markdown files with Jekyll front matter
	if stats.DocsGenerated > 0 {
		log.Printf("[Pipeline] Normalizing markdown files...")
		normalized, normErrors := p.normalizer.NormalizeDir(p.config.OutputDir)
		log.Printf("[Pipeline] Normalized %d markdown files", normalized)

		if len(normErrors) > 0 {
			log.Printf("[Pipeline] %d normalization errors occurred", len(normErrors))
		}
	}

	// Step 4: Generate LLM-powered welcome page if enabled
	if p.config.GenerateWelcome && p.llmOrch != nil {
		log.Printf("[Pipeline] Generating welcome page...")
		if err := p.generateWelcomePage(ctx); err != nil {
			log.Printf("[Pipeline] Warning: Welcome page generation failed: %v", err)
		} else {
			log.Printf("[Pipeline] Welcome page generated successfully")
		}
	}

	// Step 5: Validate Jekyll site if enabled
	if p.config.ValidateJekyll {
		log.Printf("[Pipeline] Validating Jekyll site...")
		if err := p.validateJekyllSite(ctx); err != nil {
			log.Printf("[Pipeline] Warning: Jekyll validation failed: %v", err)
		} else {
			log.Printf("[Pipeline] Jekyll site validation successful")
		}
	}

	// Step 6: Deploy to gh-pages if enabled
	if p.config.DeployGHPages {
		log.Printf("[Pipeline] Deploying to gh-pages...")
		if err := p.deployToGHPages(ctx); err != nil {
			return fmt.Errorf("gh-pages deployment failed: %w", err)
		}
		log.Printf("[Pipeline] Deployed to gh-pages successfully")
	}

	// Step 7: Update incremental cache
	if p.config.Incremental {
		log.Printf("[Pipeline] Updating incremental cache...")
		if err := p.incrementalMgr.UpdateCommit(ctx); err != nil {
			log.Printf("[Pipeline] Warning: Failed to update cache: %v", err)
		}
		if err := p.incrementalMgr.SaveCache(); err != nil {
			log.Printf("[Pipeline] Warning: Failed to save cache: %v", err)
		}
	}

	log.Printf("[Pipeline] Documentation pipeline completed successfully")
	return nil
}

// determineFilesToProcess determines which files need documentation extraction
func (p *ExtractorPipeline) determineFilesToProcess(ctx context.Context) (map[extractors.Language][]string, error) {
	files := make(map[extractors.Language][]string)

	if p.config.Incremental {
		// Load existing cache
		if err := p.incrementalMgr.LoadCache(); err != nil {
			log.Printf("[Pipeline] Warning: Failed to load cache: %v", err)
		}

		// Get changed files
		changedFiles, err := p.incrementalMgr.GetChangedFiles(ctx)
		if err != nil {
			return nil, err
		}

		log.Printf("[Pipeline] Incremental mode: %d changed files detected", len(changedFiles))

		// Group by language
		files = p.groupFilesByLanguage(changedFiles)
	} else {
		// Full extraction mode - find all source files
		log.Printf("[Pipeline] Full extraction mode")

		var allFiles []string
		err := filepath.Walk(p.config.SourceDir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}

			// Skip common non-source directories
			if shouldSkipPath(path) {
				return nil
			}

			allFiles = append(allFiles, path)
			return nil
		})

		if err != nil {
			return nil, err
		}

		files = p.groupFilesByLanguage(allFiles)
	}

	// Filter by configured languages if specified
	if len(p.config.Languages) > 0 {
		filtered := make(map[extractors.Language][]string)
		for _, langStr := range p.config.Languages {
			lang := extractors.Language(langStr)
			if fileList, ok := files[lang]; ok {
				filtered[lang] = fileList
			}
		}
		files = filtered
	}

	return files, nil
}

// groupFilesByLanguage groups files by their programming language
func (p *ExtractorPipeline) groupFilesByLanguage(files []string) map[extractors.Language][]string {
	grouped := make(map[extractors.Language][]string)

	for _, file := range files {
		lang := detectLanguageFromFile(file)
		if lang != "" {
			grouped[lang] = append(grouped[lang], file)
		}
	}

	return grouped
}

// extractDocumentation extracts documentation for all files
func (p *ExtractorPipeline) extractDocumentation(
	ctx context.Context,
	filesByLanguage map[extractors.Language][]string,
) (extractors.ExtractionStats, []error) {

	totalStats := extractors.ExtractionStats{}
	var allErrors []error

	for lang, files := range filesByLanguage {
		log.Printf("[Pipeline] Extracting %s documentation (%d files)...", lang, len(files))

		extractor, err := p.registry.Get(lang)
		if err != nil {
			allErrors = append(allErrors, fmt.Errorf("no extractor for %s: %w", lang, err))
			continue
		}

		// Validate extractor is available
		if err := extractor.Validate(ctx); err != nil {
			allErrors = append(allErrors, fmt.Errorf("%s tools not available: %w", lang, err))
			continue
		}

		// Extract documentation
		request := &extractors.ExtractRequest{
			Language:  lang,
			SourceDir: p.config.SourceDir,
			OutputDir: filepath.Join(p.config.OutputDir, string(lang)),
		}

		result, err := extractor.Extract(ctx, request)
		if err != nil {
			allErrors = append(allErrors, fmt.Errorf("%s extraction failed: %w", lang, err))
			continue
		}

		// Aggregate statistics
		totalStats.FilesProcessed += result.Stats.FilesProcessed
		totalStats.DocsGenerated += result.Stats.DocsGenerated

		// Track errors
		allErrors = append(allErrors, result.Errors...)

		// Register in incremental cache
		if p.config.Incremental {
			for _, file := range files {
				p.incrementalMgr.RegisterDocumentation(file, result.Files...)
			}
			p.incrementalMgr.RegisterLanguage(string(lang), files...)
		}

		log.Printf("[Pipeline] %s: %d files processed, %d docs generated",
			lang, result.Stats.FilesProcessed, result.Stats.DocsGenerated)
	}

	return totalStats, allErrors
}

// generateWelcomePage generates LLM-powered welcome page from README
func (p *ExtractorPipeline) generateWelcomePage(ctx context.Context) error {
	readmePath := filepath.Join(p.config.SourceDir, "README.md")
	if _, err := os.Stat(readmePath); os.IsNotExist(err) {
		return fmt.Errorf("README.md not found")
	}

	opts := welcome.GenerateOptions{
		ReadmePath: readmePath,
		OutputPath: filepath.Join(p.config.DocsDir, "index.md"),
		ProjectDir: p.config.SourceDir,
		Title:      "Home",
		NavOrder:   1,
	}

	_, err := p.welcomeGen.Generate(ctx, opts)
	return err
}

// validateJekyllSite validates the Jekyll site can be built
func (p *ExtractorPipeline) validateJekyllSite(ctx context.Context) error {
	// Check if Jekyll is available
	_, err := p.runner.Run(ctx, "bundle", []string{"--version"}, p.config.DocsDir, nil)
	if err != nil {
		return fmt.Errorf("bundler not available")
	}

	// Try to build the site
	_, err = p.runner.Run(ctx, "bundle", []string{"exec", "jekyll", "build"}, p.config.DocsDir, nil)
	if err != nil {
		return fmt.Errorf("jekyll build failed: %w", err)
	}

	return nil
}

// deployToGHPages deploys documentation to gh-pages branch
func (p *ExtractorPipeline) deployToGHPages(ctx context.Context) error {
	// This would implement gh-pages deployment logic
	// For now, just a placeholder
	log.Printf("[Pipeline] gh-pages deployment not yet implemented")
	return nil
}

// detectLanguageFromFile detects language from file extension
func detectLanguageFromFile(file string) extractors.Language {
	ext := filepath.Ext(file)

	switch ext {
	case ".go":
		return extractors.LanguageGo
	case ".js", ".mjs":
		return extractors.LanguageJavaScript
	case ".ts":
		return extractors.LanguageTypeScript
	case ".py":
		return extractors.LanguagePython
	case ".cs":
		return extractors.LanguageCSharp
	case ".java":
		return extractors.LanguageJava
	case ".cpp", ".cc", ".cxx", ".h", ".hpp":
		return extractors.LanguageCPP
	case ".rs":
		return extractors.LanguageRust
	case ".sh":
		return extractors.LanguageBash
	case ".ps1", ".psm1":
		return extractors.LanguagePowerShell
	default:
		return ""
	}
}

// shouldSkipPath checks if path should be skipped during file discovery
func shouldSkipPath(path string) bool {
	skipDirs := map[string]struct{}{
		"node_modules": {},
		".git":         {},
		".github":      {},
		"vendor":       {},
		"target":       {},
		"dist":         {},
		"build":        {},
		"_site":        {},
		".taskmaster":  {},
		".aurumcode":   {},
	}

	clean := filepath.Clean(path)
	for {
		base := filepath.Base(clean)
		if _, skip := skipDirs[base]; skip {
			return true
		}

		parent := filepath.Dir(clean)
		if parent == clean || parent == "." || parent == string(filepath.Separator) {
			break
		}
		clean = parent
	}

	return false
}
