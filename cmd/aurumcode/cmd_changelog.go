// The changelog command (AUR-509): the required check that every pull
// request adds a useful, concise changelog entry. The rules live in
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
	"github.com/Mpaape/AurumCode/internal/security/redaction"
	"github.com/Mpaape/AurumCode/pkg/types"
)

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
		summary:    "Require a useful, concise changelog entry in the pull request (stable check for branch protection).",
		example:    "aurumcode changelog --base origin/main",
		flags:      func() *flag.FlagSet { fs, _ := newChangelogFlagSet(); return fs },
		run: func(args []string, stdout, stderr io.Writer, _ *redaction.Filter) int {
			return runChangelog(args, stdout, stderr, gitChangelogDiff)
		},
	}
}

// runChangelog exits 0 for a valid entry (or a repository that does not
// require one), 1 for a refused entry or anything it could not establish,
// 2 for a usage error.
func runChangelog(args []string, stdout, stderr io.Writer, differ changelogDiffer) int {
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
	diff, notices, err := differ(f.repo, f.base, f.head)
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
	verdict, required, err := evaluateChangelog(f.repo, policyDir, diff, notices, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "aurumcode changelog: indeterminado: %v\n", err)
		return 1
	}
	if !required {
		fmt.Fprintln(stdout, "changelog: não exigido (changelog_check.mode: off na base)")
		return 0
	}
	if !verdict.OK {
		fmt.Fprintf(stdout, "changelog: reprovado (%s): %s\n", verdict.Reason, verdict.Detail)
		return 1
	}
	fmt.Fprintf(stdout, "changelog: aprovado (%s): %s\n", verdict.Reason, verdict.Detail)
	return 0
}
