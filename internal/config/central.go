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

// ValidatePolicyOutsideReviewedTree refuses a policy directory that resolves
// (after symlinks) to the reviewed tree itself or anywhere under it. A
// policy's entire point is that the repository under review cannot change
// it; a policy directory living inside that same tree would let whoever
// controls the reviewed checkout (a pull request's own head, in the --pr
// path) edit the policy it is being judged against. Either path missing is
// not this function's problem to report -- a missing policyDir surfaces as
// LoadCentralPolicy's own "not found" error, and reviewedRoot always exists
// (it is the process's own working directory) -- so a resolution failure
// here falls back to the unresolved, absolute path rather than masking the
// real error with one about symlinks.
func ValidatePolicyOutsideReviewedTree(policyDir, reviewedRoot string) error {
	policyDir = strings.TrimSpace(policyDir)
	reviewedRoot = strings.TrimSpace(reviewedRoot)
	if policyDir == "" || reviewedRoot == "" {
		return nil
	}
	resolvedPolicy := resolvePathBestEffort(policyDir)
	resolvedRoot := resolvePathBestEffort(reviewedRoot)
	if resolvedPolicy == resolvedRoot || strings.HasPrefix(resolvedPolicy, resolvedRoot+string(filepath.Separator)) {
		return fmt.Errorf("central policy: %s is inside the reviewed repository (%s); a policy must come from outside the reviewed tree", policyDir, reviewedRoot)
	}
	return nil
}

// resolvePathBestEffort returns dir's absolute, symlink-resolved form. A dir
// that does not exist yet (EvalSymlinks fails) or an unresolvable absolute
// path falls back to the plain absolute form -- there is nothing further to
// resolve, and the caller that actually needs the directory to exist (e.g.
// LoadCentralPolicy) still reports that failure on its own.
func resolvePathBestEffort(dir string) string {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return dir
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return resolved
	}
	return abs
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

	// AUR-519: the gate is governed exactly like Rules and Ignore above --
	// under a policy, only the policy's own Gate ever applies. A
	// repository's own gate declaration is dropped with a named warning:
	// the repo opt-in (AC-005) only has authority when no policy is in
	// play at all.
	if repo.Gate.Declared() {
		warnings = append(warnings, ProviderWarning{
			Provider: "politica central",
			Reason:   "gate do config do repositório foi ignorado: a política central decide sozinha",
		})
	}
	effective.Gate = central.Gate

	// AUR-520: exceptions are governed exactly like Rules/Ignore/Gate
	// above -- under a policy, only the policy's own Exceptions ever
	// apply. A repository cannot declare its own exception for a
	// policy-governed finding (AC-004): every repo-declared exception is
	// dropped, each with its own named warning (repo/rule/path, the exact
	// identifying triple), never silently merged with the policy's list.
	for _, exc := range repo.Exceptions {
		warnings = append(warnings, ProviderWarning{
			Provider: "politica central",
			Reason: fmt.Sprintf(
				"exceção do repositório para a regra %q no caminho %q (repositório %q) foi ignorada: a política central decide sozinha",
				exc.Rule, exc.Path, exc.Repo,
			),
		})
	}
	effective.Exceptions = central.Exceptions

	// AUR-548 (coordinator-directed, per-section precedence): unlike
	// Gate/Rules/Ignore/Exceptions above, quality_gates is governed PER
	// SECTION, not as one wholesale block -- a policy that declares
	// quality_gates.sast says nothing at all about quality_gates.
	// ssor_dtrack or quality_gates.supply_chain, and a repository's own,
	// undeclared sections must survive untouched (effective.QualityGates
	// starts as repo's own value, below, instead of central's). Only a
	// section the policy DOES declare (non-nil) is taken over wholesale,
	// with a named warning when the repository had declared that exact
	// section -- CR-TRUST-001 still holds (a repository cannot disable or
	// loosen a policy-enabled SAST), scoped to that one section.
	effective.QualityGates = repo.QualityGates
	if central.QualityGates.Sast != nil {
		if repo.QualityGates.Sast != nil {
			warnings = append(warnings, ProviderWarning{
				Provider: "politica central",
				Reason:   "quality_gates.sast do config do repositório foi ignorado: a política central decide sozinha",
			})
		}
		effective.QualityGates.Sast = central.QualityGates.Sast
	}
	if central.QualityGates.SsorDtrack != nil {
		effective.QualityGates.SsorDtrack = central.QualityGates.SsorDtrack
	}
	if central.QualityGates.SupplyChain != nil {
		effective.QualityGates.SupplyChain = central.QualityGates.SupplyChain
	}

	return &effective, warnings
}
