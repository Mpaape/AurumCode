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
	csharpExtractor "github.com/Mpaape/AurumCode/internal/documentation/extractors/csharp"
	goExtractor "github.com/Mpaape/AurumCode/internal/documentation/extractors/go"
	javascriptExtractor "github.com/Mpaape/AurumCode/internal/documentation/extractors/javascript"
	powershellExtractor "github.com/Mpaape/AurumCode/internal/documentation/extractors/powershell"
	pythonExtractor "github.com/Mpaape/AurumCode/internal/documentation/extractors/python"
	rustExtractor "github.com/Mpaape/AurumCode/internal/documentation/extractors/rust"
	"github.com/Mpaape/AurumCode/internal/documentation/site"
	"github.com/Mpaape/AurumCode/internal/llm"
	"github.com/Mpaape/AurumCode/internal/llm/cost"
	litellmProvider "github.com/Mpaape/AurumCode/internal/llm/provider/litellm"
	openaiProvider "github.com/Mpaape/AurumCode/internal/llm/provider/openai"
	"github.com/Mpaape/AurumCode/internal/pipeline"
	"gopkg.in/yaml.v3"
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
	envBaseURL        = "AURUMCODE_BASE_URL"
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
	report(runErr, docs, inspectSite(config, docs), config)

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

// siteStatus records what this run's own generated artifacts say about
// whether the output directory is a working, publishable site. Markdown alone
// is not a site: with no _config.yml the host has no reason to render the
// pages, and a file named index.md that lists zero pages is not a landing
// page a visitor can use.
//
// Earlier this recorded HasIndex as "does index.md exist as a regular file".
// That is a false green: a Jekyll build renders index.md from whatever the
// scaffold wrote to it regardless of whether the pages it links to are
// actually published, and a consumer's own _config.yml `exclude:` list can
// hide every one of them without the file ever stopping existing. This
// binary shipped 0 with index=true config=true having just written a home
// page with 85 dead links for exactly that reason. IndexPages and
// ExcludedPages replace the existence check with the real count.
type siteStatus struct {
	IndexPath  string
	ConfigPath string
	HasConfig  bool

	// IndexPages is how many documentation pages this run generated: the
	// pages a site scaffold pointed at this output would list on its index.
	// It comes from docs, the same list collectMarkdown produced for the
	// docs=%d field of the summary line, because internal/pipeline's
	// ExtractorPipeline.Run returns only an error - the ScaffoldResult it
	// computes internally (including its own exclude-aware page count) never
	// reaches this binary.
	IndexPages int

	// ExcludedPages is how many of those pages the consumer's own Jekyll
	// _config.yml `exclude:` list hides from the build. Each one is a link
	// the index carries that a published site answers with a 404.
	ExcludedPages int
}

// Servable reports whether the artifact can be published as a site: a
// _config.yml for the host to render pages with, and at least one page on
// the index to render them from.
//
// This deliberately does not also require ExcludedPages to be zero. A site
// with some excluded pages is still a site with working links; the case
// where every listed page is excluded is a distinct failure mode that report
// checks for on its own, because it demands a non-zero exit rather than a
// downgraded "servable".
func (s siteStatus) Servable() bool { return s.HasConfig && s.IndexPages > 0 }

// AllPagesExcluded reports the ghost-index defect this run's own scaffold
// cannot see on its own: at least one page was generated and listed, and the
// consumer's own exclude list hides every single one of them. index.md still
// exists, _config.yml still exists, and every link the index carries answers
// 404 anyway.
func (s siteStatus) AllPagesExcluded() bool {
	return s.IndexPages > 0 && s.ExcludedPages == s.IndexPages
}

// inspectSite reports what this run's own generated artifacts say about
// publishability. docs is the list of markdown pages collectMarkdown found
// for this run - the authoritative answer to "what did this run actually
// produce", and the same list a site scaffold pointed at this output would
// list on its index.
func inspectSite(config *pipeline.ExtractorPipelineConfig, docs []string) siteStatus {
	docsDir := config.DocsDir
	if docsDir == "" {
		docsDir = config.OutputDir
	}

	status := siteStatus{
		IndexPath:  filepath.ToSlash(filepath.Join(docsDir, "index.md")),
		ConfigPath: filepath.ToSlash(filepath.Join(docsDir, "_config.yml")),
		IndexPages: len(docs),
	}

	status.HasConfig = isRegularFile(status.ConfigPath)
	if status.HasConfig {
		status.ExcludedPages = excludedPageCount(status.ConfigPath, docsDir, docs)
	}

	return status
}

func isRegularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// jekyllExcludeConfig reads the one key this binary cares about from a
// consumer-owned _config.yml: the exclude list that keeps Jekyll from
// building a source path into a page. It is deliberately narrow - the rest of
// the file is the consumer's business, and a key this binary does not
// recognise must not become a parse error.
type jekyllExcludeConfig struct {
	Exclude []string `yaml:"exclude"`
}

// excludedPageCount reports how many of docs a consumer-owned _config.yml
// hides from the Jekyll build. A missing, unreadable, or malformed config
// excludes nothing rather than erroring: this count is diagnostic, and the
// run itself never depends on it to proceed.
func excludedPageCount(configPath, docsDir string, docs []string) int {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return 0
	}

	var cfg jekyllExcludeConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return 0
	}
	if len(cfg.Exclude) == 0 {
		return 0
	}

	count := 0
	for _, doc := range docs {
		rel, err := filepath.Rel(docsDir, filepath.FromSlash(doc))
		if err != nil {
			continue
		}
		if excludeHidesSource(cfg.Exclude, filepath.ToSlash(rel)) {
			count++
		}
	}
	return count
}

// excludeHidesSource mirrors the exclude-vs-source matching Jekyll performs,
// which internal/documentation/site.Scaffold already computes per run in
// ScaffoldResult.ExcludedPages. That result never reaches this binary -
// ExtractorPipeline.Run returns only an error - so this reproduces the one
// matching predicate rather than duplicating exclude-list parsing (done above
// with a real YAML decoder) or page discovery (done above with docs, already
// collected from disk by collectMarkdown). An exclude entry hides a source
// path when the path is the entry itself or sits under it as a directory.
func excludeHidesSource(excludes []string, source string) bool {
	for _, pattern := range excludes {
		dir := strings.TrimSuffix(strings.TrimSpace(pattern), "/")
		if dir == "" {
			continue
		}
		if source == dir || strings.HasPrefix(source, dir+"/") {
			return true
		}
	}
	return false
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

	case len(docs) > 0 && siteInfo.AllPagesExcluded():
		// The pipeline produced markdown and the scaffold's index lists it, but
		// the consumer's own Jekyll exclude list hides every one of those
		// pages: a published site would answer 404 on every link the index
		// carries. This is the ghost-index defect - index.md exists,
		// _config.yml exists, and the run used to exit 0 anyway - so it is a
		// build failure, not a warning folded into an otherwise green run.
		if partial {
			log.Printf("⚠️  PARTIAL SUCCESS - %d language(s) skipped, %d extraction error(s)",
				len(extractionErr.Skipped), len(extractionErr.Errors))
			logDiagnostics(extractionErr)
		}
		logDocs(docs, config.OutputDir)
		logSite(siteInfo)
		log.Print(summaryLine("unpublishable", docs, siteInfo, extractionErr, config))
		fatalf("❌ %s lists %d page(s) but %s excludes all %d of them: every link a "+
			"published site would carry answers 404",
			siteInfo.IndexPath, siteInfo.IndexPages, siteInfo.ConfigPath, siteInfo.ExcludedPages)

	case partial:
		log.Printf("⚠️  PARTIAL SUCCESS - %d language(s) skipped, %d extraction error(s)",
			len(extractionErr.Skipped), len(extractionErr.Errors))
		logDiagnostics(extractionErr)
		logDocs(docs, config.OutputDir)
		logSite(siteInfo)
		log.Print(summaryLine("partial", docs, siteInfo, extractionErr, config))

	case len(docs) == 0:
		// AUR-425: this branch used to log a warning and let main return,
		// so a run that documented nothing exited 0 and a consumer had no
		// pages and no failure. Zero pages is the outcome this binary exists
		// to prevent; it now states the reason and terminates non-zero. The
		// summary keeps its distinct result=empty token so a machine
		// consumer can still tell "nothing was documentable" from "the
		// pipeline broke" (result=failed).
		log.Printf("❌ NO DOCUMENTATION PRODUCED - %s", emptyRunReason(config))
		log.Print(summaryLine("empty", docs, siteInfo, nil, config))
		fatalf("❌ no documentation page was produced: %s", emptyRunReason(config))

	default:
		log.Println("✅ Documentation regeneration completed!")
		logDocs(docs, config.OutputDir)
		logSite(siteInfo)
		log.Print(summaryLine("ok", docs, siteInfo, nil, config))
	}
}

