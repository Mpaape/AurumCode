package review

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/Mpaape/AurumCode/internal/analyzer"
	"github.com/Mpaape/AurumCode/internal/deliberation"
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
	completer     Completer
	diffAnalyzer  *analyzer.DiffAnalyzer
	promptBuilder *prompt.PromptBuilder
	parser        *prompt.ResponseParser
	filter        *redaction.Filter
	cfg           Config
	// extraRules is AUR-519's per-run dynamic rule set (skill sections from
	// the central policy and, when the repository opts in, its own
	// skills), installed by SetDynamicRules. nil is the zero-config
	// default: no dynamic rule exists and enforceRuleCitations behaves
	// exactly as before this card.
	extraRules map[string]Rule
	// deliberation, when set, lets the model ask for tools (deliberate.go);
	// transcript is the last deliberation's record.
	deliberation *Deliberation
	transcript   *deliberation.Transcript
	// batches records the batches of the last review, nil when the diff
	// fit one prompt.
	batches []Batch
}

// Completer is what the Reviewer needs from the model side: one call that
// takes the prompt as separate system and user messages. *llm.Orchestrator
// implements it (with its fallback chain and cost ceiling); a test may pass
// any other implementation.
type Completer interface {
	CompleteMessages(ctx context.Context, messages []llm.Message, opts llm.Options) (llm.Response, error)
}

// Config holds reviewer configuration.
type Config struct {
	// MaxTokens, when positive, is both the prompt budget and (minus
	// ReserveReply) the reply cap sent to the provider.
	MaxTokens   int
	Temperature float64
	// ReserveReply is subtracted from MaxTokens for the reply.
	ReserveReply int
	// PromptTokenBudget bounds the assembled prompt when MaxTokens is zero.
	// It never caps the reply.
	PromptTokenBudget int
	// Batches bounds a review split in batches (batches.go).
	Batches BatchLimits
}

// ReviewContext contains optional evidence available beside the diff. It is
// kept separate from the diff so a caller can add CI status without changing
// the code-review input shape or requiring repository-specific prompt files.
type ReviewContext struct {
	CI              string
	Language        string
	History         string // Attributed PR observations, not review instructions
	CodebaseContext string // Untrusted, bounded, heuristic codebase dependency context
	MemoryNotes     string // Untrusted, attributed observations from review memory
	// RepositoryContext is the rendered repository context block
	// (config.BuildContextBlockWithWarnings). It travels in its own prompt
	// slot, inside the budget, instead of being appended by a provider
	// decorator.
	RepositoryContext string
	// Evidence is deterministic findings offered to the model for
	// assessment. Every field is redacted before it reaches the prompt.
	Evidence []prompt.EvidenceItem
	// Tools are the tools the model may ask for, with declared cost.
	Tools []prompt.ToolOffer
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
		// Zero means no client-side output cap. The selected provider/model
		// owns its supported response limit.
		MaxTokens:    0,
		ReserveReply: 0,
		// The prompt itself is bounded by the default ceiling declared in
		// internal/prompt/templates/limits.yml: a diff that does not fit is
		// trimmed by whole hunks and declared in the coverage section.
		PromptTokenBudget: prompt.DefaultLimits().PromptMaxTokens,
		Batches:           DefaultBatchLimits(),
	}
}

