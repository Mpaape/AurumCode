// AUR-438: the PR review path for `aurumcode review`.
//
//	aurumcode review --pr <numero> --repo <dono>/<projeto> --publicar --na-linha
//
// reads a pull request's changes through the restored GitHub client
// (AUR-437, internal/git/githubclient), runs them through the exact same
// review engine and provider selection the --base path already uses
// (internal/review.Reviewer, selectProvider), and publishes every finding
// as a pull request comment: at the file's exact changed line when the
// diff actually added that line, or as a general pull request comment when
// the finding sits outside the changed lines -- so a real finding is never
// silently dropped just because it cannot be anchored to a line the diff
// touched (see MUT-001), and one finding's failure to post never costs
// another finding its own comment (the publish loop aggregates failures
// instead of aborting on the first one). Publishing delegates authorization
// to the GitHub write endpoints: a workflow token may have
// pull-requests:write or statuses:write while the repository role reports
// push=false, so GET /repos/{owner}/{repo} is not a valid preflight here.
// The client still fails closed on an actual API denial and refuses when an
// inline comment would need a commit SHA that is not available (GITHUB_SHA
// unset): never a POST is built with an empty commit_id.
//
// This file owns only the wiring: flag handling, the diff-shape conversion
// from the client's package-local types to pkg/types (the client
// deliberately cannot import pkg/types itself -- see
// internal/git/githubclient/diff.go), the changed/unchanged-line
// classification, and the publish loop. It reuses the client and the
// engine exactly as they already exist; see docs/specs/AUR-438.md.
//
// AUR-439 adds --check: after the comment publish loop above, when --check
// was given, it publishes one commit status via the same restored client's
// SetStatus (internal/git/githubclient; the API response is authoritative in
// the reusable workflow) -- "failure" when at least one finding is grave (error
// severity), "success" otherwise -- so a branch protection rule that
// requires this check blocks the pull request's merge until the grave
// finding is fixed. See publishCheckStatus below and docs/specs/AUR-439.md.
//
// AUR-451 closes the gap its own measurement named: before this card,
// --seguranca/--fail-on/--limite/--modelo were parsed but never reached
// this path at all, so the security pass, the severity gate, the cost
// ceiling and the model choice only ever worked on --base -- the product's
// main use case, reviewing a pull request, ran none of them. This card
// wires all four into prReviewOptions below by calling the EXACT functions
// the --base path already uses (review.SecurityScanWithCoverage,
// severityRank/countAtOrAbove/exitFindings, printSecurityCoverage,
// costPrice/buildCostTracker/fixedModelProvider/printCostEstimate/
// printRealCost/reportBudgetExceeded, selectProviderForModel/
// reportModelUnavailable -- all in cmd/aurumcode/main.go and
// cmd/aurumcode/cost.go), never a second implementation. A security
// finding becomes its own published comment exactly like a quality
// finding does -- inline when the diff added that line, general
// otherwise -- because its Message already carries its rule citation
// (review.enforceRuleCitations), so no separate PR-comment section is
// needed the way the --base path's stdout report has one. Without any of
// the four flags, prReviewOptions is its zero value and this path's
// behavior is exactly AUR-438's/AUR-439's, unchanged.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/git/githubclient"
	"github.com/Mpaape/AurumCode/internal/llm"
	"github.com/Mpaape/AurumCode/internal/llm/cost"
	"github.com/Mpaape/AurumCode/internal/prompt"
	"github.com/Mpaape/AurumCode/internal/review"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// prReviewOptions carries the AUR-451 capabilities onto the PR path: the
// same four things the --base path already offers, reused here rather than
// reimplemented (see the package doc above). The zero value means none of
// the four flags was given, so a caller that never sets a field gets
// exactly the pre-AUR-451 --pr behavior.
type prReviewOptions struct {
	seguranca bool
	failOnSet bool
	failOn    string
	modeloSet bool
	modelo    string
	limiteSet bool
	limite    string
}

