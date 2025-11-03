package main

import (
	"context"
	"log"
	"os"

	"aurumcode/internal/documentation/extractors"
	"aurumcode/internal/documentation/site"
	"aurumcode/internal/llm"
	"aurumcode/internal/llm/cost"
	"aurumcode/internal/llm/provider/openai"
	"aurumcode/internal/pipeline"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Println("🚀 AurumCode - Regenerating Complete Documentation")
	log.Println("================================================")

	// Load environment variables
	provedorAPIKey := os.Getenv("PROVEDOR_API_KEY")
	provedorBaseURL := os.Getenv("PROVEDOR_BASE_URL")

	if provedorAPIKey == "" {
		log.Println("⚠️  PROVEDOR_API_KEY not set - LLM features will be disabled")
	} else {
		log.Printf("✓ Using PROVEDOR DTA: %s", provedorBaseURL)
	}

	// Create LLM orchestrator if API key is available
	var llmOrch *llm.Orchestrator
	if provedorAPIKey != "" {
		provider := openai.NewProvider(provedorAPIKey, "gpt-4")
		if customProvider, ok := provider.(interface{ SetBaseURL(string) }); ok && provedorBaseURL != "" {
			customProvider.SetBaseURL(provedorBaseURL)
			log.Println("✓ Configured PROVEDOR DTA endpoint")
		}

		priceMap := cost.NewPriceMap()
		tracker := cost.NewTracker(1000.0, 10000.0, priceMap)
		llmOrch = llm.NewOrchestrator(provider, nil, tracker)
		log.Println("✓ LLM Orchestrator created")
	}

	// Register all extractors
	jsExtractor := extractors.NewJSExtractor(site.NewRealRunner())

	registry := extractors.NewRegistry()
	registry.Register(extractors.LanguageGo, extractors.NewGoExtractor(site.NewRealRunner()))
	registry.Register(extractors.LanguageJavaScript, jsExtractor) // Same extractor for JS & TS
	registry.Register(extractors.LanguageTypeScript, jsExtractor)
	registry.Register(extractors.LanguagePython, extractors.NewPythonExtractor(site.NewRealRunner()))
	registry.Register(extractors.LanguageCSharp, extractors.NewCSharpExtractor(site.NewRealRunner()))
	registry.Register(extractors.LanguageCPP, extractors.NewCPPExtractor(site.NewRealRunner()))
	registry.Register(extractors.LanguageRust, extractors.NewRustExtractor(site.NewRealRunner()))
	registry.Register(extractors.LanguageBash, extractors.NewBashExtractor(site.NewRealRunner()))
	registry.Register(extractors.LanguagePowerShell, extractors.NewPowerShellExtractor(site.NewRealRunner()))
	log.Println("✓ Registered 8 language extractors (9 with JS/TS)")

	// Configure pipeline
	config := &pipeline.ExtractorPipelineConfig{
		SourceDir:       ".",           // Current directory
		OutputDir:       "docs",        // Output to docs/
		DocsDir:         "docs",        // Jekyll docs directory
		Languages:       []string{},    // Empty = all languages
		Incremental:     false,         // Full regeneration
		GenerateWelcome: llmOrch != nil, // Only if LLM available
		ValidateJekyll:  false,         // Skip validation for now
		DeployGHPages:   false,         // No deployment
	}

	// Create pipeline
	runner := site.NewRealRunner()
	extractorPipeline := pipeline.NewExtractorPipeline(config, runner, llmOrch)
	log.Println("✓ Extractor Pipeline created")

	// Run the pipeline
	log.Println("\n📝 Running Documentation Extraction...")
	log.Println("────────────────────────────────────────")

	ctx := context.Background()
	if err := extractorPipeline.Run(ctx); err != nil {
		log.Fatalf("❌ Pipeline failed: %v", err)
	}

	log.Println("────────────────────────────────────────")
	log.Println("✅ Documentation regeneration completed!")
	log.Println("\n📊 Generated documentation in:")
	log.Println("   - docs/go/")
	log.Println("   - docs/javascript/")
	log.Println("   - docs/python/")
	log.Println("   - docs/ (other languages)")
	log.Println("\n🌐 Build Jekyll site with:")
	log.Println("   cd docs && bundle install && bundle exec jekyll build")
	log.Println("\n🎉 Done!")

	// Show token usage if LLM was used
	if llmOrch != nil {
		usage := llmOrch.GetTracker().GetUsage()
		log.Printf("\n💰 LLM Token Usage:")
		log.Printf("   - Total Tokens: %d", usage.TotalTokens)
		log.Printf("   - Estimated Cost: $%.4f", usage.TotalCost)
	}
}
