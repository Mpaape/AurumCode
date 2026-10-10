// The suggested changelog entry in the review's PR body: when the
// repository requires or suggests an entry (changelog_check mode required
// or suggest) and the pull request does
// not touch the changelog file at all, the body carries the same
// ready-to-paste block the changelog check prints. The API diff is
// windowed, so a present changelog file cannot be judged here; only the
// unambiguous "missing" case gets the block. The review itself spends no
// extra model call on it: the entry comes from the commit subjects. The
// wording follows the mode for the PR's author: a bot's PR gets
// the changelog_check.bots mode, and no block when that mode is off.
package main

import (
	"fmt"
	"path"
	"strings"

	"github.com/Mpaape/AurumCode/internal/changelog"
	"github.com/Mpaape/AurumCode/internal/config"
)

// resolveChangelogSuggestion never fails the review: a missing source is a
// stderr note and an omitted block.
func (p *prReview) resolveChangelogSuggestion() (int, bool) {
	if p.cfg == nil || !p.cfg.ChangelogCheck.Active() || p.diff == nil {
		return 0, false
	}
	req, err := changelogRequirement(p.cfg.ChangelogCheck)
	if err != nil {
		fmt.Fprintf(p.stderr, "aurumcode review: changelog_check: %v\n", err)
		return 0, false
	}
	if fileSides(req.File, p.diff, nil).found || changelogPathIgnored(req.File, p.ignoredPaths) {
		return 0, false
	}
	verdict := req.Verify(changelog.Change{State: changelog.FileUntouched})
	if verdict.OK || verdict.Reason != changelog.ReasonMissing {
		return 0, false
	}
	commits, author, err := pullRequestChangelogSource(p.ctx, p.client, p.owner, p.repoName, p.prNumber)
	if err != nil {
		fmt.Fprintf(p.stderr, "aurumcode review: entrada de changelog sugerida omitida: %v\n", err)
		return 0, false
	}
	mode := p.cfg.ChangelogCheck.EffectiveModeFor(author.IsBot())
	if mode == config.ChangelogOff {
		return 0, false
	}
	s := req.SuggestFromCommits(commits)
	if s.Empty() {
		return 0, false
	}
	p.changelogSuggestion = changelogSuggestionMarkdown(req, mode, s.Source, redactSuggestion(p.filter, s.Block()), p.reviewLanguage)
	return 0, false
}

// changelogPathIgnored reports whether the review's ignore patterns hid the
// changelog file from the diff: then its absence proves nothing.
func changelogPathIgnored(file string, ignored []string) bool {
	want := path.Clean(strings.TrimSpace(file))
	for _, p := range ignored {
		if path.Clean(p) == want {
			return true
		}
	}
	return false
}