// runPRReview is reached only when --pr was explicitly given (see the
// fs.Visit dispatch in runReview); every other flag's published behavior
// is therefore untouched by this function's existence.
//
// --repo and --publicar are always required here, mirroring how --base is
// already required on the base-diff path: this card builds and tests
// exactly the one command the card declares
// (`review --pr N --repo o/r --publicar --na-linha`), and Non-goals rules
// out inventing an alternate, unobserved behavior for what any of these
// flags would mean if omitted.
//
// --na-linha is required too, UNLESS --check is given: AUR-439 declares
// its own command surface, `review --pr N --repo o/r --publicar --check`,
// with no --na-linha at all -- --check names its own behavior (a commit
// status) just as clearly as --na-linha names its own (an inline
// comment), so requiring the user to also spell out --na-linha would be
// asking for an opt-in this card's own declared command never gives. This
// changes nothing about the pre-existing, byte-pinned contract: without
// --check, an absent --na-linha still refuses exactly as it always has
// (see cmd/aurumcode/pr_test.go and docs/specs/AUR-438.md).
func runPRReview(stdout, stderr io.Writer, prNumber int, repoFlag string, publicar, naLinha, check bool, opts prReviewOptions) int {
	if repoFlag == "" {
		fmt.Fprintln(stderr, "aurumcode review: --repo is required with --pr")
		return 2
	}
	if !naLinha && !check {
		fmt.Fprintln(stderr, "aurumcode review: --na-linha is required with --pr")
		return 2
	}
	if !publicar {
		fmt.Fprintln(stderr, "aurumcode review: --publicar is required with --pr")
		return 2
	}
	if prNumber <= 0 {
		fmt.Fprintln(stderr, "aurumcode review: --pr must be a positive pull request number")
		return 2
	}
	owner, repoName, err := parseOwnerRepo(repoFlag)
	if err != nil {
		fmt.Fprintf(stderr, "aurumcode review: --repo: %v\n", err)
		return 2
	}

	// AUR-451: --fail-on/--modelo/--limite are validated here exactly as
	// the --base path validates them (runReview, cmd/aurumcode/main.go) --
	// same functions (parseFailOnLevel, parseLimiteUSD), same "explicitly
	// empty is a usage error, never a silently-disabled flag" rule -- so an
	// unknown --fail-on level, an empty --modelo, or an empty/unparsable
	// --limite is refused before any write-permission check, diff fetch or
	// model call, exactly like every other --pr usage error already is.
	threshold := 0
	thresholdName := ""
	if opts.failOnSet {
		var err error
		threshold, thresholdName, err = parseFailOnLevel(opts.failOn)
		if err != nil {
			fmt.Fprintf(stderr, "aurumcode review: %v\n", err)
			return 2
		}
	}
	if opts.modeloSet && opts.modelo == "" {
		fmt.Fprintln(stderr, "aurumcode review: --modelo: model name must not be empty")
		return 2
	}
	limiteUSD := 0.0
	if opts.limiteSet {
		if opts.limite == "" {
			fmt.Fprintln(stderr, "aurumcode review: --limite: value must not be empty")
			return 2
		}
		var err error
		limiteUSD, err = parseLimiteUSD(opts.limite)
		if err != nil {
			fmt.Fprintf(stderr, "aurumcode review: %v\n", err)
			return 2
		}
	}

	ctx := context.Background()
	client := newGitHubClient()
	// The reusable GitHub workflow opts into endpoint-scoped authorization.
	// Keep the direct CLI's historical repository-role preflight unless the
	// service explicitly selects this mode; that preserves its fail-closed
	// behavior for personal-token usage while fixing Actions' narrower token.
	if os.Getenv("AURUMCODE_PR_PERMISSION_MODE") == "endpoint" {
		client.AllowPullRequestWrites()
	}

	ghDiff, err := client.GetPullRequestDiff(ctx, owner, repoName, prNumber)
	if err != nil {
		fmt.Fprintf(stderr, "aurumcode review: fetching pull request diff: %v\n", err)
		return 1
	}
	diff := convertDiff(ghDiff)
	reviewConfig, reviewLanguage, err := loadPullRequestConfig(ctx, client, owner, repoName, os.Getenv("GITHUB_SHA"), os.Getenv("AURUMCODE_BASE_SHA"))
	if err != nil {
		fmt.Fprintf(stderr, "aurumcode review: loading repository review config: %v\n", err)
		return 1
	}
	diff = config.FilterIgnoredPaths(diff, reviewConfig)

	// Provider selection (AUR-451): --modelo picks which model reviews,
	// exactly the --base path's selectProviderForModel; without it,
	// selectProvider keeps the pre-AUR-451 selection verbatim. Neither
	// function is redefined here.
	var provider llm.Provider
	var providerVia string
	if opts.modelo != "" {
		provider, providerVia, err = selectProviderForModel(opts.modelo)
	} else {
		provider, err = selectProvider()
	}
	if err != nil {
		if opts.modelo != "" {
			return reportModelUnavailable(stderr, opts.modelo, err)
		}
		fmt.Fprintf(stderr, "aurumcode review: %v\n", err)
		return 1
	}
	if opts.modelo != "" {
		// stdout stays unaffected; this mirrors the --base path's own
		// --modelo selection note (runReview, cmd/aurumcode/main.go).
		fmt.Fprintf(stderr, "aurumcode review: reviewing with model %q (%s)\n", opts.modelo, providerVia)
	}

	// --limite (AUR-451): wire internal/llm/cost.Tracker into the
	// orchestrator exactly as the --base path does (buildCostTracker,
	// cmd/aurumcode/cost.go) -- the one place that estimates the cost and
	// refuses, before the model is ever called, when it exceeds the
	// ceiling. Without the flag, tracker stays nil and behavior is exactly
	// AUR-438's/AUR-439's: unmetered.
	var tracker *cost.Tracker
	if opts.limiteSet {
		price, err := costPrice()
		if err != nil {
			fmt.Fprintf(stderr, "aurumcode review: %v\n", err)
			return 2
		}
		modelKey := costModelKey(opts.modelo)
		provider = &fixedModelProvider{Provider: provider, model: modelKey}
		tracker = buildCostTracker(limiteUSD, modelKey, price)
		printCostEstimate(stderr, estimateCostUSD(diff, price), limiteUSD)
	}

	orchestrator := llm.NewOrchestrator(provider, nil, tracker)
	reviewer := review.NewReviewer(orchestrator, review.DefaultConfig())

	result, err := reviewer.GenerateReviewWithContext(ctx, diff, review.ReviewContext{
		CI:       readCIContext(),
		Language: reviewLanguage,
	})
	if err != nil {
		// --limite: the tracker refused before the model was called, so
		// nothing was spent -- checked first, exactly like the --base
		// path's own ordering (runReview).
		if errors.Is(err, llm.ErrBudgetExceeded) {
			return reportBudgetExceeded(stderr, limiteUSD, err)
		}
		var parseErr *prompt.ParseError
		if errors.As(err, &parseErr) {
			fmt.Fprintf(stderr, "aurumcode review: could not understand the model's response (%s)\n", parseErr.Kind)
			return 1
		}
		if opts.modelo != "" && errors.Is(err, llm.ErrAllProvidersFailed) {
			return reportModelUnavailable(stderr, opts.modelo, err)
		}
		fmt.Fprintf(stderr, "aurumcode review: %v\n", err)
		return 1
	}
	if opts.limiteSet {
		printRealCost(stderr, realCostUSD(tracker, limiteUSD), limiteUSD)
	}

	// --seguranca (AUR-451): the exact deterministic pass the --base path
	// runs (review.SecurityScanWithCoverage) over the exact same diff this
	// function already fetched -- no second scan, no second catalog. A
	// security finding is published as its own pull request comment below,
	// exactly like a quality finding: its Message already carries its rule
	// citation (review.enforceRuleCitations), so no separate section is
	// needed the way the --base path's stdout report has one.
	var securityFindings []types.ReviewIssue
	if opts.seguranca {
		var coverageApplied []string
		var coverageTotal int
		securityFindings, coverageApplied, coverageTotal, err = review.SecurityScanWithCoverage(diff)
		if err != nil {
			fmt.Fprintf(stderr, "aurumcode review: %v\n", err)
			return 1
		}
		printSecurityCoverage(stderr, coverageApplied, coverageTotal)
	}
	if len(securityFindings) > 0 {
		combined := make([]types.ReviewIssue, 0, len(result.Issues)+len(securityFindings))
		combined = append(combined, result.Issues...)
		combined = append(combined, securityFindings...)
		result.Issues = combined
	}
	result.Issues = config.ApplyRuleConfig(result.Issues, reviewConfig)
	result.Suggestions = filterSuggestionsToChangedLines(diff, result.Suggestions)

	// The engine already redacted every model-authored field on result
	// (internal/review.redactReviewResult, called inside GenerateReview
	// before it returns -- AUR-432). The comment bodies below are built
	// only from those already-redacted fields plus trusted literals (the
	// "[severity]" tag, "publicado..." suffixes), exactly like
	// printFindings' own comment explains for stdout, so nothing here
	// needs a second pass through the filter.
	//
	// Sorted (and computed) before the zero-issues check below on purpose:
	// AUR-439's --check needs this same, already-sorted slice (empty or
	// not) to decide its commit status, so both branches share one
	// definition instead of two.
	issues := sortedIssues(result.Issues)

	// GITHUB_SHA is the standard GitHub Actions convention for "the commit
	// under review" (see docs/specs/AUR-438.md's Compatibility note): the
	// restored client's GetPullRequestDiff reads the unified diff format,
	// which carries no commit SHA, and this card does not extend the
	// client (internal/git/githubclient is a read_path here) to add an
	// endpoint that would fetch one.
	//
	// A real GitHub review-comment POST with an empty commit_id is
	// rejected (422): an inline comment cannot be anchored to no commit at
	// all. So when at least one finding needs an inline comment and no SHA
	// is available, this refuses before any comment -- inline or general
	// -- is posted, the same fail-closed shape as the permission check
	// above, never a POST built with an empty commit_id. A run whose
	// findings are all general (nothing anchors to a specific added line)
	// does not need a commit SHA at all -- PostIssueComment carries no
	// commit_id -- so it is not blocked by this gate.
	//
	// AUR-439: SetStatus needs a commit SHA just as unconditionally as an
	// inline comment does (the statuses endpoint is
	// /repos/{owner}/{repo}/statuses/{sha} -- there is no shape of that
	// call without one), so --check folds into this exact gate --
	// needsCommitID starts at check's value instead of only being set
	// true by the loop below -- rather than growing a second, parallel
	// fail-closed check. This is true even when there will turn out to be
	// zero findings: --check must still be able to publish a "success"
	// status on an all-clear commit, and doing so needs the same SHA.
	commitID := os.Getenv("GITHUB_SHA")
	needsCommitID := check
	for _, issue := range issues {
		if isInlineEligible(diff, issue) {
			needsCommitID = true
			break
		}
	}
	if needsCommitID && commitID == "" {
		fmt.Fprintln(stderr, "aurumcode review: refusing to publish: an inline comment requires a commit SHA and none is available; set GITHUB_SHA")
		return 1
	}

	// The publish loop never lets one finding's POST failure swallow the
	// rest: a failure is recorded and the loop continues, so an inline
	// comment that fails to post does not also cost the general comment
	// for a different, unrelated finding (or vice versa). Every recorded
	// failure is reported after the loop, and the command exits non-zero
	// if any occurred -- but every finding that COULD be published still
	// was.
	var failures []string
	inlineCount, generalCount := 0, 0
	for _, issue := range issues {
		line := fmt.Sprintf("%s:%d: [%s] %s", issue.File, issue.Line, issue.Severity, issue.Message)
		if isInlineEligible(diff, issue) {
			comment := githubclient.ReviewComment{
				Body:     formatInlineIssueForLanguage(issue, reviewLanguage),
				CommitID: commitID,
				Path:     issue.File,
				Line:     issue.Line,
			}
			key := fmt.Sprintf("aurumcode/%d/%s/%d/%s", prNumber, issue.File, issue.Line, issue.RuleID)
			if err := client.PostReviewComment(ctx, owner, repoName, prNumber, comment, key); err != nil {
				fmt.Fprintf(stderr, "aurumcode review: publishing inline comment on %s:%d: %v\n", issue.File, issue.Line, err)
				failures = append(failures, fmt.Sprintf("%s:%d (na linha): %v", issue.File, issue.Line, err))
				continue
			}
			fmt.Fprintf(stdout, "%s -- publicado na linha\n", line)
			inlineCount++
			continue
		}

		// Outside the changed lines: still published, as a general pull
		// request comment, never dropped. MUT-001 mutates exactly this
		// branch.
		if err := client.PostIssueComment(ctx, owner, repoName, prNumber, formatInlineIssueForLanguage(issue, reviewLanguage)); err != nil {
			fmt.Fprintf(stderr, "aurumcode review: publishing general comment for %s:%d: %v\n", issue.File, issue.Line, err)
			failures = append(failures, fmt.Sprintf("%s:%d (geral): %v", issue.File, issue.Line, err))
			continue
		}
		fmt.Fprintf(stdout, "%s -- publicado como comentario geral (fora das linhas alteradas)\n", line)
		generalCount++
	}

	// One summary comment is the human-facing review. Operational progress,
	// provider details and publish failures stay in the Action log; the PR gets
	// only the structured review itself.
	if err := client.PostIssueComment(ctx, owner, repoName, prNumber, formatReviewSummaryForLanguage(result, reviewLanguage)); err != nil {
		fmt.Fprintf(stderr, "aurumcode review: publishing review summary: %v\n", err)
		failures = append(failures, "summary: "+err.Error())
	}

	fmt.Fprintf(stdout, "%d comentario(s) publicado(s) no pull request #%d (%d na linha, %d geral).\n",
		inlineCount+generalCount, prNumber, inlineCount, generalCount)

	// The check status is published before the comment-failure return
	// below, not after: a grave finding must still get its failing check
	// even when a separate, unrelated comment POST failed to publish (and
	// symmetrically, a comment failure must not silently cost the check
	// too). The two outcomes are independent, so neither is allowed to
	// swallow the other.
	checkExit := 0
	if check {
		checkExit = publishCheckStatus(ctx, client, stdout, stderr, owner, repoName, commitID, issues, prNumber)
	}

	if len(failures) > 0 {
		fmt.Fprintf(stderr, "aurumcode review: %d comentario(s) falharam ao publicar:\n", len(failures))
		for _, f := range failures {
			fmt.Fprintf(stderr, "  %s\n", f)
		}
		return 1
	}

	// --fail-on (AUR-451): after publishing, exit the same distinct code
	// exitFindings the --base path already uses (countAtOrAbove/
	// severityRank, cmd/aurumcode/main.go) when a published finding --
	// quality or security -- sits at the chosen severity or above.
	// Independent of --check's own grave-only gate above: either alone can
	// close the gate. checkExit == 1 (SetStatus itself could not be
	// published, a transport failure rather than a finding) still takes
	// priority, exactly as it already did before this card.
	if checkExit == 1 {
		return checkExit
	}
	if threshold > 0 {
		if n := countAtOrAbove(issues, threshold); n > 0 {
			fmt.Fprintf(stderr, "aurumcode review: %d finding(s) at severity %s or above (--fail-on %s)\n", n, thresholdName, thresholdName)
			return exitFindings
		}
	}
	if check {
		return checkExit
	}
	return 0
}

