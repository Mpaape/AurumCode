package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/changelog"
	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
	"github.com/Mpaape/AurumCode/pkg/types"
)

const (
	aur604Suggest = "changelog_check:\n  mode: suggest\n"
	aur604Sugerir = "changelog_check:\n  mode: sugerir\n"
	aur604Off     = "changelog_check:\n  mode: off\n"
)

// aur604Commits are the subjects the deterministic suggestion is built from.
var aur604Commits = []changelog.Commit{{Subject: "feat: o relatorio aceita filtro por periodo"}}

// aur604Run runs the check, without a model, on a PR that changes only
// app.go in a repository whose config.yml says cfg; policy, when set, is a
// central policy directory. It returns the job summary too.
func aur604Run(t *testing.T, cfg, policy string) (int, string, string, string) {
	t.Helper()
	aur602NoModel(t)
	t.Setenv("AURUMCODE_POLICY", policy)
	summary := filepath.Join(t.TempDir(), "step-summary.md")
	t.Setenv(stepSummaryEnv, summary)
	root := aur509Repo(t, cfg)
	deps := changelogDeps{
		differ:   aur509Differ([]types.DiffFile{{Path: "app.go"}}, nil, nil),
		commits:  func(string, string, string) ([]changelog.Commit, error) { return aur604Commits, nil },
		provider: changelogProviderFromEnv,
		filter:   redaction.NewFilter(),
	}
	var stdout, stderr bytes.Buffer
	code := runChangelogWith([]string{"--base", "base-sha", "--repo", root}, &stdout, &stderr, deps)
	data, err := os.ReadFile(summary)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return code, stdout.String(), stderr.String(), string(data)
}

// AC-001: mode suggest without an entry passes, prints the suggestion ready
// to paste, writes it to the job summary and to the review's PR body, and
// a central policy declaring suggest decides over a repository that
// requires an entry.
func TestAUR604AC001SuggestModeSuggestsAndPasses(t *testing.T) {
	for _, cfg := range []string{aur604Suggest, aur604Sugerir} {
		code, out, errOut, summary := aur604Run(t, cfg, "")
		if code != 0 {
			t.Fatalf("suggest mode failed the check: exit %d\nout %q\nstderr %q", code, out, errOut)
		}
		for _, want := range []string{"sem entrada útil (entrada_ausente)", "modo suggest, não reprova", "entrada sugerida (fonte: commits)", "- o relatorio aceita filtro por periodo"} {
			if !strings.Contains(out, want) {
				t.Fatalf("suggest mode output lacks %q:\n%s", want, out)
			}
		}
		if strings.Contains(out, "reprovado") {
			t.Fatalf("suggest mode reported a refusal:\n%s", out)
		}
		if !strings.Contains(summary, "Sugestão (não reprova a PR)") || !strings.Contains(summary, "filtro por periodo") {
			t.Fatalf("job summary lacks the suggest-mode suggestion:\n%s", summary)
		}
		aur602Pasted(t, out)
	}
	policy := aur509Repo(t, aur604Suggest)
	if code, out, errOut, _ := aur604Run(t, aur509Required, policy); code != 0 || !strings.Contains(out, "modo suggest") {
		t.Fatalf("a central policy in suggest mode did not decide over the repository: exit %d\nout %q\nstderr %q", code, out, errOut)
	}
	code, errOut, body := aur602PR(t, "", "--politica", policy)
	if code != 0 {
		t.Fatalf("the review failed under suggest mode: exit %d stderr %s", code, errOut)
	}
	for _, want := range []string{"Suggested changelog entry", "Suggestion only (the pull request does not fail)", "## Unreleased", "- the review offers the changelog entry"} {
		if !strings.Contains(body, want) {
			t.Fatalf("suggest mode left the suggestion out of the review body (%q):\n%s\nstderr: %s", want, body, errOut)
		}
	}
}

// AC-002: mode required without an entry still fails and still carries the
// suggestion, with the required-mode wording.
func TestAUR604AC002RequiredModeUnchanged(t *testing.T) {
	code, out, errOut, summary := aur604Run(t, aur509Required, "")
	if code != exitChangelogRefused {
		t.Fatalf("required mode without an entry: exit %d, want %d\nout %q\nstderr %q", code, exitChangelogRefused, out, errOut)
	}
	for _, want := range []string{"reprovado (entrada_ausente)", "entrada sugerida (fonte: commits)"} {
		if !strings.Contains(out, want) {
			t.Fatalf("required mode output lacks %q:\n%s", want, out)
		}
	}
	if !strings.Contains(summary, "Esta PR exige entrada de changelog") {
		t.Fatalf("required mode lost its summary wording:\n%s", summary)
	}
	policy := aur509Repo(t, aur509Required)
	if _, errOut, body := aur602PR(t, "", "--politica", policy); !strings.Contains(body, "requires a changelog entry") {
		t.Fatalf("required mode lost the review-body suggestion:\n%s\nstderr: %s", body, errOut)
	}
}

// AC-003: mode off, or no section at all: nothing about the changelog,
// exit 0, and no block in the review body.
func TestAUR604AC003OffOrAbsentDoesNothing(t *testing.T) {
	for name, cfg := range map[string]string{"off": aur604Off, "absent": ""} {
		code, out, errOut, summary := aur604Run(t, cfg, "")
		if code != 0 || !strings.Contains(out, "não exigido") || strings.Contains(out, "entrada sugerida") || summary != "" {
			t.Fatalf("%s: exit %d\nout %q\nstderr %q\nsummary %q", name, code, out, errOut, summary)
		}
	}
	policy := aur509Repo(t, aur604Off)
	if code, errOut, body := aur602PR(t, "", "--politica", policy); code != 0 || strings.Contains(body, "Suggested changelog entry") {
		t.Fatalf("mode off, yet the review body carries a changelog block: exit %d stderr %s\n%s", code, errOut, body)
	}
}

// AC-004: an unknown mode is refused at parse time with the three modes
// listed; every synonym maps to its mode.
func TestAUR604AC004UnknownModeRefused(t *testing.T) {
	_, err := config.Parse([]byte("changelog_check:\n  mode: talvez\n"), "config.yml")
	if err == nil {
		t.Fatal("unknown mode accepted")
	}
	for _, want := range []string{"talvez", "off", "suggest", "required"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error lacks %q: %v", want, err)
		}
	}
	for raw, want := range map[string]config.ChangelogMode{"": config.ChangelogOff, "off": config.ChangelogOff, "desligado": config.ChangelogOff, "suggest": config.ChangelogSuggest, "Sugerir": config.ChangelogSuggest, "required": config.ChangelogRequired, "obrigatório": config.ChangelogRequired} {
		got, err := config.ParseChangelogMode(raw)
		if err != nil || got != want {
			t.Fatalf("mode %q: got %q err %v, want %q", raw, got, err, want)
		}
	}
}
