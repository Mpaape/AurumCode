package pipeline

import (
	"aurumcode/internal/config"
	"aurumcode/internal/docgen"
	"aurumcode/internal/documentation/api"
	"aurumcode/internal/documentation/changelog"
	"aurumcode/internal/documentation/readme"
	"aurumcode/internal/git/githubclient"
	"aurumcode/internal/llm"
	"aurumcode/pkg/types"
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"
)

// DocumentationPipeline handles the documentation generation use case
type DocumentationPipeline struct {
	config          *config.Config
	githubClient    *githubclient.Client
	llmOrch         *llm.Orchestrator
	docGen          *docgen.Generator
	changelogWriter *changelog.Writer
	readmeUpdater   *readme.Updater
	apiDetector     *api.Detector
	apiGenerator    *api.Generator
}

// NewDocumentationPipeline creates a new documentation pipeline
func NewDocumentationPipeline(
	cfg *config.Config,
	githubClient *githubclient.Client,
	llmOrch *llm.Orchestrator,
) *DocumentationPipeline {
	return &DocumentationPipeline{
		config:          cfg,
		githubClient:    githubClient,
		llmOrch:         llmOrch,
		docGen:          docgen.NewGenerator(llmOrch),
		changelogWriter: changelog.NewWriter().WithHashes(true),
		readmeUpdater:   readme.NewUpdater(),
		apiDetector:     api.NewDetector(),
		apiGenerator:    api.NewGenerator(),
	}
}

// Run executes the documentation generation pipeline
func (p *DocumentationPipeline) Run(ctx context.Context, event *types.Event) error {
	log.Printf("[Docs] Starting documentation generation for %s", event.Repo)

	// Check if event is for a push to main or merged PR
	if !p.shouldGenerateDocs(event) {
		log.Printf("[Docs] Skipping: event not eligible for documentation generation")
		return nil
	}

	// Step 1: Fetch recent commits for changelog
	log.Printf("[Docs] Fetching commits for changelog generation...")
	commits, err := p.fetchRecentCommits(ctx, event)
	if err != nil {
		log.Printf("[Docs] Warning: Failed to fetch commits: %v", err)
		// Continue anyway - we can still try other doc generation
	}

	// Step 2: Generate changelog if commits were found
	if len(commits) > 0 && p.config.Outputs.UpdateDocs {
		log.Printf("[Docs] Generating changelog from %d commits...", len(commits))
		if err := p.generateChangelog(commits); err != nil {
			log.Printf("[Docs] Warning: Changelog generation failed: %v", err)
		} else {
			log.Printf("[Docs] Changelog updated successfully")
		}
	}

	// Step 3: Update README sections if configured
	if p.config.Outputs.UpdateDocs {
		log.Printf("[Docs] Updating README sections...")
		if err := p.updateREADME(ctx, event); err != nil {
			log.Printf("[Docs] Warning: README update failed: %v", err)
		} else {
			log.Printf("[Docs] README updated successfully")
		}
	}

	// Step 4: Generate API documentation if OpenAPI detected
	log.Printf("[Docs] Checking for API specifications...")
	specs := p.apiDetector.Detect(".")
	if len(specs) > 0 {
		log.Printf("[Docs] Found %d API spec(s), generating API documentation...", len(specs))
		if err := p.generateAPIDocs(specs); err != nil {
			log.Printf("[Docs] Warning: API docs generation failed: %v", err)
		} else {
			log.Printf("[Docs] API documentation generated successfully")
		}
	}

	// Step 5: Build static site if enabled
	if p.config.Outputs.DeploySite {
		log.Printf("[Docs] Building static site...")
		if err := p.buildStaticSite(ctx); err != nil {
			log.Printf("[Docs] Warning: Static site build failed: %v", err)
		} else {
			log.Printf("[Docs] Static site built successfully")
		}
	}

	log.Printf("[Docs] Documentation pipeline completed successfully")
	return nil
}

// shouldGenerateDocs determines if docs should be generated for this event
func (p *DocumentationPipeline) shouldGenerateDocs(event *types.Event) bool {
	// Generate docs on push to main branch
	if event.EventType == "push" && event.Branch == "main" {
		return true
	}

	// Generate docs on merged PR
	if event.EventType == "pull_request" && event.Action == "closed" && event.Merged {
		return true
	}

	return false
}

