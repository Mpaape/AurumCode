// Command aurumcode is the entrypoint for this reconstruction's local code
// review engine (AUR-430). It currently supports one subcommand:
//
//	aurumcode review --base <ref> [--fail-on <level>] [--modelo <nome>] [--seguranca]
//
// which diffs <ref> against HEAD in the git repository rooted at the
// current working directory, sends that diff to an LLM through
// internal/llm.Orchestrator, and prints the findings the model reports.
//
// With --fail-on (AUR-431), the command additionally acts as a CI gate: it
// exits with the distinct code 3 when any finding sits at the chosen
// severity or above, and 0 otherwise. Without --fail-on, behavior is
// exactly AUR-430's: findings never change the exit code.
//
// With --modelo (AUR-436), the user chooses which model reviews --
// including a local one, by pointing LLM_BASE_URL at a local
// OpenAI-compatible endpoint -- and when nothing is configured to serve
// the chosen model the command fails with a clear, actionable error on
// stderr and exit 1, never an empty review with exit 0. Without --modelo,
// provider selection is exactly AUR-430's.
//
// With --seguranca (AUR-435), the command additionally runs the project's
// deterministic security pass over the diff: the ADDED lines are matched
// against the patterns carried by the security-category rules of the
// embedded catalog (internal/review/rules/security.yml, scoped by
// standards/security-review), and the findings print in their own section
// AFTER the unchanged quality output, each citing its sustaining rule and
// the standard rule that scopes it. Without --seguranca, stdout is
// byte-identical to the published contract.
//
// With --pr (AUR-438), the command reviews a GitHub pull request instead of
// a local ref:
//
//	aurumcode review --pr <numero> --repo <dono>/<projeto> --publicar --na-linha
//
// It reads the pull request's diff through the restored GitHub client
// (AUR-437, internal/git/githubclient), reviews it through the same engine
// and provider selection as the --base path, and publishes every finding
// as a pull request comment: at the file's exact changed line when the
// diff added that line, or as a general pull request comment when the
// finding sits outside the changed lines, so it is never silently dropped.
// Publishing refuses, before anything is posted, when the token lacks
// write permission on the repository. --repo and --publicar are always
// required with --pr; --na-linha is required too, UNLESS --check (below)
// is given -- every other flag (--base, --fail-on, --modelo, --seguranca)
// and its published behavior is unchanged when --pr is absent.
//
// With --check (AUR-439), the command additionally publishes a commit
// status (internal/git/githubclient.SetStatus, restored by AUR-437) on the
// pull request's head commit: "failure" when at least one finding is grave
// (error severity, the same rank --fail-on high|error already names),
// "success" otherwise -- so a branch protection rule that requires this
// check blocks the merge until the grave finding is fixed. --check needs a
// commit SHA exactly like an inline comment already does (GITHUB_SHA), and
// folds into the very same fail-closed gate instead of a second one.
//
//	aurumcode review --pr 42 --repo dono/projeto --publicar --check
//
// With --limite (AUR-433), the command caps what one run may spend calling
// the model: it estimates the cost before the model is ever invoked and
// refuses to call it -- spending nothing -- when the estimate exceeds the
// USD ceiling given, reporting both the estimated and, on success, the
// real cost on stderr. Without --limite, behavior and output are exactly
// as already published: no budget is enforced. Like the other --base-path
// flags, --limite is inert when --pr is given.
//
// See docs/specs/AUR-430.md for the base command reference,
// docs/specs/AUR-431.md for the --fail-on gate,
// docs/specs/AUR-436.md for --modelo, docs/specs/AUR-435.md for
// --seguranca, docs/specs/AUR-438.md for --pr, docs/specs/AUR-433.md for
// --limite, and docs/specs/AUR-439.md for --check, each with an offline,
// secret-free example.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"sort"
	"strings"

	"github.com/Mpaape/AurumCode/internal/analyzer"
	"github.com/Mpaape/AurumCode/internal/llm"
	"github.com/Mpaape/AurumCode/internal/llm/cost"
	"github.com/Mpaape/AurumCode/internal/llm/provider/litellm"
	"github.com/Mpaape/AurumCode/internal/prompt"
	"github.com/Mpaape/AurumCode/internal/review"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
	"github.com/Mpaape/AurumCode/pkg/types"
)

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

	if len(args) == 0 {
		fmt.Fprintln(errW, "usage: aurumcode <review|docs> [flags]")
		return 2
	}

	switch args[0] {
	case "review":
		return runReview(args[1:], stdout, errW, filter)
	case "docs":
		return runDocs(args[1:], stdout, errW, filter)
	default:
		fmt.Fprintf(errW, "aurumcode: unknown command %q\n", args[0])
		return 2
	}
}