// checkContext is the commit status "context" AUR-439's --check publishes.
// Branch protection keys on this exact string to decide which status
// checks are required before a merge is allowed -- a silent rename here
// would make every existing branch protection rule that requires it stop
// seeing it, which un-blocks every merge instead of gating it. Treat it as
// part of this card's public contract (see docs/specs/AUR-439.md).
const checkContext = "aurumcode/review"

// publishCheckStatus is AUR-439: it classifies issues as grave — rank
// rankError, the exact severity --fail-on high|error already names (see
// severityRank/countAtOrAbove in cmd/aurumcode/main.go, reused here rather
// than a second severity ladder) — and publishes exactly one commit status
// through the restored AUR-437 client's SetStatus: "failure" when at least
// one grave finding is present, "success" otherwise. issues may be empty
// (the "No issues found." path still needs a "success" status to clear an
// earlier failing check on the same commit once it is fixed).
//
// The returned int is this function's contribution to runPRReview's own
// exit code: exitFindings (3, the same code --fail-on already uses for
// "the review ran fine and found something that matters") when the
// published status is "failure", 0 when it is "success", 1 when SetStatus
// itself could not be published (a transport/API failure, not a finding).
// MUT-001 is exactly the defect of this function reporting success (either
// the exit code or the published state) while a grave finding is present.
func publishCheckStatus(ctx context.Context, client *githubclient.Client, stdout, stderr io.Writer, owner, repoName, commitID string, issues []types.ReviewIssue, prNumber int) int {
	grave := countAtOrAbove(issues, rankError)
	status := githubclient.CommitStatus{Context: checkContext}
	if grave > 0 {
		status.State = "failure"
		status.Description = fmt.Sprintf("%d achado(s) grave(s) no pull request #%d", grave, prNumber)
	} else {
		status.State = "success"
		status.Description = fmt.Sprintf("nenhum achado grave no pull request #%d", prNumber)
	}

	if err := client.SetStatus(ctx, owner, repoName, commitID, status); err != nil {
		fmt.Fprintf(stderr, "aurumcode review: publishing check status: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "check %q publicado no commit %s: %s (%s)\n", checkContext, commitID, status.State, status.Description)

	if grave > 0 {
		return exitFindings
	}
	return 0
}

// newGitHubClient builds the restored AUR-437 client.
// AURUMCODE_GITHUB_API_URL overrides the API base so this card's own tests
// can point it at a loopback httptest server (the sealed profile denies
// real network access); production use leaves it unset and gets
// githubclient.DefaultBaseURL. GITHUB_TOKEN follows the same convention
// GitHub Actions already exposes to a step (see docs/specs/AUR-440.md); an
// empty token still builds a working client -- reading a public repository
// needs no auth. Direct CLI publishing retains the repository-role preflight;
// the reusable workflow sets AURUMCODE_PR_PERMISSION_MODE=endpoint so GitHub
// itself enforces pull-requests:write and statuses:write on the actual POST.
func newGitHubClient() *githubclient.Client {
	token := os.Getenv("GITHUB_TOKEN")
	if base := os.Getenv("AURUMCODE_GITHUB_API_URL"); base != "" {
		return githubclient.NewClientWithBaseURL(token, base)
	}
	return githubclient.NewClient(token)
}

// loadPullRequestConfig reads the explicit repository config without
// checking out pull-request code. GitHub Actions supplies the PR head SHA,
// so a config added by the change is available to that review; a direct local
// invocation falls back to the current working tree. A missing file is the
// zero-config contract and selects the stable English default.
func loadPullRequestConfig(ctx context.Context, client *githubclient.Client, owner, repo, headRef, baseRef string) (*config.Config, string, error) {
	var cfg *config.Config
	var err error
	if strings.TrimSpace(headRef) == "" {
		cwd, cwdErr := os.Getwd()
		if cwdErr != nil {
			return nil, "", cwdErr
		}
		cfg, err = config.Load(cwd)
	} else if strings.TrimSpace(baseRef) == "" {
		data, found, fetchErr := client.GetRepositoryFile(ctx, owner, repo, config.DefaultConfigPath, headRef)
		if fetchErr != nil {
			return nil, "", fetchErr
		}
		if !found {
			cfg = &config.Config{}
		} else {
			cfg, err = config.Parse(data, config.DefaultConfigPath)
		}
	} else {
		// Gate-affecting settings come from the base branch. A PR may
		// propose a language change, but it cannot weaken its own review by
		// changing rules or ignored paths in the same diff.
		baseData, baseFound, fetchErr := client.GetRepositoryFile(ctx, owner, repo, config.DefaultConfigPath, baseRef)
		if fetchErr != nil {
			return nil, "", fetchErr
		}
		if baseFound {
			cfg, err = config.Parse(baseData, config.DefaultConfigPath)
		} else {
			cfg = &config.Config{}
		}
		if err != nil {
			return nil, "", err
		}
		headData, headFound, fetchErr := client.GetRepositoryFile(ctx, owner, repo, config.DefaultConfigPath, headRef)
		if fetchErr != nil {
			return nil, "", fetchErr
		}
		if headFound {
			headCfg, parseErr := config.Parse(headData, config.DefaultConfigPath)
			if parseErr != nil {
				return nil, "", parseErr
			}
			if strings.TrimSpace(headCfg.Review.Language) != "" {
				cfg.Review.Language = headCfg.Review.Language
			}
		}
	}
	if err != nil {
		return nil, "", err
	}
	language, err := cfg.ReviewLanguage()
	if err != nil {
		return nil, "", err
	}
	return cfg, language, nil
}

// parseOwnerRepo splits the --repo flag's "owner/repo" form. Anything else
// -- no slash, an empty owner or repo segment, or more than one slash -- is
// a usage error.
func parseOwnerRepo(repo string) (owner, name string, err error) {
	idx := strings.IndexByte(repo, '/')
	if idx <= 0 || idx == len(repo)-1 {
		return "", "", fmt.Errorf("expected the form owner/repo, got %q", repo)
	}
	owner, name = repo[:idx], repo[idx+1:]
	if strings.IndexByte(name, '/') != -1 {
		return "", "", fmt.Errorf("expected the form owner/repo, got %q", repo)
	}
	return owner, name, nil
}

// convertDiff mirrors internal/git/githubclient's package-local diff shape
// into pkg/types.Diff, field by field. The client's Diff/DiffFile/DiffHunk
// are deliberately local to its own package -- AUR-437's sealed acceptance
// materializes only that package's declared paths/read_paths, so it cannot
// import pkg/types -- and the two shapes mirror each other exactly (same
// field names, same meaning) precisely so this conversion, owned by this
// card, is lossless and trivial at the one boundary where a caller holds
// both packages.
func convertDiff(d *githubclient.Diff) *types.Diff {
	out := &types.Diff{Files: make([]types.DiffFile, len(d.Files))}
	for i, f := range d.Files {
		hunks := make([]types.DiffHunk, len(f.Hunks))
		for j, h := range f.Hunks {
			lines := make([]string, len(h.Lines))
			copy(lines, h.Lines)
			hunks[j] = types.DiffHunk{
				OldStart: h.OldStart,
				OldLines: h.OldLines,
				NewStart: h.NewStart,
				NewLines: h.NewLines,
				Lines:    lines,
			}
		}
		out.Files[i] = types.DiffFile{Path: f.Path, Lang: f.Lang, Hunks: hunks}
	}
	return out
}

// addedLineNumbers returns the set of new-side line numbers hunk h adds.
// The restored client's parser (internal/git/githubclient/client.go,
// parseDiff) keeps every hunk content line's leading +/-/space marker: an
// added ('+') line is recorded at the current new-file counter and
// advances it; a context (' ') line only advances it; a removed ('-') line
// does neither, because it has no line on the new side to advance past or
// to anchor a comment to.
func addedLineNumbers(h types.DiffHunk) map[int]bool {
	added := make(map[int]bool)
	n := h.NewStart
	for _, l := range h.Lines {
		if l == "" {
			continue
		}
		switch l[0] {
		case '+':
			added[n] = true
			n++
		case ' ':
			n++
		}
	}
	return added
}

// isInlineEligible reports whether issue names a file and line this diff
// actually added. Only an added line can carry an exact-line pull request
// review comment (AC-001's "na linha exata"); a finding on any other line
// -- a file the diff never touched, a context/unchanged line, or a line
// number past every hunk -- is not dropped, it is published as a general
// comment instead (see runPRReview and MUT-001).
func isInlineEligible(diff *types.Diff, issue types.ReviewIssue) bool {
	for _, f := range diff.Files {
		if f.Path != issue.File {
			continue
		}
		for _, h := range f.Hunks {
			if addedLineNumbers(h)[issue.Line] {
				return true
			}
		}
	}
	return false
}

// sortedIssues returns a copy of issues ordered by (file, line) -- the same
// order printFindings already uses for the --base contract -- so the
// sequence of PostReviewComment/PostIssueComment calls this card makes is
// deterministic. AC-001 requires that repeating the same input produces the
// same output; for the PR path that output includes the publish
// transcript (what got posted, in what order), not only stdout.
func sortedIssues(issues []types.ReviewIssue) []types.ReviewIssue {
	out := make([]types.ReviewIssue, len(issues))
	copy(out, issues)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].File != out[j].File {
			return out[i].File < out[j].File
		}
		return out[i].Line < out[j].Line
	})
	return out
}