// emptyRunReason states why a run that reported no pipeline error still left
// zero documentation pages on disk. This branch is only reachable when file
// discovery matched nothing - a detected-but-unsupported language ends as a
// skip inside a pipeline error, never here - so the reason is built from the
// run's own configuration: the tree that was scanned, plus the two knobs
// that can empty a scan of a documentable tree (the language filter and
// incremental mode). Each condition is stated only when it was actually in
// force, so the operator reads causes that apply to their run, not a generic
// list.
func emptyRunReason(config *pipeline.ExtractorPipelineConfig) string {
	reason := fmt.Sprintf("no supported source file was documented under %s", config.SourceDir)
	if len(config.Languages) > 0 {
		reason += fmt.Sprintf("; the language filter %s=%s may exclude every supported file",
			envLanguages, strings.Join(config.Languages, ","))
	}
	if config.Incremental {
		reason += fmt.Sprintf("; incremental mode (%s=true) documents only files changed since the last documented commit",
			envIncremental)
	}
	return reason
}

// logSite makes an unpublishable, or partially unpublishable, artifact visible
// in the run log instead of letting a green exit status imply a fully working
// site.
func logSite(siteInfo siteStatus) {
	if !siteInfo.Servable() {
		if !siteInfo.HasConfig {
			log.Printf("⚠️  %s is missing - the published pages would be served as raw markdown", siteInfo.ConfigPath)
		}
		if siteInfo.IndexPages == 0 {
			log.Printf("⚠️  %s lists no pages - a published site would answer 404 at its root", siteInfo.IndexPath)
		}
		return
	}

	log.Printf("\n🌐 Site scaffold:\n   - %s\n   - %s", siteInfo.IndexPath, siteInfo.ConfigPath)

	if siteInfo.ExcludedPages > 0 {
		log.Printf("⚠️  %s excludes %d of the %d page(s) listed in %s - those links would answer 404",
			siteInfo.ConfigPath, siteInfo.ExcludedPages, siteInfo.IndexPages, siteInfo.IndexPath)
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

	return fmt.Sprintf("aurumcode: result=%s docs=%d skipped=%d failed=%d languages_skipped=%s output=%s index_pages=%d index_pages_excluded=%d config=%t",
		result, len(docs), skippedCount, failedCount, languages, config.OutputDir,
		siteInfo.IndexPages, siteInfo.ExcludedPages, siteInfo.HasConfig)
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

	// LookupEnv, never Getenv: an empty AURUMCODE_BASE_URL is the operator
	// saying "publish at the root", which is the only way a site on a custom
	// domain can stop the pipeline from deriving a "/repo" prefix out of
	// GITHUB_REPOSITORY. os.Getenv would report the same "" for that as for a
	// variable nobody set, the derivation would run anyway, and every
	// documentation link on such a site would answer 404.
	basePath, basePathDeclared := os.LookupEnv(envBaseURL)

	return &pipeline.ExtractorPipelineConfig{
		SourceDir:       sourceDir,
		OutputDir:       outputDir,
		DocsDir:         envOrDefault(envDocsDir, outputDir),
		Languages:       splitLanguages(os.Getenv(envLanguages)),
		Incremental:     incremental,
		GenerateWelcome: generateWelcome,
		ValidateJekyll:  validateJekyll,
		DeployGHPages:   false,
		BaseURL:         basePath,
		BaseURLDeclared: basePathDeclared,
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

	// Rust and C# default to the native, tool-free doc-comment parser
	// (AUR-427): no cargo, no dotnet/xmldocmd, no execution of
	// repository-controlled code, so both register unconditionally UNLESS the
	// operator has explicitly opted that language into the repository-code-
	// executing toolchain below via AURUMCODE_ALLOW_REPO_CODE_EXECUTION (see
	// repo_code_execution.go for what each one runs and why there is no safe
	// mode to fall back to). The two paths are mutually exclusive per
	// language: the registry rejects a second extractor for a language that
	// already has one, so the native extractor must not be registered for a
	// language this run is about to also register the code-executing
	// extractor for.
	repoCodeOptIn, err := parseRepoCodeExecutionOptIn(os.Getenv(envAllowRepoCodeExecution))
	if err != nil {
		return err
	}

	if !repoCodeOptIn["rust"] {
		if err := register(rustExtractor.NewNativeExtractor()); err != nil {
			return err
		}
	}
	if !repoCodeOptIn["csharp"] {
		if err := register(csharpExtractor.NewNativeExtractor()); err != nil {
			return err
		}
	}

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
