package review

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"

	"github.com/Mpaape/AurumCode/internal/analyzer"
	"github.com/Mpaape/AurumCode/internal/llm"
	"github.com/Mpaape/AurumCode/internal/prompt"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// Reviewer orchestrates the code review process: given -> analyze -> prompt
// -> llm -> parse -> rule gate -> findings. The pipeline up to the parse is
// the restored c12d7ab reviewer.go, deliberately narrowed to that path by
// AUR-430.
//
// AUR-434 wires the restored RulesLoader (internal/review/rules.go) back in
// and adds the gate the historical code never had: the c12d7ab
// mapRulesToIssues only ENRICHED issues (filling an empty Message or
// Severity from the rule), it never rejected an issue whose RuleID was
// missing or unknown. enforceRuleCitations below keeps that enrichment and
// adds the rejection: every issue GenerateReview returns cites a rule of
// the project review standard that sustains it, and an issue that cannot
// never reaches the caller. iso25010 scoring remains out of scope here (it
// belongs to another card), so this Reviewer still has no iso25010
// dependency.
type Reviewer struct {
	orchestrator  *llm.Orchestrator
	diffAnalyzer  *analyzer.DiffAnalyzer
	promptBuilder *prompt.PromptBuilder
	parser        *prompt.ResponseParser
	filter        *redaction.Filter
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
		// The single AUR-009 redaction filter (AUR-432). FromEnv registers
		// the AURUM_SECRET_CANARY value when present, so the filter is
		// built at construction, after the caller's environment is final.
		filter: redaction.FromEnv(),
		cfg:    cfg,
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

	// The assembled prompt is the material that leaves this process toward
	// a model, and it carries the raw diff, so it passes the single
	// AUR-009 redaction filter before any provider can see it (AUR-432).
	// The filter replaces secret values with the stable marker while the
	// surrounding context -- key names, file paths, line structure --
	// survives, so the model still sees THAT a credential sits on a line
	// without ever seeing the credential. MUT-001 disables exactly the
	// next line.
	fullPrompt = r.filter.Redact(fullPrompt)

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

	// Model output is untrusted input: a model may echo a secret from the
	// diff back in a finding. Every string of the parsed result that can
	// reach a sink (report, stdout, cache, evidence) is redacted here, at
	// the boundary where it enters the process, and deliberately BEFORE
	// enforceRuleCitations appends the trusted rule-citation suffix from
	// the embedded catalog -- redacting after would also rewrite the
	// catalog's own "...-secret: <title>" spelling and change the
	// published output format for a secret-free review (AUR-432).
	redactReviewResult(r.filter, result)

	// Rule gate (AUR-434): every issue must cite a rule of the project
	// review standard. A broken or empty embedded catalog is a loud
	// error here, never a silent zero-rule review.
	rules, err := sharedRules()
	if err != nil {
		return nil, fmt.Errorf("review rules unavailable: %w", err)
	}
	rejected := enforceRuleCitations(rules, result)

	// Add metadata
	if result.Metadata == nil {
		result.Metadata = make(map[string]string)
	}
	result.Metadata["issues_rejected_without_rule"] = fmt.Sprintf("%d", rejected)
	result.Metadata["total_files"] = fmt.Sprintf("%d", metrics.TotalFiles)
	result.Metadata["lines_added"] = fmt.Sprintf("%d", metrics.LinesAdded)
	result.Metadata["lines_deleted"] = fmt.Sprintf("%d", metrics.LinesDeleted)
	result.Metadata["segments_used"] = promptParts.Meta["segments_used"]
	result.Metadata["estimated_tokens"] = promptParts.Meta["estimated_tokens"]

	return result, nil
}

// sharedRules loads the embedded rules catalog exactly once per process.
// The load can only fail if the embedded catalog itself is broken, so a
// failure is permanent for the binary and cached as such.
var sharedRules = sync.OnceValues(func() (*RulesLoader, error) {
	loader := NewRulesLoader()
	if err := loader.Load(); err != nil {
		return nil, err
	}
	return loader, nil
})

// redactReviewResult applies the AUR-009 redaction filter, in place, to
// every model-authored string of result that can reach a sink. Severity is
// left alone (the parser admits only error/warning/info) and line numbers
// are integers. A RuleID that carried a secret-shaped value stops matching
// the embedded catalog after redaction and is then rejected by the rule
// gate: fail closed, never a leak.
func redactReviewResult(f *redaction.Filter, result *types.ReviewResult) {
	result.Summary = f.Redact(result.Summary)
	result.CommitComment = f.Redact(result.CommitComment)
	for i := range result.Issues {
		issue := &result.Issues[i]
		issue.ID = f.Redact(issue.ID)
		issue.File = f.Redact(issue.File)
		issue.RuleID = f.Redact(issue.RuleID)
		issue.Message = f.Redact(issue.Message)
		issue.Suggestion = f.Redact(issue.Suggestion)
	}
	for i := range result.LineComments {
		result.LineComments[i].Path = f.Redact(result.LineComments[i].Path)
		result.LineComments[i].Body = f.Redact(result.LineComments[i].Body)
	}
	for i := range result.FileComments {
		result.FileComments[i].Path = f.Redact(result.FileComments[i].Path)
		result.FileComments[i].Body = f.Redact(result.FileComments[i].Body)
	}
}

// enforceRuleCitations applies AUR-434's rule gate to result, in place,
// and returns how many issues it rejected.
//
// For each issue, the cited rule is resolved against the embedded project
// review standard. An issue whose RuleID is missing or unknown is
// discarded: a finding that cannot cite the rule that sustains it never
// reaches the user. A surviving issue keeps the c12d7ab mapRulesToIssues
// enrichment (empty Message/Severity filled from the rule, file path
// cleaned) and gains the citation itself, appended to the message the
// user sees as " (rule <id>: <title>)".
func enforceRuleCitations(rules *RulesLoader, result *types.ReviewResult) int {
	kept := make([]types.ReviewIssue, 0, len(result.Issues))
	rejected := 0
	for _, issue := range result.Issues {
		rule, ok := rules.Get(issue.RuleID)
		if !ok {
			rejected++
			continue
		}
		if issue.Message == "" {
			issue.Message = rule.Description
		}
		if issue.Severity == "" {
			issue.Severity = rule.Severity
		}
		if issue.File != "" {
			issue.File = filepath.Clean(issue.File)
		}
		issue.Message = fmt.Sprintf("%s (rule %s: %s)", issue.Message, rule.ID, rule.Title)
		kept = append(kept, issue)
	}
	result.Issues = kept
	return rejected
}
