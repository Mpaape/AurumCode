package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/Mpaape/AurumCode/internal/documentation/extractors"
	bashExtractor "github.com/Mpaape/AurumCode/internal/documentation/extractors/bash"
	cppExtractor "github.com/Mpaape/AurumCode/internal/documentation/extractors/cpp"
	goExtractor "github.com/Mpaape/AurumCode/internal/documentation/extractors/go"
	javascriptExtractor "github.com/Mpaape/AurumCode/internal/documentation/extractors/javascript"
	powershellExtractor "github.com/Mpaape/AurumCode/internal/documentation/extractors/powershell"
	pythonExtractor "github.com/Mpaape/AurumCode/internal/documentation/extractors/python"
	"github.com/Mpaape/AurumCode/internal/documentation/site"
	"github.com/Mpaape/AurumCode/internal/llm"
	"github.com/Mpaape/AurumCode/internal/llm/cost"
	litellmProvider "github.com/Mpaape/AurumCode/internal/llm/provider/litellm"
	openaiProvider "github.com/Mpaape/AurumCode/internal/llm/provider/openai"
	"github.com/Mpaape/AurumCode/internal/pipeline"
)

// Pipeline knobs are read from AURUMCODE_-prefixed variables on purpose. The
// composite action already resolves its own `source-dir` input by changing the
// working directory before launching this binary, so honouring a bare
// SOURCE_DIR here would apply the same relative path twice.
const (
	envSourceDir      = "AURUMCODE_SOURCE_DIR"
	envOutputDir      = "AURUMCODE_OUTPUT_DIR"
	envDocsDir        = "AURUMCODE_DOCS_DIR"
	envLanguages      = "AURUMCODE_LANGUAGES"
	envIncremental    = "AURUMCODE_INCREMENTAL"
	envValidateJekyll = "AURUMCODE_VALIDATE_JEKYLL"
	envDeployGHPages  = "AURUMCODE_DEPLOY_GH_PAGES"
)

type extractorAlias struct {
	base extractors.Extractor
	lang extractors.Language
}

func (a *extractorAlias) Extract(ctx context.Context, req *extractors.ExtractRequest) (*extractors.ExtractResult, error) {
	reqCopy := *req
	reqCopy.Language = a.lang
	return a.base.Extract(ctx, &reqCopy)
}

func (a *extractorAlias) Validate(ctx context.Context) error {
	return a.base.Validate(ctx)
}