func runReview(args []string, stdout, stderr io.Writer, filter *redaction.Filter) int {
	fs := flag.NewFlagSet("review", flag.ContinueOnError)
	fs.SetOutput(stderr)
	base := fs.String("base", "", "ref to diff against HEAD (required), e.g. HEAD~1 or a branch name")
	failOn := fs.String("fail-on", "", "minimum severity that makes the command exit 3: high|error, medium|warning, low|info (default: findings never change the exit code)")
	modelo := fs.String("modelo", "", "model that reviews, e.g. local, llama3 or gpt-4; served offline via AURUMCODE_LLM_FIXTURE or live via LLM_API_KEY and LLM_BASE_URL (default: AUR-430's selection, unchanged)")
	seguranca := fs.Bool("seguranca", false, "additionally run the project's security pass: match the diff's added lines against the security rules of the embedded catalog (standards/security-review) and print the findings in their own section (default: off, output unchanged)")
	pr := fs.Int("pr", 0, "pull request number to review (AUR-438); activates the PR path and requires --repo, --publicar and --na-linha (default: off, --base path unchanged)")
	repoFlag := fs.String("repo", "", "owner/repo of the pull request; required with --pr")
	publicar := fs.Bool("publicar", false, "publish findings as comments on the pull request; required with --pr (default: off)")
	naLinha := fs.Bool("na-linha", false, "comment at the file's exact changed line, falling back to a general comment for a finding outside the changed lines; required with --pr (default: off)")
	limite := fs.String("limite", "", "maximum USD this run may spend calling the model; the command estimates the cost before calling it and refuses -- spending nothing -- when the estimate exceeds this value (default: no limit enforced)")
	check := fs.Bool("check", false, "publish a commit status (AUR-439) that fails when a grave (error-severity) finding is present, blocking the pull request's merge; with --pr, satisfies the --na-linha requirement on its own (default: off)")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	// --pr (AUR-438) activates the PR review path and is checked before
	// anything below: every other flag's published behavior must stay
	// byte-identical when --pr is absent, so this dispatch has to happen
	// before the --base requirement (and everything after it) ever runs.
	// Detected via fs.Visit, which reports flags by their canonical name
	// after parsing -- never by scanning args for a "--" prefix, since
	// Go's flag package treats "-pr" and "--pr" identically and a
	// prefix-only guard would silently miss the single-dash spelling (see
	// docs/specs/AUR-438.md's inherited-finding note).
	prGiven := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "pr" {
			prGiven = true
		}
	})
	if prGiven {
		return runPRReview(stdout, stderr, *pr, *repoFlag, *publicar, *naLinha, *check)
	}

	if *base == "" {
		fmt.Fprintln(stderr, "aurumcode review: --base is required")
		return 2
	}

	// Validate --fail-on before doing any work: an unknown level is a
	// usage error (exit 2), never a silently-open gate. fs.Visit only
	// visits flags that were actually set, so an explicitly empty value
	// (`--fail-on=`, or `--fail-on "$VAR"` with VAR unset in CI) is
	// distinguished from an absent flag and rejected the same way any
	// other unknown level is, instead of silently disabling the gate.
	failOnSet := false
	modeloSet := false
	limiteSet := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "fail-on" {
			failOnSet = true
		}
		if f.Name == "modelo" {
			modeloSet = true
		}
		if f.Name == "limite" {
			limiteSet = true
		}
	})
	threshold := 0
	thresholdName := ""
	if failOnSet {
		var err error
		threshold, thresholdName, err = parseFailOnLevel(*failOn)
		if err != nil {
			fmt.Fprintf(stderr, "aurumcode review: %v\n", err)
			return 2
		}
	}

	// A present-but-empty model name (`--modelo "$VAR"` with VAR unset) is
	// a usage error, mirroring --fail-on's treatment above: the flag was
	// given, so silently falling back to the default selection would review
	// with a model the user never chose.
	if modeloSet && *modelo == "" {
		fmt.Fprintln(stderr, "aurumcode review: --modelo: model name must not be empty")
		return 2
	}

	// --limite (AUR-433) is validated the same way: an explicitly empty or
	// unparsable value is a usage error, never a silently-disabled limit.
	limiteUSD := 0.0
	if limiteSet {
		if *limite == "" {
			fmt.Fprintln(stderr, "aurumcode review: --limite: value must not be empty")
			return 2
		}
		var err error
		limiteUSD, err = parseLimiteUSD(*limite)
		if err != nil {
			fmt.Fprintf(stderr, "aurumcode review: %v\n", err)
			return 2
		}
	}

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(stderr, "aurumcode review: %v\n", err)
		return 1
	}

	diff, notices, err := computeDiff(cwd, *base)
	if err != nil {
		fmt.Fprintf(stderr, "aurumcode review: %v\n", err)
		return 1
	}

	// Provider selection. With --modelo the flag commands which model
	// reviews (AUR-436); without it, selectProvider keeps AUR-430's
	// published behavior verbatim.
	var provider llm.Provider
	var providerErr error
	providerVia := ""
	if *modelo != "" {
		provider, providerVia, providerErr = selectProviderForModel(*modelo)
	} else {
		provider, providerErr = selectProvider()
	}
	if providerErr != nil {
		if *modelo != "" {
			return reportModelUnavailable(stderr, *modelo, providerErr)
		}
		fmt.Fprintf(stderr, "aurumcode review: %v\n", providerErr)
		return 1
	}
	if *modelo != "" {
		// The note goes to stderr so stdout stays byte-identical with and
		// without the flag, exactly like --fail-on's gate note.
		fmt.Fprintf(stderr, "aurumcode review: reviewing with model %q (%s)\n", *modelo, providerVia)
	}

	// --limite (AUR-433): wire internal/llm/cost.Tracker into the
	// orchestrator so it is the one place that estimates the cost and
	// refuses -- before the model is ever called -- when it exceeds the
	// ceiling (see cost.go). Without the flag, tracker stays nil and
	// llm.NewOrchestrator behaves exactly as already published: unmetered.
	var tracker *cost.Tracker
	if limiteSet {
		price, err := costPrice()
		if err != nil {
			fmt.Fprintf(stderr, "aurumcode review: %v\n", err)
			return 2
		}
		modelKey := costModelKey(*modelo)
		provider = &fixedModelProvider{Provider: provider, model: modelKey}
		tracker = buildCostTracker(limiteUSD, modelKey, price)
		printCostEstimate(stderr, estimateCostUSD(diff, price), limiteUSD)
	}

	orchestrator := llm.NewOrchestrator(provider, nil, tracker)
	reviewer := review.NewReviewer(orchestrator, review.DefaultConfig())

	result, err := reviewer.GenerateReview(context.Background(), diff)
	if err != nil {
		// --limite (AUR-433): the tracker refused before the model was
		// called, so nothing was spent. This is checked first because it is
		// a distinct, more specific outcome than "the provider chain could
		// not complete" below. Not gated on limiteSet: without --limite,
		// tracker is nil and the orchestrator cannot produce this error, so
		// the check is a no-op then and unconditionally correct whenever it
		// does fire (limiteUSD is 0 only in the case where it cannot).
		if errors.Is(err, llm.ErrBudgetExceeded) {
			return reportBudgetExceeded(stderr, limiteUSD, err)
		}
		var parseErr *prompt.ParseError
		if errors.As(err, &parseErr) {
			fmt.Fprintf(stderr, "aurumcode review: could not understand the model's response (%s)\n", parseErr.Kind)
			return 1
		}
		// With --modelo, a provider chain that could not complete means the
		// chosen model is unavailable (endpoint down, wrong URL, model not
		// served): name the model and say how to fix it, instead of only
		// surfacing the transport error.
		if *modelo != "" && errors.Is(err, llm.ErrAllProvidersFailed) {
			return reportModelUnavailable(stderr, *modelo, err)
		}
		fmt.Fprintf(stderr, "aurumcode review: %v\n", err)
		return 1
	}

	if limiteSet {
		printRealCost(stderr, realCostUSD(tracker, limiteUSD), limiteUSD)
	}

	// The security pass (AUR-435) runs before anything prints, so a broken
	// rules catalog fails loudly with nothing on stdout instead of after a
	// partial report. It is deterministic and calls no model.
	var securityFindings []types.ReviewIssue
	if *seguranca {
		securityFindings, err = review.SecurityScan(diff)
		if err != nil {
			fmt.Fprintf(stderr, "aurumcode review: %v\n", err)
			return 1
		}
	}

	printNotices(stdout, filter, notices)
	printFindings(stdout, result)
	if *seguranca {
		printSecurityFindings(stdout, filter, securityFindings)
	}

	// The --fail-on CI gate (AUR-431): after the findings have printed
	// exactly as they always do, exit with the distinct, documented code 3
	// when any finding sits at the chosen severity or above. The note goes
	// to stderr so stdout stays byte-identical with and without the flag.
	// With --seguranca the security findings count toward the gate too: a
	// gate that fails a review over style but waves through a matched
	// vulnerability would be open exactly when it must be closed.
	if threshold > 0 {
		gated := result.Issues
		if len(securityFindings) > 0 {
			gated = make([]types.ReviewIssue, 0, len(result.Issues)+len(securityFindings))
			gated = append(gated, result.Issues...)
			gated = append(gated, securityFindings...)
		}
		if n := countAtOrAbove(gated, threshold); n > 0 {
			fmt.Fprintf(stderr, "aurumcode review: %d finding(s) at severity %s or above (--fail-on %s)\n", n, thresholdName, thresholdName)
			return exitFindings
		}
	}
	return 0
}

