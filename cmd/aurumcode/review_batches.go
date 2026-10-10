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
// zero and the reviewer applies its embedded default.
func configuredBatchLimits(cfg *config.Config) review.BatchLimits {
	if cfg == nil {
		return review.BatchLimits{}
	}
	maxBatches, maxTokens := cfg.Batches.Limits()
	return review.BatchLimits{MaxBatches: maxBatches, MaxPromptTokens: maxTokens}
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
