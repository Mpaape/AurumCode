// Command aurumcode reviews code changes: `aurumcode review --base <ref>` for a
// local diff and `aurumcode review --pr <n> --repo <owner>/<name>` for a pull
// request, plus the compliance subcommands (sbom, sign, xbom, fix, changelog). Flags are
// parsed and dependencies assembled here; the rules live in internal/.
// `aurumcode --help` lists the subcommands.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/Mpaape/AurumCode/internal/apply"
	"github.com/Mpaape/AurumCode/internal/llm"
	"github.com/Mpaape/AurumCode/internal/llm/provider/litellm"
	"github.com/Mpaape/AurumCode/internal/llm/provider/profiles"
	"github.com/Mpaape/AurumCode/internal/render"
	"github.com/Mpaape/AurumCode/internal/review"
	"github.com/Mpaape/AurumCode/internal/review/cache"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// version is aurumcode's build-time version string. This reconstruction
// publishes no numbered release yet, so it defaults to "dev"; a future
// release pipeline stamps a real value with no other code change via:
//
//	go build -ldflags "-X main.version=1.2.3" ./cmd/aurumcode
//
// `aurumcode --version` (or `aurumcode version`) prints it. See
// docs/specs/AUR-443.md.
var version = "dev"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run wires this process's sinks through the single AUR-009 redaction
// filter before any subcommand can write to them (AUR-432).
//
// stderr is wrapped with redaction.NewWriter: every canonical stderr line
// this command emits is filter-identity (pinned by TestStderrLinesSurvive
// TheRedactionWriter), so the wrapper costs nothing on the published
// contract while screening everything else -- transport errors that echo
// response bodies, provider errors, an endpoint URL that still carried
// credentials.
//
// stdout deliberately uses the same filter at field level instead of the
// line writer: the published finding line ends with the AUR-434 rule
// citation "(rule security/hardcoded-secret: <title>)", whose "-secret:
// <title>" spelling the line filter would itself rewrite, changing the
// byte-stable AUR-430 output for secret-free reviews. So every
// model-authored field is redacted at the model boundary
// (internal/review.redactReviewResult) before the trusted citation is
// appended, and the diff-derived notices are redacted here in
// printNotices; a secret-free review keeps its exact published bytes.
func run(args []string, stdout, stderr *os.File) int {
	filter := redaction.FromEnv()
	errW, err := filter.NewWriter(redaction.SinkStderr, stderr)
	if err != nil {
		// Fail closed: without a redacted writer nothing may be written to
		// the sink. The message below is a static literal.
		fmt.Fprintln(stderr, "aurumcode: stderr redaction writer unavailable")
		return 1
	}
	defer errW.Flush()
	fallbackNoticeSink = errW

	if len(args) == 0 {
		fmt.Fprintln(errW, "usage: aurumcode review [flags]")
		return 2
	}

	switch args[0] {
	case "--help", "-h", "help":
		// An explicitly requested top-level --help is a fulfilled request,
		// not a failure: stdout, exit 0, the same convention `review
		// --help` is a fulfilled request. This is static text, so it
		// deliberately bypasses the redaction writer.
		printTopLevelHelp(stdout)
		return 0
	case "--version", "version":
		printVersion(stdout)
		return 0
	}
	if sc, ok := findSubcommand(args[0]); ok {
		return sc.run(args[1:], stdout, errW, filter)
	}
	fmt.Fprintf(errW, "aurumcode: unknown command %q\n", args[0])
	return 2
}

// printVersion answers `aurumcode --version` / `aurumcode version`.
func printVersion(stdout io.Writer) {
	fmt.Fprintf(stdout, "aurumcode %s\n", version)
}

// runFix turns review suggestions into an applyable unified diff (one-click
// fix). It reads either a JSON array of ReviewSuggestion objects or a full
// review response object with a "suggestions" field (so `aurumcode fix <
// review-response.json` works directly), builds the patch safely
// (internal/apply fails closed on any unsafe suggestion), and prints the
// patch to stdout -- it never modifies the repository or a remote.
//
// AUR-489/AC-003: before printing a non-empty patch and exiting 0, the
// patch is checked against the REAL working tree rooted at the current
// directory (validateFixPatch, fixvalidate.go). Before this card, a
// suggestion fabricated with a current_code that did not exist anywhere in
// the named file still produced a patch and a silent exit 0 -- runFix
// never read the file at all. A patch that does not apply now exits 1 and
// names the offending file and line on stderr.
func runFix(args []string, stdout, stderr io.Writer) int {
	fs, file := newFixFlagSet()
	if exit, ok := parseSubcommandFlags("fix", fs, args, stdout, stderr); !ok {
		return exit
	}
	data, err := readFixSuggestions(*file)
	if err != nil {
		fmt.Fprintf(stderr, "aurumcode fix: %v\n", err)
		return 1
	}
	suggestions, err := parseFixSuggestions(data)
	if err != nil {
		fmt.Fprintf(stderr, "aurumcode fix: parsing suggestions: %v\n", err)
		return 1
	}
	dir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(stderr, "aurumcode fix: %v\n", err)
		return 1
	}
	// BuildPlan and BuildPatch read the same suggestions and the same
	// working tree, so the plan used to validate and the patch printed on
	// success can never diverge. The hunks carry three lines of real file
	// context, so the printed patch applies with plain `git apply`.
	tree := os.DirFS(dir)
	plan, err := apply.BuildPlan(suggestions, tree)
	if err != nil {
		fmt.Fprintf(stderr, "aurumcode fix: %v\n", err)
		return 1
	}
	patch, err := apply.BuildPatch(suggestions, tree)
	if err != nil {
		fmt.Fprintf(stderr, "aurumcode fix: %v\n", err)
		return 1
	}
	if strings.TrimSpace(patch) != "" {
		if err := validateFixPatch(dir, patch, plan); err != nil {
			fmt.Fprintf(stderr, "aurumcode fix: %v\n", err)
			return 1
		}
		fmt.Fprint(stdout, patch)
	}
	return 0
}