// filterSuggestionsToChangedLines keeps the published review actionable. A
// non-blocking suggestion may be general, but once it claims a file and line
// it must point at actually added lines in this pull request. A code proposal
// may cover a range, but every line in that range must be added; this gives a
// future apply operation an exact, reviewable replacement boundary. Models
// often emit line zero or cite nearby context files; publishing those
// locations makes the review look authoritative while giving the author
// nowhere useful to act. Suggestions without a location remain valid general
// advice.
func filterSuggestionsToChangedLines(diff *types.Diff, suggestions []types.ReviewSuggestion) []types.ReviewSuggestion {
	filtered := make([]types.ReviewSuggestion, 0, len(suggestions))
	for _, suggestion := range suggestions {
		start, end := suggestionRange(suggestion)
		if strings.TrimSpace(suggestion.File) == "" && start <= 0 && end <= 0 {
			filtered = append(filtered, suggestion)
			continue
		}
		if strings.TrimSpace(suggestion.File) == "" || start <= 0 || end < start || end-start > 1000 {
			continue
		}
		valid := true
		for line := start; ; line++ {
			if !isInlineEligible(diff, types.ReviewIssue{File: suggestion.File, Line: line}) {
				valid = false
				break
			}
			if line == end {
				break
			}
		}
		if !valid {
			continue
		}
		filtered = append(filtered, suggestion)
	}
	return filtered
}

