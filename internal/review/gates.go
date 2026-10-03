package review

import (
	"fmt"

	"github.com/Mpaape/AurumCode/internal/prompt"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// gateOutcome records what the engine's gates removed from a parsed
// review, so the result's metadata can say so.
type gateOutcome struct {
	workflowSuppressed int
	scopeDiscarded     scopeDiscardSummary
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
	result.Issues, outcome.scopeDiscarded = filterModelIssues(diff, result.Issues)

	// Rule gate (AUR-434): every issue must cite a rule of the project
	// review standard. A broken or empty embedded catalog is a loud
	// error here, never a silent zero-rule review.
	rules, err := sharedRules()
	if err != nil {
		return gateOutcome{}, fmt.Errorf("review rules unavailable: %w", err)
	}
	outcome.rejected, outcome.discarded = enforceRuleCitations(rules, r.extraRules, result)

	// AUR-517: the model wrote result.Summary knowing every finding it
	// proposed, including the ones the gates above removed; a removed
	// finding can still be named in that prose. The gate is on the FACT that
	// something was discarded, never on the summary's content. The degraded
	// parse notice (prompt.IsDegradedParse) is the one summary kept, because
	// it is the only sentence telling a reader the reply was unusable.
	if !prompt.IsDegradedParse(result) {
		result.Summary = withholdSummaryWhenFiltered(result.Summary, outcome.total())
	}
	return outcome, nil
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
	result.Metadata["scope_discard_warning"] = outcome.scopeDiscarded.warning()
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
