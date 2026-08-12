package review

import (
	"context"
	"fmt"
	"os"

	"github.com/Mpaape/AurumCode/internal/analyzer"
	"github.com/Mpaape/AurumCode/internal/llm"
	"github.com/Mpaape/AurumCode/internal/prompt"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// Reviewer orchestrates the code review process: given -> analyze -> prompt
// -> llm -> parse -> findings. This is the restored c12d7ab reviewer.go,
// deliberately narrowed to that path.
//
// The restored version also loaded a RulesLoader (internal/review/rules.go)
// and an iso25010.Scorer to enrich issues with rule metadata and compute
// ISO/IEC 25010 quality scores. AUR-430's own card explicitly puts both out
// of scope ("Não ligue rules.go nem iso25010 aqui" -- they belong to
// dependent cards even though they would compile together), so this
// Reviewer has no RulesLoader or iso25010 dependency at all: GenerateReview
// returns exactly what the model reported, parsed and validated, nothing
// enriched or scored.
type Reviewer struct {
	orchestrator  *llm.Orchestrator
	diffAnalyzer  *analyzer.DiffAnalyzer
	promptBuilder *prompt.PromptBuilder
	parser        *prompt.ResponseParser
	cfg           Config
}

// Config holds reviewer configuration.
type Config struct {
	MaxTokens    int
	Temperature  float64
	ReserveReply int
}

// DefaultConfig returns sensible defaults for Config.
func DefaultConfig() Config {
	return Config{
		MaxTokens:    4000,
		Temperature:  0.3,
		ReserveReply: 1000,
	}
}

// NewReviewer creates a new reviewer. orchestrator is an *llm.Orchestrator,
// the same concrete type and Complete signature the original engine used --
// see internal/llm.Orchestrator.Complete -- so wiring a real provider chain
// here needs no adapter. A test or an offline CLI invocation builds that
// Orchestrator around a FakeProvider (see fakeprovider.go) instead of a
// vendor provider; the Reviewer itself never knows the difference.
func NewReviewer(orchestrator *llm.Orchestrator, cfg Config) *Reviewer {
	if cfg.MaxTokens == 0 {
		cfg = DefaultConfig()
	}
	return &Reviewer{
		orchestrator:  orchestrator,
		diffAnalyzer:  analyzer.NewDiffAnalyzer(),
		promptBuilder: prompt.NewPromptBuilder(),
		parser:        prompt.NewResponseParser(),
		cfg:           cfg,
	}
}

// GenerateReview generates a code review for diff.
func (r *Reviewer) GenerateReview(ctx context.Context, diff *types.Diff) (*types.ReviewResult, error) {
	cfg := r.cfg

	// Analyze diff
	metrics := r.diffAnalyzer.AnalyzeDiff(diff)

	// Build prompt with token budgeting
	opts := prompt.BuildOptions{
		MaxTokens:    cfg.MaxTokens,
		SchemaKind:   "review",
		Role:         "reviewer",
		ReserveReply: cfg.ReserveReply,
	}

	promptParts, err := r.promptBuilder.BuildPrompt(diff, metrics, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to build prompt: %w", err)
	}

	// Combine system and user prompts
	fullPrompt := promptParts.System + "\n\n" + promptParts.User

	// Call LLM
	resp, err := r.orchestrator.Complete(ctx, fullPrompt, llm.Options{
		MaxTokens:   cfg.MaxTokens - cfg.ReserveReply,
		Temperature: cfg.Temperature,
	})
	if err != nil {
		return nil, fmt.Errorf("LLM request failed: %w", err)
	}

	// Parse response
	result, err := r.parser.ParseReviewResponse(resp.Text)
	if err != nil {
		return nil, fmt.Errorf("parse failed: %w", err)
	}

	// Add metadata
	if result.Metadata == nil {
		result.Metadata = make(map[string]string)
	}
	result.Metadata["total_files"] = fmt.Sprintf("%d", metrics.TotalFiles)
	result.Metadata["lines_added"] = fmt.Sprintf("%d", metrics.LinesAdded)
	result.Metadata["lines_deleted"] = fmt.Sprintf("%d", metrics.LinesDeleted)
	result.Metadata["segments_used"] = promptParts.Meta["segments_used"]
	result.Metadata["estimated_tokens"] = promptParts.Meta["estimated_tokens"]

	// MUT-001 hook: AUR-430's skeptical mutation is "return an empty issue
	// list for a diff with a known problem." Rather than have the
	// acceptance script patch source inside a read-only container rootfs,
	// the mutation is a source-level, env-gated no-op identical in shape to
	// the AURUM_A402_MUTATION / AURUM_A403_MUTATION hooks already used
	// elsewhere on this board: unset (the default, and the only thing a
	// production build ever does), it does nothing. See
	// tests/acceptance/AUR-430.sh's mutation_case and docs/specs/AUR-430.md.
	if os.Getenv("AURUM_A430_MUTATION") == "empty-issues" {
		result.Issues = nil
	}

	return result, nil
}