// suggestionRange keeps the old single-line field compatible while allowing
// newer model responses to describe a complete replacement range.
func suggestionRange(suggestion types.ReviewSuggestion) (start, end int) {
	start = suggestion.StartLine
	if start <= 0 {
		start = suggestion.Line
	}
	end = suggestion.EndLine
	if end <= 0 {
		end = start
	}
	return start, end
}

const maxCIContextBytes = 16000

// readCIContext reads only the optional, workflow-produced check summary. It
// is intentionally not a required user setting: a review still runs when CI
// has not reported anything yet. The bound keeps a large provider response
// from consuming the whole review prompt.
func readCIContext() string {
	path := strings.TrimSpace(os.Getenv("AURUMCODE_CI_CONTEXT_FILE"))
	if path == "" {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return ""
	}
	if len(data) > maxCIContextBytes {
		data = data[:maxCIContextBytes]
	}
	return string(data)
}

func formatInlineIssue(issue types.ReviewIssue) string {
	return formatInlineIssueForLanguage(issue, "en-US")
}

func formatInlineIssueForLanguage(issue types.ReviewIssue, language string) string {
	copy := reviewCopyFor(language)
	var b strings.Builder
	fmt.Fprintf(&b, "**[%s] %s**", issue.Severity, issue.Message)
	writeReviewField(&b, copy.impact, issue.Impact)
	writeReviewField(&b, copy.evidence, issue.Evidence)
	writeReviewField(&b, copy.suggestedFix, issue.Suggestion)
	writeReviewField(&b, copy.verify, issue.Verification)
	return b.String()
}

