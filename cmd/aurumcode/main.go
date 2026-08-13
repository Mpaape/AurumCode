// Command aurumcode is the entrypoint for this reconstruction's local code
// review engine (AUR-430). It currently supports one subcommand:
//
//	aurumcode review --base <ref> [--fail-on <level>] [--modelo <nome>]
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
// See docs/specs/AUR-430.md for the base command reference,
// docs/specs/AUR-431.md for the --fail-on gate and
// docs/specs/AUR-436.md for --modelo, each with an offline, secret-free
// example.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/Mpaape/AurumCode/internal/analyzer"
	"github.com/Mpaape/AurumCode/internal/llm"
	"github.com/Mpaape/AurumCode/internal/llm/provider/litellm"
	"github.com/Mpaape/AurumCode/internal/prompt"
	"github.com/Mpaape/AurumCode/internal/review"
	"github.com/Mpaape/AurumCode/pkg/types"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr *os.File) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: aurumcode <review> [flags]")
		return 2
	}

	switch args[0] {
	case "review":
		return runReview(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "aurumcode: unknown command %q\n", args[0])
		return 2
	}
}

func runReview(args []string, stdout, stderr *os.File) int {
	fs := flag.NewFlagSet("review", flag.ContinueOnError)
	fs.SetOutput(stderr)
	base := fs.String("base", "", "ref to diff against HEAD (required), e.g. HEAD~1 or a branch name")
	failOn := fs.String("fail-on", "", "minimum severity that makes the command exit 3: high|error, medium|warning, low|info (default: findings never change the exit code)")
	modelo := fs.String("modelo", "", "model that reviews, e.g. local, llama3 or gpt-4; served offline via AURUMCODE_LLM_FIXTURE or live via LLM_API_KEY and LLM_BASE_URL (default: AUR-430's selection, unchanged)")
	if err := fs.Parse(args); err != nil {
		return 2
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
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "fail-on" {
			failOnSet = true
		}
		if f.Name == "modelo" {
			modeloSet = true
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

	orchestrator := llm.NewOrchestrator(provider, nil, nil)
	reviewer := review.NewReviewer(orchestrator, review.DefaultConfig())

	result, err := reviewer.GenerateReview(context.Background(), diff)
	if err != nil {
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

	printNotices(stdout, notices)
	printFindings(stdout, result)

	// The --fail-on CI gate (AUR-431): after the findings have printed
	// exactly as they always do, exit with the distinct, documented code 3
	// when any finding sits at the chosen severity or above. The note goes
	// to stderr so stdout stays byte-identical with and without the flag.
	if threshold > 0 {
		if n := countAtOrAbove(result.Issues, threshold); n > 0 {
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
func printNotices(stdout *os.File, notices []analyzer.DiffNotice) {
	for _, n := range notices {
		fmt.Fprintln(stdout, n.Message)
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
		return &review.FakeProvider{Response: string(content), NameStr: "fixture"}, nil
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
		return &review.FakeProvider{Response: string(content), NameStr: model}, "offline fixture provider", nil
	}

	apiKey := os.Getenv("LLM_API_KEY")
	baseURL := os.Getenv("LLM_BASE_URL")
	if apiKey != "" && baseURL != "" {
		return litellm.NewProvider(apiKey, baseURL, model), "litellm endpoint " + baseURL, nil
	}

	return nil, "", errors.New("no LLM provider is configured to serve it")
}

// reportModelUnavailable prints the clear, actionable error the card
// promises when the model chosen with --modelo cannot review: which model,
// why it is unavailable, and how to configure a provider that serves it --
// including a local one. It returns the command's exit code, 1, the same
// "the review itself failed" code AUR-430 already documents; the one thing
// this path must never do is report an empty review with exit 0.
func reportModelUnavailable(stderr *os.File, model string, reason error) int {
	fmt.Fprintf(stderr, "aurumcode review: model %q is unavailable: %v\n", model, reason)
	fmt.Fprintf(stderr, "aurumcode review: to serve model %q: set AURUMCODE_LLM_FIXTURE=<response-file> for a deterministic offline run, or set LLM_API_KEY and LLM_BASE_URL to an OpenAI-compatible endpoint that serves it -- a local endpoint works, e.g. LLM_BASE_URL=http://localhost:11434/v1 (ollama) or a litellm proxy in front of any local model -- then re-run with --modelo %s\n", model, model)
	return 1
}

// printFindings prints one line per issue: "<file>:<line>: [<severity>]
// <message>", sorted by (file, line) so the same review result always
// prints in the same order regardless of the order the model listed
// findings in.
func printFindings(stdout *os.File, result *types.ReviewResult) {
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