// exitFindings is the exit code for "the review ran fine and found at least
// one issue at or above the --fail-on threshold". It is distinct from 0
// (clean run), 1 (the review itself failed) and 2 (usage error) so a CI
// pipeline can tell "gate closed" apart from "tool broke". Documented in
// docs/specs/AUR-431.md.
const exitFindings = 3

// parseFailOnLevel maps a --fail-on level to the internal severity rank it
// gates on, returning the canonical engine severity name alongside. The
// accepted spellings are the engine's own severity vocabulary -- error,
// warning, info, exactly the three values internal/prompt.ResponseParser
// admits -- plus the CI-conventional aliases high, medium and low.
// Matching is case-insensitive; anything else is an error.
func parseFailOnLevel(level string) (int, string, error) {
	switch strings.ToLower(level) {
	case "high", "error":
		return rankError, "error", nil
	case "medium", "warning":
		return rankWarning, "warning", nil
	case "low", "info":
		return rankInfo, "info", nil
	default:
		return 0, "", fmt.Errorf("--fail-on: unknown level %q (accepted: high|error, medium|warning, low|info)", level)
	}
}

// Severity ranks, ordered so that "at the chosen severity or above" is a
// plain >= comparison. 0 is reserved for "no gate configured".
const (
	rankInfo    = 1
	rankWarning = 2
	rankError   = 3
)