func writeReviewField(b *strings.Builder, label, value string) {
	if value == "" {
		return
	}
	fmt.Fprintf(b, "\n\n**%s:** %s", label, strings.TrimSpace(value))
}

func formatReviewSummary(result *types.ReviewResult) string {
	return formatReviewSummaryForLanguage(result, "en-US")
}

func formatReviewSummaryForLanguage(result *types.ReviewResult, language string) string {
	copy := reviewCopyFor(language)
	var b strings.Builder
	b.WriteString("<!-- aurumcode-review -->\n")
	fmt.Fprintf(&b, "## AurumCode %s\n\n", copy.title)
	fmt.Fprintf(&b, "**%s:** %s\n\n", copy.verdict, reviewVerdictForLanguage(result, copy))
	b.WriteString(reviewSummaryTextForLanguage(result, copy))
	b.WriteString("\n\n")

	if len(result.Strengths) > 0 {
		fmt.Fprintf(&b, "### %s\n\n", copy.strengths)
		writeReviewBullets(&b, result.Strengths)
		b.WriteString("\n")
	}

	if issues := sortedIssues(result.Issues); len(issues) > 0 {
		fmt.Fprintf(&b, "### %s\n\n", copy.findings)
		for _, issue := range issues {
			fmt.Fprintf(&b, "- **[%s] %s:%d** — %s\n", issue.Severity, issue.File, issue.Line, issue.Message)
			if issue.Impact != "" {
				fmt.Fprintf(&b, "  - %s: %s\n", copy.impact, issue.Impact)
			}
			if issue.Evidence != "" {
				fmt.Fprintf(&b, "  - %s: %s\n", copy.evidence, issue.Evidence)
			}
			if issue.Suggestion != "" {
				fmt.Fprintf(&b, "  - %s: %s\n", copy.suggestedFix, issue.Suggestion)
			}
			if issue.Verification != "" {
				fmt.Fprintf(&b, "  - %s: %s\n", copy.verify, issue.Verification)
			}
		}
		b.WriteString("\n")
	}

	if len(result.Suggestions) > 0 {
		fmt.Fprintf(&b, "### %s\n\n", copy.suggestions)
		for _, suggestion := range result.Suggestions {
			if strings.TrimSpace(suggestion.Title) == "" && strings.TrimSpace(suggestion.Description) == "" {
				continue
			}
			fmt.Fprintf(&b, "- **%s**", strings.TrimSpace(suggestion.Title))
			if suggestion.Description != "" {
				fmt.Fprintf(&b, " — %s", strings.TrimSpace(suggestion.Description))
			}
			if suggestion.File != "" {
				start, end := suggestionRange(suggestion)
				if start > 0 && end > 0 {
					if start == end {
						fmt.Fprintf(&b, " (`%s:%d`)", suggestion.File, start)
					} else {
						fmt.Fprintf(&b, " (`%s:%d-%d`)", suggestion.File, start, end)
					}
				}
			}
			b.WriteByte('\n')
			if strings.TrimSpace(suggestion.ProposedCode) != "" {
				fmt.Fprintf(&b, "  - **%s:**\n\n    ```\n%s\n    ```\n", copy.proposedImplementation, strings.TrimSpace(suggestion.ProposedCode))
			}
			writeSummaryField(&b, copy.rationale, suggestion.Rationale)
			writeSummaryField(&b, copy.verify, suggestion.Verification)
		}
		b.WriteString("\n")
	}

	if len(result.CIAnalysis) > 0 {
		fmt.Fprintf(&b, "### %s\n\n", copy.ciStatus)
		for _, analysis := range result.CIAnalysis {
			fmt.Fprintf(&b, "- **%s — %s**\n", analysis.Check, analysis.Status)
			writeSummaryField(&b, copy.cause, analysis.Cause)
			writeSummaryField(&b, copy.evidence, analysis.Evidence)
			writeSummaryField(&b, copy.fix, analysis.Fix)
			writeSummaryField(&b, copy.nextVerification, analysis.NextVerification)
		}
		b.WriteString("\n")
	}

	if len(result.TestPlan) > 0 {
		fmt.Fprintf(&b, "### %s\n\n", copy.tests)
		writeReviewBullets(&b, result.TestPlan)
		b.WriteString("\n")
	}

	if len(result.Limitations) > 0 {
		fmt.Fprintf(&b, "### %s\n\n", copy.limits)
		writeReviewBullets(&b, result.Limitations)
	}

	return strings.TrimSpace(b.String()) + "\n"
}