func (a *extractorAlias) Language() extractors.Language {
	return a.lang
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Println("🚀 AurumCode - Regenerating Complete Documentation")
	log.Println("================================================")

	llmAPIKey := os.Getenv("LLM_API_KEY")
	llmBaseURL := os.Getenv("LLM_BASE_URL")
	openaiAPIKey := os.Getenv("OPENAI_API_KEY")

	var llmOrch *llm.Orchestrator

	switch {
	case llmAPIKey != "" && llmBaseURL != "":
		model := envOrDefault(envLLMModel, defaultLLMModel)

		// This is the primary production path. It used to build its tracker
		// with an empty price table, which cannot convert tokens into dollars,
		// so neither of the ceilings handed to it could ever bind and no spend
		// was ever booked. The budget is now resolved before the provider is
		// built, and a model that cannot be priced stops the run instead of
		// starting an uncapped one.
		budget, err := resolveLLMBudget(model)
		if err != nil {
			log.Fatalf("❌ LLM cost control: %v", err)
		}

		provider := litellmProvider.NewProvider(llmAPIKey, llmBaseURL, model)
		tracker := cost.NewTracker(budget.PerRunUSD, budget.DailyUSD, budget.Prices)
		llmOrch = llm.NewOrchestrator(provider, nil, tracker)
		log.Printf("✓ LiteLLM configured (%s) model=%s", llmBaseURL, model)
		logBudget(budget)
	case llmAPIKey != "" && llmBaseURL == "":
		log.Println("⚠️  LLM_BASE_URL not set - skipping LiteLLM provider")
	default:
		log.Println("ℹ️  LLM_API_KEY not set - LLM features will be disabled (docs generation will still work)")
	}

	if llmOrch == nil && openaiAPIKey != "" {
		// The OpenAI provider substitutes "gpt-4" when the caller names no
		// model, and that is the key the orchestrator resolves and charges, so
		// that is the key the price table has to carry.
		budget, err := resolveLLMBudget(openaiDefaultModel)
		if err != nil {
			log.Fatalf("❌ LLM cost control: %v", err)
		}

		provider := openaiProvider.NewProvider(openaiAPIKey)
		tracker := cost.NewTracker(budget.PerRunUSD, budget.DailyUSD, budget.Prices)
		llmOrch = llm.NewOrchestrator(provider, nil, tracker)
		log.Printf("✓ OpenAI provider configured model=%s", openaiDefaultModel)
		logBudget(budget)
	}

	if llmOrch != nil {
		log.Printf("✓ LLM Orchestrator created (providers: %v)", llmOrch.GetProviderChain())
	} else {
		log.Println("⚠️  No LLM provider configured - welcome page generation disabled")
	}

	runner := site.NewDefaultRunner()

	config, err := resolveConfig(llmOrch != nil)
	if err != nil {
		log.Fatalf("❌ Invalid configuration: %v", err)
	}
	log.Printf("✓ Source: %s | Output: %s | Docs: %s", config.SourceDir, config.OutputDir, config.DocsDir)

	extractorPipeline := pipeline.NewExtractorPipeline(config, runner, llmOrch)
	if err := registerLanguageExtractors(extractorPipeline, runner); err != nil {
		log.Fatalf("❌ Failed to register language extractors: %v", err)
	}

	log.Println("\n📝 Running Documentation Extraction...")
	log.Println("────────────────────────────────────────")

	ctx := context.Background()
	runErr := extractorPipeline.Run(ctx)

	docs, err := collectMarkdown(config)
	if err != nil {
		log.Fatalf("❌ Failed to inspect generated documentation in %s: %v", config.OutputDir, err)
	}

	log.Println("────────────────────────────────────────")
	report(runErr, docs, inspectSite(config), config)

	log.Printf("\n📝 Custom pages location:\n   - %s (your guides, tutorials, etc.)", config.DocsDir)
	log.Printf("\n🌐 Build Jekyll site with:\n   cd %s && bundle install && bundle exec jekyll build", config.DocsDir)

	if llmOrch != nil {
		perRun, daily := llmOrch.RemainingBudget()
		log.Printf("\n💰 Remaining LLM budget: per-run $%.2f | daily $%.2f", perRun, daily)

		// A completion that could not be priced was paid for but booked
		// nowhere. Reporting zero silently would make an uncapped run
		// indistinguishable from a metered one.
		if n := llmOrch.UntrackedSpends(); n > 0 {
			log.Printf("⚠️  %d LLM completion(s) were paid for but charged to no budget "+
				"(no price entry for the model in use)", n)
		}
	}
}

// openaiDefaultModel mirrors the model the OpenAI provider substitutes when the
// caller names none. The budget has to be keyed on the same string, or the
// ceiling and the charge look at different rows.
const openaiDefaultModel = "gpt-4"

// logBudget makes the enforcement status of a run explicit. A run without a
// price table has no ceiling, and that must be stated rather than implied by
// two large dollar figures in the log.
func logBudget(budget llmBudget) {
	if !budget.Enforced {
		log.Printf("⚠️  LLM cost control DISABLED (%s) - spend is unmetered and uncapped", budget.Source)
		return
	}
	log.Printf("✓ LLM budget: per-run $%.2f | daily $%.2f (prices from %s)",
		budget.PerRunUSD, budget.DailyUSD, budget.Source)
}

// siteStatus records whether the run left behind the two files that turn the
// generated markdown into something a static host can serve. Markdown alone is
// not a site: with no index.md the published root is a 404, and with no
// _config.yml the host has no reason to render the pages.
type siteStatus struct {
	IndexPath  string
	ConfigPath string
	HasIndex   bool
	HasConfig  bool
}

// Servable reports whether the artifact can be published as a site.
func (s siteStatus) Servable() bool { return s.HasIndex && s.HasConfig }

func inspectSite(config *pipeline.ExtractorPipelineConfig) siteStatus {
	docsDir := config.DocsDir
	if docsDir == "" {
		docsDir = config.OutputDir
	}

	status := siteStatus{
		IndexPath:  filepath.ToSlash(filepath.Join(docsDir, "index.md")),
		ConfigPath: filepath.ToSlash(filepath.Join(docsDir, "_config.yml")),
	}

	status.HasIndex = isRegularFile(status.IndexPath)
	status.HasConfig = isRegularFile(status.ConfigPath)

	return status
}

func isRegularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// fatalf is the process-terminating log call used by report. It is a variable so
// a test can observe the message and the branch that produced it without killing
// the test binary; the released build always resolves to log.Fatalf, and the exit
// status it produces is covered separately by a subprocess test.
var fatalf = log.Fatalf

// report turns the pipeline outcome into an exit status that matches what is
// actually on disk. A run only claims success for markdown files that exist,
// and a partially extracted run is reported as such instead of being flattened
// into either a clean success or a total failure.
func report(runErr error, docs []string, siteInfo siteStatus, config *pipeline.ExtractorPipelineConfig) {
	var extractionErr *pipeline.ExtractionError
	structured := errors.As(runErr, &extractionErr)
	partial := structured && extractionErr.Partial

	switch {
	case runErr != nil && !partial:
		log.Printf("❌ FAILED - no documentation was produced")
		logDiagnostics(extractionErr)
		log.Print(summaryLine("failed", docs, siteInfo, extractionErr, config))
		fatalf("❌ Pipeline failed: %v", runErr)

	case partial && len(docs) == 0:
		log.Print(summaryLine("failed", docs, siteInfo, extractionErr, config))
		fatalf("❌ Pipeline reported partial extraction but no markdown file exists under %s: %v",
			config.OutputDir, runErr)

	case partial:
		log.Printf("⚠️  PARTIAL SUCCESS - %d language(s) skipped, %d extraction error(s)",
			len(extractionErr.Skipped), len(extractionErr.Errors))
		logDiagnostics(extractionErr)
		logDocs(docs, config.OutputDir)
		logSite(siteInfo)
		log.Print(summaryLine("partial", docs, siteInfo, extractionErr, config))

	case len(docs) == 0:
		log.Printf("⚠️  NO DOCUMENTATION PRODUCED - no supported source file was documented under %s", config.SourceDir)
		log.Print(summaryLine("empty", docs, siteInfo, nil, config))

	default:
		log.Println("✅ Documentation regeneration completed!")
		logDocs(docs, config.OutputDir)
		logSite(siteInfo)
		log.Print(summaryLine("ok", docs, siteInfo, nil, config))
	}
}

// logSite makes an unpublishable artifact visible in the run log instead of
// letting a green exit status imply a working site.
func logSite(siteInfo siteStatus) {
	if siteInfo.Servable() {
		log.Printf("\n🌐 Site scaffold:\n   - %s\n   - %s", siteInfo.IndexPath, siteInfo.ConfigPath)
		return
	}

	if !siteInfo.HasIndex {
		log.Printf("⚠️  %s is missing - a published site would answer 404 at its root", siteInfo.IndexPath)
	}
	if !siteInfo.HasConfig {
		log.Printf("⚠️  %s is missing - the published pages would be served as raw markdown", siteInfo.ConfigPath)
	}
}

func logDiagnostics(extractionErr *pipeline.ExtractionError) {
	if extractionErr == nil {
		return
	}

	for _, skip := range extractionErr.Skipped {
		log.Printf("   - skipped: %s", skip)
	}
	for _, cause := range extractionErr.Errors {
		log.Printf("   - failed: %v", cause)
	}
}

// summaryLine is the machine-readable verdict of the run. It is the single line
// a caller can grep to tell a complete run from a degraded one.
func summaryLine(result string, docs []string, siteInfo siteStatus, extractionErr *pipeline.ExtractionError, config *pipeline.ExtractorPipelineConfig) string {
	skippedCount, failedCount := 0, 0
	languages := "none"

	if extractionErr != nil {
		skippedCount = len(extractionErr.Skipped)
		failedCount = len(extractionErr.Errors)

		if skippedCount > 0 {
			names := make([]string, 0, skippedCount)
			for _, skip := range extractionErr.Skipped {
				names = append(names, string(skip.Language))
			}
			languages = strings.Join(names, ",")
		}
	}

	return fmt.Sprintf("aurumcode: result=%s docs=%d skipped=%d failed=%d languages_skipped=%s output=%s index=%t config=%t",
		result, len(docs), skippedCount, failedCount, languages, config.OutputDir,
		siteInfo.HasIndex, siteInfo.HasConfig)
}

func logDocs(docs []string, outputDir string) {
	log.Printf("\n📊 Generated %d markdown file(s) in %s:", len(docs), outputDir)
	for _, doc := range docs {
		log.Printf("   - %s", doc)
	}
}

