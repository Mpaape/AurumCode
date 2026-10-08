package main

import (
	"fmt"
	"strings"

	"github.com/Mpaape/AurumCode/internal/git/githubclient"
	"github.com/Mpaape/AurumCode/internal/review"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// publish resolves the commit the review anchors to, writes the compliance
// artifacts, posts the review, and decides the exit code.
func (p *prReview) publish() (int, bool) {
	// Sorted (and computed) before the commit resolution on purpose:
	// --check needs the same already-sorted slice, empty or not, for its
	// commit status, so both branches share one definition.
	p.issues = sortedIssues(p.result.Issues)
	p.shown = p.presentFindings()
	p.round = p.planRound()
	if code, done := p.resolveCommit(); done {
		return code, true
	}
	artifactFailures := p.writeArtifacts()
	return p.finish(p.postReview(), len(artifactFailures) > 0), true
}

// resolveCommit picks the commit SHA the review, its inline comments and the
// --check status anchor to. GITHUB_SHA anchors a plain run; --check anchors
// on the pull request HEAD read from the API (AUR-504), never GITHUB_SHA
// (the synthetic merge commit on a pull_request event). A real review-comment
// POST with an empty commit_id is rejected (422), so when an inline comment
// or --check needs a SHA and none is available this refuses before anything
// is posted -- fail closed, never a POST built with an empty commit_id.
func (p *prReview) resolveCommit() (int, bool) {
	stderr := p.stderr
	p.commitID = p.env().githubSHA
	needsCommitID := p.check
	if p.inlineComments {
		for _, issue := range p.issues {
			if isInlineEligible(p.diff, issue) {
				needsCommitID = true
				break
			}
		}
		if p.publication == "review" && !needsCommitID {
			for _, suggestion := range p.result.Suggestions {
				if isNativeSuggestion(p.diff, suggestion) {
					needsCommitID = true
					break
				}
			}
		}
	}
	// AC-003: when the head cannot be determined, refuse and name the reason.
	if p.check {
		headSHA, resolveErr := resolvePullRequestHeadSHA(p.ctx, p.client, p.owner, p.repoName, p.prNumber)
		if resolveErr != nil {
			fmt.Fprintf(stderr, "aurumcode review: refusing to publish: %v\n", resolveErr)
			return 1, true
		}
		p.commitID = headSHA
	}
	if needsCommitID && p.commitID == "" {
		fmt.Fprintln(stderr, "aurumcode review: refusing to publish: inline comments or --check require a commit SHA; set GITHUB_SHA")
		return 1, true
	}
	p.artifactCommit = p.commitID
	return 0, false
}

// postReview publishes the parecer and the inline comments of what blocks.
// The loop never lets one POST failure swallow the rest: failures are
// recorded and the loop continues, so every comment that COULD be published
// still was.
func (p *prReview) postReview() []string {
	summaryBody := p.reviewBody()
	if p.publication == "review" {
		return p.postFormalReview(summaryBody)
	}
	return p.postSeparateComments(summaryBody)
}

// reviewBody assembles the parecer: the document, then the actionable
// sections (dependency reach, changelog, proposed exceptions) before the
// collapsed details, and the round and consolidation notes inside them.
func (p *prReview) reviewBody() string {
	body := formatGatedReviewBody(p.shown.publishedResult(p.result, p.reviewLanguage), p.diff, p.reviewLanguage, p.publication == "review" && p.inlineComments, "", p.blockingRule())
	body = appendBodySection(body, reachSection(p.reachLines, p.reviewLanguage))
	body = appendBodySection(body, p.changelogText)
	body = appendBodySection(body, p.changelogSuggestion)
	body = appendBodySection(body, proposedExceptionsBlock(p.proposedExceptions))
	copy := reviewCopyFor(p.reviewLanguage)
	body = appendDetailSection(body, copy.details, roundNotice(p.round, p.reviewLanguage))
	body = appendDetailSection(body, copy.details, presentationNotice(p.shown, p.reviewLanguage))
	return body
}

// postFormalReview posts one formal GitHub review carrying the inline
// comments of the blocking findings and the native suggestions.
func (p *prReview) postFormalReview(summaryBody string) (failures []string) {
	formalComments := make([]githubclient.ReviewLineComment, 0)
	if p.inlineComments {
		for i, issue := range p.shown.withSources(p.reviewLanguage) {
			if !p.commentsOwnLine(issue) || !p.round.posts(i) {
				continue
			}
			formalComments = append(formalComments, githubclient.ReviewLineComment{
				Body: p.round.findingBody(i, issue, p.reviewLanguage),
				Path: issue.File,
				Line: issue.Line,
				Side: review.FindingSide(issue),
			})
		}
		for _, suggestion := range p.result.Suggestions {
			if comment, ok := nativeSuggestionComment(p.diff, suggestion, p.reviewLanguage); ok {
				formalComments = append(formalComments, comment)
			}
		}
	}
	formal := githubclient.PullRequestReview{
		Body:     summaryBody,
		Event:    p.blockingRule().Event(formalReviewEvent(p.result)),
		CommitID: p.commitID,
		Comments: formalComments,
	}
	key := fmt.Sprintf("aurumcode/review/%d/%s", p.prNumber, p.commitID)
	if err := p.client.PostPullRequestReview(p.ctx, p.owner, p.repoName, p.prNumber, formal, key); err != nil {
		fmt.Fprintf(p.stderr, "aurumcode review: publishing formal review: %v\n", err)
		failures = append(failures, "formal review: "+err.Error())
	} else {
		fmt.Fprintf(p.stdout, "review formal %q publicado no pull request #%d (%d comentário(s) na linha).\n", formal.Event, p.prNumber, len(formalComments))
	}
	p.markResolvedComments()
	return failures
}

// commentsOwnLine reports whether a finding gets a comment on its line: it
// blocks the merge and sits on a changed line. Everything else is read in
// the parecer, which lists every finding.
func (p *prReview) commentsOwnLine(issue types.ReviewIssue) bool {
	return p.blockingRule().Blocks(issue) && isInlineEligible(p.diff, issue)
}

// postSeparateComments is the default mode: an inline comment per blocking
// finding (when inline comments are on), then the parecer, edited in place
// when an earlier round published one.
func (p *prReview) postSeparateComments(summaryBody string) (failures []string) {
	inlineCount := 0
	stdout, stderr := p.stdout, p.stderr
	for i, issue := range p.shown.withSources(p.reviewLanguage) {
		line := fmt.Sprintf("%s:%d: [%s] %s", issue.File, issue.Line, issue.Severity, issue.Message)
		if issue.Side == "LEFT" {
			line += " [LEFT/base]"
		}
		switch {
		case !p.round.posts(i):
			fmt.Fprintf(stdout, "%s -- %s\n", line, roundRepeatedMarker(p.reviewLanguage))
		case p.inlineComments && p.commentsOwnLine(issue):
			comment := githubclient.ReviewComment{
				Body:     p.round.findingBody(i, issue, p.reviewLanguage),
				CommitID: p.commitID,
				Path:     issue.File,
				Line:     issue.Line,
				Side:     review.FindingSide(issue),
			}
			key := fmt.Sprintf("aurumcode/%d/%s/%s/%d/%s/%s", p.prNumber, p.commitID, issue.File, issue.Line, review.FindingSide(issue), issue.RuleID)
			if err := p.client.PostReviewComment(p.ctx, p.owner, p.repoName, p.prNumber, comment, key); err != nil {
				fmt.Fprintf(stderr, "aurumcode review: publishing inline comment on %s:%d: %v\n", issue.File, issue.Line, err)
				failures = append(failures, fmt.Sprintf("%s:%d (na linha): %v", issue.File, issue.Line, err))
				continue
			}
			fmt.Fprintf(stdout, "%s -- publicado na linha\n", line)
			inlineCount++
		default:
			fmt.Fprintf(stdout, "%s %s\n", line, inParecerMarker)
		}
	}
	for _, issue := range review.OutsideDiffFindings(p.result) {
		fmt.Fprintf(stdout, "%s %s\n", outsideDiffLine(issue), inParecerMarker)
	}
	how, err := p.postParecer(summaryBody)
	if err != nil {
		fmt.Fprintf(stderr, "aurumcode review: publishing review summary: %v\n", err)
		failures = append(failures, "summary: "+err.Error())
	} else {
		fmt.Fprintf(stdout, "parecer %s no pull request #%d (%d comentário(s) na linha).\n", how, p.prNumber, inlineCount)
	}
	p.markResolvedComments()
	return failures
}

// inParecerMarker ends the stdout line of a finding published only in the
// parecer.
const inParecerMarker = "-- no parecer"

// postParecer publishes the parecer: edited in place when this product
// already published one on the pull request (one parecer per pull request,
// the latest round), posted otherwise. An edit that fails falls back to a
// new comment, so a round is never lost.
func (p *prReview) postParecer(body string) (string, error) {
	if id, ok := p.previousParecer(); ok {
		if err := p.client.UpdateIssueComment(p.ctx, p.owner, p.repoName, id, body); err == nil {
			return "atualizado", nil
		} else {
			fmt.Fprintf(p.stderr, "aurumcode review: editing the earlier parecer (%d): %v; posting a new one\n", id, err)
		}
	}
	if err := p.client.PostIssueComment(p.ctx, p.owner, p.repoName, p.prNumber, body); err != nil {
		return "", err
	}
	return "publicado", nil
}

// previousParecer is the latest parecer this product published on the pull
// request: a general comment by the publisher login carrying the body
// marker. Without a readable conversation or a known publisher there is
// none to edit (another author's comment is never edited).
func (p *prReview) previousParecer() (int64, bool) {
	publisher := strings.TrimSpace(p.env().publisherLogin)
	if p.historyErr != nil || publisher == "" {
		return 0, false
	}
	var id int64
	found := false
	for _, e := range p.historyEntries {
		if e.Kind == "comment" && strings.EqualFold(strings.TrimSpace(e.Author), publisher) && strings.Contains(e.Body, reviewBodyMarker) {
			id, found = e.ID, true
		}
	}
	return id, found
}

// finish saves review memory, publishes the commit statuses and returns the
// session's exit decision. The statuses are published before a comment
// failure is reported: a grave finding must still get its failing check
// even when an unrelated comment failed (and vice versa); the two outcomes
// are independent.
func (p *prReview) finish(failures []string, artifactsMissing bool) int {
	stderr := p.stderr
	// AUR-505: a degraded run has no model answer to remember; the
	// deterministic findings are not quality observations.
	if !p.modelDegraded() {
		persistReviewMemory(p.memoryStore, p.cfg.Review.Memory, p.memoryNotes, p.result.Issues, stderr, p.filter)
	}
	out := publishOutcome{failures: len(failures), artifactsMissing: artifactsMissing}
	if p.check {
		out.checkExit = publishCheckStatus(p.ctx, p.client, p.stdout, stderr, p.owner, p.repoName, p.commitID, p.issues, p.prNumber, (p.opts.exigirQualidade && p.modelDegraded()) || p.model == modelDeliberationLimit, p.model == modelProviderFailed, p.blockingRule())
		// AUR-519: the policy gate's own status, independent of --check's
		// grave-finding status; a no-op when no gate was declared.
		out.gateCheckExit = publishPolicyGateStatus(p.ctx, p.client, p.stdout, stderr, p.owner, p.repoName, p.commitID, *p.gateRes, p.prNumber)
	}
	if len(failures) > 0 {
		fmt.Fprintf(stderr, "aurumcode review: %d comentario(s) falharam ao publicar:\n", len(failures))
		for _, f := range failures {
			fmt.Fprintf(stderr, "  %s\n", f)
		}
	}
	return p.decideExit(out)
}