func reviewVerdict(result *types.ReviewResult) string {
	return reviewVerdictForLanguage(result, reviewCopyFor("en-US"))
}

func reviewVerdictForLanguage(result *types.ReviewResult, copy reviewCopy) string {
	for _, issue := range result.Issues {
		switch strings.ToLower(issue.Severity) {
		case "error", "warning":
			return copy.changesRequested
		}
	}
	if len(result.Issues) > 0 {
		return copy.comment
	}
	for _, suggestion := range result.Suggestions {
		if strings.TrimSpace(suggestion.Title) != "" || strings.TrimSpace(suggestion.Description) != "" {
			return copy.comment
		}
	}
	return copy.approve
}

// reviewSummaryText is deliberately derived from the filtered result rather
// than copied from result.Summary. The model summary can become stale when a
// source-aware gate removes a false positive; publishing it would produce a
// contradictory verdict and review comment.
func reviewSummaryText(result *types.ReviewResult) string {
	return reviewSummaryTextForLanguage(result, reviewCopyFor("en-US"))
}

func reviewSummaryTextForLanguage(result *types.ReviewResult, copy reviewCopy) string {
	blocking := 0
	for _, issue := range result.Issues {
		switch strings.ToLower(issue.Severity) {
		case "error", "warning":
			blocking++
		}
	}
	if blocking > 0 {
		return fmt.Sprintf(copy.blockingFindings, blocking)
	}
	if len(result.Issues) > 0 {
		return copy.nonBlockingFindings
	}
	for _, suggestion := range result.Suggestions {
		if strings.TrimSpace(suggestion.Title) != "" || strings.TrimSpace(suggestion.Description) != "" {
			return copy.optionalSuggestions
		}
	}
	return copy.noBlockingFindings
}

