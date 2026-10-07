// The --pr diff computed from the verified checkout: when the API refuses
// the pull request's diff because of its size, the same base...head range is
// read locally, only from a checkout already proven to be the pull request's
// repository at its head, with no uncommitted content.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/Mpaape/AurumCode/internal/analyzer"
	"github.com/Mpaape/AurumCode/internal/git/githubclient"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// errLocalDiffUnverified names a checkout that could not be proven to be the
// pull request's own: its diff is never reviewed in place of the API's.
var errLocalDiffUnverified = errors.New("the local checkout is not verified as the pull request head")

// pullRequestRange is the base...head range read from the checkout.
type pullRequestRange struct {
	base, head string
}

// useLocalPullRequestDiff replaces the diff the API refused with the one the
// verified checkout yields for the same range. Any doubt fails the review:
// an unverified checkout, an unknown base, no git binary.
func (p *prReview) useLocalPullRequestDiff() (int, bool) {
	diff, rng, err := localPullRequestDiff(p.ctx, p.client, p.owner, p.repoName, p.prNumber, p.env().baseSHA)
	if err != nil {
		fmt.Fprintf(p.stderr, "aurumcode review: fetching pull request diff: %v; the diff could not be computed from the checkout either: %v\n", githubclient.ErrDiffTooLarge, err)
		return 1, true
	}
	fmt.Fprintf(p.stderr, "aurumcode review: %v; reviewing the same range computed from the verified checkout (%s...%s, %d file(s))\n",
		githubclient.ErrDiffTooLarge, shortSHA(rng.base), shortSHA(rng.head), len(diff.Files))
	p.diff = diff
	return 0, false
}

// localPullRequestDiff verifies the working directory as the pull request's
// checkout (repository and head, AUR-515; clean tree, AUR-536) and returns
// the diff of base...head read from it.
func localPullRequestDiff(ctx context.Context, client *githubclient.Client, owner, repo string, number int, eventBase string) (*types.Diff, pullRequestRange, error) {
	dir, err := os.Getwd()
	if err != nil {
		return nil, pullRequestRange{}, err
	}
	if reason := codebaseContextMismatch(ctx, client, owner, repo, number); reason != "" {
		return nil, pullRequestRange{}, fmt.Errorf("%w (%s)", errLocalDiffUnverified, reason)
	}
	if reason, _ := verifiedCleanCheckoutReason(dir); reason != "" {
		return nil, pullRequestRange{}, fmt.Errorf("%w (%s)", errLocalDiffUnverified, reason)
	}
	checkout, err := analyzer.OpenRepo(dir)
	if err != nil {
		return nil, pullRequestRange{}, err
	}
	rng, err := pullRequestRangeIn(ctx, client, owner, repo, number, eventBase, checkout)
	if err != nil {
		return nil, pullRequestRange{}, err
	}
	text, err := checkout.RangeDiff(rng.base, rng.head)
	if err != nil {
		return nil, pullRequestRange{}, err
	}
	parsed, err := githubclient.ParseUnifiedDiff(text)
	if err != nil {
		return nil, pullRequestRange{}, err
	}
	return convertDiff(parsed), rng, nil
}

// pullRequestRangeIn resolves the range in the checkout. The head is the
// verified HEAD. The base is the event's base commit when it names a commit
// the checkout has, else the base commit the API reports; neither present
// is an error, never a guess.
func pullRequestRangeIn(ctx context.Context, client *githubclient.Client, owner, repo string, number int, eventBase string, checkout *analyzer.Repo) (pullRequestRange, error) {
	head, err := checkout.ResolveRef("HEAD")
	if err != nil {
		return pullRequestRange{}, fmt.Errorf("resolving the checkout head: %w", err)
	}
	candidates := []string{strings.TrimSpace(eventBase)}
	if meta, metaErr := client.GetPullRequestMetadata(ctx, owner, repo, number); metaErr == nil {
		candidates = append(candidates, strings.TrimSpace(meta.BaseSHA))
	}
	for _, base := range candidates {
		if !looksLikeCommitSHA(base) {
			continue
		}
		if resolved, resolveErr := checkout.ResolveRef(base); resolveErr == nil {
			return pullRequestRange{base: resolved, head: head}, nil
		}
	}
	return pullRequestRange{}, errors.New("the pull request base commit is not in the checkout (a full-history checkout is needed)")
}

// shortSHA abbreviates a commit id for a log line.
func shortSHA(sha string) string {
	if len(sha) > 12 {
		return sha[:12]
	}
	return sha
}
