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
// With the --base path (AUR-441), the command does not pay twice for the
// same file: before calling the review engine, it checks
// internal/review/cache for each changed file's content, under the exact
// model and prompt version this run would use. A file whose cached entry
// matches is never resent; a run where every file matches skips the model
// call entirely. Reused files, when any, are reported on stderr -- stdout
// is byte-identical whether or not caching engaged. The cache lives at
// AURUMCODE_CACHE_DIR when the caller sets it, so two invocations that
// name the same directory reuse each other's entries; without it, each
// invocation gets its own process-scoped cache and behaves exactly as if
// caching were off, so nothing shares state across separate `aurumcode`
// runs unless asked to (see internal/review/cache.ResolveDir). --pr's
// review path is unaffected.
//
// With AUR-443, a user who has never read the source can discover what
// this binary does and run a first review without reading code: top-level
// `aurumcode --help` / `-h` (also `help`) lists every subcommand with a
// one-line summary and a runnable example, on stdout, exit 0; `aurumcode
// --version` (also `version`) prints a build-stamped version, on stdout,
// exit 0 -- see the version var below for how to inject a real value at
// build time. `review --help` now follows the exact convention `docs
// --help` already established (AUR-426): an explicitly requested --help is
// a fulfilled request (stdout, exit 0), and a genuine usage error is a
// refusal (stderr, exit 2) -- the same channel and exit code for both
// subcommands, where before this card `review --help` printed usage to
// stderr with exit 2 and `docs --help` printed to stdout with exit 0.
// Provider-missing errors (selectProvider, reportModelUnavailable) now
// point at a concrete, versioned fixture example
// (tests/fixtures/review/known-problem-response.json) instead of only
// naming the environment variable to set. computeDiff's git-repository and
// ref-resolution errors no longer wrap internal/analyzer's own message
// verbatim: OpenRepo's single error case used to produce a literally
// duplicated "not a git repository: not a git repository (...)" phrase,
// and a ref that does not resolve used to leak a raw filesystem path (the
// pure-Go path, e.g. "open <repo>/.git/refs/heads/<ref>: no such file or
// directory") or git's own "fatal: ..." wording (the git-binary path);
// both are now one clean, actionable sentence, and the two backends report
// the identical text for the same user mistake. See docs/specs/AUR-443.md
// for the --limite exit code and the model response's unused `summary`
// field, both of which this card investigated and deliberately left
// unchanged: docs/specs/AUR-443.md records why.
//
// With AUR-449, `--seguranca` alone no longer needs a provider: the
// security pass it runs (restored by AUR-442) is a deterministic regex
// matcher over the diff's added lines and calls no model, so requiring a
// provider for it was an artificial lock -- the product's only free,
// offline, deterministic path sat behind the one thing that needs a
// credential. When --seguranca is given, --modelo is NOT (an explicit
// model choice is a specific request that must still fail loudly when it
// cannot be served -- reportModelUnavailable, unchanged), and no provider
// is configured at all (selectProvider's errNoProviderConfigured, not some
// other provider failure such as an unreadable fixture path), the command
// now skips the quality review and runs the security pass alone,
// reporting its findings -- and says so plainly on stderr before anything
// prints, never silently. With a provider configured, or with --modelo, or
// without --seguranca, behavior and output are byte-identical to what was
// already published: this card does not touch that path. See
// docs/specs/AUR-449.md.
//
// See docs/specs/AUR-430.md for the base command reference,
// docs/specs/AUR-431.md for the --fail-on gate,
// docs/specs/AUR-436.md for --modelo, docs/specs/AUR-435.md for
// --seguranca, docs/specs/AUR-438.md for --pr, docs/specs/AUR-433.md for
// --limite, docs/specs/AUR-439.md for --check, docs/specs/AUR-441.md for
// the review cache, docs/specs/AUR-443.md for the top-level help, version
// and error-message cleanups, docs/specs/AUR-448.md for the complete
// no-provider fixture shape (rule_id included) and the stderr warning a
// discarded finding now gets, and docs/specs/AUR-449.md for running
// --seguranca without a provider, each with an offline, secret-free
// example.
package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Mpaape/AurumCode/internal/analyzer"
	"github.com/Mpaape/AurumCode/internal/llm"
	"github.com/Mpaape/AurumCode/internal/llm/cost"
	"github.com/Mpaape/AurumCode/internal/llm/provider/litellm"
	"github.com/Mpaape/AurumCode/internal/prompt"
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

	if len(args) == 0 {
		fmt.Fprintln(errW, "usage: aurumcode <review|docs> [flags]")
		return 2
	}

	switch args[0] {
	case "--help", "-h", "help":
		// An explicitly requested top-level --help is a fulfilled request,
		// not a failure: stdout, exit 0, the same convention `review
		// --help` and `docs --help` both follow after this card (see the
		// package doc above). Static text, no repository- or
		// operator-controlled input, so it deliberately bypasses the
		// redaction writer -- exactly like `docs --help`'s own usage text
		// already does (runDocs, cmd/aurumcode/docs.go).
		printTopLevelHelp(stdout)
		return 0
	case "--version", "version":
		printVersion(stdout)
		return 0
	case "review":
		return runReview(args[1:], stdout, errW, filter)
	case "docs":
		return runDocs(args[1:], stdout, errW, filter)
	default:
		fmt.Fprintf(errW, "aurumcode: unknown command %q\n", args[0])
		return 2
	}
}

