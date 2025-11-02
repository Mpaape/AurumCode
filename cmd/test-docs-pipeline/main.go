package main

import (
	"aurumcode/internal/config"
	"aurumcode/internal/git/githubclient"
	"aurumcode/internal/llm"
	"aurumcode/internal/llm/cost"
	"aurumcode/internal/llm/provider/openai"
	"aurumcode/internal/pipeline"
	"aurumcode/pkg/types"
	"context"
	"fmt"
	"log"
	"os"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Println("🚀 Testing AurumCode Documentation Pipeline")

	// Load environment variables
	provedorAPIKey := os.Getenv("PROVEDOR_API_KEY")
	provedorBaseURL := os.Getenv("PROVEDOR_BASE_URL")

	if provedorAPIKey == "" {
		log.Fatal("❌ PROVEDOR_API_KEY not set in environment")
	}
	if provedorBaseURL == "" {
		log.Fatal("❌ PROVEDOR_BASE_URL not set in environment")
	}

	log.Printf("✓ Using PROVEDOR DTA: %s", provedorBaseURL)

	// Create OpenAI-compatible provider pointing to PROVEDOR DTA
	// PROVEDOR DTA uses OpenAI-compatible API
	provider := openai.NewProvider(provedorAPIKey, "gpt-4")
	// Override base URL to point to PROVEDOR
	if customProvider, ok := provider.(interface{ SetBaseURL(string) }); ok {
		customProvider.SetBaseURL(provedorBaseURL)
		log.Printf("✓ Configured to use PROVEDOR DTA endpoint")
	}

	// Create cost tracker (no limits for testing)
	priceMap := cost.NewPriceMap()
	tracker := cost.NewTracker(1000.0, 10000.0, priceMap)

	// Create LLM orchestrator
	llmOrch := llm.NewOrchestrator(provider, nil, tracker)
	log.Println("✓ LLM Orchestrator created")

	// Load AurumCode configuration
	cfg, err := config.LoadFromPath(".aurumcode/config.yml")
	if err != nil {
		log.Printf("⚠️  Failed to load config, using defaults: %v", err)
		cfg = types.NewDefaultConfig()
	}

	// Enable documentation generation
	cfg.Outputs.UpdateDocs = true
	cfg.Outputs.DeploySite = false // Don't build Hugo for now
	log.Println("✓ Configuration loaded")

	// Create GitHub client (not needed for local test, but required by pipeline)
	ghClient := githubclient.NewClient("")

	// Create Documentation Pipeline
	docsPipeline := pipeline.NewDocumentationPipeline(cfg, ghClient, llmOrch)
	log.Println("✓ Documentation Pipeline created")

	// Create a simulated "push to main" event
	event := &types.Event{
		EventType:  "push",
		Branch:     "main",
		Repo:       "AurumCode",
		RepoOwner:  "Mpaape",
		CommitSHA:  "HEAD",
		DeliveryID: "test-local-run",
	}
	log.Printf("✓ Simulated event: push to %s", event.Branch)

	// Run the pipeline!
	log.Println("\n📝 Running Documentation Pipeline...")
	log.Println("─────────────────────────────────────────")

	ctx := context.Background()
	if err := docsPipeline.Run(ctx, event); err != nil {
		log.Fatalf("❌ Pipeline failed: %v", err)
	}

	log.Println("─────────────────────────────────────────")
	log.Println("✅ Documentation Pipeline completed successfully!")
	log.Println("\n📊 Check the following files:")
	log.Println("   - CHANGELOG.md")
	log.Println("   - README.md (updated sections)")

	// Show token usage
	usage := tracker.GetUsage()
	log.Printf("\n💰 Token Usage:")
	log.Printf("   - Total Tokens: %d", usage.TotalTokens)
	log.Printf("   - Estimated Cost: $%.4f", usage.TotalCost)

	fmt.Println("\n🎉 Done! Check the generated files.")
}
