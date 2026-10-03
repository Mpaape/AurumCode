package prompt

// contextSections are the user-message slots whose size does not depend on
// how many diff hunks fit: each is rendered whole (or not at all, when its
// input is empty) before the hunks are budgeted around them.
type contextSections struct {
	history  string
	codebase string
	memory   string
	evidence string
	// evidenceIDs are the evidence items the evidence section admitted.
	evidenceIDs []string
	tools       string
	repository  string
}

func (b *PromptBuilder) renderContextSections(opts BuildOptions) contextSections {
	evidence, evidenceIDs := renderEvidenceSlot(opts.Evidence, b.limits.EvidenceMaxTokens, b.estimator)
	return contextSections{
		history:     renderOptionalSlot(slotPRHistory, opts.ReviewHistory),
		codebase:    renderOptionalSlot(slotCodebaseContext, opts.CodebaseContext),
		memory:      renderOptionalSlot(slotReviewMemory, opts.MemoryNotes),
		evidence:    evidence,
		evidenceIDs: evidenceIDs,
		tools:       renderToolsSlot(opts.Tools, b.limits.ToolsMaxTokens, b.estimator),
		repository:  renderOptionalSlot(slotRepositoryContextUser, opts.RepositoryContext),
	}
}

// tokens estimates every section the same way the budget loop does: one
// estimate per rendered part.
func (s contextSections) tokens(est TokenEstimator) int {
	total := 0
	for _, part := range []string{s.history, s.codebase, s.memory, s.evidence, s.tools, s.repository} {
		if part != "" {
			total += est.Estimate(part)
		}
	}
	return total
}

// assemble lays the user message out in its fixed slot order: code-change
// header, background sections, the coverage declaration, and the
// repository context last.
func (s contextSections) assemble(header, coverage string) string {
	return header + s.history + s.codebase + s.memory + s.evidence + s.tools + "\n" + coverage + s.repository
}