// collectMarkdown lists the markdown files that really exist under the output
// directory. A missing directory is not an error: it simply means nothing was
// generated. The site landing page is excluded on purpose - it is scaffolding
// the pipeline writes for every run, so counting it would let an empty run
// claim it documented something.
func collectMarkdown(config *pipeline.ExtractorPipelineConfig) ([]string, error) {
	docsDir := config.DocsDir
	if docsDir == "" {
		docsDir = config.OutputDir
	}
	landingPage := filepath.Clean(filepath.Join(docsDir, "index.md"))

	var docs []string

	err := filepath.Walk(config.OutputDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.EqualFold(filepath.Ext(path), ".md") {
			return nil
		}
		if filepath.Clean(path) == landingPage {
			return nil
		}

		docs = append(docs, filepath.ToSlash(path))
		return nil
	})

	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return docs, nil
}

func resolveConfig(generateWelcome bool) (*pipeline.ExtractorPipelineConfig, error) {
	// The source tree is resolved to an absolute path before it reaches the
	// pipeline: the Go extractor skips every directory whose base name starts
	// with a dot, so a literal "." would skip the whole tree, and gomarkdoc
	// rejects a bare relative package argument as an import path.
	sourceDir, err := filepath.Abs(envOrDefault(envSourceDir, "."))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", envSourceDir, err)
	}

	info, err := os.Stat(sourceDir)
	if err != nil {
		return nil, fmt.Errorf("source directory %s: %w", sourceDir, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("source directory %s is not a directory", sourceDir)
	}

	outputDir := envOrDefault(envOutputDir, ".aurumcode")

	incremental, err := envBool(envIncremental)
	if err != nil {
		return nil, err
	}

	validateJekyll, err := envBool(envValidateJekyll)
	if err != nil {
		return nil, err
	}

	deployGHPages, err := envBool(envDeployGHPages)
	if err != nil {
		return nil, err
	}
	if deployGHPages {
		return nil, fmt.Errorf("%s is set but gh-pages deployment is not implemented in this build; "+
			"publish the contents of the output directory with a dedicated step instead", envDeployGHPages)
	}

	return &pipeline.ExtractorPipelineConfig{
		SourceDir:       sourceDir,
		OutputDir:       outputDir,
		DocsDir:         envOrDefault(envDocsDir, outputDir),
		Languages:       splitLanguages(os.Getenv(envLanguages)),
		Incremental:     incremental,
		GenerateWelcome: generateWelcome,
		ValidateJekyll:  validateJekyll,
		DeployGHPages:   false,
	}, nil
}

func envOrDefault(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func envBool(name string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(name))) {
	case "", "0", "false", "no":
		return false, nil
	case "1", "true", "yes":
		return true, nil
	default:
		return false, fmt.Errorf("%s must be one of true/false (got %q)", name, os.Getenv(name))
	}
}

func splitLanguages(raw string) []string {
	languages := []string{}
	for _, part := range strings.Split(raw, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			languages = append(languages, trimmed)
		}
	}
	return languages
}

func registerLanguageExtractors(p *pipeline.ExtractorPipeline, runner site.CommandRunner) error {
	register := func(ext extractors.Extractor) error {
		if err := p.RegisterExtractor(ext); err != nil {
			return fmt.Errorf("register %s extractor: %w", ext.Language(), err)
		}
		return nil
	}

	if err := register(goExtractor.NewGoExtractor(runner)); err != nil {
		return err
	}

	jsExtractor := javascriptExtractor.NewJSExtractor(runner)
	if err := register(jsExtractor); err != nil {
		return err
	}

	if err := register(&extractorAlias{base: jsExtractor, lang: extractors.LanguageTypeScript}); err != nil {
		return err
	}

	if err := register(pythonExtractor.NewPythonExtractor(runner)); err != nil {
		return err
	}

	if err := register(cppExtractor.NewCPPExtractor(runner)); err != nil {
		return err
	}

	// Rust and C# are NOT registered here. Their toolchains execute code that
	// lives in the documented repository, so they are opt-in per language; see
	// repo_code_execution.go for what each one runs and why there is no safe
	// mode to fall back to.
	if err := registerRepoCodeExecutingExtractors(register, runner, os.Getenv(envAllowRepoCodeExecution)); err != nil {
		return err
	}

	if err := register(bashExtractor.NewBashExtractor(runner)); err != nil {
		return err
	}

	if err := register(powershellExtractor.NewPowerShellExtractor(runner)); err != nil {
		return err
	}

	return nil
}
