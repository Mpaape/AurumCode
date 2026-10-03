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
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
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
	// A policy is gate-relevant content a repository cannot override; Parse
	// decodes it strictly (decodeStrict), exactly like the repository's own
	// config.yml, so a misspelled key is a loud error, never a silently
	// ignored field that leaves a rule or pattern unprotected.
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
// context or presentation choices. Who decides each section is declared in
// governedSections (governance.go), applied in that order.
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