// severityRank ranks a finding's severity. The parser guarantees every
// issue carries one of error/warning/info (in any letter case, see
// internal/prompt/parser.go's validation), so the default arm is
// unreachable in practice; it still ranks unknown values as error so the
// gate fails closed rather than silently waving a finding through.
func severityRank(severity string) int {
	switch strings.ToLower(severity) {
	case "info":
		return rankInfo
	case "warning":
		return rankWarning
	case "error":
		return rankError
	default:
		return rankError
	}
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

// computeDiff reads the diff between base and HEAD directly from the git
// object database at repoRoot. See internal/analyzer/gitrepo.go for why
// this reads git's on-disk format in pure Go instead of shelling out to a
// `git` binary.
func computeDiff(repoRoot, base string) (*types.Diff, []analyzer.DiffNotice, error) {
	repo, err := analyzer.OpenRepo(repoRoot)
	if err != nil {
		return nil, nil, fmt.Errorf("%s is not a git repository: %w", repoRoot, err)
	}
	diff, notices, err := repo.Diff(base, "HEAD")
	if err != nil {
		return nil, nil, fmt.Errorf("computing diff %s..HEAD: %w", base, err)
	}
	return diff, notices, nil
}

// printNotices reports the changed files that were deliberately not
// reviewed -- binary blobs and files past the size limits documented in
// docs/specs/AUR-430.md. They print before the findings, on stdout, in the
// diff's own sorted path order, and they do not change the exit code: a
// skipped file is a normal, reportable outcome, not a failure. A run with
// nothing to skip prints nothing here, so ordinary output is unaffected.
// Notice text derives from diff paths -- repository-controlled input -- so
// it passes the redaction filter before reaching the sink (AUR-432); an
// ordinary path is filter-identity.
func printNotices(stdout io.Writer, filter *redaction.Filter, notices []analyzer.DiffNotice) {
	for _, n := range notices {
		fmt.Fprintln(stdout, filter.Redact(n.Message))
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
//     internal/llm/provider/litellm.Provider (the same OpenAI-compatible
//     proxy path cmd/regenerate-docs already uses), naming a model via
//     LLM_MODEL (defaulting to "gpt-4").
//
// Neither set: a clear, typed-by-message error, not a panic or a silent
// no-op provider.
func selectProvider() (llm.Provider, error) {
	if fixturePath := os.Getenv("AURUMCODE_LLM_FIXTURE"); fixturePath != "" {
		content, err := os.ReadFile(fixturePath)
		if err != nil {
			return nil, fmt.Errorf("reading AURUMCODE_LLM_FIXTURE=%s: %w", fixturePath, err)
		}
		return &review.FakeProvider{
			Response:    string(content),
			NameStr:     "fixture",
			CapturePath: os.Getenv("AURUMCODE_PROMPT_CAPTURE"),
		}, nil
	}

	apiKey := os.Getenv("LLM_API_KEY")
	baseURL := os.Getenv("LLM_BASE_URL")
	if apiKey != "" && baseURL != "" {
		model := os.Getenv("LLM_MODEL")
		if model == "" {
			model = "gpt-4"
		}
		return litellm.NewProvider(apiKey, baseURL, model), nil
	}

	return nil, errors.New("no LLM provider configured: set AURUMCODE_LLM_FIXTURE for offline use, or LLM_API_KEY and LLM_BASE_URL for a live provider")
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
	if fixturePath := os.Getenv("AURUMCODE_LLM_FIXTURE"); fixturePath != "" {
		content, err := os.ReadFile(fixturePath)
		if err != nil {
			return nil, "", fmt.Errorf("reading AURUMCODE_LLM_FIXTURE=%s: %w", fixturePath, err)
		}
		return &review.FakeProvider{
			Response:    string(content),
			NameStr:     model,
			CapturePath: os.Getenv("AURUMCODE_PROMPT_CAPTURE"),
		}, "offline fixture provider", nil
	}

	apiKey := os.Getenv("LLM_API_KEY")
	baseURL := os.Getenv("LLM_BASE_URL")
	if apiKey != "" && baseURL != "" {
		return litellm.NewProvider(apiKey, baseURL, model), "litellm endpoint " + redactedEndpoint(baseURL), nil
	}

	return nil, "", errors.New("no LLM provider is configured to serve it")
}

// redactedEndpoint renders a configured endpoint URL for the stderr
// selection note with any userinfo password already masked at the origin
// (url.Redacted, AUR-432): LLM_BASE_URL=http://user:PASS@host must never
// echo PASS back on the success path. The stderr redaction writer
// additionally replaces the entire userinfo component with the stable
// marker, so not even the username reaches the terminal; this origin fix
// exists so no call path -- present or future -- starts from a string
// that still carries the password. An unparseable value is returned as
// given and left to the writer's structural rules.
func redactedEndpoint(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.User == nil {
		return raw
	}
	return u.Redacted()
}

// reportModelUnavailable prints the clear, actionable error the card
// promises when the model chosen with --modelo cannot review: which model,
// why it is unavailable, and how to configure a provider that serves it --
// including a local one. It returns the command's exit code, 1, the same
// "the review itself failed" code AUR-430 already documents; the one thing
// this path must never do is report an empty review with exit 0.
func reportModelUnavailable(stderr io.Writer, model string, reason error) int {
	fmt.Fprintf(stderr, "aurumcode review: model %q is unavailable: %v\n", model, reason)
	fmt.Fprintf(stderr, "aurumcode review: to serve model %q: set AURUMCODE_LLM_FIXTURE=<response-file> for a deterministic offline run, or set LLM_API_KEY and LLM_BASE_URL to an OpenAI-compatible endpoint that serves it -- a local endpoint works, e.g. LLM_BASE_URL=http://localhost:11434/v1 (ollama) or a litellm proxy in front of any local model -- then re-run with --modelo %s\n", model, model)
	return 1
}

// printFindings prints one line per issue: "<file>:<line>: [<severity>]
// <message>", sorted by (file, line) so the same review result always
// prints in the same order regardless of the order the model listed
// findings in.
// Every model-authored field printed here was already redacted at the
// model boundary (internal/review.redactReviewResult, AUR-432), before the
// trusted rule citation was appended, so the published byte-stable format
// survives while no echoed secret can reach this sink.
func printFindings(stdout io.Writer, result *types.ReviewResult) {
	issues := make([]types.ReviewIssue, len(result.Issues))
	copy(issues, result.Issues)
	sort.SliceStable(issues, func(i, j int) bool {
		if issues[i].File != issues[j].File {
			return issues[i].File < issues[j].File
		}
		return issues[i].Line < issues[j].Line
	})

	if len(issues) == 0 {
		fmt.Fprintln(stdout, "No issues found.")
		return
	}

	for _, issue := range issues {
		fmt.Fprintf(stdout, "%s:%d: [%s] %s\n", issue.File, issue.Line, issue.Severity, issue.Message)
	}
}

// printSecurityFindings prints the AUR-435 security pass section: a blank
// separator line, a header naming the project security standard, then one
// line per finding in the quality block's own "<file>:<line>: [<severity>]
// <message>" format, sorted by (file, line, rule) for determinism -- or the
// honest "No security findings." when the pass matched nothing. The section
// only exists when --seguranca was given, so the published no-flag output
// keeps its exact bytes.
//
// The file path derives from the reviewed diff -- repository-controlled
// input -- so it passes the redaction filter before reaching the sink
// (AUR-432); an ordinary path is filter-identity. The message is trusted
// catalog text (rule description, standard citation, rule citation) and is
// deliberately NOT re-filtered, for the same reason printFindings gives:
// the filter would rewrite catalog spellings like "-secret:" and change the
// published format of a secret-free review.
func printSecurityFindings(stdout io.Writer, filter *redaction.Filter, issues []types.ReviewIssue) {
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Security findings (standards/security-review):")
	if len(issues) == 0 {
		fmt.Fprintln(stdout, "No security findings.")
		return
	}

	sorted := make([]types.ReviewIssue, len(issues))
	copy(sorted, issues)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].File != sorted[j].File {
			return sorted[i].File < sorted[j].File
		}
		if sorted[i].Line != sorted[j].Line {
			return sorted[i].Line < sorted[j].Line
		}
		return sorted[i].RuleID < sorted[j].RuleID
	})
	for _, issue := range sorted {
		fmt.Fprintf(stdout, "%s:%d: [%s] %s\n", filter.Redact(issue.File), issue.Line, issue.Severity, issue.Message)
	}
}