type reviewCopy struct {
	title, verdict, strengths, findings, suggestions, ciStatus, tests, limits      string
	impact, evidence, suggestedFix, verify, rationale, proposedImplementation      string
	cause, fix, nextVerification                                                   string
	changesRequested, comment, approve                                             string
	blockingFindings, nonBlockingFindings, optionalSuggestions, noBlockingFindings string
}

func reviewCopyFor(language string) reviewCopy {
	if strings.EqualFold(strings.TrimSpace(language), "pt-BR") || strings.EqualFold(strings.TrimSpace(language), "pt") {
		return reviewCopy{
			title: "revisão de código", verdict: "Veredito", strengths: "Pontos fortes", findings: "Achados", suggestions: "Sugestões", ciStatus: "Status do CI", tests: "Testes", limits: "Limitações da revisão",
			impact: "Impacto", evidence: "Evidência", suggestedFix: "Correção sugerida", verify: "Verificação", rationale: "Motivação", proposedImplementation: "Implementação sugerida", cause: "Causa", fix: "Correção", nextVerification: "Próxima verificação",
			changesRequested: "Alterações solicitadas", comment: "Comentário", approve: "Aprovado",
			blockingFindings:    "A revisão encontrou %d achado(s) bloqueante(s) que devem ser tratados antes do merge.",
			nonBlockingFindings: "A revisão encontrou observações, mas nenhum achado bloqueante permanece na mudança revisada.",
			optionalSuggestions: "Nenhum achado bloqueante foi identificado; as sugestões abaixo são melhorias opcionais.",
			noBlockingFindings:  "Nenhum achado bloqueante foi identificado na mudança revisada.",
		}
	}
	return reviewCopy{
		title: "code review", verdict: "Verdict", strengths: "Strengths", findings: "Findings", suggestions: "Suggestions", ciStatus: "CI status", tests: "Tests", limits: "Review limits",
		impact: "Impact", evidence: "Evidence", suggestedFix: "Suggested fix", verify: "Verify", rationale: "Rationale", proposedImplementation: "Proposed implementation", cause: "Cause", fix: "Fix", nextVerification: "Next verification",
		changesRequested: "Changes requested", comment: "Comment", approve: "Approve",
		blockingFindings:    "The review found %d blocking finding(s) that should be addressed before merge.",
		nonBlockingFindings: "The review found observations, but no blocking finding remains in the reviewed change.",
		optionalSuggestions: "No blocking finding was identified; the suggestions below are optional improvements.",
		noBlockingFindings:  "No blocking finding was identified in the reviewed change.",
	}
}

func writeReviewBullets(b *strings.Builder, values []string) {
	for _, value := range values {
		if text := strings.TrimSpace(value); text != "" {
			fmt.Fprintf(b, "- %s\n", text)
		}
	}
}

func writeSummaryField(b *strings.Builder, label, value string) {
	if strings.TrimSpace(value) != "" {
		fmt.Fprintf(b, "  - **%s:** %s\n", label, strings.TrimSpace(value))
	}
}
