package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/git/githubclient"
	"github.com/Mpaape/AurumCode/internal/render"
	"github.com/Mpaape/AurumCode/internal/review/session"
)

// resolveInputs gathers everything the analyses need: the pull request's
// diff, the effective configuration (repository folded under the central
// policy), the publication settings, the verified checkout and review
// memory. Nothing here calls a model.
func (p *prReview) resolveInputs() (int, bool) {
	steps := []session.Step{
		p.fetchPullRequest, p.loadPolicy, p.resolvePublication,
		p.resolveChangelog, p.resolveCheckout, p.declareBinaries, p.openMemory,
	}
	for _, step := range steps {
		if code, done := step(); done {
			return code, true
		}
	}
	return 0, false
}

// fetchPullRequest builds the GitHub client, reads the diff and loads the
// repository's review configuration at the reviewed commit.
func (p *prReview) fetchPullRequest() (int, bool) {
	var clientErr error
	p.client, clientErr = newGitHubClient()
	if clientErr != nil {
		fmt.Fprintf(p.stderr, "aurumcode review: %v\n", clientErr)
		return 1, true
	}
	// The reusable GitHub workflow opts into endpoint-scoped authorization.
	// Keep the direct CLI's historical repository-role preflight unless the
	// service explicitly selects this mode.
	if p.env().permissionMode == "endpoint" {
		p.client.AllowPullRequestWrites()
	}
	ghDiff, err := p.client.GetPullRequestDiff(p.ctx, p.owner, p.repoName, p.prNumber)
	switch {
	case errors.Is(err, githubclient.ErrDiffTooLarge):
		if code, done := p.useLocalPullRequestDiff(); done {
			return code, true
		}
	case err != nil:
		fmt.Fprintf(p.stderr, "aurumcode review: fetching pull request diff: %v\n", err)
		return 1, true
	default:
		p.diff = convertDiff(ghDiff)
	}
	p.cfg, p.reviewLanguage, err = loadPullRequestConfig(p.ctx, p.client, p.owner, p.repoName, p.env().githubSHA, p.env().baseSHA)
	if err != nil {
		fmt.Fprintf(p.stderr, "aurumcode review: loading repository review config: %v\n", err)
		return 1, true
	}
	return 0, false
}

// loadPolicy folds the central policy (AUR-518), when declared, over the
// repository configuration before any model call. A missing or invalid
// policy fails closed here (AC-005); none declared leaves everything
// untouched (AC-006).
func (p *prReview) loadPolicy() (int, bool) {
	stderr := p.stderr
	var err error
	if p.opts.policyDir != "" {
		// A policy must come from outside the tree being reviewed. In this
		// path the reviewed tree is the process's working directory.
		cwd, cwdErr := os.Getwd()
		if cwdErr != nil {
			fmt.Fprintf(stderr, "aurumcode review: %v\n", cwdErr)
			return 1, true
		}
		if err := config.ValidatePolicyOutsideReviewedTree(p.opts.policyDir, cwd); err != nil {
			fmt.Fprintf(stderr, "aurumcode review: %v\n", err)
			return 1, true
		}
		p.centralCfg, err = config.LoadCentralPolicy(p.opts.policyDir)
		if err != nil {
			fmt.Fprintf(stderr, "aurumcode review: %v\n", err)
			return 1, true
		}
	}
	p.cfg, p.policyWarnings = config.ApplyCentralPolicy(p.cfg, p.centralCfg)
	if p.filter != nil {
		for i := range p.policyWarnings {
			p.policyWarnings[i].Provider = p.filter.Redact(p.policyWarnings[i].Provider)
			p.policyWarnings[i].Reason = p.filter.Redact(p.policyWarnings[i].Reason)
		}
	}
	for _, warning := range p.policyWarnings {
		fmt.Fprintf(stderr, "aurumcode review: %s: %s\n", warning.Provider, warning.Reason)
	}
	if p.reviewLanguage, err = p.cfg.ReviewLanguage(); err != nil {
		fmt.Fprintf(stderr, "aurumcode review: %v\n", err)
		return 1, true
	}
	return 0, false
}

