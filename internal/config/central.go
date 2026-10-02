// AUR-518: the central policy a required, reusable CI workflow carries with
// precedence over a repository's own .aurumcode/config.yml. The policy is a
// second .aurumcode/ directory (its own config.yml, prompt.md, skills/*.md,
// docs/*.md) checked out by the workflow, never fetched by this program over
// the network -- see internal/config's package doc for why provider text can
// never reach this decision either way.
//
// The policy governs exactly three things: which rules are enabled/overridden
// (Rules), which paths never reach either review pass (Ignore), and, when it
// sets them, the review's language and publication mode. Everything else a
// repository configures -- its own context files, memory, changelog,
// profiles -- stays the repository's own, additive choice.
package config

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// LoadCentralPolicy reads root/.aurumcode/config.yml -- a separate checkout
// the CI workflow controls, never the reviewed repository -- with the same
// parsing and validation a repository's own config already goes through
// (Parse). Unlike Load's zero-config contract, a policy that was declared
// must actually be there: a missing config.yml is a loud error naming the
// path, never an empty, silently-permissive policy (AC-005, fail closed).
//
// Every context file the policy explicitly lists (review.context's prompt,
// any skill, any doc) must also exist under root; a policy's skill list is
// itself gate-relevant content, so a broken reference is refused the same
// way, naming the missing file. The historical implicit
// .aurumcode/prompt.md stays optional, exactly as it is for a repository's
// own config.
func LoadCentralPolicy(root string) (*Config, error) {
	configPath := filepath.Join(root, DefaultConfigPath)
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("central policy: %s not found", configPath)
		}
		return nil, fmt.Errorf("central policy: reading %s: %w", configPath, err)
	}
	// A policy is gate-relevant content a repository cannot override, so a
	// typo in its own key names (a misspelled "enabeld" instead of
	// "enabled") must be a loud error, never a silently-ignored field that
	// leaves a rule or pattern unprotected. Parse (shared with a
	// repository's own, more permissive config.yml) stays lenient on
	// purpose -- this extra, policy-only strict decode is this function's
	// own, additional check. An empty file (io.EOF, nothing to decode) is
	// the policy's own valid zero-config case.
	var strict Config
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&strict); err != nil && err != io.EOF {
		return nil, fmt.Errorf("central policy: parsing %s: %w", configPath, err)
	}
	cfg, err := Parse(data, configPath)
	if err != nil {
		return nil, fmt.Errorf("central policy: %w", err)
	}
	for _, file := range cfg.Review.ContextFiles() {
		if file.Optional {
			continue
		}
		clean := path.Clean(strings.ReplaceAll(file.Path, "\\", "/"))
		full := filepath.Join(root, filepath.FromSlash(clean))
		if _, statErr := os.Stat(full); statErr != nil {
			return nil, fmt.Errorf("central policy: review %s %q: %w", file.Kind, file.Path, statErr)
		}
	}
	return cfg, nil
}

// ApplyCentralPolicy folds a central policy over a repository's own config.
// central == nil means no policy was declared at all: repo is returned
// completely unchanged (the same pointer) and the warnings are nil, so the
// result is byte-identical to this card never having run (AC-006).
//
// With a policy, the effective config is a copy of repo where the policy
// takes exclusive authority over the two fields it governs outright --
// Rules and Ignore -- plus Review.Language and Review.Publication when the
// policy sets them. Every repository entry in a policy-governed field is
// dropped, never merged with the policy's, and produces one deterministic
// warning naming exactly what was ignored (AC-001, AC-002, AC-003): a
// repository cannot disable a policy rule, loosen its severity, or hide a
// path from it by also declaring an override of its own. Everything else
// about Review -- InlineComments, Context, Memory, Changelog, Version,
// Profiles -- stays the repository's own setting (AC-004): the policy only
// subtracts rule/ignore authority, it never replaces a repository's own
// context or presentation choices.
func ApplyCentralPolicy(repo, central *Config) (*Config, []ProviderWarning) {
	if central == nil {
		return repo, nil
	}
	if repo == nil {
		repo = &Config{}
	}
	effective := *repo
	var warnings []ProviderWarning

	ruleIDs := make([]string, 0, len(repo.Rules))
	for id := range repo.Rules {
		ruleIDs = append(ruleIDs, id)
	}
	sort.Strings(ruleIDs)
	for _, id := range ruleIDs {
		warnings = append(warnings, ProviderWarning{
			Provider: "politica central",
			Reason:   fmt.Sprintf("override da regra %q no config do repositório foi ignorado: a política central decide sozinha", id),
		})
	}
	effective.Rules = central.Rules

	for _, pattern := range repo.Ignore {
		warnings = append(warnings, ProviderWarning{
			Provider: "politica central",
			Reason:   fmt.Sprintf("padrão de ignore %q do repositório não foi aplicado: a política central decide sozinha", pattern),
		})
	}
	effective.Ignore = central.Ignore

	if strings.TrimSpace(central.Review.Language) != "" {
		effective.Review.Language = central.Review.Language
	}
	if strings.TrimSpace(central.Review.Publication) != "" {
		effective.Review.Publication = central.Review.Publication
	}

	return &effective, warnings
}
