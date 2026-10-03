// The resolve phase of the --base path: resolve the inputs. Everything here happens
// before any model call or analysis runs.
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/render"
	"github.com/Mpaape/AurumCode/internal/review/session"
)

// resolveInputs validates usage, computes the diff, loads the effective
// config (repository + central policy), selects profiles and gathers the
// context the model will see (codebase, memory, changelog).
func (b *baseReview) resolveInputs() (int, bool) {
	steps := []session.Step{
		b.validateUsage, b.loadDiff, b.loadConfig, b.resolveProfiles, b.gatherContext,
	}
	for _, step := range steps {
		if code, done := step(); done {
			return code, true
		}
	}
	return 0, false
}

// validateUsage refuses an unusable command line before any work: --base
// missing, an unknown or empty --fail-on, an empty --modelo, an unparsable
// --limite. An explicit empty value is a usage error, never a silently
// open gate or a silently disabled limit (AUR-431/433/436).
func (b *baseReview) validateUsage() (int, bool) {
	f := b.f
	if f.base == "" {
		fmt.Fprintln(b.stderr, "aurumcode review: --base is required")
		return 2, true
	}
	if f.given["fail-on"] {
		var err error
		b.threshold, b.thresholdName, err = parseFailOnLevel(f.failOn)
		if err != nil {
			fmt.Fprintf(b.stderr, "aurumcode review: %v\n", err)
			return 2, true
		}
	}
	if f.given["modelo"] && f.modelo == "" {
		fmt.Fprintln(b.stderr, "aurumcode review: --modelo: model name must not be empty")
		return 2, true
	}
	b.limiteSet = f.given["limite"]
	if b.limiteSet {
		if f.limite == "" {
			fmt.Fprintln(b.stderr, "aurumcode review: --limite: value must not be empty")
			return 2, true
		}
		var err error
		b.limiteUSD, err = parseLimiteUSD(f.limite)
		if err != nil {
			fmt.Fprintf(b.stderr, "aurumcode review: %v\n", err)
			return 2, true
		}
	}
	return 0, false
}

// loadDiff computes the diff of --base against HEAD in the working
// directory. The raw file count, captured before the ignore filter runs,
// is the coverage denominator (AUR-476).
func (b *baseReview) loadDiff() (int, bool) {
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(b.stderr, "aurumcode review: %v\n", err)
		return 1, true
	}
	b.cwd = cwd
	// AUR-520: the verified repo identity comes from the local checkout's
	// "origin" remote, never from the model or the diff. Unknown fails
	// closed (the exceptions contributor says so).
	b.repoIdentity, b.repoIdentityKnown = localRepoIdentity(cwd)
	b.diff, b.notices, err = computeDiff(cwd, b.f.base)
	if err != nil {
		fmt.Fprintf(b.stderr, "aurumcode review: %v\n", err)
		return 1, true
	}
	b.rawDiffFileCount = len(b.diff.Files)
	return 0, false
}

// loadConfig loads the repository's own config and folds the central
// policy over it (AUR-452, AUR-518), before any model call. A missing or
// invalid policy fails closed here. The ignored-path filter is applied to
// the ONE diff both the quality and the security pass read (AUR-452), after
// capturing which paths the config hid (AUR-476).
func (b *baseReview) loadConfig() (int, bool) {
	var err error
	b.cfg, err = config.Load(b.cwd)
	if err != nil {
		fmt.Fprintf(b.stderr, "aurumcode review: %v\n", err)
		return 1, true
	}
	if b.policyDir != "" {
		// A policy must come from outside the tree being reviewed.
		if err := config.ValidatePolicyOutsideReviewedTree(b.policyDir, b.cwd); err != nil {
			fmt.Fprintf(b.stderr, "aurumcode review: %v\n", err)
			return 1, true
		}
		b.centralCfg, err = config.LoadCentralPolicy(b.policyDir)
		if err != nil {
			fmt.Fprintf(b.stderr, "aurumcode review: %v\n", err)
			return 1, true
		}
	}
	b.cfg, b.policyWarnings = config.ApplyCentralPolicy(b.cfg, b.centralCfg)
	if b.filter != nil {
		for i := range b.policyWarnings {
			b.policyWarnings[i].Provider = b.filter.Redact(b.policyWarnings[i].Provider)
			b.policyWarnings[i].Reason = b.filter.Redact(b.policyWarnings[i].Reason)
		}
	}
	for _, warning := range b.policyWarnings {
		fmt.Fprintf(b.stderr, "aurumcode review: %s: %s\n", warning.Provider, warning.Reason)
	}
	b.reviewLanguage, err = b.cfg.ReviewLanguage()
	if err != nil {
		fmt.Fprintf(b.stderr, "aurumcode review: %v\n", err)
		return 1, true
	}
	b.ignoredPaths = ignoredDiffPaths(b.diff, b.cfg)
	b.diff = config.FilterIgnoredPaths(b.diff, b.cfg)
	return 0, false
}