// printTopLevelHelp is what makes `aurumcode --help` (or `-h`, or `help`)
// answer "what does this command do" without the reader ever opening
// source: every subcommand, one line each, plus one runnable example.
// MUT-001 (docs/specs/AUR-443.md, tests/acceptance/AUR-443.sh) removes the
// "--help", "-h", "help" case above so this function becomes unreachable
// dead code -- `aurumcode --help` then falls through to the `default` arm
// and reports "unknown command", exit 2, which is exactly the discoverability
// regression this card's Outcome exists to fix.
func printTopLevelHelp(stdout io.Writer) {
	fmt.Fprintln(stdout, "usage: aurumcode <command> [flags]")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Commands:")
	fmt.Fprintln(stdout, "  review   Review a git diff with an LLM (or the project's deterministic security rules) and print, gate on, or publish the findings.")
	fmt.Fprintln(stdout, "  docs     Generate project documentation from source code.")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, `Run "aurumcode <command> --help" for that command's own flags.`)
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Example:")
	fmt.Fprintln(stdout, "  aurumcode review --base HEAD~1")
}

// printVersion answers `aurumcode --version` / `aurumcode version`.
func printVersion(stdout io.Writer) {
	fmt.Fprintf(stdout, "aurumcode %s\n", version)
}

func runReview(args []string, stdout, stderr io.Writer, filter *redaction.Filter) int {
	// Buffered, then dispatched by errors.Is(err, flag.ErrHelp) below: the
	// exact pattern `docs --help` already established (runDocs,
	// cmd/aurumcode/docs.go). Before this card, fs.SetOutput(stderr) sent
	// --help's usage text straight to stderr with exit 2 -- the opposite
	// convention from `docs --help` (stdout, exit 0) in the same binary.
	// An explicitly requested --help is a fulfilled request, not a usage
	// error; a genuine usage error (an unknown flag, a value that does not
	// parse) still goes to stderr, exit 2, unchanged. See
	// docs/specs/AUR-443.md.
	var helpBuf bytes.Buffer
	fs := flag.NewFlagSet("review", flag.ContinueOnError)
	fs.SetOutput(&helpBuf)
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
		if errors.Is(err, flag.ErrHelp) {
			io.Copy(stdout, &helpBuf) //nolint:errcheck // best-effort; nothing left to report to on failure
			return 0
		}
		io.Copy(stderr, &helpBuf) //nolint:errcheck // best-effort; nothing left to report to on failure
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
	// AUR-451: the PR path did not read --seguranca/--fail-on/--limite/
	// --modelo at all before this card, so whether each was actually given
	// (as opposed to left at its zero value) has to be known here too --
	// same fs.Visit technique as --pr itself, just extended to the other
	// three string-valued flags the PR path now also validates
	// (prReviewOptions, cmd/aurumcode/pr.go). *seguranca needs no such
	// detection: it is a bool flag with no distinct "set but empty" state,
	// so its zero value already means "not given", exactly like the --base
	// path treats it.
	prGiven := false
	prFailOnGiven := false
	prModeloGiven := false
	prLimiteGiven := false
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "pr":
			prGiven = true
		case "fail-on":
			prFailOnGiven = true
		case "modelo":
			prModeloGiven = true
		case "limite":
			prLimiteGiven = true
		}
	})
	if prGiven {
		return runPRReview(stdout, stderr, *pr, *repoFlag, *publicar, *naLinha, *check, prReviewOptions{
			seguranca: *seguranca,
			failOnSet: prFailOnGiven,
			failOn:    *failOn,
			modeloSet: prModeloGiven,
			modelo:    *modelo,
			limiteSet: prLimiteGiven,
			limite:    *limite,
		})
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

	// AUR-449: the security pass below is deterministic and calls no
	// model (see the package doc above). When --seguranca was given,
	// --modelo was NOT (an explicit model choice is a specific request
	// that must still fail loudly -- reportModelUnavailable below,
	// unchanged), and selectProvider's failure is specifically "nothing is
	// configured" (errNoProviderConfigured, not some other provider
	// failure such as an unreadable fixture path or a broken endpoint),
	// skip the quality review instead of failing the whole command: only
	// the security pass runs, and stderr says plainly, before anything
	// prints, that quality review was skipped and why -- never silence. A
	// caller who attempted configuration and got it wrong is still told
	// the review failed (the else-if below), never silently downgraded.
	// --seguranca combined with a configured provider is unaffected:
	// providerErr is nil then, so this branch is never taken and every
	// line below it runs exactly as already published.
	qualitySkipped := false
	if providerErr != nil && *modelo == "" && *seguranca && errors.Is(providerErr, errNoProviderConfigured) {
		qualitySkipped = true
		fmt.Fprintf(stderr, "aurumcode review: no LLM provider configured: quality review skipped, running --seguranca only (%v)\n", providerErr)
	} else if providerErr != nil {
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

	// AUR-449: everything from here to the security pass talks to a
	// model -- cost tracking, the review cache, GenerateReview itself --
	// so none of it has anything to do when qualitySkipped (no provider,
	// running --seguranca alone). result stays the zero ReviewResult: no
	// quality issues, nothing to gate or print for that section.
	var tracker *cost.Tracker
	var result *types.ReviewResult
	if qualitySkipped {
		result = &types.ReviewResult{}
	} else {
		// --limite (AUR-433): wire internal/llm/cost.Tracker into the
		// orchestrator so it is the one place that estimates the cost and
		// refuses -- before the model is ever called -- when it exceeds the
		// ceiling (see cost.go). Without the flag, tracker stays nil and
		// llm.NewOrchestrator behaves exactly as already published: unmetered.
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

		// AUR-441: do not resend a file whose content, under this exact model
		// and prompt version, a previous run already reviewed. GenerateReview
		// sends whatever diff it is given as one prompt in one call (see
		// cmd/aurumcode/review_cache.go's package doc), so the filtering has to
		// happen here, before the call: toSend keeps only the cache misses, and
		// a cache directory that cannot be opened (cacheErr != nil) degrades to
		// AUR-430's original behavior -- every file sent, every run, exactly as
		// before. Caching is a performance optimization, never a correctness
		// gate. Composed with --limite: printCostEstimate above already priced
		// the FULL diff, unchanged, before this filtering ever runs -- the
		// published --limite estimate and refusal decision are exactly AUR-433's
		// contract, oblivious to caching. A full cache hit means Complete is
		// never called, so the tracker records nothing and printRealCost below
		// correctly reports $0.0000 spent.
		revCache, cacheErr := cache.Open(cache.ResolveDir())
		toSend := diff
		var cacheStatuses []fileCacheStatus
		if cacheErr == nil {
			var missFiles []types.DiffFile
			missFiles, cacheStatuses = partitionByCache(revCache, diff, modelCacheKey(provider))
			toSend = &types.Diff{Files: missFiles}
		}

		// A diff with no files at all is the pre-existing "nothing changed"
		// edge case (e.g. --base HEAD): the published contract has always sent
		// that empty diff to the model like any other (the deterministic
		// offline fixture answers regardless of prompt content), and that must
		// keep happening here even though "zero files to send" would otherwise
		// look identical to "every file was a cache hit". Only a genuine full
		// cache hit -- diff.Files was non-empty and every one of them hit --
		// skips the call.
		if cacheErr != nil || len(toSend.Files) > 0 || len(diff.Files) == 0 {
			result, err = reviewer.GenerateReview(context.Background(), toSend)
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
			if cacheErr == nil {
				persistFreshResults(revCache, cacheStatuses, result.Issues, filter)
			}
		} else {
			result = &types.ReviewResult{}
		}

		// AUR-448: a discard the rule gate (AUR-434) makes is never silent.
		// GenerateReview already names how many findings it discarded and why
		// in result.Metadata["discard_warning"] (internal/review/reviewer.go);
		// this is just the I/O boundary, printing it verbatim to stderr like
		// the cache-reuse note below. It is "" whenever nothing was discarded,
		// so stdout AND stderr stay byte-identical to the published contract
		// on that path. MUT-001 (tests/acceptance/AUR-448.sh) removes exactly
		// this Fprintf while leaving the gate itself untouched. Reached only
		// inside this else branch: result.Metadata is populated by
		// GenerateReview, so there is nothing to warn about on the AUR-449
		// skip path (qualitySkipped's result is the zero ReviewResult, and
		// map access on its nil Metadata is a safe zero-value read).
		if warning := result.Metadata["discard_warning"]; warning != "" {
			fmt.Fprintf(stderr, "aurumcode review: %s\n", warning)
		}

		// AUR-459: the rule gate is not the only place a finding can be
		// dropped. The parser adopts findings the model reported under
		// "line_comments" (internal/prompt.adoptLineComments) and does not
		// adopt a comment that repeats a finding already reported, carries
		// no path, or carries a blank body. Those are announced on the
		// same channel and in the same shape as the rule gate's, because
		// this card's answer to "a finding must never vanish quietly" has
		// to hold for its own conversion too. "" whenever the response
		// carried no line_comments at all, which is every response the
		// current prompt asks for -- so the published stdout AND stderr
		// bytes are untouched on that path.
		if warning := result.Metadata["parse_discard_warning"]; warning != "" {
			fmt.Fprintf(stderr, "aurumcode review: %s\n", warning)
		}

		reused := 0
		if cacheErr == nil {
			reused = mergeCacheHits(result, cacheStatuses, filter)
		}
		if reused > 0 {
			// stdout stays byte-identical with and without cache reuse, exactly
			// like --fail-on's and --modelo's stderr notes above: this line
			// only ever appears on stderr, and only when a file was actually
			// reused.
			fmt.Fprintf(stderr, "aurumcode review: reused %d file(s) from cache (not resent to the model)\n", reused)
		}

		if limiteSet {
			printRealCost(stderr, realCostUSD(tracker, limiteUSD), limiteUSD)
		}
	}

	// The security pass (AUR-435) runs before anything prints, so a broken
	// rules catalog fails loudly with nothing on stdout instead of after a
	// partial report. It is deterministic and calls no model.
	//
	// AUR-450: alongside the findings, the pass now also reports its own
	// coverage on stderr -- how many and which security-category rules of
	// the embedded catalog actually carry a matcher, against how many the
	// category declares in total. Five of the eight security rules are
	// metadata only (internal/review/rules/security.yml, AUR-442's
	// documented, scoped decision); before this card nothing told the
	// caller that, so an empty "No security findings." read exactly like
	// "the code was scanned by the full catalog" when only three of eight
	// rules ever ran. The note goes to stderr, not stdout: it is additive,
	// catalog-derived information, and tests/acceptance/AUR-449.sh pins an
	// exact sha256 of this exact command's stdout with a provider
	// configured -- a byte-for-byte guarantee this card does not own and
	// must not disturb (see docs/specs/AUR-450.md's "Why stderr" section
	// and the "aurumcode review: " stderr notes AUR-433/AUR-441/AUR-448/
	// AUR-449 already established for exactly this situation).
	var securityFindings []types.ReviewIssue
	if *seguranca {
		var coverageApplied []string
		var coverageTotal int
		securityFindings, coverageApplied, coverageTotal, err = review.SecurityScanWithCoverage(diff)
		if err != nil {
			fmt.Fprintf(stderr, "aurumcode review: %v\n", err)
			return 1
		}
		printSecurityCoverage(stderr, coverageApplied, coverageTotal)
	}

	// result.Summary (pkg/types.ReviewResult, parsed by
	// internal/prompt.ResponseParser) is deliberately never printed here.
	// AUR-443 investigated printing it and found the change is blocked, not
	// merely undesired: tests/acceptance/AUR-426.sh's review-contract-broken
	// check and tests/unit/AUR-426.go's byte-for-byte assertion both run
	// this exact command against a fixture whose "summary" field IS
	// non-empty ({"issues":[],"summary":"Nothing to report."}) and require
	// stdout to be exactly "No issues found.\n" -- so printing Summary here
	// would break a published, done card's contract on the very next run of
	// its own acceptance. The parser that produces this field
	// (internal/prompt) is also a read_path this card cannot edit, so
	// removing it from the parse is equally out of reach. See
	// docs/specs/AUR-443.md's "summary field" section for the full
	// evidence trail; this is a recorded, deliberate no-op, not an
	// oversight.
	printNotices(stdout, filter, notices)
	// AUR-449: when quality review was skipped, result carries no issues
	// because no call was ever made -- printing "No issues found." here
	// would assert something the tool never checked. Silently omitting the
	// quality block, with the skip already explained on stderr above, is
	// the honest shape for a section that never ran; result.Issues is
	// otherwise always exactly what the model (or the cache) answered, so
	// this is a no-op whenever a provider was configured.
	if !qualitySkipped {
		printFindings(stdout, result)
	}
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
		// analyzer.OpenRepo has exactly one error case (gitrepo.go), and its
		// own message already says "not a git repository (...)"; wrapping
		// it with another "is not a git repository" prefix used to print a
		// literally duplicated phrase ("... is not a git repository: not a
		// git repository (no .git dir ...)"). That message is fully known
		// (see the package doc above), so this replaces it outright with
		// one clean, actionable sentence instead of restating the same
		// fact twice.
		return nil, nil, fmt.Errorf("%s is not a git repository (no .git directory here, and this path is not itself a bare repository)", repoRoot)
	}
	diff, notices, err := repo.Diff(base, "HEAD")
	if err != nil {
		return nil, nil, cleanRefError(repoRoot, base, err)
	}
	return diff, notices, nil
}

