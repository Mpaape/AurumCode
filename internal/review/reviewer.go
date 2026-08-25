package review

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
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

// ReviewContext contains optional evidence available beside the diff. It is
// kept separate from the diff so a caller can add CI status without changing
// the code-review input shape or requiring repository-specific prompt files.
type ReviewContext struct {
	CI       string
	Language string
}

// DefaultConfig returns sensible defaults for Config.
//
// Temperature stays at its zero value, deliberately (AUR-460): the litellm
// provider this reviewer's orchestrator sends requests through never
// serializes an explicit temperature at all now, so a non-zero default
// here would only be misleading -- it would look load-bearing while
// actually going nowhere. See internal/llm/provider/litellm/provider.go
// and internal/llm.DefaultOptions.
func DefaultConfig() Config {
	return Config{
		MaxTokens:    4000,
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
	return r.GenerateReviewWithContext(ctx, diff, ReviewContext{})
}

// GenerateReviewWithContext generates a review using the diff and optional
// external evidence such as completed CI check statuses.
func (r *Reviewer) GenerateReviewWithContext(ctx context.Context, diff *types.Diff, reviewContext ReviewContext) (*types.ReviewResult, error) {
	cfg := r.cfg

	// Analyze diff (counts only; metrics carry no content into the prompt)
	metrics := r.diffAnalyzer.AnalyzeDiff(diff)

	// The diff is the untrusted material that will leave this process
	// toward a model, so it passes the single AUR-009 redaction filter
	// BEFORE the prompt is assembled (AUR-432). Composition matters, not
	// just coverage: a diff line carries a +/-/space marker, and the
	// filter's header rule is anchored at line start, so redacting the
	// assembled prompt would let "+Authorization: Bearer x" through
	// verbatim. redactDiff strips each line's marker, redacts the bodies
	// as the filter would see them on their own, and re-prefixes -- values
	// become the stable marker while key names, header names, file paths
	// and line structure survive, so the model still sees THAT a
	// credential sits on a line without ever seeing it. MUT-001 disables
	// exactly the next line.
	diff = redactDiff(r.filter, diff)

	// Build prompt with token budgeting
	opts := prompt.BuildOptions{
		MaxTokens:    cfg.MaxTokens,
		SchemaKind:   "review",
		Role:         "reviewer",
		ReserveReply: cfg.ReserveReply,
		CIContext:    r.filter.Redact(reviewContext.CI),
		Language:     reviewContext.Language,
	}

	promptParts, err := r.promptBuilder.BuildPrompt(diff, metrics, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to build prompt: %w", err)
	}

	// Combine system and user prompts. The System half is the trusted
	// embedded template plus numeric metrics; everything untrusted in the
	// User half derives from the diff redacted above.
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

	// Model output is untrusted input: a model may echo a secret from the
	// diff back in a finding. Every string of the parsed result that can
	// reach a sink (report, stdout, cache, evidence) is redacted here, at
	// the boundary where it enters the process, and deliberately BEFORE
	// enforceRuleCitations appends the trusted rule-citation suffix from
	// the embedded catalog -- redacting after would also rewrite the
	// catalog's own "...-secret: <title>" spelling and change the
	// published output format for a secret-free review (AUR-432).
	redactReviewResult(r.filter, result)
	// Model output is also allowed to propose a hardcoded-secret finding on a
	// safe workflow reference. Apply the narrow, source-aware filter before
	// the rule gate so `${{ secrets.NAME }}`, permission scopes, and event
	// types cannot become a blocking issue while literal values remain visible
	// to the deterministic security pass.
	result.Issues = suppressWorkflowReferenceFindings(diff, result.Issues)
	result.Suggestions = suppressWorkflowReferenceSuggestions(diff, result.Suggestions)

	// Rule gate (AUR-434): every issue must cite a rule of the project
	// review standard. A broken or empty embedded catalog is a loud
	// error here, never a silent zero-rule review.
	rules, err := sharedRules()
	if err != nil {
		return nil, fmt.Errorf("review rules unavailable: %w", err)
	}
	rejected, discarded := enforceRuleCitations(rules, result)

	// Add metadata
	if result.Metadata == nil {
		result.Metadata = make(map[string]string)
	}
	result.Metadata["issues_rejected_without_rule"] = fmt.Sprintf("%d", rejected)
	// AUR-448: a discard the rule gate makes is never silent. The caller
	// (cmd/aurumcode) prints discardWarning verbatim to stderr when it is
	// non-empty; it is deliberately "" on the happy path (rejected == 0) so
	// a caller that prints unconditionally on a non-empty string never
	// writes a byte to stderr for a review where every finding cites a
	// resolvable rule -- see formatDiscardWarning.
	result.Metadata["discard_warning"] = formatDiscardWarning(discarded)
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

// splitDiffMarker splits a diff line into its one-character +/-/space
// marker and the line body the redaction filter should see. A line without
// a marker is all body.
func splitDiffMarker(line string) (marker, body string) {
	if line == "" {
		return "", ""
	}
	switch line[0] {
	case '+', '-', ' ':
		return line[:1], line[1:]
	}
	return "", line
}

// redactLinesKeepingMarkers redacts text the way the redaction filter
// would see it without diff markers: each line's +/-/space marker is
// stripped, the bodies are redacted together (so the line-anchored header
// rules and the multi-line private-key rule both apply), and the markers
// are re-attached positionally. When a multi-line secret collapsed the
// line count, positional markers no longer map, so it fails closed and
// returns the redacted bodies without markers rather than guessing.
func redactLinesKeepingMarkers(f *redaction.Filter, text string) string {
	lines := strings.Split(text, "\n")
	markers := make([]string, len(lines))
	bodies := make([]string, len(lines))
	for i, line := range lines {
		markers[i], bodies[i] = splitDiffMarker(line)
	}
	out := strings.Split(f.Redact(strings.Join(bodies, "\n")), "\n")
	if len(out) != len(markers) {
		return strings.Join(out, "\n")
	}
	for i := range out {
		out[i] = markers[i] + out[i]
	}
	return strings.Join(out, "\n")
}

// redactDiff returns a copy of diff with every hunk line and file path
// passed through the AUR-009 redaction filter, markers preserved. The
// original diff is left untouched: it never leaves the process, and the
// metrics were already computed from it.
func redactDiff(f *redaction.Filter, diff *types.Diff) *types.Diff {
	out := &types.Diff{Files: make([]types.DiffFile, len(diff.Files))}
	for i, file := range diff.Files {
		redactedFile := file
		redactedFile.Path = f.Redact(file.Path)
		redactedFile.Hunks = make([]types.DiffHunk, len(file.Hunks))
		for j, hunk := range file.Hunks {
			redactedHunk := hunk
			redactedHunk.Lines = strings.Split(
				redactLinesKeepingMarkers(f, strings.Join(hunk.Lines, "\n")), "\n")
			redactedFile.Hunks[j] = redactedHunk
		}
		out.Files[i] = redactedFile
	}
	return out
}

// redactReviewResult applies the AUR-009 redaction filter, in place, to
// every model-authored string of result that can reach a sink. Prose
// fields go through redactLinesKeepingMarkers because a model quoting the
// offending diff line echoes it marker and all, and the marker would
// otherwise defeat the filter's line-anchored header rules. Severity is
// left alone (the parser admits only error/warning/info) and line numbers
// are integers. A RuleID that carried a secret-shaped value stops matching
// the embedded catalog after redaction and is then rejected by the rule
// gate: fail closed, never a leak.
func redactReviewResult(f *redaction.Filter, result *types.ReviewResult) {
	result.Verdict = f.Redact(result.Verdict)
	result.Summary = redactLinesKeepingMarkers(f, result.Summary)
	result.CommitComment = redactLinesKeepingMarkers(f, result.CommitComment)
	for i := range result.Strengths {
		result.Strengths[i] = redactLinesKeepingMarkers(f, result.Strengths[i])
	}
	for i := range result.Issues {
		issue := &result.Issues[i]
		issue.ID = f.Redact(issue.ID)
		issue.File = f.Redact(issue.File)
		issue.RuleID = f.Redact(issue.RuleID)
		issue.Message = redactLinesKeepingMarkers(f, issue.Message)
		issue.Impact = redactLinesKeepingMarkers(f, issue.Impact)
		issue.Evidence = redactLinesKeepingMarkers(f, issue.Evidence)
		issue.Suggestion = redactLinesKeepingMarkers(f, issue.Suggestion)
		issue.Verification = redactLinesKeepingMarkers(f, issue.Verification)
	}
	for i := range result.Suggestions {
		suggestion := &result.Suggestions[i]
		suggestion.Title = redactLinesKeepingMarkers(f, suggestion.Title)
		suggestion.Description = redactLinesKeepingMarkers(f, suggestion.Description)
		suggestion.Kind = f.Redact(suggestion.Kind)
		suggestion.File = f.Redact(suggestion.File)
		suggestion.CurrentCode = redactLinesKeepingMarkers(f, suggestion.CurrentCode)
		suggestion.ProposedCode = redactLinesKeepingMarkers(f, suggestion.ProposedCode)
		suggestion.Rationale = redactLinesKeepingMarkers(f, suggestion.Rationale)
		suggestion.Verification = redactLinesKeepingMarkers(f, suggestion.Verification)
	}
	for i := range result.CIAnalysis {
		result.CIAnalysis[i].Check = f.Redact(result.CIAnalysis[i].Check)
		result.CIAnalysis[i].Status = f.Redact(result.CIAnalysis[i].Status)
		result.CIAnalysis[i].Cause = redactLinesKeepingMarkers(f, result.CIAnalysis[i].Cause)
		result.CIAnalysis[i].Evidence = redactLinesKeepingMarkers(f, result.CIAnalysis[i].Evidence)
		result.CIAnalysis[i].Fix = redactLinesKeepingMarkers(f, result.CIAnalysis[i].Fix)
		result.CIAnalysis[i].NextVerification = redactLinesKeepingMarkers(f, result.CIAnalysis[i].NextVerification)
		result.CIAnalysis[i].Confidence = f.Redact(result.CIAnalysis[i].Confidence)
	}
	for i := range result.TestPlan {
		result.TestPlan[i] = redactLinesKeepingMarkers(f, result.TestPlan[i])
	}
	for i := range result.Limitations {
		result.Limitations[i] = redactLinesKeepingMarkers(f, result.Limitations[i])
	}
	for i := range result.LineComments {
		result.LineComments[i].Path = f.Redact(result.LineComments[i].Path)
		result.LineComments[i].Body = redactLinesKeepingMarkers(f, result.LineComments[i].Body)
	}
	for i := range result.FileComments {
		result.FileComments[i].Path = f.Redact(result.FileComments[i].Path)
		result.FileComments[i].Body = redactLinesKeepingMarkers(f, result.FileComments[i].Body)
	}
}

// discardSummary tallies WHY enforceRuleCitations rejected findings, not
// only how many: Missing counts issues whose RuleID was empty (the model
// cited no rule at all), Unknown counts issues whose RuleID was non-empty
// but did not resolve against the embedded catalog, and UnknownIDs is the
// distinct set of unknown ids cited, sorted for a deterministic message
// (AUR-448 -- a discard must be nameable, never only a count).
type discardSummary struct {
	Missing    int
	Unknown    int
	UnknownIDs []string
}

// enforceRuleCitations applies AUR-434's rule gate to result, in place,
// and returns how many issues it rejected plus the discardSummary a caller
// uses to explain the rejection (AUR-448).
//
// For each issue, the cited rule is resolved against the embedded project
// review standard. An issue whose RuleID is missing or unknown is
// discarded: a finding that cannot cite the rule that sustains it never
// reaches the user. A surviving issue keeps the c12d7ab mapRulesToIssues
// enrichment (empty Message/Severity filled from the rule, file path
// cleaned) and gains the citation itself, appended to the message the
// user sees as " (rule <id>: <title>)".
func enforceRuleCitations(rules *RulesLoader, result *types.ReviewResult) (int, discardSummary) {
	kept := make([]types.ReviewIssue, 0, len(result.Issues))
	var discarded discardSummary
	seenUnknown := make(map[string]bool)
	for _, issue := range result.Issues {
		rule, ok := rules.Get(issue.RuleID)
		if !ok {
			if issue.RuleID == "" {
				discarded.Missing++
			} else {
				discarded.Unknown++
				if !seenUnknown[issue.RuleID] {
					seenUnknown[issue.RuleID] = true
					discarded.UnknownIDs = append(discarded.UnknownIDs, issue.RuleID)
				}
			}
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
	sort.Strings(discarded.UnknownIDs)
	return discarded.Missing + discarded.Unknown, discarded
}

// formatDiscardWarning renders discarded as the one-line explanation
// GenerateReview's caller prints to stderr: how many findings the rule gate
// discarded and why (a missing rule_id, an unknown rule_id, or both) --
// never a silent "No issues found." when the model actually reported
// something the gate then hid (AUR-448). Returns "" when nothing was
// discarded, so the happy path (every finding cites a resolvable rule)
// never receives a byte on stderr.
func formatDiscardWarning(discarded discardSummary) string {
	total := discarded.Missing + discarded.Unknown
	if total == 0 {
		return ""
	}
	var reasons []string
	if discarded.Missing > 0 {
		reasons = append(reasons, fmt.Sprintf("%d with no rule_id", discarded.Missing))
	}
	if discarded.Unknown > 0 {
		reasons = append(reasons, fmt.Sprintf("%d citing an unknown rule_id (%s)", discarded.Unknown, strings.Join(discarded.UnknownIDs, ", ")))
	}
	return fmt.Sprintf("%d finding(s) discarded: %s", total, strings.Join(reasons, ", "))
}
