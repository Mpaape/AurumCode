// The realimentacao command: the feedback loop of the gate. It
// collects usage signals from GitHub, asks the model to group them into
// proposals and opens one pull request in the policy repository. The rules
// live in internal/feedback; this file parses flags and wires dependencies.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/feedback"
	"github.com/Mpaape/AurumCode/internal/llm"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
)

type feedbackFlags struct {
	org, repos, policy, since string
	before, after             string
	publish, measure          bool
}

func newFeedbackFlagSet() (*flag.FlagSet, *feedbackFlags) {
	fs := flag.NewFlagSet("realimentacao", flag.ContinueOnError)
	f := &feedbackFlags{}
	fs.StringVar(&f.org, "org", "", "organization whose repositories are read for signals")
	fs.StringVar(&f.repos, "repos", "", "comma-separated owner/name repositories read for signals (in addition to --org)")
	fs.StringVar(&f.policy, "repo-politica", "", "owner/name of the central policy repository that receives the single feedback pull request")
	fs.StringVar(&f.since, "desde", "", "RFC 3339 instant; /aurum perdeu comments before it are not read (default: the bounded recent pages)")
	fs.StringVar(&f.before, "medicao-antes", "", "AUR-523 corpus report (multilang-report.json) of the current policy")
	fs.StringVar(&f.after, "medicao-depois", "", "AUR-523 corpus report of the proposed policy")
	fs.BoolVar(&f.publish, "publicar", false, "open or update the pull request in the policy repository (default: print the plan only)")
	fs.BoolVar(&f.measure, "medir", false, "only compare --medicao-antes and --medicao-depois; exit 1 on a regression or a missing report")
	return fs, f
}

func feedbackSubcommand() subcommand {
	return subcommand{
		name:       "realimentacao",
		docSection: "Realimentação da política (AUR-532)",
		summary:    "Turn usage signals (dismissed alerts, fixed findings, /aurum perdeu) into one pull request to the policy repository.",
		example:    "aurumcode realimentacao --org example --repo-politica example/politica --publicar",
		flags:      func() *flag.FlagSet { fs, _ := newFeedbackFlagSet(); return fs },
		run: func(args []string, stdout, stderr io.Writer, filter *redaction.Filter) int {
			return runFeedback(args, stdout, stderr, filter, os.Getenv)
		},
	}
}

func runFeedback(args []string, stdout, stderr io.Writer, filter *redaction.Filter, getenv func(string) string) int {
	fs, f := newFeedbackFlagSet()
	if exit, ok := parseSubcommandFlags("realimentacao", fs, args, stdout, stderr); !ok {
		return exit
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(stderr, "aurumcode realimentacao: argumento inesperado %q\n", fs.Arg(0))
		return 2
	}
	before, after, err := readCorpusReports(f.before, f.after)
	if err != nil {
		fmt.Fprintf(stderr, "aurumcode realimentacao: %v\n", err)
		return 1
	}
	if f.measure {
		cmp := feedback.Compare(before, after)
		fmt.Fprint(stdout, cmp.Markdown)
		if !cmp.Measured || cmp.Regression {
			return 1
		}
		return 0
	}
	if f.policy == "" || (f.org == "" && f.repos == "") {
		fmt.Fprintln(stderr, "aurumcode realimentacao: --repo-politica e --org ou --repos são obrigatórios")
		return 2
	}
	opts, err := feedbackOptions(f, filter, getenv)
	if err != nil {
		fmt.Fprintf(stderr, "aurumcode realimentacao: %v\n", err)
		return 1
	}
	opts.Before, opts.After = before, after
	res, err := feedback.Run(opts)
	if err != nil {
		fmt.Fprintf(stderr, "aurumcode realimentacao: %v\n", err)
		return 1
	}
	printFeedbackResult(stdout, res, f.publish)
	return 0
}

// feedbackOptions assembles the GitHub client, the repositories and the
// model. A missing provider is only an error when there is something to
// propose, which Run decides.
func feedbackOptions(f *feedbackFlags, filter *redaction.Filter, getenv func(string) string) (feedback.Options, error) {
	api, err := config.GitHubAPIURL(getenv)
	if err != nil {
		return feedback.Options{}, err
	}
	// AURUMCODE_SIGNALS_TOKEN reads the organization; GITHUB_TOKEN writes
	// only the policy repository. Without the first, GITHUB_TOKEN reads too.
	readToken := getenv("AURUMCODE_SIGNALS_TOKEN")
	if readToken == "" {
		readToken = getenv("GITHUB_TOKEN")
	}
	gh := &feedback.GitHub{BaseURL: api, Token: readToken}
	writer := &feedback.GitHub{BaseURL: api, Token: getenv("GITHUB_TOKEN")}
	var repos []string
	if f.org != "" {
		if repos, err = gh.OrgRepos(f.org); err != nil {
			return feedback.Options{}, fmt.Errorf("repositórios de %s: %w", f.org, err)
		}
	}
	for _, r := range strings.Split(f.repos, ",") {
		if r = strings.TrimSpace(r); r != "" {
			repos = append(repos, r)
		}
	}
	var provider llm.Provider
	p, err := selectProvider()
	switch {
	case err == nil:
		provider = p
	case !errors.Is(err, errNoProviderConfigured):
		return feedback.Options{}, err
	}
	return feedback.Options{
		GitHub: gh, PolicyGitHub: writer, Repos: repos, Policy: f.policy, Since: f.since,
		Provider: provider, Filter: filter, Publish: f.publish,
	}, nil
}

func readCorpusReports(beforePath, afterPath string) (*feedback.CorpusReport, *feedback.CorpusReport, error) {
	read := func(path string) (*feedback.CorpusReport, error) {
		if path == "" {
			return nil, nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		return feedback.ParseCorpusReport(data)
	}
	before, err := read(beforePath)
	if err != nil {
		return nil, nil, fmt.Errorf("--medicao-antes: %w", err)
	}
	after, err := read(afterPath)
	if err != nil {
		return nil, nil, fmt.Errorf("--medicao-depois: %w", err)
	}
	return before, after, nil
}

func printFeedbackResult(stdout io.Writer, res feedback.Result, publish bool) {
	fmt.Fprintf(stdout, "realimentação: %d sinal(is), %d novo(s)\n", res.Signals, res.Fresh)
	for _, n := range res.Notes {
		fmt.Fprintf(stdout, "nota: %s\n", n)
	}
	switch {
	case !res.Planned:
		fmt.Fprintln(stdout, "nada novo: nenhuma PR aberta ou alterada")
	case publish:
		fmt.Fprintf(stdout, "PR #%d no repositório da política (%s)\n", res.PullRequest, feedback.Branch)
	default:
		fmt.Fprintf(stdout, "plano (sem --publicar nada foi gravado): %s\n", res.Plan.Title)
		for _, p := range res.Plan.Paths() {
			fmt.Fprintf(stdout, "  %s\n", p)
		}
		fmt.Fprintln(stdout)
		fmt.Fprint(stdout, res.Plan.Body)
	}
}
