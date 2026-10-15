// The central security policy (AUR-518): the configuration layer a
// required, reusable CI workflow carries with authority over a repository's
// own .aurumcode/config.yml. The policy is a second .aurumcode/ directory
// (its own config.yml, prompt.md, skills/*.md, docs/*.md) checked out by the
// workflow, never fetched by this program over the network -- see
// internal/config's package doc for why provider text can never reach this
// decision either way. The layers themselves are declared once in layer.go:
// this policy, the engineering standards layer (AUR-613: a second central
// .aurumcode/ that only adds skills and analysts, with no config.yml
// section at all) and the repository.
//
// Which sections the policy decides, and how, is declared once in
// governance.go (governedSections, eleven sections): rules, ignore, gate and
// exceptions are the policy's entirely when it declares them; language and
// publication, analysis_data and each quality_gates subsection are the
// policy's when it declares them and the repository's otherwise; the rest
// resolves by its own rule there (merged or engine by engine). The review
// keys that are always the repository's own, additive choice are listed in
// reviewRepositoryFields.
package config

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// ValidatePolicyOutsideReviewedTree refuses a policy directory that resolves
// (after symlinks) to the reviewed tree itself or anywhere under it: the
// containment every central layer shares (ValidateLayerOutsideReviewedTree),
// in the policy's own words.
func ValidatePolicyOutsideReviewedTree(policyDir, reviewedRoot string) error {
	return ValidateLayerOutsideReviewedTree(Layers[LayerPolicy], policyDir, reviewedRoot, "")
}

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
	// A policy is gate-relevant content a repository cannot override; Parse
	// decodes it strictly (decodeStrict), exactly like the repository's own
	// config.yml, so a misspelled key is a loud error, never a silently
	// ignored field that leaves a rule or pattern unprotected.
	cfg, err := Parse(data, configPath)
	if err != nil {
		return nil, fmt.Errorf("central policy: %w", err)
	}
	if err := cfg.Scheduled.ValidatePolicy(); err != nil {
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
// central == nil means no policy was declared: repo is returned unchanged
// (the same pointer) and the warnings are nil, so a review without a policy
// is byte-identical to one before policies existed. Of the three layers,
// only the policy reaches this function: the engineering standards layer
// declares no configuration section (layer.go, NoSection), so it never
// changes the effective config.
//
// With a policy, the effective config is a copy of repo resolved section by
// section as governedSections (governance.go) declares, in that order.
// Rules, ignore, gate and exceptions are replaced entirely by the policy's
// when it declares them (governRules, governIgnore, governGate,
// governExceptions): every repository entry there is dropped, never merged,
// and produces one deterministic warning naming what was ignored, so a
// repository cannot disable a policy rule, loosen its severity, hide a path
// from it, weaken the gate or grant itself an exception. Language and
// publication, analysis_data and each quality_gates subsection are the
// policy's when it declares them and the repository's otherwise; scanners
// resolve engine by engine; deliberation, batches, dependencies and
// changelog_check follow their own rule there. The review keys in
// reviewRepositoryFields stay the repository's own, additive choice.
func ApplyCentralPolicy(repo, central *Config) (*Config, []ProviderWarning) {
	if central == nil {
		return repo, nil
	}
	if repo == nil {
		repo = &Config{}
	}
	effective := *repo
	var warnings []ProviderWarning
	for _, section := range governedSections {
		warnings = append(warnings, section.apply(&effective, repo, central)...)
	}
	return &effective, warnings
}