// NewReviewer creates a new reviewer around completer -- in production an
// *llm.Orchestrator, whose provider chain may be a FakeProvider (see
// fakeprovider.go) in a test or an offline CLI invocation; the Reviewer
// itself never knows the difference.
func NewReviewer(completer Completer, cfg Config) *Reviewer {
	if cfg.MaxTokens == 0 && cfg.PromptTokenBudget == 0 {
		defaults := DefaultConfig()
		cfg.PromptTokenBudget = defaults.PromptTokenBudget
	}
	cfg.Batches = cfg.Batches.withDefaults()
	return &Reviewer{
		completer:     completer,
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

// SetDynamicRules installs AUR-519's per-run dynamic rule set, keyed by rule
// id exactly as enforceRuleCitations resolves it (resolveRule,
// skillrules.go). cmd/aurumcode builds this map by parsing the central
// policy's and, when the repository opts in, the repository's own skill
// files (ParseSkillSections) before calling GenerateReview. nil/empty keeps
// today's behavior: only the embedded catalog's ids are accepted.
func (r *Reviewer) SetDynamicRules(rules map[string]Rule) {
	r.extraRules = rules
}

// SetRuleCatalog passes an expanded rule_id list down to the prompt
// builder, so the model is taught exactly the ids SetDynamicRules will
// accept -- the embedded catalog's ids plus AUR-519's dynamic skill-section
// ids. It validates eagerly (the same token-budget and non-empty/sorted
// checks prompt.ValidateRuleCatalog already applies) and never truncates: a
// catalog that no longer fits the prompt budget is a loud error here.
func (r *Reviewer) SetRuleCatalog(ids []string) error {
	return r.promptBuilder.SetRuleCatalog(ids)
}

// GenerateReview generates a code review for diff.
func (r *Reviewer) GenerateReview(ctx context.Context, diff *types.Diff) (*types.ReviewResult, error) {
	return r.GenerateReviewWithContext(ctx, diff, ReviewContext{})
}

// GenerateReviewWithContext generates a review using the diff and optional
// external evidence such as completed CI check statuses. It runs four
// stages: assemble the redacted prompt, ask the model, parse its reply, and
// pass the parsed findings through the engine's gates.
//
// A diff that does not fit one prompt is reviewed in batches (batches.go):
// the same four stages per batch, one consolidated result.
func (r *Reviewer) GenerateReviewWithContext(ctx context.Context, diff *types.Diff, reviewContext ReviewContext) (*types.ReviewResult, error) {
	r.batches = nil
	prepared, err := r.preparePrompt(diff, reviewContext)
	if err != nil {
		return nil, err
	}
	split, err := r.needsBatches(diff, prepared, reviewContext)
	if err != nil {
		return nil, err
	}
	if split {
		return r.reviewInBatches(ctx, diff, reviewContext)
	}
	return r.reviewPrepared(ctx, prepared)
}

// reviewPrepared asks the model about one assembled prompt and passes the
// answer through the parser and the engine's gates.
func (r *Reviewer) reviewPrepared(ctx context.Context, prepared preparedPrompt) (*types.ReviewResult, error) {
	resp, err := r.answer(ctx, prepared.parts)
	if err != nil {
		return nil, err
	}
	result, err := r.parse(resp)
	if err != nil {
		return nil, err
	}
	weighAssessments(result, admittedEvidence(prepared.parts))
	outcome, err := r.applyGates(prepared.diff, result)
	if err != nil {
		return nil, err
	}
	annotateResult(result, prepared, outcome)
	return result, nil
}

// PromptDigest returns the digest of the exact system and user messages
// GenerateReviewWithContext would send for diff and reviewContext, without
// calling the model. Equal inputs give equal digests; any change to the
// rendered text -- evidence included -- moves it.
func (r *Reviewer) PromptDigest(diff *types.Diff, reviewContext ReviewContext) (string, error) {
	prepared, err := r.preparePrompt(diff, reviewContext)
	if err != nil {
		return "", err
	}
	return prepared.parts.Digest(), nil
}

// preparedPrompt is the output of the assembly stage.
type preparedPrompt struct {
	diff    *types.Diff // redacted copy: everything downstream sees this one
	metrics *analyzer.DiffMetrics
	parts   prompt.PromptParts
}

// preparePrompt redacts every untrusted input and assembles the budgeted
// prompt from the review template's slots.
func (r *Reviewer) preparePrompt(diff *types.Diff, reviewContext ReviewContext) (preparedPrompt, error) {
	// Analyze diff (counts only; metrics carry no content into the prompt)
	metrics := r.diffAnalyzer.AnalyzeDiff(diff)

	// The diff is the untrusted material that will leave this process
	// toward a model, so it passes the single AUR-009 redaction filter
	// BEFORE the prompt is assembled (AUR-432). Composition matters, not
	// just coverage: a diff line carries a +/-/space marker, and the
	// filter's header rule is anchored at line start, so redacting the
	// assembled prompt would let "+Authorization: Bearer x" through
	// verbatim. redactDiff strips each line's marker, redacts the bodies
	// as the filter would see them on their own, and re-prefixes.
	diff = redactDiff(r.filter, diff)

	opts := prompt.BuildOptions{
		MaxTokens:         r.promptBudget(),
		SchemaKind:        "review",
		Role:              "reviewer",
		ReserveReply:      r.cfg.ReserveReply,
		CIContext:         r.filter.Redact(reviewContext.CI),
		ReviewHistory:     r.filter.Redact(reviewContext.History),
		CodebaseContext:   r.filter.Redact(reviewContext.CodebaseContext),
		MemoryNotes:       r.filter.Redact(reviewContext.MemoryNotes),
		Language:          reviewContext.Language,
		RepositoryContext: r.filter.Redact(reviewContext.RepositoryContext),
		Evidence:          redactEvidence(r.filter, reviewContext.Evidence),
		Tools:             redactTools(r.filter, reviewContext.Tools),
	}

	parts, err := r.promptBuilder.BuildPrompt(diff, metrics, opts)
	if err != nil {
		return preparedPrompt{}, fmt.Errorf("failed to build prompt: %w", err)
	}
	return preparedPrompt{diff: diff, metrics: metrics, parts: parts}, nil
}

// promptBudget is the explicit MaxTokens when set, else the default prompt
// ceiling.
func (r *Reviewer) promptBudget() int {
	if r.cfg.MaxTokens > 0 {
		return r.cfg.MaxTokens
	}
	return r.cfg.PromptTokenBudget
}

// complete sends the prompt as separate system and user messages. The
// system message is the trusted embedded template; everything untrusted in
// the user message was redacted by preparePrompt.
func (r *Reviewer) complete(ctx context.Context, parts prompt.PromptParts) (llm.Response, error) {
	maxReplyTokens := 0
	if r.cfg.MaxTokens > r.cfg.ReserveReply {
		maxReplyTokens = r.cfg.MaxTokens - r.cfg.ReserveReply
	}
	// The parser requires JSON. Ask the provider to constrain syntax instead
	// of relying on prompt wording alone; schema and finding evidence are
	// still validated locally after the response arrives.
	resp, err := r.completer.CompleteMessages(ctx, parts.Messages(), llm.Options{
		MaxTokens:   maxReplyTokens,
		Temperature: r.cfg.Temperature,
		JSONMode:    true,
	})
	if err != nil {
		return llm.Response{}, fmt.Errorf("LLM request failed: %w", err)
	}
	return resp, nil
}

// parse decodes the model reply and redacts every model-authored string.
func (r *Reviewer) parse(resp llm.Response) (*types.ReviewResult, error) {
	result, err := r.parser.ParseReviewResponse(resp.Text)
	if err != nil {
		var parseErr *prompt.ParseError
		if errors.As(err, &parseErr) {
			parseErr.FinishReason = resp.FinishReason
		}
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
	//
	// A finding that already cites the redaction marker in the raw reply
	// echoes the mask the model saw in its input, so it is removed here,
	// before redactReviewResult: after it, a finding quoting a real
	// secret-shaped value would carry the same marker and be
	// indistinguishable.
	var echoed int
	result.Issues, echoed = discardRedactedModelFindings(result.Issues)
	redactReviewResult(r.filter, result)
	if result.Metadata == nil {
		result.Metadata = make(map[string]string)
	}
	result.Metadata[RedactionMarkerDiscardKey] = strconv.Itoa(echoed)
	return result, nil
}

// gateOutcome records what the engine's gates removed from a parsed
// review, so the result's metadata can say so.
type gateOutcome struct {
	workflowSuppressed int
	scopeDiscarded     scopeDiscardSummary
	outsideDiff        []types.ReviewIssue
	rejected           int
	discarded          discardSummary
}

func (o gateOutcome) total() int {
	return o.workflowSuppressed + o.scopeDiscarded.total() + o.rejected
}

// applyGates passes the parsed findings through the workflow-reference
// filter, the scope/evidence precision gate and the rule gate, in that
// order, and withholds the model's summary when any of them removed
// something.
func (r *Reviewer) applyGates(diff *types.Diff, result *types.ReviewResult) (gateOutcome, error) {
	var outcome gateOutcome
	// Model output is also allowed to propose a hardcoded-secret finding on a
	// safe workflow reference. Apply the narrow, source-aware filter before
	// the rule gate so `${{ secrets.NAME }}`, permission scopes, and event
	// types cannot become a blocking issue while literal values remain visible
	// to the deterministic security pass.
	issuesBefore := len(result.Issues)
	suggestionsBefore := len(result.Suggestions)
	result.Issues = suppressWorkflowReferenceFindings(diff, result.Issues)
	result.Suggestions = suppressWorkflowReferenceSuggestions(diff, result.Suggestions)
	outcome.workflowSuppressed = (issuesBefore - len(result.Issues)) + (suggestionsBefore - len(result.Suggestions))

	// Precision gate: the model may use repository context, language knowledge
	// and configured prompts to reason about a change, but it cannot promote a
	// concern about untouched code into a finding for this patch. The finding
	// also has to carry the three pieces of proof the prompt requests.
	// A proved finding outside the changed lines is set apart, never kept
	// in result.Issues: it becomes a general comment that the policy gate,
	// the threshold and the verdict never see.
	var outside []types.ReviewIssue
	result.Issues, outside, outcome.scopeDiscarded = filterModelIssues(diff, result.Issues)
	outcome.scopeDiscarded.CitesRedactionMarker, _ = strconv.Atoi(result.Metadata[RedactionMarkerDiscardKey])

	// Rule gate (AUR-434): every issue must cite a rule of the project
	// review standard. A broken or empty embedded catalog is a loud
	// error here, never a silent zero-rule review.
	rules, err := sharedRules()
	if err != nil {
		return gateOutcome{}, fmt.Errorf("review rules unavailable: %w", err)
	}
	outcome.rejected, outcome.discarded = enforceRuleCitations(rules, r.extraRules, result)
	outcome.outsideDiff = r.citedOutsideDiff(rules, outside, &outcome)

	// AUR-517: the model wrote result.Summary knowing every finding it
	// proposed, including the ones the gates above removed; a removed
	// finding can still be named in that prose. The gate is on the FACT that
	// something was discarded, never on the summary's content. The degraded
	// parse notice (prompt.IsDegradedParse) is the one summary kept, because
	// it is the only sentence telling a reader the reply was unusable.
	discardedByPipeline := outcome.total()
	if !prompt.IsDegradedParse(result) {
		result.Summary = withholdSummaryWhenFiltered(result.Summary, discardedByPipeline)
	}
	return outcome, nil
}

// citedOutsideDiff passes the general-comment findings through the same
// rule gate as every inline finding; its rejections join the outcome's
// counts and warning, so the requirement is never weaker outside the diff.
func (r *Reviewer) citedOutsideDiff(rules *RulesLoader, outside []types.ReviewIssue, outcome *gateOutcome) []types.ReviewIssue {
	if len(outside) == 0 {
		return nil
	}
	held := &types.ReviewResult{Issues: outside}
	rejected, discarded := enforceRuleCitations(rules, r.extraRules, held)
	outcome.rejected += rejected
	outcome.discarded.Missing += discarded.Missing
	outcome.discarded.Unknown += discarded.Unknown
	outcome.discarded.UnknownIDs = mergeUnknownIDs(outcome.discarded.UnknownIDs, discarded.UnknownIDs)
	return held.Issues
}

// mergeUnknownIDs returns the sorted union of two unknown rule id lists.
func mergeUnknownIDs(a, b []string) []string {
	seen := make(map[string]bool, len(a)+len(b))
	out := make([]string, 0, len(a)+len(b))
	for _, id := range append(append([]string(nil), a...), b...) {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}

// annotateResult writes the engine-derived metadata: what the gates
// discarded, the diff's counts, and the prompt builder's own coverage
// counts. Every key here is engine-owned; the parser already scrubbed any
// same-named key a model's JSON supplied, so these overwrite, never merge.
func annotateResult(result *types.ReviewResult, prepared preparedPrompt, outcome gateOutcome) {
	if result.Metadata == nil {
		result.Metadata = make(map[string]string)
	}
	result.Metadata["issues_rejected_without_rule"] = fmt.Sprintf("%d", outcome.rejected)
	result.Metadata["issues_rejected_by_scope"] = fmt.Sprintf("%d", outcome.scopeDiscarded.total())
	result.Metadata[RedactionMarkerDiscardKey] = fmt.Sprintf("%d", outcome.scopeDiscarded.CitesRedactionMarker)
	result.Metadata["scope_discard_warning"] = outcome.scopeDiscarded.warning()
	setOutsideDiffFindings(result.Metadata, outcome.outsideDiff)
	result.Metadata["summary_discarded_findings"] = fmt.Sprintf("%d", outcome.total())
	// AUR-448: a discard the rule gate makes is never silent; "" on the
	// happy path so a caller printing a non-empty warning writes nothing.
	result.Metadata["discard_warning"] = formatDiscardWarning(outcome.discarded)
	result.Metadata["total_files"] = fmt.Sprintf("%d", prepared.metrics.TotalFiles)
	result.Metadata["lines_added"] = fmt.Sprintf("%d", prepared.metrics.LinesAdded)
	result.Metadata["lines_deleted"] = fmt.Sprintf("%d", prepared.metrics.LinesDeleted)
	result.Metadata["segments_used"] = prepared.parts.Meta["segments_used"]
	result.Metadata["estimated_tokens"] = prepared.parts.Meta["estimated_tokens"]
	// AUR-519 (B-C): the builder's per-file coverage counts reach the
	// result so a budget-truncated file is never read as "complete".
	for _, key := range []string{"code_files_total", "code_files_complete", "code_files_partial", "code_files_omitted"} {
		result.Metadata[key] = prepared.parts.Meta[key]
	}
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
		issue.Side = f.Redact(issue.Side)
		issue.RuleID = f.Redact(issue.RuleID)
		issue.Message = redactLinesKeepingMarkers(f, issue.Message)
		issue.Impact = redactLinesKeepingMarkers(f, issue.Impact)
		issue.Evidence = redactLinesKeepingMarkers(f, issue.Evidence)
		issue.Suggestion = redactLinesKeepingMarkers(f, issue.Suggestion)
		issue.Verification = redactLinesKeepingMarkers(f, issue.Verification)
		if issue.Assessment != nil {
			issue.Assessment.EvidenceID = f.Redact(issue.Assessment.EvidenceID)
			issue.Assessment.Justification = redactLinesKeepingMarkers(f, issue.Assessment.Justification)
		}
	}
	for i := range result.EvidenceAssessments {
		a := &result.EvidenceAssessments[i]
		a.EvidenceID = f.Redact(a.EvidenceID)
		a.Status = f.Redact(a.Status)
		a.Priority = f.Redact(a.Priority)
		a.Justification = redactLinesKeepingMarkers(f, a.Justification)
		a.Suggestion = redactLinesKeepingMarkers(f, a.Suggestion)
		for j := range a.Correlates {
			a.Correlates[j] = f.Redact(a.Correlates[j])
		}
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
func enforceRuleCitations(rules *RulesLoader, extra map[string]Rule, result *types.ReviewResult) (int, discardSummary) {
	kept := make([]types.ReviewIssue, 0, len(result.Issues))
	var discarded discardSummary
	seenUnknown := make(map[string]bool)
	for _, issue := range result.Issues {
		rule, ok := resolveRule(rules, extra, issue.RuleID)
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

// withholdSummaryWhenFiltered returns summary unchanged when discardedCount
// is zero, and "" otherwise. It is the single anchor GenerateReviewWithContext
// uses to stop the model's free-text summary from re-presenting, as though
// still current, a finding the scope/evidence gate, the rule gate, or the
// workflow-reference filter already removed (AUR-517 AC-001). The decision is
// structural -- did the pipeline discard anything at all -- never a search
// for an accusation inside the prose itself.
func withholdSummaryWhenFiltered(summary string, discardedCount int) string {
	if discardedCount > 0 {
		return ""
	}
	return summary
}
