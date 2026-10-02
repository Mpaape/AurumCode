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
	"github.com/Mpaape/AurumCode/internal/llm/cost"
	"github.com/Mpaape/AurumCode/internal/memory"
	"github.com/Mpaape/AurumCode/internal/prompt"
	"github.com/Mpaape/AurumCode/internal/render"
	"github.com/Mpaape/AurumCode/internal/review"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
	"github.com/Mpaape/AurumCode/internal/testgen"
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

// runPRReview is reached only when --pr was explicitly given (see the
// fs.Visit dispatch in runReview); every other flag's published behavior
// is therefore untouched by this function's existence.
//
// --repo and --publicar are always required here. Publication mode and inline
// comments are optional: the repository config or the command line selects
// them, and the zero-value mode keeps the historical separate-comment path.
func runPRReview(stdout, stderr io.Writer, prNumber int, repoFlag string, publicar, naLinha, check bool, filter *redaction.Filter, opts prReviewOptions) int {
	if repoFlag == "" {
		fmt.Fprintln(stderr, "aurumcode review: --repo is required with --pr")
		return 2
	}
	// Keep the legacy command spelling stable. A caller that wants the new
	// formal mode explicitly selects it with --modo-publicacao review; the
	// historical comments command still requires --na-linha unless --check
	// is the requested publication. This avoids silently changing existing
	// scripts while making the new mode free of the old ceremony.
	if !naLinha && !check && !opts.publicationSet {
		fmt.Fprintln(stderr, "aurumcode review: --na-linha is required with --pr in the legacy comments mode; use --modo-publicacao review for a formal review")
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
	if opts.publicationSet {
		if _, err := config.NormalizeReviewPublication(opts.publication); err != nil {
			fmt.Fprintf(stderr, "aurumcode review: --modo-publicacao: %v\n", err)
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

	// AUR-518: fold the central policy (when declared) over reviewConfig
	// right after it loads, before any model call -- same placement and
	// reasoning as the --base path (runReview, cmd/aurumcode/main.go). A
	// missing or invalid policy fails closed here (AC-005); no policy
	// declared leaves reviewConfig and reviewLanguage untouched (AC-006).
	var centralCfg *config.Config
	var policyWarnings []config.ProviderWarning
	if opts.policyDir != "" {
		// A policy must come from outside the tree being reviewed. In this
		// path the reviewed tree is the process's working directory (the
		// container's -w /github/workspace, mounted read-only from the pull
		// request's own head) -- never the PR under review itself.
		cwd, cwdErr := os.Getwd()
		if cwdErr != nil {
			fmt.Fprintf(stderr, "aurumcode review: %v\n", cwdErr)
			return 1
		}
		if err := config.ValidatePolicyOutsideReviewedTree(opts.policyDir, cwd); err != nil {
			fmt.Fprintf(stderr, "aurumcode review: %v\n", err)
			return 1
		}
		centralCfg, err = config.LoadCentralPolicy(opts.policyDir)
		if err != nil {
			fmt.Fprintf(stderr, "aurumcode review: %v\n", err)
			return 1
		}
	}
	reviewConfig, policyWarnings = config.ApplyCentralPolicy(reviewConfig, centralCfg)
	if filter != nil {
		for i := range policyWarnings {
			policyWarnings[i].Provider = filter.Redact(policyWarnings[i].Provider)
			policyWarnings[i].Reason = filter.Redact(policyWarnings[i].Reason)
		}
	}
	for _, warning := range policyWarnings {
		fmt.Fprintf(stderr, "aurumcode review: %s: %s\n", warning.Provider, warning.Reason)
	}
	if reviewLanguage, err = reviewConfig.ReviewLanguage(); err != nil {
		fmt.Fprintf(stderr, "aurumcode review: %v\n", err)
		return 1
	}

	publication, err := reviewConfig.ReviewPublication()
	if err != nil {
		fmt.Fprintf(stderr, "aurumcode review: loading repository review publication: %v\n", err)
		return 1
	}
	if opts.publicationSet {
		publication, err = config.NormalizeReviewPublication(opts.publication)
		if err != nil {
			fmt.Fprintf(stderr, "aurumcode review: --modo-publicacao: %v\n", err)
			return 2
		}
	}
	inlineComments := reviewConfig.Review.InlineComments || naLinha
	// AUR-476: capture the paths the repository config explicitly hides
	// before the filter drops them, so the published coverage notice can
	// name the ignored count and its cause. The raw count is the coverage
	// denominator when config hid files the prompt builder never measured.
	ignoredPaths := ignoredDiffPaths(diff, reviewConfig)
	rawDiffFileCount := len(diff.Files)
	diff = config.FilterIgnoredPaths(diff, reviewConfig)

	// AUR-499: the release section is opt-in (review.changelog, off by default;
	// --changelog forces it on). --pr reads the PR title/body and the PR's
	// commit messages through the read-only client, then renders them with the
	// same shared AUR-498 pass the --base path uses. Commit and PR text is
	// untrusted: buildChangelogSection redacts it through the same sink
	// pr_history.go uses, and it never authorizes a publication on its own.
	// A missing source omits the section with a declared limitation and never
	// crashes the review.
	changelogOn := opts.changelog
	if !changelogOn {
		on, cfgErr := reviewConfig.ReviewChangelog()
		if cfgErr != nil {
			fmt.Fprintf(stderr, "aurumcode review: %v\n", cfgErr)
			return 1
		}
		changelogOn = on
	}
	changelogText, changelogLimitation := "", ""
	if changelogOn {
		if commits, sourceErr := pullRequestChangelogSource(ctx, client, owner, repoName, prNumber); sourceErr != nil {
			changelogLimitation = changelogUnavailableNotice(reviewLanguage)
			fmt.Fprintf(stderr, "aurumcode review: %s\n", changelogLimitation)
		} else if section, limit := buildChangelogSection(reviewConfig.Review.Version, commits, filter); limit != "" {
			changelogLimitation = limit
			fmt.Fprintf(stderr, "aurumcode review: %s\n", limit)
		} else {
			changelogText = render.ChangelogSection(section.Version, section.Bump, section.Entry, reviewLanguage)
			if werr := writeChangelogOutput(os.Getenv("AURUMCODE_OUTPUT_FILE"), section); werr != nil {
				fmt.Fprintf(stderr, "aurumcode review: writing changelog output: %v\n", werr)
			}
		}
	}

	// Codebase context (zero-config, bounded heuristic): resolved from the
	// checkout so the model sees what else the change can affect -- the same
	// "full repo" advantage Greptile/CodeRabbit offer, without a graph
	// database. It is an enhancement, never a gate: any failure degrades to
	// empty context and the review continues on the diff alone.
	// AUR-490: the shared codebase-context pass (resolveCodebaseContext,
	// cmd/aurumcode/passes.go), identical to the one the --base path now runs.
	// AUR-515: on --pr the checkout at cwd is not necessarily the pull
	// request's own repository or head commit (see cmd/aurumcode/aur515.go);
	// resolveCodebaseContext only ever runs once that is verified, and a
	// mismatch is recorded as a limitation below instead of silently
	// sending an unrelated checkout's files to the provider.
	// AUR-536: a verified repository and HEAD (above) still say nothing
	// about the working tree itself -- an uncommitted edit, an untracked
	// file, or a nested clone's files all sit on disk and would otherwise
	// reach resolveCodebaseContext unfiltered. verifiedCleanCheckoutReason
	// (aur536.go) proves every file under the checkout matches committed
	// content before the pass is ever allowed to run, and names exactly
	// which files it proved -- resolveVerifiedCodebaseContext then reads
	// only those, never a second, independent walk of its own.
	var codebaseContextText, codebaseContextLimitation string
	var verifiedDir string
	var verifiedFiles []string
	mismatch := codebaseContextMismatch(ctx, client, owner, repoName, prNumber)
	if mismatch == "" {
		if dir, wdErr := os.Getwd(); wdErr == nil {
			if reason, files := verifiedCleanCheckoutReason(dir); reason != "" {
				mismatch = reason
			} else {
				verifiedDir, verifiedFiles = dir, files
			}
		} else {
			mismatch = codebaseContextReasonUnverifiable
		}
	}
	if mismatch == "" {
		codebaseContextText = resolveVerifiedCodebaseContext(diff, verifiedDir, verifiedFiles)
	} else {
		codebaseContextLimitation = codebaseContextOmittedNotice(reviewLanguage, mismatch)
	}

	// Review memory (opt-in, default off): prior findings and preferences
	// loaded as untrusted observations and saved back after publication.
	// Memory is observation, never instruction: it cannot change a rule, a
	// severity, redaction, cost, or the verdict.
	//
	// AUR-489: dir is ALWAYS derived from this repository (memoryDirFor),
	// never "" -- an empty dir collapses onto memory.go's single
	// process-wide fallback file, so a note saved while reviewing one
	// repository is loaded, as an "observation", into every other
	// repository's prompt. See memorydir.go.
	// AUR-490: the shared memory pass's open half (openReviewMemory,
	// cmd/aurumcode/passes.go), identical to the one the --base path now runs.
	memoryStore, memoryNotes, memoryNotesText := openReviewMemory(reviewConfig.Review.Memory, owner, repoName, stderr, filter)

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

	// Repository context is read from the trusted base ref when the workflow
	// supplies one. A pull request may still choose its language from its head
	// config, but it cannot change the prompt, skills or documentation used to
	// judge that same change. The contents are untrusted model background and
	// pass through the same redaction, warning and token-accounting wrapper as
	// the local review path.
	contextRef := os.Getenv("AURUMCODE_BASE_SHA")
	if strings.TrimSpace(contextRef) == "" {
		contextRef = os.Getenv("GITHUB_SHA")
	}
	contextProviders, contextErr := loadPullRequestContext(ctx, client, owner, repoName, reviewConfig, contextRef)
	if contextErr != nil {
		fmt.Fprintf(stderr, "aurumcode review: loading repository review context: %v\n", contextErr)
		return 1
	}
	// AUR-518: the policy's own context files come first, then the
	// repository's, as today (AC-004).
	if centralCfg != nil {
		contextProviders = append(config.ConfiguredProviders(opts.policyDir, centralCfg), contextProviders...)
	}
	wrapped, warnings, wrapErr := config.WrapProviderWithWarnings(ctx, provider, contextProviders, diffPaths(diff), filter)
	if wrapErr != nil {
		fmt.Fprintf(stderr, "aurumcode review: %v\n", wrapErr)
		return 1
	}
	for _, warning := range warnings {
		fmt.Fprintf(stderr, "aurumcode review: context provider %q unavailable: %s; continuing without that context\n", warning.Provider, warning.Reason)
	}
	provider = wrapped

	// AUR-519: the dynamic, skill-section rule set this run accepts
	// citations against -- policy skills (origin "policy") read locally
	// from the policy's own checkout, repo skills (origin "repo") read
	// through the same GitHub contents API at the same trusted base ref
	// the context block above already uses (never the pull request's own
	// head, which the author controls). Zero skills configured leaves
	// both exactly as before this card.
	policySkillRules := map[string]review.Rule{}
	if centralCfg != nil {
		policySkillRules = dynamicRulesFromLocalSkills(opts.policyDir, centralCfg.Review.Context.Skills, gateOriginPolicy)
	}
	repoSkillRules := dynamicRulesFromRemoteSkills(ctx, client, owner, repoName, reviewConfig.Review.Context.Skills, contextRef, gateOriginRepo)
	dynamicRules := mergeDynamicRules(policySkillRules, repoSkillRules)
	ruleCatalogIDs := mergedRuleCatalogIDs(prompt.DefaultRuleCatalog, dynamicRules)

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
	// AUR-519: teach the model the expanded catalog and accept its
	// citations against the same dynamic set computed above.
	reviewer.SetDynamicRules(dynamicRules)
	if err := reviewer.SetRuleCatalog(ruleCatalogIDs); err != nil {
		fmt.Fprintf(stderr, "aurumcode review: %v\n", err)
		return 2
	}
	history, historyErr := pullRequestHistoryContext(ctx, client, owner, repoName, prNumber,
		os.Getenv("GITHUB_SHA"), os.Getenv("AURUMCODE_BASE_SHA"), filter)
	if historyErr != nil {
		fmt.Fprintf(stderr, "aurumcode review: PR history unavailable: %s; reviewing the current diff without conversation history\n", filter.Redact(historyErr.Error()))
		history = historyUnavailableNotice(reviewLanguage)
	}

	// AUR-505: qualityDegraded records a review whose model half was
	// inconclusive (the provider answered, but the parser could not
	// validate the response). Unlike a provider that never answered, the
	// deterministic half -- static analysis and the security pass -- is
	// still valid and must be published. The model half is recorded as a
	// limitation, never as a finding, and the final exit code is decided
	// by the deterministic gate alone. See docs/specs/AUR-505.md.
	qualityDegraded := false
	result, err := reviewer.GenerateReviewWithContext(ctx, diff, review.ReviewContext{
		CI:              readCIContext(),
		Language:        reviewLanguage,
		History:         history,
		CodebaseContext: codebaseContextText,
		MemoryNotes:     memoryNotesText,
	})
	// AUR-537: a provider/transport failure here -- every configured
	// provider failed in the orchestrator (llm.ErrAllProvidersFailed) or
	// --limite refused the call before any provider was ever reached
	// (llm.ErrBudgetExceeded) -- used to return immediately, before the
	// policy gate (evaluateGate, below) was ever reached: no
	// aurumcode/policy-gate status, inconclusive: warn not honored, no
	// audit/SARIF written. gateDeclared (reviewConfig.Gate is already
	// policy-resolved by config.ApplyCentralPolicy above) decides whether
	// that stays true -- AC-003: no gate configured, byte-identical to
	// before this card, same diagnosis, same exit code -- or whether the
	// failure instead falls through as the gate's own inconclusive reason
	// gateReasonProviderFailure (gateInconclusiveReason, below), exactly
	// like a degraded model parse (qualityDegraded, right below) already
	// does. Selecting a provider in the first place (selectProvider/
	// selectProviderForModel, above) is a separate, earlier failure this
	// card does not touch: a model that was never even chosen has nothing
	// for the gate to grade either way.
	gateDeclared := reviewConfig.Gate.Declared()
	providerFailed := false
	if err != nil {
		// --limite: the tracker refused before the model was called, so
		// nothing was spent -- checked first, exactly like the --base
		// path's own ordering (runReview). AUR-537: still a pre-call
		// refusal, not a transport failure, but the gate treats both as
		// the same "no review happened" reason, and only once a gate is
		// declared -- otherwise this is exactly today's return.
		if errors.Is(err, llm.ErrBudgetExceeded) {
			rc := reportBudgetExceeded(stderr, limiteUSD, err)
			if !gateDeclared {
				return rc
			}
			providerFailed = true
		} else {
			var parseErr *prompt.ParseError
			if errors.As(err, &parseErr) {
				// AUR-505: an unparseable model answer degrades, it does not
				// crash. Before this card the command printed the diagnosis
				// and exited 1 with no deterministic finding published, so a
				// weak model failed a CI pull request even when the
				// deterministic analysis and security passes were clean. The
				// fix keeps the diagnosis, records the failure as a declared
				// limitation (never silent), publishes the deterministic
				// findings, and lets the deterministic gate decide the exit
				// code. NO finding is fabricated from the invalid response:
				// result starts empty (AC-003).
				fmt.Fprintf(stderr, "aurumcode review: could not understand the model's response (%s)\n", parseErr.Kind)
				fmt.Fprintf(stderr, "aurumcode review: response diagnostics: bytes=%d raw_json_valid=%t finish_reason=%q syntax_offset=%d\n", parseErr.InputBytes, parseErr.RawJSONValid, parseErr.FinishReason, parseErr.SyntaxOffset)
				if parseErr.TypeField != "" {
					fmt.Fprintf(stderr, "aurumcode review: response schema mismatch: field=%q expected=%q actual=%q\n", parseErr.TypeField, parseErr.ExpectedType, parseErr.ActualType)
				}
				if parseErr.ValidationCode != "" {
					fmt.Fprintf(stderr, "aurumcode review: response validation: code=%s\n", parseErr.ValidationCode)
				}
				fmt.Fprintln(stderr, "aurumcode review: degrading to deterministic analysis; the model review is inconclusive")
				qualityDegraded = true
				result = &types.ReviewResult{Metadata: map[string]string{"quality_degraded": "true"}}
				result.Limitations = append(result.Limitations, modelInvalidOutputNotice(reviewLanguage, string(parseErr.Kind)))
			} else if errors.Is(err, llm.ErrAllProvidersFailed) {
				// AUR-537: the diagnosis is unchanged either way -- naming
				// the model --modelo asked for is strictly more useful
				// than the bare transport error -- but whether this
				// returns now or falls through depends on gateDeclared,
				// mirroring --base's own runReview, where this exact
				// sentinel already becomes gateInconclusiveReason
				// "provider_failure" regardless of --modelo.
				var rc int
				if opts.modelo != "" {
					rc = reportModelUnavailable(stderr, opts.modelo, err)
				} else {
					fmt.Fprintf(stderr, "aurumcode review: %v\n", err)
					rc = 1
				}
				if !gateDeclared {
					return rc
				}
				providerFailed = true
			} else {
				fmt.Fprintf(stderr, "aurumcode review: %v\n", err)
				return 1
			}
		}
		if providerFailed {
			// Reuses qualityDegraded/the "quality_degraded" metadata key
			// on purpose: reviewVerdictForLanguage/formalReviewEvent/
			// reviewSummaryTextForLanguage (below) already turn that key
			// into "never approve, say the quality review did not
			// complete" without this file re-deriving any of that
			// rendering. providerFailureNotice (aur537.go) is the one
			// piece of text specific to THIS failure (never reached the
			// model at all) those shared paths do not already carry.
			qualityDegraded = true
			result = &types.ReviewResult{Metadata: map[string]string{"quality_degraded": "true"}}
			result.Limitations = append(result.Limitations, providerFailureNotice(reviewLanguage))
		}
	}
	if historyErr != nil {
		// The caller's coverage notice survives even if the model omits it.
		result.Limitations = append(result.Limitations, historyUnavailableNotice(reviewLanguage))
	}
	if warning := result.Metadata["discard_warning"]; warning != "" {
		fmt.Fprintf(stderr, "aurumcode review: %s\n", warning)
	}
	if warning := result.Metadata["scope_discard_warning"]; warning != "" {
		fmt.Fprintf(stderr, "aurumcode review: %s\n", warning)
	}
	if sections := result.Metadata["optional_sections_discarded"]; sections != "" {
		fmt.Fprintf(stderr, "aurumcode review: optional model sections discarded for schema mismatch: %s\n", sections)
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
	// Deterministic static analysis (zero-config): findings from the embedded
	// regex catalog merge with the model findings so security/quality patterns
	// that need no model are always reported, not only when --seguranca is
	// given. These carry their own "analysis/*" rule ids and pass the same
	// rule-config override below.
	// AUR-490: the shared static-analysis pass (mergeStaticAnalysis,
	// cmd/aurumcode/passes.go), identical to the one the --base path now runs.
	mergeStaticAnalysis(diff, result)
	// Deterministic test proposal (zero-config): proposed test cases join the
	// published test plan so a reviewer gets a concrete "what to test" list.
	if plan := testgen.Propose(diff); plan != nil {
		for _, c := range plan.Cases {
			if strings.TrimSpace(c.Name) == "" {
				continue
			}
			result.TestPlan = append(result.TestPlan, fmt.Sprintf("%s (package %s)", c.Name, c.Package))
		}
	}
	result.Issues = config.ApplyRuleConfig(result.Issues, reviewConfig)
	// AUR-548: quality_gates.sast's own Semgrep pass, over the EXACT same
	// verified, clean checkout resolveVerifiedCodebaseContext already
	// requires (verifiedDir/mismatch, computed above for AUR-515/536) --
	// never an unverified checkout, a different repository, or a
	// divergent HEAD. mismatch != "" here means that verification already
	// failed: SAST is reported inconclusive with its own reason
	// (sastReasonUnverifiedCheckout) WITHOUT ever invoking Semgrep, so a
	// stale or unrelated local checkout can never be scanned under the
	// reviewed pull request's name. sastIssues deliberately never passes
	// through config.ApplyRuleConfig (called just above, on the MODEL/
	// deterministic-analysis issues only) -- it is appended straight into
	// result.Issues afterward, exactly like --base's own runReview, so a
	// repository's own `rules:` override was never meant to reach it
	// either (AC-005's boundary drawn at the same place on both paths).
	// sastOrigin mirrors gateOrigin's own policy/repo determination
	// (computed again, just below, for evaluateGate) since
	// reviewConfig.QualityGates.Sast is already the precedence-resolved
	// value either way.
	sastOrigin := gateOriginRepo
	if centralCfg != nil {
		sastOrigin = gateOriginPolicy
	}
	var sastIssues []types.ReviewIssue
	sastReason := ""
	if reviewConfig.QualityGates.Sast.IsEnabled() {
		if mismatch != "" {
			sastReason = sastReasonUnverifiedCheckout
		} else {
			sastIssues, sastReason = runSASTPass(ctx, verifiedDir, reviewConfig.QualityGates.Sast, realSemgrepRunner)
		}
	}
	if sastReason != "" {
		notice := sastInconclusiveNotice(reviewLanguage, sastReason)
		fmt.Fprintf(stderr, "aurumcode review: %s\n", notice)
		result.Limitations = append(result.Limitations, notice)
	} else if len(sastIssues) > 0 {
		result.Issues = append(result.Issues, sastIssues...)
	}
	result.Suggestions = filterSuggestionsToChangedLines(diff, result.Suggestions)
	suppressOperationalStrengths(diff, result)
	result.Limitations = filterLimitationsAgainstDiff(diff, result.Limitations)
	// AUR-476: the deterministic coverage notice is added AFTER
	// filterLimitationsAgainstDiff on purpose. That filter removes a model
	// limitation that names a changed path as "unavailable"; this notice is
	// not model output -- it is derived from the diff, the base-ref config
	// and the prompt builder's own coverage metadata -- so it must survive
	// even though it names filtered paths. It is present even when the model
	// claims complete coverage (AC-003).
	coverageBreakdown := mergeReviewCoverage(result.Metadata, nil, rawDiffFileCount, ignoredPaths)
	if notice := coverageNotice(reviewCopyFor(reviewLanguage), coverageBreakdown); notice != "" {
		result.Limitations = append(result.Limitations, notice)
	}
	if changelogLimitation != "" {
		result.Limitations = append(result.Limitations, changelogLimitation)
	}
	if codebaseContextLimitation != "" {
		result.Limitations = append(result.Limitations, codebaseContextLimitation)
	}
	// AUR-518: the policy warnings already printed to stderr above also join
	// the published review body, so the PR sees the same declaration the
	// terminal does (AC-001/AC-002/AC-003).
	for _, warning := range policyWarnings {
		result.Limitations = append(result.Limitations, warning.Provider+": "+warning.Reason)
	}

	// AUR-519: the gate, evaluated once every other pass/limitation above
	// has run so its decision lines can still join the published review
	// body below. gateInconclusiveReason's priority mirrors AUR-505/
	// AUR-458/AUR-537: a provider that never produced an answer at all
	// (providerFailed -- the orchestrator exhausted every provider, or
	// --limite refused the call before one was ever reached) outranks a
	// provider answer this run DID receive but could not parse
	// (qualityDegraded, a prompt.ParseError GenerateReviewWithContext
	// already turned into the zero ReviewResult above), which outranks one
	// that parsed as the degraded free-text fallback (prompt.IsDegradedParse,
	// AC-008), which outranks AUR-476's own partial coverage (AC-004). A
	// no-op unless a gate was actually declared (evaluateGate's own
	// Declared() guard) -- see gateDeclared, above, which is this exact
	// same Declared() call, computed once and reused by both decisions.
	//
	// AUR-537: a genuine provider/transport failure (llm.ErrAllProvidersFailed,
	// llm.ErrBudgetExceeded) now falls through to here -- it no longer
	// returns before the gate is reached -- whenever gateDeclared is true;
	// see docs/specs/AUR-537.md. Without a gate, it still returns earlier,
	// exactly as before (AC-003).
	gateInconclusiveReason := ""
	switch {
	case providerFailed:
		gateInconclusiveReason = gateReasonProviderFailure
	case qualityDegraded:
		gateInconclusiveReason = "model_parse_failure"
	case prompt.IsDegradedParse(result):
		gateInconclusiveReason = "degraded_parse"
	case sastReason != "":
		// AUR-548: see main.go's own identical case for why this feeds
		// the top-level reason too, even though applySASTGate (below)
		// already marks gateResult.Inconclusive independently of it --
		// writeComplianceArtifacts' SARIF document reads ONLY this
		// variable for executionSuccessful, never gateResult.Inconclusive.
		gateInconclusiveReason = sastReason
	case coverageBreakdown.partial():
		gateInconclusiveReason = "partial_coverage"
	}
	gateOrigin := gateOriginRepo
	if centralCfg != nil {
		gateOrigin = gateOriginPolicy
	}
	// AUR-520: on --pr the repo identity is simply owner/repoName -- the
	// exact "owner/repo" the pull request belongs to, already parsed and
	// verified by parseOwnerRepo/the authenticated GitHub API call above,
	// never anything derived from the PR's own (author-controlled) diff
	// or head checkout.
	gateResult, gateErr := evaluateGate(reviewConfig.Gate, gateOrigin, dynamicRules, result.Issues, gateInconclusiveReason, reviewConfig.Exceptions, owner+"/"+repoName, time.Now())
	if gateErr != nil {
		fmt.Fprintf(stderr, "aurumcode review: gate: %v\n", gateErr)
		return 2
	}
	// AUR-548: quality_gates.sast's own, independent gate decision,
	// folded into the same gateResult evaluateGate just returned -- see
	// aur548.go's own package doc for why this never goes through
	// evaluateGate itself.
	if err := applySASTGate(&gateResult, reviewConfig.QualityGates.Sast, sastOrigin, sastIssues, sastReason); err != nil {
		fmt.Fprintf(stderr, "aurumcode review: gate: %v\n", err)
		return 2
	}
	if gateResult.Active {
		for _, line := range gateResult.Lines {
			fmt.Fprintf(stderr, "aurumcode review: policy gate: %s\n", line)
			result.Limitations = append(result.Limitations, "policy gate: "+line)
		}
		if gateResult.Fail || gateResult.Inconclusive {
			// B-V: PolicyGateWithheldKey is the ONLY mechanism that
			// withholds approval here -- reviewVerdictForLanguage/
			// formalReviewEvent/canonicalVerdict check this key, never
			// result.Verdict, which the model controls and which those
			// functions must keep ignoring (AUR-538 removed a dead
			// `result.Verdict = "comment"` assignment that used to sit
			// here: nothing downstream of this function ever reads it).
			// Firing on gateResult.Fail||Inconclusive alone (never
			// conditioned on what the model's own Verdict happened to
			// say) means a model reply of "", "changes_requested" or
			// "approve" are all withheld alike.
			if result.Metadata == nil {
				result.Metadata = make(map[string]string)
			}
			result.Metadata[prompt.PolicyGateWithheldKey] = "true"
		}
	}

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
	// under review" (see docs/specs/AUR-438.md's Compatibility note) and
	// remains the anchor for a plain --pr run that only needs a SHA for its
	// inline comments. It is NOT the anchor for --check, however: on a
	// pull_request event GitHub reserves GITHUB_SHA to the synthetic merge
	// commit, so a status published there is invisible to a branch
	// protection rule that requires this context on the pull request head
	// (AUR-504, docs/specs/AUR-504.md). When --check is given, the head SHA
	// is read from the API (GetPullRequestMetadata.HeadSHA) and used for the
	// status, the formal review commit_id and every inline comment's
	// commit_id, so all of them anchor to the exact revision that was
	// reviewed.
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
	if inlineComments {
		for _, issue := range issues {
			if isInlineEligible(diff, issue) {
				needsCommitID = true
				break
			}
		}
		if publication == "review" && !needsCommitID {
			for _, suggestion := range result.Suggestions {
				if isNativeSuggestion(diff, suggestion) {
					needsCommitID = true
					break
				}
			}
		}
	}
	// AUR-504: --check anchors on the pull request HEAD read from the API,
	// never on GITHUB_SHA (the synthetic merge commit on a pull_request
	// event). AC-003: when that head cannot be determined, refuse before
	// publishing anything and name the reason -- never fall back to the
	// wrong SHA.
	if check {
		headSHA, resolveErr := resolvePullRequestHeadSHA(ctx, client, owner, repoName, prNumber)
		if resolveErr != nil {
			fmt.Fprintf(stderr, "aurumcode review: refusing to publish: %v\n", resolveErr)
			return 1
		}
		commitID = headSHA
	}
	if needsCommitID && commitID == "" {
		fmt.Fprintln(stderr, "aurumcode review: refusing to publish: inline comments or --check require a commit SHA; set GITHUB_SHA")
		return 1
	}

	// AUR-521: the compliance audit record and SARIF document, written once
	// the gate's own decision above is final and the commit identity this
	// run reviewed is resolved. A no-op unless --auditoria or --sarif was
	// given (writeComplianceArtifacts's own guard).
	writeComplianceArtifacts(complianceArtifactInputs{
		auditoriaPath:          opts.auditoriaPath,
		sarifPath:              opts.sarifPath,
		policyDir:              opts.policyDir,
		centralCfg:             centralCfg,
		repo:                   owner + "/" + repoName,
		reviewedSHA:            commitID,
		model:                  firstNonEmpty(opts.modelo, os.Getenv("LLM_MODEL")),
		verdict:                canonicalVerdict(result),
		gate:                   gateResult,
		gateInconclusiveReason: gateInconclusiveReason,
		diff:                   diff,
		issues:                 result.Issues,
		dynamicRules:           dynamicRules,
		coverageComplete:       !coverageBreakdown.partial(),
		omittedFiles:           append(append([]string{}, coverageBreakdown.IgnoredPaths...), coverageBreakdown.FilteredPaths...),
	}, filter, stderr)

	// The publish loop never lets one finding's POST failure swallow the
	// rest: a failure is recorded and the loop continues, so an inline
	// comment that fails to post does not also cost the general comment
	// for a different, unrelated finding (or vice versa). Every recorded
	// failure is reported after the loop, and the command exits non-zero
	// if any occurred -- but every finding that COULD be published still
	// was.
	var failures []string
	inlineCount, generalCount := 0, 0
	summaryBody := formatPublishedReviewBody(result, diff, reviewLanguage, publication == "review" && inlineComments, changelogText)
	if publication == "review" {
		formalComments := make([]githubclient.ReviewLineComment, 0)
		if inlineComments {
			for _, issue := range issues {
				if !isInlineEligible(diff, issue) {
					continue
				}
				formalComments = append(formalComments, githubclient.ReviewLineComment{
					Body: formatInlineIssueForLanguage(issue, reviewLanguage),
					Path: issue.File,
					Line: issue.Line,
					Side: review.FindingSide(issue),
				})
			}
			for _, suggestion := range result.Suggestions {
				if comment, ok := nativeSuggestionComment(diff, suggestion, reviewLanguage); ok {
					formalComments = append(formalComments, comment)
				}
			}
		}
		formal := githubclient.PullRequestReview{
			Body:     summaryBody,
			Event:    formalReviewEvent(result),
			CommitID: commitID,
			Comments: formalComments,
		}
		key := fmt.Sprintf("aurumcode/review/%d/%s", prNumber, commitID)
		if err := client.PostPullRequestReview(ctx, owner, repoName, prNumber, formal, key); err != nil {
			fmt.Fprintf(stderr, "aurumcode review: publishing formal review: %v\n", err)
			failures = append(failures, "formal review: "+err.Error())
		} else {
			inlineCount = len(formalComments)
			fmt.Fprintf(stdout, "review formal %q publicado no pull request #%d (%d comentário(s) na linha).\n", formal.Event, prNumber, inlineCount)
		}
	} else {
		for _, issue := range issues {
			line := fmt.Sprintf("%s:%d: [%s] %s", issue.File, issue.Line, issue.Severity, issue.Message)
			if issue.Side == "LEFT" {
				line += " [LEFT/base]"
			}
			if inlineComments && isInlineEligible(diff, issue) {
				comment := githubclient.ReviewComment{
					Body:     formatInlineIssueForLanguage(issue, reviewLanguage),
					CommitID: commitID,
					Path:     issue.File,
					Line:     issue.Line,
					Side:     review.FindingSide(issue),
				}
				key := fmt.Sprintf("aurumcode/%d/%s/%s/%d/%s/%s", prNumber, commitID, issue.File, issue.Line, review.FindingSide(issue), issue.RuleID)
				if err := client.PostReviewComment(ctx, owner, repoName, prNumber, comment, key); err != nil {
					fmt.Fprintf(stderr, "aurumcode review: publishing inline comment on %s:%d: %v\n", issue.File, issue.Line, err)
					failures = append(failures, fmt.Sprintf("%s:%d (na linha): %v", issue.File, issue.Line, err))
					continue
				}
				fmt.Fprintf(stdout, "%s -- publicado na linha\n", line)
				inlineCount++
				continue
			}

			if err := client.PostIssueComment(ctx, owner, repoName, prNumber, formatInlineIssueForLanguage(issue, reviewLanguage)); err != nil {
				fmt.Fprintf(stderr, "aurumcode review: publishing general comment for %s:%d: %v\n", issue.File, issue.Line, err)
				failures = append(failures, fmt.Sprintf("%s:%d (geral): %v", issue.File, issue.Line, err))
				continue
			}
			fmt.Fprintf(stdout, "%s -- publicado como comentario geral\n", line)
			generalCount++
		}

		if err := client.PostIssueComment(ctx, owner, repoName, prNumber, summaryBody); err != nil {
			fmt.Fprintf(stderr, "aurumcode review: publishing review summary: %v\n", err)
			failures = append(failures, "summary: "+err.Error())
		}

		fmt.Fprintf(stdout, "%d comentario(s) publicado(s) no pull request #%d (%d na linha, %d geral).\n",
			inlineCount+generalCount, prNumber, inlineCount, generalCount)
	}

	// Review memory save (opt-in, default off): persist this round's findings
	// as observations for the next one. Memory is observation, never
	// instruction; a save failure is reported and never affects the verdict.
	// AUR-490: the shared memory pass's save half (persistReviewMemory,
	// cmd/aurumcode/passes.go), identical to the one the --base path now runs.
	// AUR-505: a degraded run has no model answer to remember. The
	// deterministic findings are not quality observations, so persisting
	// them would teach the next review the wrong thing; the declared
	// limitation is enough. This mirrors the --base path, which does not
	// persist a run whose quality half failed.
	if !qualityDegraded {
		persistReviewMemory(memoryStore, reviewConfig.Review.Memory, memoryNotes, result.Issues, stderr, filter)
	}

	// The check status is published before the comment-failure return
	// below, not after: a grave finding must still get its failing check
	// even when a separate, unrelated comment POST failed to publish (and
	// symmetrically, a comment failure must not silently cost the check
	// too). The two outcomes are independent, so neither is allowed to
	// swallow the other.
	checkExit := 0
	if check {
		checkExit = publishCheckStatus(ctx, client, stdout, stderr, owner, repoName, commitID, issues, prNumber, opts.exigirQualidade && qualityDegraded, providerFailed)
	}
	// AUR-519: the policy gate's own commit status (policyGateContext),
	// independent of --check's grave-finding status above -- an org's
	// ruleset can require either, both, or neither. publishPolicyGateStatus
	// itself no-ops (returns 0, publishes nothing) when the gate was never
	// declared, so a review with no `gate:` key grows no second status.
	gateCheckExit := 0
	if check {
		gateCheckExit = publishPolicyGateStatus(ctx, client, stdout, stderr, owner, repoName, commitID, gateResult, prNumber)
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
	if opts.exigirQualidade && qualityDegraded {
		fmt.Fprintln(stderr, "aurumcode review: --exigir-qualidade: the model review was inconclusive; the published deterministic findings do not approve this pull request")
		return exitQualityNotReviewed
	}
	if checkExit == 1 || gateCheckExit == 1 {
		return 1
	}
	// AUR-519: the policy gate closes exactly like --fail-on/--check above,
	// reusing the same two exit codes instead of a third. A real severity
	// breach (gateResult.Breach) always returns exitFindings -- the same
	// code --fail-on already uses -- regardless of whether the review was
	// also inconclusive (B1). Fail without a Breach can only come from
	// gate.inconclusive: block, which returns exitQualityNotReviewed (the
	// same "half a review" signal --exigir-qualidade already returns
	// above).
	if gateResult.Breach {
		return exitFindings
	}
	if gateResult.Fail {
		return exitQualityNotReviewed
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
	coverageHeading, coverageSummary, coveragePartial, coverageBudget, coverageIgnored, coverageFiltered string
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
			coverageFiltered:        "%d arquivo(s) foram filtrados antes da revisão (binário ou grande demais).",
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
		coverageFiltered:        "%d file(s) were filtered before the review (binary or too large).",
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
