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
	"path"
	"path/filepath"
	"strings"

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

// ReviewContextConfig is the small, repository-owned list of optional context
// files that can enrich a review. All three entries are Markdown or plain-text
// guidance: they never replace the built-in review prompt and never control
// rules, gates, redaction, cost, or permissions.
//
// Prompt is one repository-wide instruction file. Skills and Docs are curated
// lists so a repository can add focused review knowledge without injecting an
// entire documentation tree into every request.
type ReviewContextConfig struct {
	Prompt string   `yaml:"prompt"`
	Skills []string `yaml:"skills"`
	Docs   []string `yaml:"docs"`
}

// ContextFile describes one configured context contribution in deterministic
// order. The default prompt is optional for backwards compatibility; every
// path explicitly written by the repository is required and produces an
// actionable configuration error if it cannot be loaded.
type ContextFile struct {
	Kind     string
	Path     string
	Optional bool
}

// ReviewConfig contains the small set of presentation and context
// preferences that are safe to keep with a repository. It deliberately does
// not expose rule gates, redaction, cost ceilings, or permissions through
// this section.
type ReviewConfig struct {
	Language       string              `yaml:"language"`
	Publication    string              `yaml:"publication"`
	InlineComments bool                `yaml:"inline_comments"`
	Context        ReviewContextConfig `yaml:"context"`
}

// DefaultReviewPublication preserves the original PR behavior for callers
// that do not opt into the formal GitHub review endpoint. Empty configuration
// keeps direct CLI consumers backwards-compatible while making the newer mode
// available without extra plumbing.
const DefaultReviewPublication = "comments"

// ReviewPublication returns the canonical publication mode. Empty config is
// the zero-configuration, backwards-compatible comments mode.
func (c *Config) ReviewPublication() (string, error) {
	return NormalizeReviewPublication(c.Review.Publication)
}

// NormalizeReviewPublication accepts the public mode names and a Portuguese
// spelling useful in repository configuration. The returned values are
// stable internal names used by the PR publisher.
func NormalizeReviewPublication(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "comments", "comment", "comentarios", "comentários":
		return DefaultReviewPublication, nil
	case "review", "reviews", "formal":
		return "review", nil
	default:
		return "", fmt.Errorf("unsupported review publication %q (use review or comments)", raw)
	}
}

const DefaultReviewPromptPath = ".aurumcode/prompt.md"

// ContextFiles returns the configured context in the order rendered to the
// model: repository prompt, skills, then documentation. An omitted prompt
// keeps the historical optional .aurumcode/prompt.md convention.
func (c ReviewConfig) ContextFiles() []ContextFile {
	promptPath := strings.TrimSpace(c.Context.Prompt)
	files := []ContextFile{{Kind: "prompt", Path: DefaultReviewPromptPath, Optional: true}}
	if promptPath != "" {
		files[0] = ContextFile{Kind: "prompt", Path: promptPath}
	}
	for _, item := range c.Context.Skills {
		files = append(files, ContextFile{Kind: "skill", Path: strings.TrimSpace(item)})
	}
	for _, item := range c.Context.Docs {
		files = append(files, ContextFile{Kind: "documentation", Path: strings.TrimSpace(item)})
	}
	return files
}

// ValidateContext rejects paths that could escape the repository in local
// mode. Context files are advisory model input, but local execution must not
// turn a repository-authored YAML value into an arbitrary filesystem read.
func (c ReviewConfig) ValidateContext() error {
	for _, file := range c.ContextFiles() {
		if err := validateContextPath(file.Path); err != nil {
			return fmt.Errorf("review.context.%s path %q: %w", file.Kind, file.Path, err)
		}
	}
	return nil
}

func validateContextPath(raw string) error {
	value := strings.TrimSpace(raw)
	if value == "" {
		return fmt.Errorf("path must not be empty")
	}
	if strings.ContainsRune(value, '\x00') {
		return fmt.Errorf("path contains a NUL byte")
	}
	clean := path.Clean(strings.ReplaceAll(value, "\\", "/"))
	if path.IsAbs(clean) || clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return fmt.Errorf("path must stay inside the repository")
	}
	return nil
}

// Config is the versioned, human-authored settings file this card reads
// from .aurumcode/config.yml at the repository root. Every field here is
// explicit configuration, never provider-contributed text -- see the
// package doc's security boundary.
type Config struct {
	// Review contains non-authoritative presentation preferences and curated
	// context paths for the published code review. An empty section keeps the
	// product default.
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
	if _, err := cfg.ReviewPublication(); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", source, err)
	}
	if err := cfg.Review.ValidateContext(); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", source, err)
	}
	return &cfg, nil
}
