package prompt

// EvidenceItem is one deterministic finding (a lint, an AST check, a SAST
// rule) offered to the model as structured evidence. Every field is
// engine-produced; the caller redacts each one before it reaches the
// builder, and the model may only assess the item, never rewrite its
// origin.
type EvidenceItem struct {
	ID       string // Stable id the model cites back in an issue's assessment
	Origin   string // Which analyzer produced it, e.g. "sast:semgrep"
	RuleID   string
	File     string
	Line     int
	Severity string
	Snippet  string // Redacted excerpt of the offending code
}

// ToolOffer is one tool the model may ask the engine to run, with the cost
// the engine declares for running it.
type ToolOffer struct {
	Name        string
	Description string
	Cost        string
}

// budgetedSlotData feeds a list slot: the items admitted under the slot's
// ceiling, already rendered, and how many were left out.
type budgetedSlotData struct {
	Items   []string
	Omitted int
}

// renderBudgetedSlot renders items into the slot named slotName, admitting
// them in order while the whole section stays within maxTokens. An item
// that does not fit is counted, never cut, and the section states how many
// were omitted. No items renders nothing at all.
func renderBudgetedSlot(slotName string, items []string, maxTokens int, est TokenEstimator) string {
	if len(items) == 0 {
		return ""
	}
	data := budgetedSlotData{}
	for _, item := range items {
		admitted := append(append([]string(nil), data.Items...), item)
		// Measure with every not-yet-admitted item counted as omitted: the
		// final omitted count can only be smaller, so the section that is
		// finally rendered never exceeds what was measured here.
		candidate := budgetedSlotData{Items: admitted, Omitted: len(items) - len(admitted)}
		if est.Estimate(renderSlot(slotName, candidate)) > maxTokens {
			continue
		}
		data.Items = admitted
	}
	data.Omitted = len(items) - len(data.Items)
	return renderSlot(slotName, data)
}

// renderEvidenceSlot renders the deterministic evidence section.
func renderEvidenceSlot(evidence []EvidenceItem, maxTokens int, est TokenEstimator) string {
	items := make([]string, 0, len(evidence))
	for _, e := range evidence {
		items = append(items, renderSlot(slotEvidenceItem, e))
	}
	return renderBudgetedSlot(slotDeterministicEvidence, items, maxTokens, est)
}

// renderToolsSlot renders the available-tools section.
func renderToolsSlot(tools []ToolOffer, maxTokens int, est TokenEstimator) string {
	items := make([]string, 0, len(tools))
	for _, t := range tools {
		items = append(items, renderSlot(slotToolItem, t))
	}
	return renderBudgetedSlot(slotAvailableTools, items, maxTokens, est)
}
