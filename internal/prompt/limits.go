package prompt

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// SlotLimits are the token ceilings of the review prompt and of its
// budgeted slots. Defaults come from templates/limits.yml, never from a
// literal in Go.
type SlotLimits struct {
	PromptMaxTokens      int `yaml:"prompt_max_tokens"`
	RuleCatalogMaxTokens int `yaml:"rule_catalog_max_tokens"`
	EvidenceMaxTokens    int `yaml:"evidence_max_tokens"`
	ToolsMaxTokens       int `yaml:"tools_max_tokens"`
}

const limitsFile = "templates/limits.yml"

// DefaultSlotLimits returns the ceilings declared in templates/limits.yml.
// The file is embedded, so a missing or malformed one is a build defect and
// is returned as an error the caller must surface.
func DefaultSlotLimits() (SlotLimits, error) {
	raw, err := templateFS.ReadFile(limitsFile)
	if err != nil {
		return SlotLimits{}, fmt.Errorf("reading %s: %w", limitsFile, err)
	}
	var limits SlotLimits
	if err := yaml.Unmarshal(raw, &limits); err != nil {
		return SlotLimits{}, fmt.Errorf("parsing %s: %w", limitsFile, err)
	}
	if limits.PromptMaxTokens <= 0 || limits.RuleCatalogMaxTokens <= 0 || limits.EvidenceMaxTokens <= 0 || limits.ToolsMaxTokens <= 0 {
		return SlotLimits{}, fmt.Errorf("%s must declare every ceiling as a positive token count: %+v", limitsFile, limits)
	}
	return limits, nil
}

// mustDefaultSlotLimits is DefaultSlotLimits for package initialization:
// the embedded file is part of the binary, so failing to read it is a
// build defect that must stop the process rather than run unbounded.
func mustDefaultSlotLimits() SlotLimits {
	limits, err := DefaultSlotLimits()
	if err != nil {
		panic(err)
	}
	return limits
}

// defaultSlotLimits is loaded once; builders copy it.
var defaultSlotLimits = mustDefaultSlotLimits()
