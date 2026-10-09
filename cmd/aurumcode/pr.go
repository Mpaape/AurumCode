// The pull request review path: `aurumcode review --pr <n> --repo
// <owner>/<name>` reads the pull request from GitHub, runs the same review
// session as --base and publishes one formal review or separate comments,
// plus the commit status; a finding that cannot anchor to a changed line is
// kept in the review body, never dropped.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Mpaape/AurumCode/internal/changelog"
	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/git/githubclient"
	"github.com/Mpaape/AurumCode/internal/prompt"
	"github.com/Mpaape/AurumCode/internal/review/blocking"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// prReviewOptions carries the AUR-451 capabilities onto the PR path: the
// same four things the --base path already offers, reused here rather than
// reimplemented (see the package doc above). The zero value means none of
// the PR flags was given, so a caller that never sets a field keeps the
// historical --pr behavior. exigirQualidade is opt-in for direct CLI callers;
// the reusable workflow enables it by default.
type prReviewOptions struct {
	// prNumber and repo name the pull request; publicar, naLinha and check
	// are --publicar, --na-linha and --check as given.
	prNumber int
	repo     string
	publicar bool
	naLinha  bool
	check    bool

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
func publishCheckStatus(ctx context.Context, client *githubclient.Client, stdout, stderr io.Writer, owner, repoName, commitID string, issues []types.ReviewIssue, prNumber int, qualityRequiredButIncomplete, providerFailed bool, rule blocking.Rule) int {
	// With a declared gate, "grave" is exactly what the gate fails on (the
	// same single rule as the review body and the formal review), so this
	// status never contradicts aurumcode/policy-gate. Without one, every
	// error-severity finding counts, as before.
	grave := countAtOrAbove(issues, rankError)
	if rule.Gated() {
		grave = rule.Count(issues)
	}
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
	p.model = modelParseFailed
	p.result = &types.ReviewResult{Metadata: map[string]string{"quality_degraded": "true"}}
	p.result.Limitations = append(p.result.Limitations, modelInvalidOutputNotice(p.reviewLanguage, string(parseErr.Kind)))
}

// newGitHubClient builds the restored AUR-437 client.
// config.GitHubAPIURLEnv (AURUMCODE_GITHUB_API_URL) overrides the API base, validated by config.GitHubAPIURL (shared with analysis_data), so this card's own tests
// can point it at a loopback httptest server (the sealed profile denies
// real network access); production use leaves it unset and gets
// githubclient.DefaultBaseURL. GITHUB_TOKEN follows the same convention
// GitHub Actions already exposes to a step (see docs/specs/AUR-440.md); an
// empty token still builds a working client -- reading a public repository
// needs no auth. Direct CLI publishing retains the repository-role preflight;
// the reusable workflow sets AURUMCODE_PR_PERMISSION_MODE=endpoint so GitHub
// itself enforces pull-requests:write and statuses:write on the actual POST.
func newGitHubClient() (*githubclient.Client, error) {
	token := os.Getenv("GITHUB_TOKEN")
	if strings.TrimSpace(os.Getenv(config.GitHubAPIURLEnv)) == "" {
		return githubclient.NewClient(token), nil
	}
	base, err := config.GitHubAPIURL(os.Getenv)
	if err != nil {
		return nil, err
	}
	return githubclient.NewClientWithBaseURL(token, base), nil
}

// pullRequestChangelogSource reads the PR title/body and the PR's commit
// messages, then folds the title/body into a synthetic first commit so the
// engine sees the whole reviewed narrative. It also returns the author the
// same metadata carries (AUR-610). Any missing source is an error and the
// caller declares a limitation; nothing here is executed.
func pullRequestChangelogSource(ctx context.Context, client *githubclient.Client, owner, repo string, number int) ([]changelog.Commit, changelog.Author, error) {
	ctx, cancel := context.WithTimeout(ctx, config.ProviderTimeout)
	defer cancel()
	meta, err := client.GetPullRequestMetadata(ctx, owner, repo, number)
	if err != nil {
		return nil, changelog.Author{}, err
	}
	author := changelog.Author{Login: meta.AuthorLogin, Type: meta.AuthorType}
	commits, err := client.GetPullRequestCommits(ctx, owner, repo, number)
	if err != nil {
		return nil, author, err
	}
	if len(commits) == 0 {
		return nil, author, errors.New("pull request has no commit messages")
	}
	out := make([]changelog.Commit, 0, len(commits)+1)
	if strings.TrimSpace(meta.Title) != "" || strings.TrimSpace(meta.Body) != "" {
		out = append(out, changelog.Commit{Subject: meta.Title, Body: meta.Body})
	}
	for _, c := range commits {
		subject, body := splitCommitMessage(c.Message)
		out = append(out, changelog.Commit{Subject: subject, Body: body, Hash: c.SHA})
	}
	return out, author, nil
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
