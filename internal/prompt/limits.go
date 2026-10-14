package prompt

// SlotLimits are the optional token ceilings of the budgeted prompt slots.
// The zero value of a field means no ceiling, and every builder starts with
// none (issue #179): a ceiling the product picked on its own broke the
// review of any input bigger than its author guessed. What bounds a prompt is
// the model's own context window, and a provider that refuses a prompt as too
// long makes the reviewer split it (internal/review/split.go). A caller that
// wants a ceiling sets it explicitly with SetSlotLimits.
type SlotLimits struct {
	// EvidenceMaxTokens, when positive, bounds the deterministic evidence
	// section: items past it are counted as omitted, never cut.
	EvidenceMaxTokens int
	// ToolsMaxTokens, when positive, bounds the available-tools section,
	// same rule.
	ToolsMaxTokens int
}
