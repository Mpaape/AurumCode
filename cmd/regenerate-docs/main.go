package main

import (
	"context"
	"fmt"
	"log"
	"os"

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

	totvsAPIKey := os.Getenv("TOTVS_DTA_API_KEY")
	totvsBaseURL := os.Getenv("TOTVS_DTA_BASE_URL")
	openaiAPIKey := os.Getenv("OPENAI_API_KEY")

	var llmOrch *llm.Orchestrator

	switch {
	case totvsAPIKey != "" && totvsBaseURL != "":
		model := os.Getenv("TOTVS_DTA_MODEL")
		if model == "" {
			model = "gpt-4o-mini"
		}
		provider := litellmProvider.NewProvider(totvsAPIKey, totvsBaseURL, model)
		tracker := cost.NewTracker(1000.0, 10000.0, map[string]cost.PriceMap{})
		llmOrch = llm.NewOrchestrator(provider, nil, tracker)
		log.Printf("✓ LiteLLM configured via TOTVS DTA (%s)", totvsBaseURL)
	case totvsAPIKey != "" && totvsBaseURL == "":
		log.Println("⚠️  TOTVS_DTA_BASE_URL not set - skipping LiteLLM provider")
	default:
		log.Println("⚠️  TOTVS_DTA_API_KEY not set - LLM features will be disabled")
	}

	if llmOrch == nil && openaiAPIKey != "" {
		provider := openaiProvider.NewProvider(openaiAPIKey)
		tracker := cost.NewTracker(1000.0, 10000.0, map[string]cost.PriceMap{
			"gpt-4": {InputPer1K: 0.03, OutputPer1K: 0.06},
		})
		llmOrch = llm.NewOrchestrator(provider, nil, tracker)
		log.Println("✓ OpenAI provider configured")
	}

	if llmOrch != nil {
		log.Printf("✓ LLM Orchestrator created (providers: %v)", llmOrch.GetProviderChain())
	} else {
		log.Println("⚠️  No LLM provider configured - welcome page generation disabled")
	}

	runner := site.NewDefaultRunner()

	config := &pipeline.ExtractorPipelineConfig{
		SourceDir:       ".",
		OutputDir:       ".aurumcode",
		DocsDir:         ".aurumcode",
		Languages:       []string{},
		Incremental:     false,
		GenerateWelcome: llmOrch != nil,
		ValidateJekyll:  false,
		DeployGHPages:   false,
	}

	extractorPipeline := pipeline.NewExtractorPipeline(config, runner, llmOrch)
	if err := registerLanguageExtractors(extractorPipeline, runner); err != nil {
		log.Fatalf("❌ Failed to register language extractors: %v", err)
	}

	log.Println("\n📝 Running Documentation Extraction...")
	log.Println("────────────────────────────────────────")

	ctx := context.Background()
	if err := extractorPipeline.Run(ctx); err != nil {
		log.Fatalf("❌ Pipeline failed: %v", err)
	}

	log.Println("────────────────────────────────────────")
	log.Println("✅ Documentation regeneration completed!")
	log.Println("\n📊 Generated documentation in:")
	log.Println("   - .aurumcode/go/")
	log.Println("   - .aurumcode/javascript/")
	log.Println("   - .aurumcode/python/")
	log.Println("   - .aurumcode/ (other languages)")
	log.Println("\n📝 Custom pages location:")
	log.Println("   - docs/ (your guides, tutorials, etc.)")
	log.Println("\n🌐 Build Jekyll site with:")
	log.Println("   cd .aurumcode && bundle install && bundle exec jekyll build")
	log.Println("\n🎉 Done!")

	if llmOrch != nil {
		perRun, daily := llmOrch.RemainingBudget()
		log.Printf("\n💰 Remaining LLM budget: per-run $%.2f | daily $%.2f", perRun, daily)
	}
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

	if err := register(csharpExtractor.NewCSharpExtractor(runner)); err != nil {
		return err
	}

	if err := register(cppExtractor.NewCPPExtractor(runner)); err != nil {
		return err
	}

	if err := register(rustExtractor.NewRustExtractor(runner)); err != nil {
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
