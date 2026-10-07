package main

import (
	"fmt"

	"github.com/Mpaape/AurumCode/internal/git/githubclient"
	"github.com/Mpaape/AurumCode/internal/review"
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

// postReview publishes the findings and the summary. The loop never lets one
// POST failure swallow the rest: failures are recorded and the loop
// continues, so every finding that COULD be published still was.
func (p *prReview) postReview() []string {
	summaryBody := formatGatedReviewBody(p.shown.publishedResult(p.result, p.reviewLanguage), p.diff, p.reviewLanguage, p.publication == "review" && p.inlineComments, p.changelogText, p.blockingRule())
	summaryBody = appendRoundNotice(summaryBody, p.round, p.reviewLanguage)
	summaryBody = appendPresentationNotice(summaryBody, p.shown, p.reviewLanguage)
	summaryBody = appendProposedExceptions(summaryBody, p.proposedExceptions)
	if p.publication == "review" {
		return p.postFormalReview(summaryBody)
	}
	return p.postSeparateComments(summaryBody)
}

// postFormalReview posts one formal GitHub review carrying every eligible
// inline comment and native suggestion.
func (p *prReview) postFormalReview(summaryBody string) (failures []string) {
	formalComments := make([]githubclient.ReviewLineComment, 0)
	if p.inlineComments {
		for i, issue := range p.shown.withSources(p.reviewLanguage) {
			if !isInlineEligible(p.diff, issue) || !p.round.posts(i) {
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
	_, outsideFailures := p.postOutsideDiffFindings()
	return append(failures, outsideFailures...)
}

// postSeparateComments is the historical mode: one comment per finding
// (inline when eligible and enabled, general otherwise), then the summary.
func (p *prReview) postSeparateComments(summaryBody string) (failures []string) {
	inlineCount, generalCount := 0, 0
	stdout, stderr := p.stdout, p.stderr
	for i, issue := range p.shown.withSources(p.reviewLanguage) {
		line := fmt.Sprintf("%s:%d: [%s] %s", issue.File, issue.Line, issue.Severity, issue.Message)
		if issue.Side == "LEFT" {
			line += " [LEFT/base]"
		}
		if !p.round.posts(i) {
			fmt.Fprintf(stdout, "%s %s\n", line, roundRepeatedMarker)
			continue
		}
		if p.inlineComments && isInlineEligible(p.diff, issue) {
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
			continue
		}
		if err := p.client.PostIssueComment(p.ctx, p.owner, p.repoName, p.prNumber, p.round.findingBody(i, issue, p.reviewLanguage)); err != nil {
			fmt.Fprintf(stderr, "aurumcode review: publishing general comment for %s:%d: %v\n", issue.File, issue.Line, err)
			failures = append(failures, fmt.Sprintf("%s:%d (geral): %v", issue.File, issue.Line, err))
			continue
		}
		fmt.Fprintf(stdout, "%s %s\n", line, outsideDiffPublishedMarker)
		generalCount++
	}
	outsidePublished, outsideFailures := p.postOutsideDiffFindings()
	generalCount += outsidePublished
	failures = append(failures, outsideFailures...)
	if err := p.client.PostIssueComment(p.ctx, p.owner, p.repoName, p.prNumber, summaryBody); err != nil {
		fmt.Fprintf(stderr, "aurumcode review: publishing review summary: %v\n", err)
		failures = append(failures, "summary: "+err.Error())
	}
	fmt.Fprintf(stdout, "%d comentario(s) publicado(s) no pull request #%d (%d na linha, %d geral).\n",
		inlineCount+generalCount, p.prNumber, inlineCount, generalCount)
	return failures
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
		out.checkExit = publishCheckStatus(p.ctx, p.client, p.stdout, stderr, p.owner, p.repoName, p.commitID, p.issues, p.prNumber, (p.opts.exigirQualidade && p.modelDegraded()) || p.model == modelDeliberationLimit, p.model == modelProviderFailed)
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
