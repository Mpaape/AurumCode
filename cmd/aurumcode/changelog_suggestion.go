// The suggested changelog entry: when a pull request lacks a useful entry,
// the required mode still fails and the suggest mode passes, and both offer
// the lines to paste. The configured model writes them from
// the changed paths and commit subjects; without a model (or with an
// unusable answer) they come deterministically from the commit subjects.
// The text is redacted once and the same string reaches every sink: the
// log, the job summary and the review's PR body.
package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Mpaape/AurumCode/internal/analyzer"
	"github.com/Mpaape/AurumCode/internal/changelog"
	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/i18n"
	"github.com/Mpaape/AurumCode/internal/llm"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// stepSummaryEnv names the file GitHub Actions renders as the job summary.
const stepSummaryEnv = "GITHUB_STEP_SUMMARY"

// changelogLogLanguage is the language of the check's own log lines.
const changelogLogLanguage = string(i18n.Portuguese)

// changelogCommitSource reads the commit messages of base..head.
type changelogCommitSource func(repoRoot, base, head string) ([]changelog.Commit, error)

// changelogProviderSource returns the configured model, nil when none is.
type changelogProviderSource func() (llm.Provider, error)

// changelogDeps are the sources of one check run.
type changelogDeps struct {
	differ   changelogDiffer
	commits  changelogCommitSource
	provider changelogProviderSource
	filter   *redaction.Filter
}

func gitChangelogCommits(repoRoot, base, head string) ([]changelog.Commit, error) {
	repo, err := analyzer.OpenRepo(repoRoot)
	if err != nil {
		return nil, err
	}
	infos, err := repo.Commits(base, head)
	if err != nil {
		return nil, err
	}
	commits := make([]changelog.Commit, 0, len(infos))
	for _, info := range infos {
		commits = append(commits, changelog.Commit{Subject: info.Subject, Body: info.Body, Hash: info.Hash})
	}
	return commits, nil
}

// changelogProviderFromEnv selects the provider the same way the review
// does (fixture, catalog profile, OpenAI-compatible endpoint).
func changelogProviderFromEnv() (llm.Provider, error) {
	p, _, err := providerFromEnv("fixture", strings.TrimSpace(os.Getenv("LLM_MODEL")))
	return p, err
}

// offerSuggestion prints the suggested entry for a missing or weak entry and
// appends it to the job summary when one is configured. An indeterminate
// verdict gets no suggestion: the file exists but could not be read.
func (d changelogDeps) offerSuggestion(req changelog.Requirement, mode config.ChangelogMode, f *changelogFlags, diff *types.Diff, stdout, stderr io.Writer) {
	commits, err := d.commits(f.repo, f.base, f.head)
	if err != nil {
		fmt.Fprintf(stderr, "aurumcode changelog: commits da PR indisponíveis para a sugestão: %v\n", err)
	}
	s := suggestChangelogEntry(req, diffPaths(diff), commits, d.provider, stderr)
	if s.Empty() {
		fmt.Fprintln(stdout, "changelog: sem entrada sugerida (nenhum commit ou resposta do modelo utilizável)")
		return
	}
	block := redactSuggestion(d.filter, s.Block())
	fmt.Fprintf(stdout, "changelog: entrada sugerida (fonte: %s); cole na seção %s de %s:\n\n%s", s.Source, req.Section, req.File, block)
	if path := strings.TrimSpace(os.Getenv(stepSummaryEnv)); path != "" {
		if err := appendStepSummary(path, changelogSuggestionMarkdown(req, mode, s.Source, block, changelogLogLanguage)); err != nil {
			fmt.Fprintf(stderr, "aurumcode changelog: resumo do job não gravado: %v\n", err)
		}
	}
}

// suggestChangelogEntry asks the model when one is configured and falls
// back to the commit subjects; neither path can fail the check.
func suggestChangelogEntry(req changelog.Requirement, paths []string, commits []changelog.Commit, source changelogProviderSource, stderr io.Writer) changelog.Suggestion {
	if source != nil {
		provider, err := source()
		switch {
		case err != nil:
			fmt.Fprintf(stderr, "aurumcode changelog: modelo indisponível para a sugestão: %v\n", err)
		case provider != nil:
			opts := llm.DefaultOptions()
			opts.JSONMode = true
			resp, err := provider.Complete(req.SuggestionPrompt(paths, commits), opts)
			if err == nil {
				s, perr := req.SuggestFromModel(resp.Text)
				if perr == nil {
					return s
				}
				err = perr
			}
			fmt.Fprintf(stderr, "aurumcode changelog: sugestão do modelo descartada, usando os commits: %v\n", err)
		}
	}
	return req.SuggestFromCommits(commits)
}

// redactSuggestion is the single redaction point of the suggested entry.
func redactSuggestion(filter *redaction.Filter, block string) string {
	if filter == nil {
		filter = redaction.FromEnv()
	}
	return filter.Redact(block)
}

// changelogSuggestionMarkdown is the block shown in the job summary and
// the PR body: heading, how to use it, and the entry in a fenced block.
func changelogSuggestionMarkdown(req changelog.Requirement, mode config.ChangelogMode, source, block, language string) string {
	intro := "changelog.suggestion.intro"
	if mode == config.ChangelogSuggest {
		intro = "changelog.suggestion.intro_suggest"
	}
	var b strings.Builder
	b.WriteString(i18n.Text(language, "changelog.suggestion.heading") + "\n\n")
	b.WriteString(i18n.Format(language, intro, req.Section, req.File, source) + "\n\n")
	b.WriteString("```markdown\n" + strings.TrimRight(block, "\n") + "\n```\n")
	return b.String()
}

func appendStepSummary(path, text string) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.WriteString(f, text); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
