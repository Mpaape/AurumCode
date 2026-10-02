package main

import (
	"fmt"
	"os"

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
	if code, done := p.resolveCommit(); done {
		return code, true
	}
	p.writeArtifacts()
	return p.finish(p.postReview()), true
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
	p.commitID = os.Getenv("GITHUB_SHA")
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
	return 0, false
}

// writeArtifacts writes AUR-521's audit record and SARIF once the gate's
// decision is final and the reviewed commit is resolved; a no-op unless
// --auditoria or --sarif was given.
func (p *prReview) writeArtifacts() {
	writeComplianceArtifacts(complianceArtifactInputs{
		auditoriaPath:          p.opts.auditoriaPath,
		sarifPath:              p.opts.sarifPath,
		policyDir:              p.opts.policyDir,
		centralCfg:             p.centralCfg,
		repo:                   p.owner + "/" + p.repoName,
		reviewedSHA:            p.commitID,
		model:                  firstNonEmpty(p.opts.modelo, os.Getenv("LLM_MODEL")),
		verdict:                canonicalVerdict(p.result),
		gate:                   *p.gateRes,
		gateInconclusiveReason: p.gateRes.Reason,
		analysisData:           p.gateRes.AnalysisData,
		diff:                   p.diff,
		issues:                 p.result.Issues,
		dynamicRules:           p.dynamicRules,
		coverageComplete:       !p.coverage.partial(),
		omittedFiles:           append(append([]string{}, p.coverage.IgnoredPaths...), p.coverage.FilteredPaths...),
	}, p.filter, p.stderr)
}

// postReview publishes the findings and the summary. The loop never lets one
// POST failure swallow the rest: failures are recorded and the loop
// continues, so every finding that COULD be published still was.
func (p *prReview) postReview() []string {
	summaryBody := formatPublishedReviewBody(p.result, p.diff, p.reviewLanguage, p.publication == "review" && p.inlineComments, p.changelogText)
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
		for _, issue := range p.issues {
			if !isInlineEligible(p.diff, issue) {
				continue
			}
			formalComments = append(formalComments, githubclient.ReviewLineComment{
				Body: formatInlineIssueForLanguage(issue, p.reviewLanguage),
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
		Event:    formalReviewEvent(p.result),
		CommitID: p.commitID,
		Comments: formalComments,
	}
	key := fmt.Sprintf("aurumcode/review/%d/%s", p.prNumber, p.commitID)
	if err := p.client.PostPullRequestReview(p.ctx, p.owner, p.repoName, p.prNumber, formal, key); err != nil {
		fmt.Fprintf(p.stderr, "aurumcode review: publishing formal review: %v\n", err)
		return append(failures, "formal review: "+err.Error())
	}
	fmt.Fprintf(p.stdout, "review formal %q publicado no pull request #%d (%d comentário(s) na linha).\n", formal.Event, p.prNumber, len(formalComments))
	return failures
}

// postSeparateComments is the historical mode: one comment per finding
// (inline when eligible and enabled, general otherwise), then the summary.
func (p *prReview) postSeparateComments(summaryBody string) (failures []string) {
	inlineCount, generalCount := 0, 0
	stdout, stderr := p.stdout, p.stderr
	for _, issue := range p.issues {
		line := fmt.Sprintf("%s:%d: [%s] %s", issue.File, issue.Line, issue.Severity, issue.Message)
		if issue.Side == "LEFT" {
			line += " [LEFT/base]"
		}
		if p.inlineComments && isInlineEligible(p.diff, issue) {
			comment := githubclient.ReviewComment{
				Body:     formatInlineIssueForLanguage(issue, p.reviewLanguage),
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
		if err := p.client.PostIssueComment(p.ctx, p.owner, p.repoName, p.prNumber, formatInlineIssueForLanguage(issue, p.reviewLanguage)); err != nil {
			fmt.Fprintf(stderr, "aurumcode review: publishing general comment for %s:%d: %v\n", issue.File, issue.Line, err)
			failures = append(failures, fmt.Sprintf("%s:%d (geral): %v", issue.File, issue.Line, err))
			continue
		}
		fmt.Fprintf(stdout, "%s -- publicado como comentario geral\n", line)
		generalCount++
	}
	if err := p.client.PostIssueComment(p.ctx, p.owner, p.repoName, p.prNumber, summaryBody); err != nil {
		fmt.Fprintf(stderr, "aurumcode review: publishing review summary: %v\n", err)
		failures = append(failures, "summary: "+err.Error())
	}
	fmt.Fprintf(stdout, "%d comentario(s) publicado(s) no pull request #%d (%d na linha, %d geral).\n",
		inlineCount+generalCount, p.prNumber, inlineCount, generalCount)
	return failures
}

// finish saves review memory, publishes the commit statuses and returns the
// exit code. The statuses are published before the comment-failure return: a
// grave finding must still get its failing check even when an unrelated
// comment failed (and vice versa); the two outcomes are independent.
func (p *prReview) finish(failures []string) int {
	stderr := p.stderr
	// AUR-505: a degraded run has no model answer to remember; the
	// deterministic findings are not quality observations.
	if !p.qualityDegraded {
		persistReviewMemory(p.memoryStore, p.cfg.Review.Memory, p.memoryNotes, p.result.Issues, stderr, p.filter)
	}
	checkExit, gateCheckExit := 0, 0
	if p.check {
		checkExit = publishCheckStatus(p.ctx, p.client, p.stdout, stderr, p.owner, p.repoName, p.commitID, p.issues, p.prNumber, p.opts.exigirQualidade && p.qualityDegraded, p.providerFailed)
		// AUR-519: the policy gate's own status, independent of --check's
		// grave-finding status; a no-op when no gate was declared.
		gateCheckExit = publishPolicyGateStatus(p.ctx, p.client, p.stdout, stderr, p.owner, p.repoName, p.commitID, *p.gateRes, p.prNumber)
	}
	if len(failures) > 0 {
		fmt.Fprintf(stderr, "aurumcode review: %d comentario(s) falharam ao publicar:\n", len(failures))
		for _, f := range failures {
			fmt.Fprintf(stderr, "  %s\n", f)
		}
		return 1
	}
	return p.exitCode(checkExit, gateCheckExit)
}

// exitCode orders the closing conditions: an inconclusive model under
// --exigir-qualidade, a status that could not be published (a transport
// failure, not a finding), the policy gate (a breach is exitFindings even
// when also inconclusive, B1; block without a breach is
// exitQualityNotReviewed), then --fail-on and --check's own status.
func (p *prReview) exitCode(checkExit, gateCheckExit int) int {
	if p.opts.exigirQualidade && p.qualityDegraded {
		fmt.Fprintln(p.stderr, "aurumcode review: --exigir-qualidade: the model review was inconclusive; the published deterministic findings do not approve this pull request")
		return exitQualityNotReviewed
	}
	if checkExit == 1 || gateCheckExit == 1 {
		return 1
	}
	if code, closed := gateExitCode(p.gateRes); closed {
		return code
	}
	if p.threshold > 0 {
		if n := countAtOrAbove(p.issues, p.threshold); n > 0 {
			fmt.Fprintf(p.stderr, "aurumcode review: %d finding(s) at severity %s or above (--fail-on %s)\n", n, p.thresholdName, p.thresholdName)
			return exitFindings
		}
	}
	if p.check {
		return checkExit
	}
	return 0
}
