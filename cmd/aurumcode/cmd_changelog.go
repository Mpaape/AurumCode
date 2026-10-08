// The changelog command: the check of the pull request's changelog entry
// in the mode the base declares (off, suggest or required). The rules live in
// internal/changelog (Verify) and internal/config (changelog_check); this
// file reads the two sides from git and maps the verdict to an exit code.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Mpaape/AurumCode/internal/analyzer"
	"github.com/Mpaape/AurumCode/internal/changelog"
	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// exitChangelogRefused is the exit of a refused entry; a suggested entry
// never turns it into a pass.
const exitChangelogRefused = 1

// exitChangelogSuggested is the exit of the suggest mode when the entry is
// missing or not useful: the suggestion is advice, never a failure.
const exitChangelogSuggested = 0

// changelogFlags are the inputs of one check.
type changelogFlags struct {
	base     string
	head     string
	repo     string
	politica string
}

func newChangelogFlagSet() (*flag.FlagSet, *changelogFlags) {
	fs := flag.NewFlagSet("changelog", flag.ContinueOnError)
	f := &changelogFlags{}
	fs.StringVar(&f.base, "base", "", "base commit of the pull request (required); the mode and limits come from this commit's .aurumcode/config.yml, never from the proposed change")
	fs.StringVar(&f.head, "head", "HEAD", "commit with the proposed change")
	fs.StringVar(&f.repo, "repo", ".", "repository directory")
	fs.StringVar(&f.politica, "politica", "", "directory containing a central policy's .aurumcode/ (same convention as `review --politica`); a changelog_check declared there decides alone; default: the AURUMCODE_POLICY environment variable, otherwise no policy")
	return fs, f
}

// changelogDiffer reads the diff of base..head. The real one is git; tests
// inject a failing or synthetic source.
type changelogDiffer func(repoRoot, base, head string) (*types.Diff, []analyzer.DiffNotice, error)

func gitChangelogDiff(repoRoot, base, head string) (*types.Diff, []analyzer.DiffNotice, error) {
	repo, err := analyzer.OpenRepo(repoRoot)
	if err != nil {
		return nil, nil, fmt.Errorf("%s não é um repositório git", repoRoot)
	}
	return repo.Diff(base, head)
}

func changelogSubcommand() subcommand {
	return subcommand{
		name:       "changelog",
		docSection: "Changelog obrigatório (AUR-509)",
		summary:    "Check the pull request's changelog entry: off, suggest (prints the suggested entry, never fails) or required (stable check for branch protection).",
		example:    "aurumcode changelog --base origin/main",
		flags:      func() *flag.FlagSet { fs, _ := newChangelogFlagSet(); return fs },
		run: func(args []string, stdout, stderr io.Writer, filter *redaction.Filter) int {
			return runChangelogWith(args, stdout, stderr, changelogDeps{differ: gitChangelogDiff, commits: gitChangelogCommits, provider: changelogProviderFromEnv, filter: filter})
		},
	}
}

// runChangelog is the check with the real commit and provider sources and
// the environment's redaction filter; tests inject the diff.
func runChangelog(args []string, stdout, stderr io.Writer, differ changelogDiffer) int {
	return runChangelogWith(args, stdout, stderr, changelogDeps{differ: differ, commits: gitChangelogCommits, provider: changelogProviderFromEnv, filter: redaction.FromEnv()})
}

// runChangelogWith exits 0 for a valid entry, a repository in mode off, or
// any entry in mode suggest; 1 for a refused entry in mode required or
// anything it could not establish; 2 for a usage error. A missing or weak
// entry also prints the suggested entry; the suggestion never changes the
// exit code.
func runChangelogWith(args []string, stdout, stderr io.Writer, deps changelogDeps) int {
	fs, f := newChangelogFlagSet()
	if exit, ok := parseSubcommandFlags("changelog", fs, args, stdout, stderr); !ok {
		return exit
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "aurumcode changelog: argumento inesperado %q\n", fs.Arg(0))
		return 2
	}
	if f.base == "" {
		fmt.Fprintln(stderr, "aurumcode changelog: --base é obrigatório")
		return 2
	}
	diff, notices, err := deps.differ(f.repo, f.base, f.head)
	if err != nil {
		fmt.Fprintf(stderr, "aurumcode changelog: indeterminado: não foi possível obter o diff: %v\n", err)
		return 1
	}
	if diff == nil {
		fmt.Fprintln(stderr, "aurumcode changelog: indeterminado: diff vazio de origem desconhecida")
		return 1
	}
	policyDir := strings.TrimSpace(f.politica)
	if policyDir == "" {
		policyDir = strings.TrimSpace(os.Getenv("AURUMCODE_POLICY"))
	}
	ev, err := evaluateChangelog(f.repo, policyDir, diff, notices, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "aurumcode changelog: indeterminado: %v\n", err)
		return 1
	}
	if ev.req == nil {
		fmt.Fprintln(stdout, "changelog: não exigido (changelog_check.mode: off na base)")
		return 0
	}
	verdict, req := ev.verdict, ev.req
	if verdict.OK {
		fmt.Fprintf(stdout, "changelog: aprovado (%s): %s\n", verdict.Reason, verdict.Detail)
		return 0
	}
	if ev.mode == config.ChangelogSuggest {
		return deps.suggestOnly(*req, verdict, f, diff, stdout, stderr)
	}
	fmt.Fprintf(stdout, "changelog: reprovado (%s): %s\n", verdict.Reason, verdict.Detail)
	if verdict.Reason != changelog.ReasonIndeterminate {
		deps.offerSuggestion(*req, ev.mode, f, diff, stdout, stderr)
	}
	return exitChangelogRefused
}

// suggestOnly is the suggest mode for an entry the required mode would
// refuse: it names what is missing, prints the suggestion and passes. An
// unreadable changelog file gets a note and no suggestion.
func (d changelogDeps) suggestOnly(req changelog.Requirement, verdict changelog.Verdict, f *changelogFlags, diff *types.Diff, stdout, stderr io.Writer) int {
	fmt.Fprintf(stdout, "changelog: sem entrada útil (%s): %s; modo suggest, não reprova\n", verdict.Reason, verdict.Detail)
	if verdict.Reason != changelog.ReasonIndeterminate {
		d.offerSuggestion(req, config.ChangelogSuggest, f, diff, stdout, stderr)
	}
	return exitChangelogSuggested
}