// resolvePublication picks the publication mode and inline setting, and
// applies the repository's ignored-path filter (AUR-476: the hidden paths
// are captured before the filter drops them, for the coverage notice).
func (p *prReview) resolvePublication() (int, bool) {
	var err error
	p.publication, err = p.cfg.ReviewPublication()
	if err != nil {
		fmt.Fprintf(p.stderr, "aurumcode review: loading repository review publication: %v\n", err)
		return 1, true
	}
	if p.opts.publicationSet {
		p.publication, err = config.NormalizeReviewPublication(p.opts.publication)
		if err != nil {
			fmt.Fprintf(p.stderr, "aurumcode review: --modo-publicacao: %v\n", err)
			return 2, true
		}
	}
	p.inlineComments = p.cfg.Review.InlineComments || p.inline
	p.ignoredPaths = ignoredDiffPaths(p.diff, p.cfg)
	p.rawDiffFileCount = len(p.diff.Files)
	p.diff = config.FilterIgnoredPaths(p.diff, p.cfg)
	return 0, false
}

// resolveChangelog builds the opt-in release section (AUR-499) from the PR
// title/body and commit messages. Commit and PR text is untrusted: it is
// redacted by buildChangelogSection and never authorizes a publication. A
// missing source omits the section with a declared limitation.
func (p *prReview) resolveChangelog() (int, bool) {
	stderr := p.stderr
	on := p.opts.changelog
	if !on {
		v, cfgErr := p.cfg.ReviewChangelog()
		if cfgErr != nil {
			fmt.Fprintf(stderr, "aurumcode review: %v\n", cfgErr)
			return 1, true
		}
		on = v
	}
	if !on {
		return 0, false
	}
	commits, sourceErr := pullRequestChangelogSource(p.ctx, p.client, p.owner, p.repoName, p.prNumber)
	if sourceErr != nil {
		p.changelogLimitation = changelogUnavailableNotice(p.reviewLanguage)
		fmt.Fprintf(stderr, "aurumcode review: %s\n", p.changelogLimitation)
		return 0, false
	}
	section, limit := buildChangelogSection(p.cfg.Review.Version, commits, p.filter)
	if limit != "" {
		p.changelogLimitation = limit
		fmt.Fprintf(stderr, "aurumcode review: %s\n", limit)
		return 0, false
	}
	p.changelogText = render.ChangelogSection(section.Version, section.Bump, section.Entry, p.reviewLanguage)
	if werr := writeChangelogOutput(p.env().outputFile, section); werr != nil {
		fmt.Fprintf(stderr, "aurumcode review: writing changelog output: %v\n", werr)
	}
	return 0, false
}

// resolveCheckout reads the codebase context from the checkout only after
// proving it is the pull request's own repository and head (AUR-515) and
// that every file matches committed content (AUR-536). A failed proof is
// recorded as a limitation and the same reason makes SAST inconclusive; an
// unrelated checkout is never sent to the provider.
func (p *prReview) resolveCheckout() (int, bool) {
	var verifiedFiles []string
	mismatch := codebaseContextMismatch(p.ctx, p.client, p.owner, p.repoName, p.prNumber)
	if mismatch == "" {
		if dir, wdErr := os.Getwd(); wdErr == nil {
			if reason, files := verifiedCleanCheckoutReason(dir); reason != "" {
				mismatch = reason
			} else {
				p.verifiedDir, verifiedFiles = dir, files
			}
		} else {
			mismatch = codebaseContextReasonUnverifiable
		}
	}
	p.checkoutMismatch = mismatch
	if mismatch == "" {
		p.codebaseText = resolveVerifiedCodebaseContext(p.deps.resolveFiles, p.diff, p.verifiedDir, includedFiles(verifiedFiles, p.cfg))
	} else {
		p.codebaseLimitation = codebaseContextOmittedNotice(p.reviewLanguage, mismatch)
	}
	return 0, false
}

// declareBinaries takes the binary files out of the diff before the model
// sees it, as --base does: nothing in them is reviewable by reading, so the
// prompt never counts them as omitted by the budget, and the coverage pass
// declares them ignored (binaryNotices) instead of partial.
func (p *prReview) declareBinaries() (int, bool) {
	p.binaryNotices, p.diff = splitBinaryFiles(p.diff, p.verifiedDir)
	return 0, false
}

// openMemory opens the opt-in review memory (AUR-489/490): observation,
// never instruction, derived from this repository.
func (p *prReview) openMemory() (int, bool) {
	p.memoryStore, p.memoryNotes, p.memoryNotesText = openReviewMemory(p.cfg.Review.Memory, p.owner, p.repoName, p.stderr, p.filter)
	return 0, false
}