// cleanRefError turns internal/analyzer's ref-resolution error into one
// clean, actionable line instead of the raw internal detail
// analyzer.Repo.Diff's error chain otherwise surfaces to the user:
//
//   - the pure-Go path (gitrepo.go's resolveSymbolic), the one this card's
//     own sealed acceptance profile exercises (bootstrap-readonly-v1: Go
//     and bash, no git binary), wraps a raw *fs.PathError, e.g. "open
//     <repo>/.git/refs/heads/<ref>: no such file or directory";
//   - the git-binary path (gitrepo.go's ResolveRef, when a `git` binary is
//     on PATH) wraps git's own "fatal: Needed a single revision" or
//     "fatal: ... unknown revision or path ..." stderr text.
//
// Both leak implementation detail (a filesystem path under .git, or a
// foreign tool's own wording) that means nothing to a user who has never
// read the source, and the two backends previously disagreed on what that
// detail even looked like. Both are recognized here and replaced by the
// identical clean sentence, naming the ref that failed and how a valid one
// is spelled, so a real repository behaves the same way regardless of
// which backend OpenRepo selected. Only the two prefixes
// analyzer.Repo.Diff itself always wraps with ("resolving base ref %q: " /
// "resolving head ref %q: ", gitrepo.go) are recognized, and only when the
// wrapped cause looks like "no such ref" specifically: any other Diff
// failure (a corrupted tree, an ambiguous parent count, and the like) is
// not concretely measured by this card's dogfooding and keeps its
// pre-existing wrapped message unchanged, so a genuinely new failure is
// never silently hidden behind a guessed diagnosis.
//
// A permission problem (the ref file exists but this process cannot read
// it -- exactly the shape a locked-down runtime like this card's own
// bootstrap-readonly-v1 profile, or any non-root uid, can hit) is
// deliberately NOT folded into "not found": refPathPermissionDenied below
// probes the concrete resource independently and is checked first, so a
// permission failure is reported as what it is instead of a confident,
// wrong "ref not found" -- a review found this card's first cut treated
// every *fs.PathError as "not found" regardless of cause (fixed: the
// pure-Go path now also requires errors.Is(err, fs.ErrNotExist)), and,
// separately, that `git rev-parse`'s own stderr text
// ("fatal: Needed a single revision") is byte-identical for ENOENT and
// EACCES on the git-binary path -- string-matching git's output alone can
// never tell the two apart, which is why the probe exists and is checked
// on both backends rather than only patching the pure-Go one.
//
// Once the switch above has matched one of the two prefixes, this failure
// is KNOWN to be about resolving `ref` specifically, and every branch from
// here on must return a clean, ref-scoped sentence -- never fall through
// to a raw %w wrap of `err` again. A second review found exactly one
// remaining fallthrough that did: `--base feature` against a repository
// whose only branch is `feature/sub` resolves refs/heads/feature to a
// DIRECTORY (a hierarchical branch namespace), so the pure-Go path's
// os.ReadFile fails with EISDIR -- neither fs.ErrNotExist nor
// fs.ErrPermission, so it fell through every existing check into the raw
// wrap, leaking the literal "refs/heads/" path this card's own acceptance
// already treats as a leak everywhere else. refPathIsDirectory closes it
// the same way refPathPermissionDenied closes EACCES: an independent
// probe of the concrete resource, checked on both backends, instead of
// inferring the cause from either backend's own text (the git-binary path
// already said a clean-but-imprecise "not found" for this exact case;
// probing first makes both backends agree on the more precise answer
// instead of only the pure-Go path being fixed).
func cleanRefError(repoRoot, base string, err error) error {
	msg := err.Error()
	ref := ""
	switch {
	case strings.HasPrefix(msg, "resolving base ref "):
		ref = base
	case strings.HasPrefix(msg, "resolving head ref "):
		ref = "HEAD"
	default:
		return fmt.Errorf("computing diff %s..HEAD: %w", base, err)
	}

	if refPathPermissionDenied(repoRoot, ref) {
		return fmt.Errorf("permission denied resolving ref %q in this repository: this process cannot read the ref's storage under .git (or the bare repository root) -- check its file permissions", ref)
	}
	if refPathIsDirectory(repoRoot, ref) {
		return fmt.Errorf("ref %q is not a branch: a directory of refs by that name exists (a hierarchical branch namespace, e.g. %q) -- name the exact branch, not a namespace prefix", ref, ref+"/...")
	}

	var pathErr *fs.PathError
	looksLikeNoSuchRef := (errors.As(err, &pathErr) && errors.Is(err, fs.ErrNotExist)) ||
		strings.Contains(msg, "Needed a single revision") ||
		strings.Contains(msg, "unknown revision or path")
	if looksLikeNoSuchRef {
		return fmt.Errorf("ref %q not found in this repository: expected a branch name, HEAD, a 40-character commit SHA, or a \"<ref>~N\" parent expression", ref)
	}

	// Some other, not concretely measured cause within ref resolution (a
	// corrupted ref file's content, an ambiguous parent count, indirection
	// too deep): still a clean, ref-scoped sentence, never the raw
	// internal chain -- see the note above this function for exactly why a
	// fallthrough here is the defect this fix closes.
	return fmt.Errorf("could not resolve ref %q in this repository", ref)
}

