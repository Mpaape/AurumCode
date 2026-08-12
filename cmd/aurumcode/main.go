// Command aurumcode is the entrypoint for this reconstruction's local code
// review engine (AUR-430). It currently supports one subcommand:
//
//	aurumcode review --base <ref>
//
// which diffs <ref> against HEAD in the git repository rooted at the
// current working directory, sends that diff to an LLM through
// internal/llm.Orchestrator, and prints the findings the model reports.
//
// See docs/specs/AUR-430.md for the full command reference, exit codes and
// an offline, secret-free example.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"sort"

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
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *base == "" {
		fmt.Fprintln(stderr, "aurumcode review: --base is required")
		return 2
	}

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(stderr, "aurumcode review: %v\n", err)
		return 1
	}

	diff, err := computeDiff(cwd, *base)
	if err != nil {
		fmt.Fprintf(stderr, "aurumcode review: %v\n", err)
		return 1
	}

	provider, providerErr := selectProvider()
	if providerErr != nil {
		fmt.Fprintf(stderr, "aurumcode review: %v\n", providerErr)
		return 1
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
		fmt.Fprintf(stderr, "aurumcode review: %v\n", err)
		return 1
	}

	printFindings(stdout, result)
	return 0
}

// computeDiff reads the diff between base and HEAD directly from the git
// object database at repoRoot. See internal/analyzer/gitrepo.go for why
// this reads git's on-disk format in pure Go instead of shelling out to a
// `git` binary.
func computeDiff(repoRoot, base string) (*types.Diff, error) {
	repo, err := analyzer.OpenRepo(repoRoot)
	if err != nil {
		return nil, fmt.Errorf("%s is not a git repository: %w", repoRoot, err)
	}
	diff, err := repo.Diff(base, "HEAD")
	if err != nil {
		return nil, fmt.Errorf("computing diff %s..HEAD: %w", base, err)
	}
	return diff, nil
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
