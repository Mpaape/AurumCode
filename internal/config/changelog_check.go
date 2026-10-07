package config

import (
	"fmt"
	"strings"
)

// ChangelogCheckConfig is the changelog_check section (AUR-509; nil = not
// declared = off): whether every pull request must add a changelog entry,
// and the limits that keep it concise. Zero values keep the embedded
// defaults of internal/changelog (require_defaults.yml). It is independent
// of review.changelog, which only suggests an entry and never gates.
type ChangelogCheckConfig struct {
	// Mode is "required" or "off" (the default).
	Mode            string   `yaml:"mode"`
	File            string   `yaml:"file"`
	Section         string   `yaml:"section"`
	MaxEntryLines   int      `yaml:"max_entry_lines"`
	MaxLineLength   int      `yaml:"max_line_length"`
	MaxReleaseLines int      `yaml:"max_release_lines"`
	MinWords        int      `yaml:"min_words"`
	AgentLogMarkers []string `yaml:"agent_log_markers"`
}

// Required reports whether the section turns the check on.
func (c *ChangelogCheckConfig) Required() bool {
	if c == nil {
		return false
	}
	mode, err := normalizeChangelogMode(c.Mode)
	return err == nil && mode
}

func normalizeChangelogMode(raw string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "off", "desligado":
		return false, nil
	case "required", "obrigatorio", "obrigatório":
		return true, nil
	default:
		return false, fmt.Errorf("changelog_check.mode: %q nao e suportado (use required ou off)", raw)
	}
}

// Validate refuses an unknown mode, negative limits and a path that leaves
// the repository.
func (c *ChangelogCheckConfig) Validate() error {
	if c == nil {
		return nil
	}
	if _, err := normalizeChangelogMode(c.Mode); err != nil {
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