// prepareCache opens the review cache and keeps only the cache misses in
// toSend (AUR-441). The prompt-version component is a digest of the fixed
// content a prompt builder renders (AUR-543). The estimate printed earlier
// priced the FULL diff, oblivious to caching.
func (b *baseReview) prepareCache() *qualityCache {
	promptVersionDigest, promptDigestErr := b.deps.digestBuilder().FixedContentDigest()
	store, cacheErr := cache.Open(cache.ResolveDir())
	if cacheErr == nil {
		cacheErr = promptDigestErr
	}
	if cacheErr == nil && b.toolsOffered {
		cacheErr = errDeliberationNotCacheable
	}
	if cacheErr == nil && len(b.evidence) > 0 {
		// The per-file cache stores issues, not the model's assessment of
		// the evidence, and an assessment may weigh evidence of several
		// files: a review that offered evidence is never served from it.
		cacheErr = errEvidenceNotCacheable
	}
	qc := &qualityCache{store: store, err: cacheErr, toSend: b.diff}
	if cacheErr == nil {
		var missFiles []types.DiffFile
		missFiles, qc.statuses = partitionByCache(store, b.diff, b.contextCacheKey(), promptVersionDigest, b.filter)
		qc.toSend = &types.Diff{Files: missFiles}
	}
	return qc
}

// countAtOrAbove counts the findings whose severity sits at threshold or
// above. The count -- not just a boolean -- feeds the gate's stderr note so
// the user sees how many findings closed the gate.
func countAtOrAbove(issues []types.ReviewIssue, threshold int) int {
	count := 0
	for _, issue := range issues {
		if severityRank(issue.Severity) >= threshold {
			count++
		}
	}
	return count
}

// selectProviderForModel serves the model the user chose with --modelo
// (AUR-436), reusing the exact provider mechanisms selectProvider already
// documents -- nothing here adds a vendor:
//
//   - AURUMCODE_LLM_FIXTURE=<path>: the deterministic offline provider
//     (review.FakeProvider) answers for the chosen model. This is how the
//     sealed, network-denied acceptance exercises `--modelo local`.
//   - LLM_API_KEY and LLM_BASE_URL: the chosen model rides the existing
//     OpenAI-compatible litellm provider, overriding LLM_MODEL: the flag,
//     not the environment, decides. A local model is chosen by pointing
//     LLM_BASE_URL at a local endpoint (an ollama or llama.cpp server's
//     OpenAI-compatible API, or a litellm proxy in front of any local
//     model).
//
// Neither set: the chosen model is unavailable. That is an error the
// caller must surface loudly (see reportModelUnavailable) -- never an
// empty review with exit 0.
//
// The second return value says which mechanism serves the model, for the
// stderr selection note.
func selectProviderForModel(model string) (llm.Provider, string, error) {
	p, mechanism, err := providerFromEnv(model, model)
	if err != nil {
		return nil, "", err
	}
	if p != nil {
		return p, mechanism, nil
	}
	return nil, "", errors.New("no LLM provider is configured to serve it")
}