// refPathCandidate computes the exact physical loose-ref location a ref
// name would resolve to under repoRoot -- refs/heads/<ref>, or HEAD for
// ref=="HEAD" -- the one location both analyzer.Repo backends ultimately
// consult for a bare branch name (see cleanRefError's doc above for why
// this card probes it directly instead of inferring from either backend's
// own error text). ok is false when there is nothing meaningful to probe:
// a raw 40-character commit SHA never touches a loose-ref file, and an
// empty ref cannot be resolved to a path.
func refPathCandidate(repoRoot, ref string) (path string, ok bool) {
	if ref == "" || looksLikeCommitSHA(ref) {
		return "", false
	}

	gitDir := filepath.Join(repoRoot, ".git")
	if info, err := os.Stat(gitDir); err != nil || !info.IsDir() {
		// No ".git" subdirectory: repoRoot is itself the bare repository
		// (see analyzer.OpenRepo's own fallback, gitrepo.go).
		gitDir = repoRoot
	}

	if ref == "HEAD" {
		return filepath.Join(gitDir, "HEAD"), true
	}
	return filepath.Join(gitDir, "refs", "heads", filepath.FromSlash(ref)), true
}

// refPathPermissionDenied independently probes whether ref's own loose-ref
// storage under repoRoot is unreadable for a permission reason (EACCES), as
// opposed to genuinely absent (ENOENT) -- the one distinction neither
// backend's own error text reliably carries (see cleanRefError's doc
// above). It reads the exact physical location either backend would have
// consulted, directly, from cmd/aurumcode, independent of which backend
// actually produced the original error: this is one of the things this
// card can do about the ambiguity without editing internal/analyzer (a
// read_path for this card).
//
// It is deliberately conservative: refPathCandidate returning ok==false
// (a raw SHA, an empty ref) leaves it alone, and any outcome other than a
// confirmed permission error (the file does not exist, it is a directory
// -- os.Open succeeds on a directory -- or the open succeeds) returns
// false and lets a later check answer instead -- a probe that cannot say
// anything useful must never manufacture a diagnosis of its own.
func refPathPermissionDenied(repoRoot, ref string) bool {
	candidate, ok := refPathCandidate(repoRoot, ref)
	if !ok {
		return false
	}

	f, err := os.Open(candidate)
	if err == nil {
		f.Close()
		return false
	}
	return errors.Is(err, fs.ErrPermission)
}

