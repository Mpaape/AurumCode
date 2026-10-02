// AUR-438: the PR review path for `aurumcode review`.
//
//	aurumcode review --pr <numero> --repo <dono>/<projeto> --publicar --modo-publicacao review
//
// reads a pull request's changes through the restored GitHub client
// (AUR-437, internal/git/githubclient), runs them through the exact same
// review engine and provider selection the --base path already uses
// (internal/review.Reviewer, selectProvider), and publishes every finding
// as either one formal GitHub review or the backwards-compatible set of
// separate comments. In both modes, --na-linha is optional: when enabled it
// anchors eligible findings to exact added lines, while findings outside the
// diff remain in the review body or become general comments. A real finding
// is never silently dropped just because it cannot be anchored to a line the
// diff touched (see MUT-001). Publishing delegates authorization
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
// AUR-439 adds --check: after the publication above, when --check
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
//
// AUR-504 fixes where --check anchors. Before it, the status was published
// on os.Getenv("GITHUB_SHA"), which GitHub Actions reserves to the synthetic
// merge commit on a pull_request event, so a branch protection rule that
// requires the context never saw it on the head. With --check, the head SHA
// now comes from the API (GetPullRequestMetadata.HeadSHA,
// resolvePullRequestHeadSHA below) and anchors the status, the formal review
// commit_id and every inline commit_id; when it cannot be determined the
// command fails closed naming the reason instead of publishing on the wrong
// commit. The verdict and severity policy are untouched, and --base never
// reaches this path. See docs/specs/AUR-504.md.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/Mpaape/AurumCode/internal/changelog"
	"github.com/Mpaape/AurumCode/internal/config"
	codebasectx "github.com/Mpaape/AurumCode/internal/context"
	"github.com/Mpaape/AurumCode/internal/git/githubclient"
	"github.com/Mpaape/AurumCode/internal/llm"
	"github.com/Mpaape/AurumCode/internal/memory"
	"github.com/Mpaape/AurumCode/internal/prompt"
	"github.com/Mpaape/AurumCode/internal/review"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// prReviewOptions carries the AUR-451 capabilities onto the PR path: the
// same four things the --base path already offers, reused here rather than
// reimplemented (see the package doc above). The zero value means none of
// the PR flags was given, so a caller that never sets a field keeps the
// historical --pr behavior. exigirQualidade is opt-in for direct CLI callers;
// the reusable workflow enables it by default.
type prReviewOptions struct {
	seguranca       bool
	exigirQualidade bool
	failOnSet       bool
	failOn          string
	modeloSet       bool
	modelo          string
	limiteSet       bool
	limite          string
	publicationSet  bool
	publication     string
	// changelog forces the AUR-499 release section on. It is false when
	// --changelog was absent, in which case review.changelog decides.
	changelog bool
	// policyDir is the central policy directory from --politica/--policy or
	// AURUMCODE_POLICY (AUR-518); empty means no policy was declared and
	// this path is unchanged (AC-006).
	policyDir string
	// auditoriaPath/sarifPath are AUR-521's compliance artifact paths
	// (--auditoria/--sarif); empty means neither file is written, this
	// path's published behavior unchanged.
	auditoriaPath string
	sarifPath     string
}

// pullRequestChangelogSource reads the PR title/body and the PR's commit
// messages, then folds the title/body into a synthetic first commit so the
// engine sees the whole reviewed narrative. Any missing source is an error and
// the caller declares a limitation; nothing here is executed.
func pullRequestChangelogSource(ctx context.Context, client *githubclient.Client, owner, repo string, number int) ([]changelog.Commit, error) {
	ctx, cancel := context.WithTimeout(ctx, config.ProviderTimeout)
	defer cancel()
	meta, err := client.GetPullRequestMetadata(ctx, owner, repo, number)
	if err != nil {
		return nil, err
	}
	commits, err := client.GetPullRequestCommits(ctx, owner, repo, number)
	if err != nil {
		return nil, err
	}
	if len(commits) == 0 {
		return nil, errors.New("pull request has no commit messages")
	}
	out := make([]changelog.Commit, 0, len(commits)+1)
	if strings.TrimSpace(meta.Title) != "" || strings.TrimSpace(meta.Body) != "" {
		out = append(out, changelog.Commit{Subject: meta.Title, Body: meta.Body})
	}
	for _, c := range commits {
		subject, body := splitCommitMessage(c.Message)
		out = append(out, changelog.Commit{Subject: subject, Body: body, Hash: c.SHA})
	}
	return out, nil
}