// providerFromEnv is the one reader of the provider environment. The offline
// fixture (AURUMCODE_LLM_FIXTURE) answers as fixtureModel; a live
// OpenAI-compatible endpoint (LLM_API_KEY and LLM_BASE_URL) serves liveModel
// ("" lets the endpoint choose). With neither configured it returns a nil
// provider and a nil error, and the caller names what is missing. The string
// says which mechanism serves the model, for the stderr selection note.
func providerFromEnv(fixtureModel, liveModel string) (llm.Provider, string, error) {
	if fixturePath := os.Getenv("AURUMCODE_LLM_FIXTURE"); fixturePath != "" {
		content, err := os.ReadFile(fixturePath)
		if err != nil {
			return nil, "", fmt.Errorf("reading AURUMCODE_LLM_FIXTURE=%s: %w", fixturePath, err)
		}
		return review.NewOfflineProvider(string(content), fixtureModel, os.Getenv("AURUMCODE_PROMPT_CAPTURE")), "offline fixture provider", nil
	}
	// LLM_PROVIDER selects a catalog profile (internal/llm/provider/profiles);
	// set but unusable is an error, never a silent fallback.
	endpoint, selected, err := profiles.FromEnv(os.Getenv)
	if selected {
		if err != nil {
			return nil, "", err
		}
		return withFallbacks(litellm.NewProviderWithDialect(endpoint.Dialect, endpoint.APIKey, endpoint.BaseURL, liveModel), endpoint.Profile+" endpoint "+redactedEndpoint(endpoint.BaseURL))
	}
	apiKey := os.Getenv("LLM_API_KEY")
	baseURL := os.Getenv("LLM_BASE_URL")
	if apiKey != "" && baseURL != "" {
		// LLM_FALLBACK_<n>_* slots, when set, answer if this one fails.
		return withFallbacks(litellm.NewProvider(apiKey, baseURL, liveModel), "litellm endpoint "+redactedEndpoint(baseURL))
	}
	return nil, "", nil
}

// printFindings prints one line per issue (and, with none, render.
// NoFindingsLine: "No issues found." only when no source was inconclusive): "<file>:<line>: [<severity>]
// <message>", sorted by (file, line) so the same review result always
// prints in the same order regardless of the order the model listed
// findings in.
// Every model-authored field printed here was already redacted at the
// model boundary (internal/review.redactReviewResult, AUR-432), before the
// trusted rule citation was appended, so the published byte-stable format
// survives while no echoed secret can reach this sink.
func printFindings(stdout io.Writer, result *types.ReviewResult, inconclusiveReason, language string) {
	issues := make([]types.ReviewIssue, len(result.Issues))
	copy(issues, result.Issues)
	sort.SliceStable(issues, func(i, j int) bool {
		if issues[i].File != issues[j].File {
			return issues[i].File < issues[j].File
		}
		return issues[i].Line < issues[j].Line
	})

	if len(issues) == 0 {
		fmt.Fprintln(stdout, render.NoFindingsLine(inconclusiveReason))
		return
	}

	for _, issue := range issues {
		fmt.Fprintf(stdout, "%s:%d: [%s] %s\n", issue.File, issue.Line, issue.Severity, issue.Message)
		if issue.Side == "LEFT" {
			fmt.Fprintln(stdout, "  Location: LEFT (removed line in base)")
		}
		writeFindingFields(stdout, issue, reviewCopyFor(language))
		printAssessment(stdout, issue)
	}
}

// selectProvider is the one place cmd/aurumcode names a specific LLM
// vendor, and it does so only to satisfy an environment variable the
// operator set -- nothing here is hardwired to one provider. Two modes are
// supported:
//
//   - AURUMCODE_LLM_FIXTURE=<path>: read that file's content and use it
//     verbatim as the model's response, via review.FakeProvider. This is
//     how tests/acceptance/AUR-430.sh runs the real binary fully offline
//     and deterministically (the sandbox this card's acceptance runs under
//     denies network access entirely).
//   - LLM_API_KEY and LLM_BASE_URL: use the existing, already-vendor-neutral
//     internal/llm/provider/litellm.Provider (an OpenAI-compatible endpoint).
//     LLM_MODEL is forwarded when present; when omitted, the endpoint may
//     choose its own configured default.
//
// Neither set: a clear, typed-by-message error, not a panic or a silent
// no-op provider.
// errNoProviderConfigured is selectProvider's error when neither an
// offline fixture (AURUMCODE_LLM_FIXTURE) nor a live endpoint
// (LLM_API_KEY + LLM_BASE_URL) is configured -- as opposed to any other
// provider failure (an AURUMCODE_LLM_FIXTURE path that does not exist, a
// malformed endpoint). AUR-449's --seguranca-only skip (runReview above)
// tests for this exact sentinel with errors.Is: "the caller configured
// nothing at all" is eligible to fall back to the deterministic security
// pass alone, but a caller who attempted configuration and got it wrong is
// still told the review failed, never silently downgraded.
// The message text is AUR-448's: the COMPLETE fixture shape the engine
// accepts, rule_id included, because enforceRuleCitations (AUR-434)
// silently discards a finding whose rule_id is missing, and the
// pre-AUR-448 shape omitted it. See selectProvider's own comment below and
// docs/specs/AUR-448.md.
var errNoProviderConfigured = errors.New(`no LLM provider configured: set AURUMCODE_LLM_FIXTURE=<path> to a JSON file shaped like {"issues":[{"file":"<path>","line":<n>,"severity":"error|warning|info","rule_id":"<id from the embedded rule catalog, e.g. security/hardcoded-secret>","message":"<text>"}]} for offline use -- a finding whose rule_id is missing or unknown is discarded, never shown, so rule_id is not optional -- if you have the AurumCode source checked out, tests/fixtures/review/known-problem-response.json is a worked example -- or set LLM_API_KEY and LLM_BASE_URL for a live provider`)