// refPathIsDirectory independently probes whether ref's own loose-ref
// location under repoRoot exists as a DIRECTORY rather than a ref file --
// the shape of a hierarchical branch namespace (refs/heads/feature/ holds
// refs/heads/feature/sub, so "feature" alone resolves to a directory, not
// a ref file). os.ReadFile on a directory fails with EISDIR, which is
// neither fs.ErrNotExist nor fs.ErrPermission, so without this check the
// failure fell through cleanRefError's classification entirely into the
// raw internal chain (see cleanRefError's doc above). Same conservative
// contract as refPathPermissionDenied: refPathCandidate returning
// ok==false leaves it alone, and anything other than a confirmed
// directory returns false.
func refPathIsDirectory(repoRoot, ref string) bool {
	candidate, ok := refPathCandidate(repoRoot, ref)
	if !ok {
		return false
	}

	info, err := os.Stat(candidate)
	return err == nil && info.IsDir()
}

// looksLikeCommitSHA reports whether ref is exactly 40 hex characters --
// the one ref shape that resolves without ever touching a loose-ref file,
// so refPathPermissionDenied has nothing to probe for it.
func looksLikeCommitSHA(ref string) bool {
	if len(ref) != 40 {
		return false
	}
	for _, c := range ref {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
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

	// The first-run message names the FORM of a fixture file, not only the
	// environment variable: a user who has never read the source cannot
	// build a valid AURUMCODE_LLM_FIXTURE payload from the variable name
	// alone. AUR-448: the shape shown must be the COMPLETE one the engine
	// accepts -- rule_id included -- because enforceRuleCitations (AUR-434)
	// silently discards a finding whose rule_id is missing, and the
	// pre-AUR-448 shape omitted it: a user who followed it literally got
	// "No issues found." for a fixture that actually planted a finding.
	// security/hardcoded-secret is a real id of the embedded catalog
	// (internal/review/rules/security.yml), not a placeholder. The example
	// file pointer is phrased conditionally because it does not exist
	// outside this repository's own checkout; see docs/specs/AUR-448.md.
	// AUR-449 turned this literal into the errNoProviderConfigured sentinel
	// above (identity preserved via errors.Is, text unchanged) so the
	// --seguranca-only skip can recognize "nothing configured" specifically.
	return nil, errNoProviderConfigured
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
	fmt.Fprintf(stderr, "aurumcode review: to serve model %q: set AURUMCODE_LLM_FIXTURE=<response-file> for a deterministic offline run -- <response-file> is a JSON file shaped like tests/fixtures/review/known-problem-response.json (an \"issues\" array of file/line/severity/message) -- or set LLM_API_KEY and LLM_BASE_URL to an OpenAI-compatible endpoint that serves it -- a local endpoint works, e.g. LLM_BASE_URL=http://localhost:11434/v1 (ollama) or a litellm proxy in front of any local model -- then re-run with --modelo %s\n", model, model)
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

// printSecurityCoverage is the AUR-450 note: how many and which
// security-category rules of the embedded catalog the --seguranca pass
// actually applied (carry a matcher, internal/review.RulesLoader.
// PatternFor), against how many the category declares in total. It always
// prints when --seguranca runs -- before printSecurityFindings, and before
// the findings are even known -- so the found and the empty-result cases
// get the byte-identical coverage line: the figures are catalog-derived,
// never diff-derived, which is exactly why they cannot lie about "No
// security findings." meaning "the whole catalog ran."
//
// It goes to stderr, the same sink and "aurumcode review: " prefix every
// other --seguranca-adjacent note already uses (AUR-433's cost lines,
// AUR-441's cache-reuse note, AUR-448's discard warning, AUR-449's skip
// explanation) -- never stdout, because tests/acceptance/AUR-449.sh pins
// an exact sha256 of stdout for this exact command with a provider
// configured, a byte-for-byte guarantee this card does not own and must
// not disturb. See docs/specs/AUR-450.md's "Why stderr" section.
//
// applied is already sorted by rule id (RulesLoader.AppliedInCategory);
// the message names every one of them, never truncated, because the whole
// point is telling the caller exactly what ran.
func printSecurityCoverage(stderr io.Writer, applied []string, total int) {
	list := "none"
	if len(applied) > 0 {
		list = strings.Join(applied, ", ")
	}
	fmt.Fprintf(stderr, "aurumcode review: security pass applied %d of %d security rules (%s); see internal/review/rules/security.yml for the full catalog\n", len(applied), total, list)
}