// splitCommitMessage splits a GitHub commit message into its first line and
// the remaining body. Both parts are untrusted.
func splitCommitMessage(msg string) (subject, body string) {
	msg = strings.TrimRight(strings.ReplaceAll(msg, "\r\n", "\n"), "\n")
	if i := strings.IndexByte(msg, '\n'); i >= 0 {
		return msg[:i], strings.TrimLeft(msg[i+1:], "\n")
	}
	return msg, ""
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
// itself could not be published or a required model review was inconclusive.
// MUT-001 is exactly the defect of this function reporting success (either
// the exit code or the published state) while a grave finding is present.
//
// AUR-537 B1: providerFailed means the quality review never ran at all --
// every configured provider failed, or --limite refused the call before one
// was ever reached. Before AUR-537 this status was simply never published
// for that run (runPRReview returned before reaching --check at all), which
// failed closed for any ruleset requiring this context. AUR-537 made the
// function keep going, but a provider outage is NOT "inconclusive the way
// AUR-505's own model-parse failure is" for THIS status: it must never read
// "success" here, in gate.inconclusive: block OR warn, and REGARDLESS of
// --exigir-qualidade -- warn only ever softens aurumcode/policy-gate
// (publishPolicyGateStatus), never this legacy grave-finding status. A prior
// round of this card left qualityRequiredButIncomplete (gated on
// --exigir-qualidade) as the only lever, which let a provider outage publish
// a false "nenhum achado grave" success whenever --exigir-qualidade was not
// also given -- worse than before this card, which left the context absent
// (failing closed) rather than falsely green.
func publishCheckStatus(ctx context.Context, client *githubclient.Client, stdout, stderr io.Writer, owner, repoName, commitID string, issues []types.ReviewIssue, prNumber int, qualityRequiredButIncomplete, providerFailed bool) int {
	grave := countAtOrAbove(issues, rankError)
	status := githubclient.CommitStatus{Context: checkContext}
	switch {
	case providerFailed:
		status.State = "failure"
		status.Description = fmt.Sprintf("revisão não executada no pull request #%d: falha do provedor (provider_failure)", prNumber)
	case qualityRequiredButIncomplete:
		status.State = "failure"
		status.Description = fmt.Sprintf("revisão por modelo inconclusiva no pull request #%d", prNumber)
	case grave > 0:
		status.State = "failure"
		status.Description = fmt.Sprintf("%d achado(s) grave(s) no pull request #%d", grave, prNumber)
	default:
		status.State = "success"
		status.Description = fmt.Sprintf("nenhum achado grave no pull request #%d", prNumber)
	}

	if err := client.SetStatus(ctx, owner, repoName, commitID, status); err != nil {
		fmt.Fprintf(stderr, "aurumcode review: publishing check status: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "check %q publicado no commit %s: %s (%s)\n", checkContext, commitID, status.State, status.Description)

	// AUR-537 B1: providerFailed's own contribution to the exit code is
	// deliberately 0 here, even though the STATE published above is always
	// "failure" -- the overall process exit for a provider outage is
	// decided once, correctly, by gateResult.Fail/Breach at the bottom of
	// runPRReview (evaluateGate already sets Fail only for
	// gate.inconclusive: block, never for warn; providerFailed can only be
	// true at all once a gate IS declared, so that cascade always runs).
	// Letting this function ALSO claim exitQualityNotReviewed here would
	// double-count the same decision and wrongly force exit 1 under warn,
	// defeating AC-002 -- the published status and the exit code are two
	// different questions, and only the status must ignore warn/block.
	if qualityRequiredButIncomplete {
		return exitQualityNotReviewed
	}
	if grave > 0 {
		return exitFindings
	}
	return 0
}

// resolvePullRequestHeadSHA reads the pull request head commit through the
// same read-only client the review already uses (GetPullRequestMetadata,
// AUR-499). It exists because GITHUB_SHA is reserved by GitHub Actions on a
// pull_request event to the synthetic merge commit, so the status a branch
// protection rule must see on the head would otherwise never be published
// where the rule looks (AUR-504, docs/specs/AUR-504.md).
//
// It fails closed and names the reason: an API failure, or a successful
// response that carries no head SHA, is an error the caller must handle by
// refusing to publish -- never by silently using a different commit.
func resolvePullRequestHeadSHA(ctx context.Context, client *githubclient.Client, owner, repo string, number int) (string, error) {
	meta, err := client.GetPullRequestMetadata(ctx, owner, repo, number)
	if err != nil {
		return "", fmt.Errorf("determining the pull request head commit: %w", err)
	}
	sha := strings.TrimSpace(meta.HeadSHA)
	if sha == "" {
		return "", errors.New("determining the pull request head commit: the API response carried no head SHA")
	}
	return sha, nil
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

// loadPullRequestContext fetches the curated prompt, skills and documentation
// declared by review.context. The base ref is selected by the caller so a PR
// cannot add or alter review guidance for its own evaluation. The historical
// default prompt remains optional; explicitly listed files are required and
// fail with their path when missing.
func loadPullRequestContext(ctx context.Context, client *githubclient.Client, owner, repo string, cfg *config.Config, ref string) ([]config.ContextProvider, error) {
	if cfg == nil {
		cfg = &config.Config{}
	}
	files := cfg.Review.ContextFiles()
	providers := make([]config.ContextProvider, 0, len(files))
	for _, file := range files {
		data, found, err := client.GetRepositoryFile(ctx, owner, repo, file.Path, ref)
		if err != nil {
			return nil, fmt.Errorf("reading review %s %q: %w", file.Kind, file.Path, err)
		}
		if !found {
			if file.Optional {
				continue
			}
			return nil, fmt.Errorf("review %s %q was configured but not found", file.Kind, file.Path)
		}
		providers = append(providers, config.NewTextContextProvider(file, string(data)))
	}
	return providers, nil
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

// isInlineEligible requires a changed line in the declared coordinate space.
// Additions use RIGHT and deletions LEFT; context cannot anchor a finding.
func isInlineEligible(diff *types.Diff, issue types.ReviewIssue) bool {
	return review.IsChangedLine(diff, issue)
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

// suppressOperationalStrengths prevents a model from presenting repository
// wiring as a code-quality achievement. A PR that changes only configuration,
// workflows, documentation or comments may still have real findings, but its
// published review should not praise the integration setup as product code.
func suppressOperationalStrengths(diff *types.Diff, result *types.ReviewResult) {
	if result != nil && !prompt.HasSubstantiveCodeChange(diff) {
		result.Strengths = nil
	}
}

// filterLimitationsAgainstDiff removes a model limitation that contradicts
// the review input itself. A file printed in the Code changes block was
// available to the model, so publishing that file as "unavailable" makes an
// otherwise valid review factually misleading. Limitations about evidence
// outside the diff, such as missing CI logs, remain untouched.
func filterLimitationsAgainstDiff(diff *types.Diff, limitations []string) []string {
	if len(limitations) == 0 {
		return limitations
	}
	paths := make([]string, 0, len(diff.Files))
	for _, file := range diff.Files {
		if path := strings.TrimSpace(file.Path); path != "" {
			paths = append(paths, path)
		}
	}
	if len(paths) == 0 {
		return limitations
	}

	filtered := make([]string, 0, len(limitations))
	for _, limitation := range limitations {
		lower := strings.ToLower(limitation)
		claimsUnavailable := strings.Contains(lower, "not available") ||
			strings.Contains(lower, "unavailable") ||
			strings.Contains(lower, "não disponível") ||
			strings.Contains(lower, "nao disponivel") ||
			strings.Contains(lower, "não estavam disponíveis") ||
			strings.Contains(lower, "nao estavam disponiveis")
		if claimsUnavailable && limitationMentionsChangedPath(lower, paths) {
			continue
		}
		filtered = append(filtered, limitation)
	}
	return filtered
}

func limitationMentionsChangedPath(lowerLimitation string, paths []string) bool {
	for _, path := range paths {
		if strings.Contains(lowerLimitation, strings.ToLower(path)) {
			return true
		}
	}
	return false
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

// readCIContext reads the optional, workflow-produced check summary. It is
// intentionally not a required user setting: a review still runs when CI has
// not reported anything yet. No client-side truncation is applied; the
// configured model/provider owns its context window and an explicit prompt
// budget, when used, remains the only review-content budget.
func readCIContext() string {
	path := strings.TrimSpace(os.Getenv("AURUMCODE_CI_CONTEXT_FILE"))
	if path == "" {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return ""
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
	if issue.Side == "LEFT" {
		label := "Removed line (base)"
		if strings.HasPrefix(language, "pt") {
			label = "Linha removida (base)"
		}
		fmt.Fprintf(&b, "\n\n%s: `%s:%d`", label, issue.File, issue.Line)
	}
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

// nativeSuggestionComment converts an implementation-ready model suggestion
// into GitHub's inline suggestion format. The range has already passed the
// changed-line filter, but this helper repeats the location check at the
// publication boundary so a malformed response can never create an
// actionable suggestion against an unchanged line.
func nativeSuggestionComment(diff *types.Diff, suggestion types.ReviewSuggestion, language string) (githubclient.ReviewLineComment, bool) {
	if strings.TrimSpace(suggestion.ProposedCode) == "" || !isNativeSuggestion(diff, suggestion) {
		return githubclient.ReviewLineComment{}, false
	}
	start, end := suggestionRange(suggestion)
	comment := githubclient.ReviewLineComment{
		Body: formatNativeSuggestionBody(suggestion, language),
		Path: suggestion.File,
		Line: end,
		Side: "RIGHT",
	}
	if start != end {
		comment.StartLine = start
		comment.StartSide = "RIGHT"
	}
	return comment, true
}

func isNativeSuggestion(diff *types.Diff, suggestion types.ReviewSuggestion) bool {
	if strings.TrimSpace(suggestion.ProposedCode) == "" || strings.TrimSpace(suggestion.File) == "" {
		return false
	}
	start, end := suggestionRange(suggestion)
	if start <= 0 || end < start || end-start > 1000 {
		return false
	}
	for line := start; ; line++ {
		if !isInlineEligible(diff, types.ReviewIssue{File: suggestion.File, Line: line}) {
			return false
		}
		if line == end {
			return true
		}
	}
}

func formatNativeSuggestionBody(suggestion types.ReviewSuggestion, language string) string {
	copy := reviewCopyFor(language)
	var b strings.Builder
	if title := strings.TrimSpace(suggestion.Title); title != "" {
		fmt.Fprintf(&b, "**%s**", title)
	}
	if description := strings.TrimSpace(suggestion.Description); description != "" {
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(description)
	}
	writeReviewField(&b, copy.rationale, suggestion.Rationale)
	writeReviewField(&b, copy.verify, suggestion.Verification)
	if b.Len() > 0 {
		b.WriteString("\n\n")
	}
	b.WriteString("```suggestion\n")
	b.WriteString(strings.Trim(suggestion.ProposedCode, "\n"))
	b.WriteString("\n```")
	return b.String()
}

// formatFormalReviewSummary keeps an actionable native suggestion from being
// duplicated as a second code block in the review summary. The title,
// description and location remain in the summary while the replacement itself
// is attached to the exact changed lines by nativeSuggestionComment.
func formatFormalReviewSummary(result *types.ReviewResult, diff *types.Diff, language string) string {
	copy := *result
	copy.Suggestions = append([]types.ReviewSuggestion(nil), result.Suggestions...)
	for i := range copy.Suggestions {
		if isNativeSuggestion(diff, copy.Suggestions[i]) {
			copy.Suggestions[i].ProposedCode = ""
		}
	}
	return formatReviewSummaryForLanguageAndDiff(&copy, diff, language)
}

// A published review is one code-review document. The deterministic TL;DR
// and Mermaid diagram remain available in local CLI output, but prepending
// them here duplicated the verdict and mislabeled files without findings as
// "none". A diagram inferred from imports is not evidence about runtime flow.
func formatPublishedReviewBody(result *types.ReviewResult, diff *types.Diff, language string, formalWithInline bool, changelogText string) string {
	body := formatReviewSummaryForLanguageAndDiff(result, diff, language)
	if formalWithInline {
		body = formatFormalReviewSummary(result, diff, language)
	}
	return appendChangelogSection(body, changelogText)
}

func formatReviewSummary(result *types.ReviewResult) string {
	return formatReviewSummaryForLanguage(result, "en-US")
}

func formatReviewSummaryForLanguage(result *types.ReviewResult, language string) string {
	return formatReviewSummaryForLanguageAndDiff(result, nil, language)
}

func formatReviewSummaryForLanguageAndDiff(result *types.ReviewResult, diff *types.Diff, language string) string {
	copy := reviewCopyFor(language)
	var b strings.Builder
	b.WriteString("<!-- aurumcode-review -->\n")
	fmt.Fprintf(&b, "## AurumCode %s\n\n", copy.title)
	fmt.Fprintf(&b, "**%s:** %s\n\n", copy.verdict, reviewVerdictForLanguage(result, copy))
	if diff != nil && prompt.HasSubstantiveCodeChange(diff) && strings.TrimSpace(result.Summary) != "" {
		fmt.Fprintf(&b, "### %s\n\n%s\n\n", copy.summary, strings.TrimSpace(result.Summary))
	} else if note := summaryWithheldNotice(result, copy); note != "" {
		fmt.Fprintf(&b, "%s\n\n", note)
	}
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
			if issue.Side == "LEFT" {
				fmt.Fprintln(&b, "  - `LEFT`: base / −")
			}
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
	if result.Metadata["quality_degraded"] == "true" {
		return copy.inconclusive
	}
	// AUR-519 (B-V): this function otherwise re-derives the verdict from
	// Issues/Suggestions alone, never from the model's own Verdict field
	// -- intentionally: a model is never trusted to self-report "comment"
	// or "changes_requested" into an outcome these structural checks did
	// not already reach on their own. The engine's OWN withholding
	// (gate.Fail/Inconclusive, cmd/aurumcode's gate section) is a
	// different, trusted signal and gets its own reserved,
	// forge-safe key instead of overloading result.Verdict's value --
	// see prompt.PolicyGateWithheldKey's own doc for why a model cannot
	// set or erase it. A prior version of this check matched
	// result.Verdict == "comment" directly, which regressed a model that
	// legitimately self-reports "comment" with no gate active at all: it
	// started publishing COMMENT instead of this function's pre-AUR-519
	// APPROVE default, an outcome this function never produced before
	// and the model's self-report alone must not be able to cause.
	if result.Metadata[prompt.PolicyGateWithheldKey] == "true" {
		return copy.comment
	}
	for _, suggestion := range result.Suggestions {
		if strings.TrimSpace(suggestion.Title) != "" || strings.TrimSpace(suggestion.Description) != "" {
			return copy.comment
		}
	}
	return copy.approve
}

// formalReviewEvent maps AurumCode's review result to GitHub's formal review
// events. Blocking findings request changes; non-blocking observations stay
// a neutral review comment; a clean review can approve the pull request.
func formalReviewEvent(result *types.ReviewResult) string {
	for _, issue := range result.Issues {
		switch strings.ToLower(strings.TrimSpace(issue.Severity)) {
		case "error", "warning":
			return "REQUEST_CHANGES"
		}
	}
	if len(result.Issues) > 0 {
		return "COMMENT"
	}
	if result.Metadata["quality_degraded"] == "true" {
		return "COMMENT"
	}
	// AUR-519 (B-V): same engine-owned marker as reviewVerdictForLanguage
	// above, never the model's own Verdict text -- see that function's
	// comment and prompt.PolicyGateWithheldKey's own doc.
	if result.Metadata[prompt.PolicyGateWithheldKey] == "true" {
		return "COMMENT"
	}
	for _, suggestion := range result.Suggestions {
		if strings.TrimSpace(suggestion.Title) != "" || strings.TrimSpace(suggestion.Description) != "" {
			return "COMMENT"
		}
	}
	return "APPROVE"
}

// summaryWithheldNotice renders AUR-517/N3a's visible notice when
// internal/review withheld result.Summary (withholdSummaryWhenFiltered):
// result.Metadata["summary_discarded_findings"] names how many of the
// model's proposed findings the scope/evidence or rule gate discarded, and
// an empty result.Summary with a nonzero count is exactly that withholding
// (never a model that happened to return no summary at all with nothing
// discarded). Returns "" in every other case, so a clean review's body is
// unchanged.
func summaryWithheldNotice(result *types.ReviewResult, copy reviewCopy) string {
	if result == nil || strings.TrimSpace(result.Summary) != "" {
		return ""
	}
	discarded := atoiOrZero(result.Metadata["summary_discarded_findings"])
	if discarded <= 0 {
		return ""
	}
	return fmt.Sprintf(copy.summaryWithheld, discarded)
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
	if result.Metadata["quality_degraded"] == "true" {
		return copy.qualityIncomplete
	}
	for _, suggestion := range result.Suggestions {
		if strings.TrimSpace(suggestion.Title) != "" || strings.TrimSpace(suggestion.Description) != "" {
			return copy.optionalSuggestions
		}
	}
	return copy.noBlockingFindings
}

type reviewCopy struct {
	title, verdict, summary, strengths, findings, suggestions, ciStatus, tests, limits string
	impact, evidence, suggestedFix, verify, rationale, proposedImplementation          string
	cause, fix, nextVerification                                                       string
	changesRequested, comment, approve, inconclusive                                   string
	blockingFindings, nonBlockingFindings, optionalSuggestions, noBlockingFindings     string
	qualityIncomplete                                                                  string
	suggestionApplicable, suggestionNotApplicable                                      string
	// coverageHeading and the coverage* templates render AUR-476's
	// deterministic "this review was partial" notice. Each reason a file was
	// not covered gets its own sentence; coverageSummary names the count and
	// the denominator so the reader sees how much of the diff actually ran.
	coverageHeading, coverageSummary, coveragePartial, coverageBudget, coverageIgnored, coverageFiltered, coverageNoStructure string
	// summaryWithheld is AUR-517's one-line notice (%d is the discard
	// count) printed in place of the "### Summary" block whenever
	// internal/review withheld the model's free-text summary because the
	// scope/evidence or rule gate discarded one of its proposed findings
	// (AC-001/N3a): the omission must be visible, never silent.
	summaryWithheld string
}

func reviewCopyFor(language string) reviewCopy {
	if strings.EqualFold(strings.TrimSpace(language), "pt-BR") || strings.EqualFold(strings.TrimSpace(language), "pt") {
		return reviewCopy{
			title: "revisão de código", verdict: "Veredito", summary: "Resumo", strengths: "Pontos fortes", findings: "Achados", suggestions: "Sugestões", ciStatus: "Status do CI", tests: "Testes", limits: "Limitações da revisão",
			impact: "Impacto", evidence: "Evidência", suggestedFix: "Correção sugerida", verify: "Verificação", rationale: "Motivação", proposedImplementation: "Implementação sugerida", cause: "Causa", fix: "Correção", nextVerification: "Próxima verificação",
			changesRequested: "Alterações solicitadas", comment: "Comentário", approve: "Aprovado", inconclusive: "Inconclusivo",
			blockingFindings:        "A revisão encontrou %d achado(s) bloqueante(s) que devem ser tratados antes do merge.",
			nonBlockingFindings:     "A revisão encontrou observações, mas nenhum achado bloqueante permanece na mudança revisada.",
			optionalSuggestions:     "Nenhum achado bloqueante foi identificado; as sugestões abaixo são melhorias opcionais.",
			noBlockingFindings:      "Nenhum achado bloqueante foi identificado na mudança revisada.",
			qualityIncomplete:       "A revisão por modelo não foi concluída. Apenas as verificações determinísticas produziram resultado; este parecer não aprova a mudança.",
			suggestionApplicable:    "Substituição aplicável em `%s`.",
			suggestionNotApplicable: "Sugestão sem localização elegível no diff adicionado; exibida como orientação, sem substituição aplicável.",
			coverageHeading:         "Cobertura da revisão",
			coverageSummary:         "%d de %d arquivo(s) do diff foram cobertos; %d não foram revisados por completo.",
			coveragePartial:         "%d arquivo(s) tiveram parte dos trechos omitida pelo limite de tokens; os achados podem não cobrir os trechos omitidos.",
			coverageBudget:          "%d arquivo(s) ficaram fora da revisão pelo limite de tokens.",
			coverageIgnored:         "%d arquivo(s) foram ocultados da revisão pela configuração `ignore` do repositório; a ausência deles no contexto NÃO prova que não existam no diff.",
			coverageFiltered:        "%d arquivo(s) foram filtrados antes da revisão (binário, gerado ou grande demais); arquivo não revisado nunca conta como aprovado.",
			coverageNoStructure:     "%d arquivo(s) não têm gramática no runtime: o contexto estrutural (símbolos e imports) não foi produzido e o modelo leu apenas o texto.",
			summaryWithheld:         "Resumo do modelo omitido: %d achado(s) propostos foram descartados pelos filtros de escopo/regra.",
		}
	}
	return reviewCopy{
		title: "code review", verdict: "Verdict", summary: "Summary", strengths: "Strengths", findings: "Findings", suggestions: "Suggestions", ciStatus: "CI status", tests: "Tests", limits: "Review limits",
		impact: "Impact", evidence: "Evidence", suggestedFix: "Suggested fix", verify: "Verify", rationale: "Rationale", proposedImplementation: "Proposed implementation", cause: "Cause", fix: "Fix", nextVerification: "Next verification",
		changesRequested: "Changes requested", comment: "Comment", approve: "Approve", inconclusive: "Inconclusive",
		blockingFindings:        "The review found %d blocking finding(s) that should be addressed before merge.",
		nonBlockingFindings:     "The review found observations, but no blocking finding remains in the reviewed change.",
		optionalSuggestions:     "No blocking finding was identified; the suggestions below are optional improvements.",
		noBlockingFindings:      "No blocking finding was identified in the reviewed change.",
		qualityIncomplete:       "The model review did not complete. Only deterministic checks produced results; this review does not approve the change.",
		suggestionApplicable:    "Applicable replacement at `%s`.",
		suggestionNotApplicable: "Suggestion has no eligible location in the added diff; shown as guidance with no applicable replacement.",
		coverageHeading:         "Review coverage",
		coverageSummary:         "%d of %d file(s) in the diff were covered; %d were not fully reviewed.",
		coveragePartial:         "%d file(s) had some hunks omitted by the token budget; findings may miss those hunks.",
		coverageBudget:          "%d file(s) were left out of the review by the token budget.",
		coverageIgnored:         "%d file(s) were hidden from the review by the repository `ignore` config; their absence from the reviewed context is NOT proof they are absent from the diff.",
		coverageFiltered:        "%d file(s) were filtered before the review (binary, generated or too large); a file that was not reviewed never counts as approved.",
		coverageNoStructure:     "%d file(s) have no grammar in the runtime: structural context (symbols and imports) was not produced and the model read the text only.",
		summaryWithheld:         "Model summary omitted: %d proposed finding(s) were discarded by the scope/rule filters.",
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

// codebaseContextPack resolves bounded codebase dependency context from the
// current checkout for the changed paths. It is an enhancement, never a gate:
// the caller degrades to empty context on any error.
func codebaseContextPack(changed []string) (*codebasectx.Pack, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	return codebasectx.NewResolver().Resolve(cwd, changed)
}

// notesFromIssues turns this round's findings into review-memory notes,
// merged with the existing ones and deduplicated by id. It is bounded so a
// pathological diff cannot grow the memory file without limit.
func notesFromIssues(existing []memory.Note, issues []types.ReviewIssue) []memory.Note {
	notes := append([]memory.Note(nil), existing...)
	seen := make(map[string]bool, len(notes))
	for _, n := range notes {
		seen[n.ID] = true
	}
	for _, issue := range issues {
		id := fmt.Sprintf("%s:%s:%d", issue.RuleID, issue.File, issue.Line)
		if seen[id] {
			continue
		}
		seen[id] = true
		notes = append(notes, memory.Note{
			ID:          id,
			RuleID:      issue.RuleID,
			PathPattern: issue.File,
			Action:      "note",
			Body:        issue.Message,
			At:          time.Now(),
		})
		if len(notes) >= 500 {
			break
		}
	}
	return notes
}

// generateReview calls the model. An unparseable answer degrades to the
// deterministic half (AUR-505, never a crash); a provider/transport failure
// returns immediately unless a gate is declared, in which case it falls
// through as the gate's own inconclusive reason (AUR-537, AC-003).
func (p *prReview) generateReview() (int, bool) {
	// stderr, limiteUSD and gateDeclared stay named as in the original
	// function: tests/acceptance/AUR-537.sh anchors its mutations on them.
	stderr, limiteUSD := p.stderr, p.limiteUSD
	gateDeclared := p.cfg.Gate.Declared()
	p.gateDeclared = gateDeclared
	result, err := p.reviewer.GenerateReviewWithContext(p.ctx, p.diff, review.ReviewContext{
		CI:              readCIContext(),
		Language:        p.reviewLanguage,
		History:         p.history,
		CodebaseContext: p.codebaseText,
		MemoryNotes:     p.memoryNotesText,
	})
	p.result = result
	if err == nil {
		return 0, false
	}
	var parseErr *prompt.ParseError
	switch {
	case errors.Is(err, llm.ErrBudgetExceeded):
		// --limite: nothing was spent; checked first, like --base.
		rc := reportBudgetExceeded(stderr, limiteUSD, err)
		if !gateDeclared {
			return rc, true
		}
		p.providerFailed = true
	case errors.As(err, &parseErr):
		p.degradeUnparseable(parseErr)
	case errors.Is(err, llm.ErrAllProvidersFailed):
		var rc int
		if p.opts.modelo != "" {
			rc = reportModelUnavailable(stderr, p.opts.modelo, err)
		} else {
			fmt.Fprintf(stderr, "aurumcode review: %v\n", err)
			rc = 1
		}
		if !gateDeclared {
			return rc, true
		}
		p.providerFailed = true
	default:
		fmt.Fprintf(stderr, "aurumcode review: %v\n", err)
		return 1, true
	}
	if p.providerFailed {
		// Reuses the "quality_degraded" key on purpose: the shared verdict
		// rendering already turns it into "never approve".
		p.qualityDegraded = true
		p.result = &types.ReviewResult{Metadata: map[string]string{"quality_degraded": "true"}}
		p.result.Limitations = append(p.result.Limitations, providerFailureNotice(p.reviewLanguage))
	}
	return 0, false
}

// degradeUnparseable records an answer the parser could not validate as a
// declared limitation, keeping the diagnosis on stderr (AUR-505).
func (p *prReview) degradeUnparseable(parseErr *prompt.ParseError) {
	stderr := p.stderr
	fmt.Fprintf(stderr, "aurumcode review: could not understand the model's response (%s)\n", parseErr.Kind)
	fmt.Fprintf(stderr, "aurumcode review: response diagnostics: bytes=%d raw_json_valid=%t finish_reason=%q syntax_offset=%d\n", parseErr.InputBytes, parseErr.RawJSONValid, parseErr.FinishReason, parseErr.SyntaxOffset)
	if parseErr.TypeField != "" {
		fmt.Fprintf(stderr, "aurumcode review: response schema mismatch: field=%q expected=%q actual=%q\n", parseErr.TypeField, parseErr.ExpectedType, parseErr.ActualType)
	}
	if parseErr.ValidationCode != "" {
		fmt.Fprintf(stderr, "aurumcode review: response validation: code=%s\n", parseErr.ValidationCode)
	}
	fmt.Fprintln(stderr, "aurumcode review: degrading to deterministic analysis; the model review is inconclusive")
	p.qualityDegraded = true
	p.result = &types.ReviewResult{Metadata: map[string]string{"quality_degraded": "true"}}
	p.result.Limitations = append(p.result.Limitations, modelInvalidOutputNotice(p.reviewLanguage, string(parseErr.Kind)))
}
