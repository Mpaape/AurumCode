package prompt

import (
	"fmt"
	"io/fs"

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
func DefaultSlotLimits() (SlotLimits, error) { return loadSlotLimits(templateFS) }

// loadSlotLimits reads and validates the limits file from fsys.
func loadSlotLimits(fsys fs.ReadFileFS) (SlotLimits, error) {
	raw, err := fsys.ReadFile(limitsFile)
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

// defaultSlotLimits is loaded once; builders copy it. A load failure is
// kept in defaultSlotLimitsErr instead of stopping the process: every
// builder entry point returns it (EmbeddedDefaultsErr), so no prompt is
// ever assembled with zero, unbounded ceilings.
var defaultSlotLimits, defaultSlotLimitsErr = DefaultSlotLimits()

// DefaultLimits returns the ceilings loaded from templates/limits.yml at
// package initialization. When that load failed it returns the zero value;
// callers that budget with it must check EmbeddedDefaultsErr first.
func DefaultLimits() SlotLimits { return defaultSlotLimits }
