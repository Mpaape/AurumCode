// A review in batches seen from the command: the configured ceilings, the
// line that says the review was split, and the batches in the audit record.
package main

import (
	"fmt"
	"strings"

	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/i18n"
	"github.com/Mpaape/AurumCode/internal/render"
	"github.com/Mpaape/AurumCode/internal/review"
)

// configuredBatchLimits reads the batches section; an undeclared ceiling is
// zero and bounds nothing.
func configuredBatchLimits(cfg *config.Config) review.BatchLimits {
	if cfg == nil {
		return review.BatchLimits{}
	}
	maxBatches, maxTokens := cfg.Batches.Limits()
	return review.BatchLimits{MaxBatches: maxBatches, MaxPromptTokens: maxTokens}
}

// noteRefusals says on stderr, once per refusal, that the model refused the
// prompt as longer than its context window (or cut its reply short) and the
// review went on in smaller parts (internal/review/split.go).
func (s *reviewState) noteRefusals(refusals []review.PromptRefusal) {
	for _, r := range refusals {
		line := i18n.Format(s.reviewLanguage, "terminal.prompt_refused_unsized", r.EstimatedTokens)
		switch {
		case r.Cut:
			line = i18n.Format(s.reviewLanguage, "terminal.reply_cut", r.EstimatedTokens)
		case r.Limit > 0:
			line = i18n.Format(s.reviewLanguage, "terminal.prompt_refused", r.EstimatedTokens, r.Limit)
		}
		fmt.Fprintf(s.stderr, "aurumcode review: %s\n", line)
	}
}

// noteBatches keeps the batches for the audit and says on stderr that the
// review was split, with the files of each batch counted.
func (s *reviewState) noteBatches(batches []review.Batch) {
	s.batches = batches
	if len(batches) == 0 {
		return
	}
	if s.result != nil {
		if s.result.Metadata == nil {
			s.result.Metadata = map[string]string{}
		}
		s.result.Metadata[metaReviewParts] = fmt.Sprintf("%d", len(batches))
	}
	sizes := make([]string, 0, len(batches))
	for _, b := range batches {
		sizes = append(sizes, fmt.Sprintf("%d", len(b.Files)))
	}
	fmt.Fprintf(s.stderr, "aurumcode review: %s\n", i18n.Format(s.reviewLanguage, batchesKey(len(batches)), len(batches), strings.Join(sizes, ", ")))
}

// auditBatches converts the batches for the audit record.
func auditBatches(batches []review.Batch) []render.AuditBatch {
	if len(batches) == 0 {
		return nil
	}
	out := make([]render.AuditBatch, 0, len(batches))
	for _, b := range batches {
		out = append(out, render.AuditBatch{Index: b.Index, Files: append([]string{}, b.Files...), EstimatedTokens: b.EstimatedTokens})
	}
	return out
}

// splitNonEmptyLines splits engine metadata holding one value per line.
func splitNonEmptyLines(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			out = append(out, line)
		}
	}
	return out
}

// batchesKey picks the singular sentence for a single batch: "1 lote", never
// "1 lotes". English keeps the plural bytes in both keys.
func batchesKey(n int) string {
	if n == 1 {
		return "terminal.batches_one"
	}
	return "terminal.batches"
}

// metaContextReduced is the metadata key that says the codebase context was
// reduced to fit the model's window (internal/review/context_fit.go): the
// terminal line, set only when it was.
const metaContextReduced = "codebase_context_reduced"

// noteContextReduction says on stderr, once per review, that the codebase
// context was reduced (or left out) to fit the model's window, and keeps
// the line for the result and the audit (internal/review/context_fit.go).
func (s *reviewState) noteContextReduction(reduction review.ContextReduction, ok bool) {
	if !ok || s.contextReduced != "" {
		return
	}
	var lines []string
	switch {
	case reduction.Discarded:
		lines = append(lines, i18n.Format(s.reviewLanguage, "terminal.context_discarded", reduction.OriginalTokens))
	case reduction.Reduction.Reduced():
		lines = append(lines, i18n.Format(s.reviewLanguage, "terminal.context_reduced", reduction.OriginalTokens, reduction.KeptTokens,
			reduction.SnippetsKept, reduction.Snippets, reduction.ReferencesKept, reduction.References))
	}
	if reduction.EvidenceKept < reduction.Evidence {
		lines = append(lines, i18n.Format(s.reviewLanguage, "terminal.evidence_reduced", reduction.Evidence-reduction.EvidenceKept, reduction.Evidence))
	}
	for _, line := range lines {
		fmt.Fprintf(s.stderr, "aurumcode review: %s\n", line)
	}
	s.contextReduced = strings.Join(lines, "; ")
	s.markContextReduced()
}

// markContextReduced records the reduction in the result's metadata, which
// the published summary reads; a no-op without a result or a reduction.
func (s *reviewState) markContextReduced() {
	if s.result == nil || s.contextReduced == "" {
		return
	}
	if s.result.Metadata == nil {
		s.result.Metadata = map[string]string{}
	}
	s.result.Metadata[metaContextReduced] = s.contextReduced
}
