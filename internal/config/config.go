// Package config is AUR-452's seam: "context providers" that feed
// additional, UNTRUSTED material into the review call, plus the small set
// of EXPLICIT, human-authored settings (rule on/off, severity override,
// ignored paths) that this repository's own .aurumcode/config.yml has
// authority to change.
//
// This card is the foundation four later cards build on: AUR-468 (skills),
// AUR-469 (MCP) and AUR-470 (RAG) each add a ContextProvider; AUR-471 adds
// an ISO policy provider. From the engine's point of view all of them are
// the same thing -- a named source of free text handed to the model as
// background -- which is exactly what ContextProvider (provider.go)
// captures.
//
// # THE SECURITY BOUNDARY THAT EVERY LATER CARD INHERITS
//
// Content that arrives through a ContextProvider -- a repository prompt
// file, a skill, an MCP tool result, a RAG chunk -- is DATA, never an
// instruction to this program. It can only ever reach the outbound model
// prompt as clearly labeled background text (see BuildContextBlock). It
// is never parsed for directives, and nothing in this package (or in
// cmd/aurumcode's wiring of it) inspects provider text to decide whether a
// rule is enabled, what severity a finding gets, where --fail-on's
// threshold sits, whether secret redaction runs, or what the cost ceiling
// is. Those five things are controlled exclusively by two things this
// package also owns: the EXPLICIT, versioned Config a human wrote
// (rules/ignore, this file) and code (--fail-on, redaction and --limite
// remain entirely outside this card, in cmd/aurumcode and
// internal/security/redaction, untouched). ApplyRuleConfig (rules.go)
// reads only Config; it has no parameter through which provider text
// could reach it even if a caller wanted that. tests/unit/AUR-452.go
// proves the split explicitly: a provider whose contributed text asks to
// disable a rule sits in the assembled prompt, verbatim, and the rule
// gate is unmoved.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// RuleConfig is one rule's explicit override. Enabled is a pointer so
// "absent from the file" (nil, meaning "leave this rule alone") is
// distinct from "enabled: false" (explicitly turned off) -- a plain bool
// could not tell those apart, and "absent" is the zero-config default
// every rule must keep.
type RuleConfig struct {
	Enabled  *bool  `yaml:"enabled"`
	Severity string `yaml:"severity"`
}

// ReviewConfig contains the small set of presentation preferences that are
// safe to keep with a repository. It deliberately does not expose rule gates,
// redaction, cost ceilings, or permissions through this section.
type ReviewConfig struct {
	Language string `yaml:"language"`
}

// Config is the versioned, human-authored settings file this card reads
// from .aurumcode/config.yml at the repository root. Every field here is
// explicit configuration, never provider-contributed text -- see the
// package doc's security boundary.
type Config struct {
	// Review contains non-authoritative presentation preferences for the
	// published code review. An empty section keeps the product default.
	Review ReviewConfig `yaml:"review"`
	// Rules maps a rule_id (e.g. "security/hardcoded-secret") to the
	// explicit override this repository wants for it. A rule_id absent
	// from this map keeps the engine's built-in behavior untouched.
	Rules map[string]RuleConfig `yaml:"rules"`
	// Ignore is a list of glob patterns (Copilot/gitignore-style; "**"
	// matches any number of path segments) whose matching files are
	// dropped from the diff before either review pass ever sees them.
	Ignore []string `yaml:"ignore"`
}

// DefaultConfigPath is where Load looks, relative to the repository root.
const DefaultConfigPath = ".aurumcode/config.yml"

// Load reads root/.aurumcode/config.yml. THE ZERO-CONFIG CONTRACT: a
// missing file returns an empty, non-nil *Config and a nil error -- every
// caller in this package and in cmd/aurumcode treats "no file" exactly
// like "a file with nothing in it", and both leave every downstream
// function (ApplyRuleConfig, FilterIgnoredPaths, WrapProvider) a
// documented no-op. A file that exists but fails to parse is a loud
// error, never a silently-empty config: a config the user actually wrote
// that this program could not read must not be read as "the user
// configured nothing".
func Load(root string) (*Config, error) {
	return LoadPath(filepath.Join(root, DefaultConfigPath))
}

// LoadPath reads one explicit configuration path. A missing path is the
// zero-config case; a path that exists but cannot be parsed is a loud error.
func LoadPath(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{}, nil
		}
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	return Parse(data, path)
}

// Parse decodes configuration bytes fetched from a repository API or read
// from disk. The source is included in errors so a remote PR failure remains
// actionable without printing the file contents.
func Parse(data []byte, source string) (*Config, error) {
	if source == "" {
		source = DefaultConfigPath
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", source, err)
	}
	if cfg.Rules == nil {
		cfg.Rules = map[string]RuleConfig{}
	}
	if _, err := cfg.ReviewLanguage(); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", source, err)
	}
	return &cfg, nil
}