// resolveProfiles resolves the reviewer-profile selection (AUR-502):
// --perfis/--profile wins over review.profiles; an unknown, empty or
// duplicate name is a usage error before any model call. The profile
// identity joins the cache key (AUR-513): two profiles must never share a
// cached answer, and order is part of the identity.
func (b *baseReview) resolveProfiles() (int, bool) {
	perfisGiven := b.f.given["perfis"] || b.f.given["profile"]
	var flagNames []string
	if perfisGiven {
		raw := b.f.perfis
		if strings.TrimSpace(raw) == "" {
			raw = b.f.perfil
		}
		if strings.TrimSpace(raw) == "" {
			fmt.Fprintln(b.stderr, "aurumcode review: --perfis: profile name must not be empty")
			return 2, true
		}
		flagNames = splitProfileNames(raw)
	}
	var err error
	b.profileRes, err = resolveReviewProfiles(b.cwd, flagNames, perfisGiven, b.cfg)
	if err != nil {
		fmt.Fprintf(b.stderr, "aurumcode review: %v\n", err)
		return 2, true
	}
	if b.profileRes.Applied {
		fmt.Fprintf(b.stderr, "aurumcode review: %s\n", b.profileRes.Declared)
	}
	b.profilesApplied = b.profileRes.Applied
	sigs := make([]string, 0, len(b.profileRes.Profiles))
	for _, p := range b.profileRes.Profiles {
		sigs = append(sigs, p.Signature())
	}
	b.profileIdentity = strings.Join(sigs, "\x1f")
	return 0, false
}

// gatherContext resolves the codebase context and review memory, and the
// opt-in changelog section (AUR-499): commit text is untrusted, redacted
// and bounded; missing metadata omits the section with a declared
// limitation and never crashes the review.
func (b *baseReview) gatherContext() (int, bool) {
	b.codebaseText = resolveCodebaseContext(b.diff)
	b.memoryStore, b.memoryNotes, b.memoryNotesText = openReviewMemory(b.cfg.Review.Memory, "", "", b.stderr, b.filter)
	changelogOn := b.f.changelog
	if !changelogOn {
		on, cfgErr := b.cfg.ReviewChangelog()
		if cfgErr != nil {
			fmt.Fprintf(b.stderr, "aurumcode review: %v\n", cfgErr)
			return 1, true
		}
		changelogOn = on
	}
	if !changelogOn {
		return 0, false
	}
	commits, commitErr := localRangeCommits(b.cwd, b.f.base)
	if commitErr != nil {
		b.changelogLimitation = changelogUnavailableNotice(b.reviewLanguage)
		fmt.Fprintf(b.stderr, "aurumcode review: %s\n", b.changelogLimitation)
	} else if section, limit := buildChangelogSection(b.cfg.Review.Version, commits, b.filter); limit != "" {
		b.changelogLimitation = limit
		fmt.Fprintf(b.stderr, "aurumcode review: %s\n", limit)
	} else {
		b.changelogText = render.ChangelogSection(section.Version, section.Bump, section.Entry, b.reviewLanguage)
		if werr := writeChangelogOutput(b.env().outputFile, section); werr != nil {
			fmt.Fprintf(b.stderr, "aurumcode review: writing changelog output: %v\n", werr)
		}
	}
	return 0, false
}