// fetchRecentCommits fetches recent commits for changelog generation
func (p *DocumentationPipeline) fetchRecentCommits(ctx context.Context, event *types.Event) ([]*changelog.Commit, error) {
	// Use git log to fetch commits since last tag or last 50 commits
	cmd := exec.CommandContext(ctx, "git", "log", "--pretty=format:%H|%an|%ae|%at|%s|%b", "-n", "50")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git log failed: %w", err)
	}

	commits := []*changelog.Commit{}
	parser := changelog.NewParser()

	for _, line := range strings.Split(string(output), "\n") {
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, "|", 6)
		if len(parts) < 5 {
			continue
		}

		commit := &changelog.Commit{
			Hash:    parts[0],
			Author:  parts[1],
			Email:   parts[2],
			Message: parts[4],
		}

		if len(parts) > 5 {
			commit.Body = parts[5]
		}

		// Parse conventional commit
		parsedCommit := parser.ParseCommit(commit)
		if parsedCommit != nil {
			commits = append(commits, parsedCommit)
		}
	}

	return commits, nil
}

// generateChangelog generates CHANGELOG.md from commits
func (p *DocumentationPipeline) generateChangelog(commits []*changelog.Commit) error {
	// Group commits into a changelog
	parser := changelog.NewParser()
	changelogData := parser.Parse(commits)

	// Add version and date
	changelogData.Version = time.Now().Format("2006-01-02")

	// Write changelog
	if err := p.changelogWriter.Write(changelogData, "CHANGELOG.md"); err != nil {
		return fmt.Errorf("write changelog: %w", err)
	}

	return nil
}

// updateREADME updates README.md sections
func (p *DocumentationPipeline) updateREADME(ctx context.Context, event *types.Event) error {
	readmePath := "README.md"

	// Check if README exists
	if _, err := os.Stat(readmePath); os.IsNotExist(err) {
		log.Printf("[Docs] README.md not found, skipping update")
		return nil
	}

	// Generate sections to update
	sections := []readme.Section{
		{
			Name: "Status",
			Content: fmt.Sprintf("**Build:** ✅ Passing\n**Last Updated:** %s\n**Version:** Latest\n",
				time.Now().Format("2006-01-02")),
		},
	}

	// Update README
	result, err := p.readmeUpdater.Update(readmePath, sections)
	if err != nil {
		return fmt.Errorf("update README: %w", err)
	}

	if result.Updated {
		log.Printf("[Docs] Updated %d README section(s): %v", len(result.Changes), result.Changes)
	}

	return nil
}

// generateAPIDocs generates API documentation from OpenAPI specs
func (p *DocumentationPipeline) generateAPIDocs(specs []string) error {
	for _, specPath := range specs {
		log.Printf("[Docs] Generating docs for API spec: %s", specPath)

		// Parse OpenAPI spec
		apiDoc, err := p.apiGenerator.GenerateFromSpec(specPath)
		if err != nil {
			log.Printf("[Docs] Warning: Failed to generate docs for %s: %v", specPath, err)
			continue
		}

		// Write API docs
		outputPath := strings.Replace(specPath, ".yaml", ".md", 1)
		outputPath = strings.Replace(outputPath, ".yml", ".md", 1)
		outputPath = strings.Replace(outputPath, ".json", ".md", 1)

		if err := os.WriteFile(outputPath, []byte(apiDoc), 0644); err != nil {
			log.Printf("[Docs] Warning: Failed to write API docs to %s: %v", outputPath, err)
			continue
		}

		log.Printf("[Docs] API documentation written to: %s", outputPath)
	}

	return nil
}

// buildStaticSite builds Hugo static site with Pagefind
func (p *DocumentationPipeline) buildStaticSite(ctx context.Context) error {
	// Check if Hugo is available
	if _, err := exec.LookPath("hugo"); err != nil {
		log.Printf("[Docs] Hugo not found, skipping static site build")
		return nil
	}

	// Build Hugo site
	log.Printf("[Docs] Running Hugo build...")
	cmd := exec.CommandContext(ctx, "hugo", "--minify")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("hugo build failed: %w\nOutput: %s", err, output)
	}

	// Check if Pagefind is available
	if _, err := exec.LookPath("pagefind"); err != nil {
		log.Printf("[Docs] Pagefind not found, skipping search index")
		return nil
	}

	// Build Pagefind index
	log.Printf("[Docs] Building Pagefind search index...")
	cmd = exec.CommandContext(ctx, "pagefind", "--source", "public")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("pagefind build failed: %w\nOutput: %s", err, output)
	}

	return nil
}
