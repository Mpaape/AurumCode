package config

import (
	"fmt"
	"strings"
)

// ChangelogMode is what the changelog_check section asks of a pull request.
type ChangelogMode string

const (
	// ChangelogOff (the default) leaves the changelog alone.
	ChangelogOff ChangelogMode = "off"
	// ChangelogSuggest writes the suggested entry when a useful one is
	// missing and never fails the check.
	ChangelogSuggest ChangelogMode = "suggest"
	// ChangelogRequired fails the check without a useful entry and still
	// offers the suggested one.
	ChangelogRequired ChangelogMode = "required"
)

// changelogModeNames lists the accepted spellings of each mode, Portuguese
// synonyms included.
var changelogModeNames = map[string]ChangelogMode{
	"":            ChangelogOff,
	"off":         ChangelogOff,
	"desligado":   ChangelogOff,
	"suggest":     ChangelogSuggest,
	"sugerir":     ChangelogSuggest,
	"sugestao":    ChangelogSuggest,
	"sugestão":    ChangelogSuggest,
	"required":    ChangelogRequired,
	"obrigatorio": ChangelogRequired,
	"obrigatório": ChangelogRequired,
}

// changelogModeRank orders the modes from the weakest to the strictest.
var changelogModeRank = map[ChangelogMode]int{ChangelogOff: 0, ChangelogSuggest: 1, ChangelogRequired: 2}

// ChangelogCheckConfig is the changelog_check section (nil = not declared =
// off): whether a pull request must add a changelog entry, may only receive
// a suggested one, or neither, and the limits that keep the entry concise.
// Zero values keep the embedded defaults of internal/changelog
// (require_defaults.yml). It is independent of review.changelog, which only
// suggests a release and never gates.
type ChangelogCheckConfig struct {
	// Mode is "off" (the default), "suggest" or "required".
	Mode string `yaml:"mode"`
	// Bots is the mode for a pull request opened by a bot account
	// (AUR-610): "suggest" (the default), "off" or "required". It only
	// lowers Mode, never raises it.
	Bots            string   `yaml:"bots"`
	File            string   `yaml:"file"`
	Section         string   `yaml:"section"`
	MaxEntryLines   int      `yaml:"max_entry_lines"`
	MaxLineLength   int      `yaml:"max_line_length"`
	MaxReleaseLines int      `yaml:"max_release_lines"`
	MinWords        int      `yaml:"min_words"`
	AgentLogMarkers []string `yaml:"agent_log_markers"`
}

// EffectiveMode is the declared mode; an absent section or an unknown mode
// (refused by Validate) is off.
func (c *ChangelogCheckConfig) EffectiveMode() ChangelogMode {
	if c == nil {
		return ChangelogOff
	}
	mode, err := ParseChangelogMode(c.Mode)
	if err != nil {
		return ChangelogOff
	}
	return mode
}

// EffectiveModeFor is the mode for one author: a person gets
// EffectiveMode; a bot gets the weaker of EffectiveMode and Bots, so the
// bots policy can relax the check but never tighten it. An unknown Bots
// (refused by Validate) lowers nothing.
func (c *ChangelogCheckConfig) EffectiveModeFor(bot bool) ChangelogMode {
	mode := c.EffectiveMode()
	if !bot || c == nil {
		return mode
	}
	bots, err := ParseChangelogBotsMode(c.Bots)
	if err != nil {
		return mode
	}
	if changelogModeRank[bots] < changelogModeRank[mode] {
		return bots
	}
	return mode
}

// Required reports whether a pull request without a useful entry fails.
func (c *ChangelogCheckConfig) Required() bool {
	return c.EffectiveMode() == ChangelogRequired
}

// Active reports whether the section asks for anything at all: a suggested
// entry (suggest) or a required one (required).
func (c *ChangelogCheckConfig) Active() bool {
	return c.EffectiveMode() != ChangelogOff
}

// ParseChangelogMode accepts the three modes and their synonyms; anything
// else is refused with the list of valid modes.
func ParseChangelogMode(raw string) (ChangelogMode, error) {
	return parseChangelogModeField("mode", raw)
}

// ParseChangelogBotsMode reads changelog_check.bots: empty is suggest, so a
// bot's pull request is never failed unless the repository asks for it.
func ParseChangelogBotsMode(raw string) (ChangelogMode, error) {
	if strings.TrimSpace(raw) == "" {
		return ChangelogSuggest, nil
	}
	return parseChangelogModeField("bots", raw)
}

func parseChangelogModeField(field, raw string) (ChangelogMode, error) {
	mode, ok := changelogModeNames[strings.ToLower(strings.TrimSpace(raw))]
	if !ok {
		return ChangelogOff, fmt.Errorf("changelog_check.%s: %q nao e suportado (use off, suggest ou required)", field, raw)
	}
	return mode, nil
}

// Validate refuses an unknown mode, negative limits and a path that leaves
// the repository.
func (c *ChangelogCheckConfig) Validate() error {
	if c == nil {
		return nil
	}
	if _, err := ParseChangelogMode(c.Mode); err != nil {
		return err
	}
	if _, err := ParseChangelogBotsMode(c.Bots); err != nil {
		return err
	}
	for name, v := range map[string]int{"max_entry_lines": c.MaxEntryLines, "max_line_length": c.MaxLineLength, "max_release_lines": c.MaxReleaseLines, "min_words": c.MinWords} {
		if v < 0 {
			return fmt.Errorf("changelog_check.%s: deve ser positivo (recebido %d)", name, v)
		}
	}
	if strings.TrimSpace(c.File) != "" {
		if err := validateContextPath(c.File); err != nil {
			return fmt.Errorf("changelog_check.file %q: %w", c.File, err)
		}
	}
	return nil
}

// mergeChangelogCheck governs the section like batches: a policy that
// declares it decides alone; a policy silent on it leaves the repository's.
func mergeChangelogCheck(effective *Config, repo, central *Config) []ProviderWarning {
	if central.ChangelogCheck == nil {
		return nil
	}
	effective.ChangelogCheck = central.ChangelogCheck
	if repo.ChangelogCheck == nil {
		return nil
	}
	return []ProviderWarning{ignoredWarning("changelog_check do config do repositório foi ignorado: a política central decide sozinha")}
}
