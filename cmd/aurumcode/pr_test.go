package main

import (
	"context"
	"encoding/base64"
	"flag"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/git/githubclient"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// TestParseOwnerRepo pins --repo's accepted and rejected shapes.
func TestParseOwnerRepo(t *testing.T) {
	owner, repo, err := parseOwnerRepo("dono/projeto")
	if err != nil {
		t.Fatalf("parseOwnerRepo(%q): unexpected error %v", "dono/projeto", err)
	}
	if owner != "dono" || repo != "projeto" {
		t.Fatalf("parseOwnerRepo(%q) = (%q, %q), want (%q, %q)", "dono/projeto", owner, repo, "dono", "projeto")
	}

	for _, bad := range []string{"", "dono", "/projeto", "dono/", "dono/projeto/extra", "/"} {
		if _, _, err := parseOwnerRepo(bad); err == nil {
			t.Errorf("parseOwnerRepo(%q): expected an error, got none", bad)
		}
	}
}

func TestLoadPullRequestConfigReadsHeadRefWithoutCheckout(t *testing.T) {
	content := base64.StdEncoding.EncodeToString([]byte("review:\n  language: pt-BR\n"))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/owner/repo/contents/.aurumcode/config.yml" {
			t.Fatalf("path = %q, want repository config path", r.URL.Path)
		}
		if r.URL.Query().Get("ref") != "head-sha" {
			t.Fatalf("ref = %q, want head-sha", r.URL.Query().Get("ref"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"content":%q,"encoding":"base64"}`, content)
	}))
	defer server.Close()

	cfg, language, err := loadPullRequestConfig(
		context.Background(),
		githubclient.NewClientWithBaseURL("", server.URL),
		"owner", "repo", "head-sha", "",
	)
	if err != nil {
		t.Fatalf("loadPullRequestConfig: %v", err)
	}
	if cfg == nil || language != "pt-BR" {
		t.Fatalf("config/language = %+v/%q, want config/pt-BR", cfg, language)
	}
}

func TestLoadPullRequestConfigKeepsGateSettingsOnBase(t *testing.T) {
	base := base64.StdEncoding.EncodeToString([]byte("rules:\n  security/hardcoded-secret:\n    severity: error\nreview:\n  language: en-US\n"))
	head := base64.StdEncoding.EncodeToString([]byte("rules:\n  security/hardcoded-secret:\n    enabled: false\nreview:\n  language: pt-BR\n"))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		content := base
		if r.URL.Query().Get("ref") == "head-sha" {
			content = head
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"content":%q,"encoding":"base64"}`, content)
	}))
	defer server.Close()

	cfg, language, err := loadPullRequestConfig(
		context.Background(),
		githubclient.NewClientWithBaseURL("", server.URL),
		"owner", "repo", "head-sha", "base-sha",
	)
	if err != nil {
		t.Fatalf("loadPullRequestConfig: %v", err)
	}
	if language != "pt-BR" {
		t.Fatalf("language = %q, want pt-BR from the head preference", language)
	}
	rule := cfg.Rules["security/hardcoded-secret"]
	if rule.Enabled != nil || rule.Severity != "error" {
		t.Fatalf("gate settings = %+v, want base severity and no head disable", rule)
	}
}

// TestConvertDiff pins the field-by-field mirror from
// internal/git/githubclient's package-local diff shape to pkg/types.Diff
// this card owns (the AUR-437 reviewer's documented finding: the client's
// types are a deliberate mirror of pkg/types, and the conversion belongs
// here). Nothing is dropped, reordered or renamed.
func TestConvertDiff(t *testing.T) {
	in := &githubclient.Diff{
		Files: []githubclient.DiffFile{
			{
				Path: "cmdb/settings.go",
				Lang: "go",
				Hunks: []githubclient.DiffHunk{
					{
						OldStart: 1, OldLines: 3, NewStart: 1, NewLines: 4,
						Lines: []string{" package cmdb", "", "+// nova linha", " const RetryLimit = 3"},
					},
				},
			},
		},
	}

	out := convertDiff(in)
	if len(out.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(out.Files))
	}
	gotFile := out.Files[0]
	wantFile := in.Files[0]
	if gotFile.Path != wantFile.Path || gotFile.Lang != wantFile.Lang {
		t.Fatalf("file fields diverged: got %+v, want path=%q lang=%q", gotFile, wantFile.Path, wantFile.Lang)
	}
	if len(gotFile.Hunks) != 1 {
		t.Fatalf("expected 1 hunk, got %d", len(gotFile.Hunks))
	}
	gotHunk, wantHunk := gotFile.Hunks[0], wantFile.Hunks[0]
	if gotHunk.OldStart != wantHunk.OldStart || gotHunk.OldLines != wantHunk.OldLines ||
		gotHunk.NewStart != wantHunk.NewStart || gotHunk.NewLines != wantHunk.NewLines {
		t.Fatalf("hunk range diverged: got %+v, want %+v", gotHunk, wantHunk)
	}
	if len(gotHunk.Lines) != len(wantHunk.Lines) {
		t.Fatalf("expected %d lines, got %d", len(wantHunk.Lines), len(gotHunk.Lines))
	}
	for i := range wantHunk.Lines {
		if gotHunk.Lines[i] != wantHunk.Lines[i] {
			t.Errorf("line %d: got %q, want %q", i, gotHunk.Lines[i], wantHunk.Lines[i])
		}
	}

	// The conversion must copy, not alias: mutating the source must not
	// reach the converted diff.
	in.Files[0].Hunks[0].Lines[0] = "MUTATED"
	if out.Files[0].Hunks[0].Lines[0] == "MUTATED" {
		t.Fatal("convertDiff aliased the source hunk's Lines slice")
	}
}

// TestIsInlineEligible pins the classification AC-001 depends on: a finding
// on a line the diff added is inline-eligible; a finding on a file the
// diff never touched, or on a line that file's hunks never added
// (including a context line and a line number past every hunk), is not --
// and must still be published, as a general comment (see runPRReview).
func TestIsInlineEligible(t *testing.T) {
	diff := &types.Diff{
		Files: []types.DiffFile{
			{
				Path: "cmdb/settings.go",
				Hunks: []types.DiffHunk{
					{
						OldStart: 1, OldLines: 3, NewStart: 1, NewLines: 4,
						Lines: []string{" package cmdb", " ", "+// limite de retentativas elevado para espelhos lentos", " const RetryLimit = 3"},
					},
				},
			},
			{
				Path: "docs/notas.md",
				Hunks: []types.DiffHunk{
					{OldStart: 1, OldLines: 1, NewStart: 1, NewLines: 2, Lines: []string{" # Notas", "+Nova linha de documentacao."}},
				},
			},
		},
	}

	cases := []struct {
		name   string
		issue  types.ReviewIssue
		inline bool
	}{
		{"added line", types.ReviewIssue{File: "cmdb/settings.go", Line: 3}, true},
		{"added line, second file", types.ReviewIssue{File: "docs/notas.md", Line: 2}, true},
		{"context line in a touched file", types.ReviewIssue{File: "cmdb/settings.go", Line: 1}, false},
		{"line past every hunk", types.ReviewIssue{File: "docs/notas.md", Line: 99}, false},
		{"file the diff never touched", types.ReviewIssue{File: "config/demo-tokens.txt", Line: 4}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isInlineEligible(diff, tc.issue); got != tc.inline {
				t.Errorf("isInlineEligible(%+v) = %v, want %v", tc.issue, got, tc.inline)
			}
		})
	}
}

// TestSortedIssuesDeterministic pins the (file, line) publish order AC-001
// needs: "repetir a execucao sobre a mesma entrada produz a mesma saida"
// covers the sequence of PostReviewComment/PostIssueComment calls, not
// only stdout, so the order the model happened to list issues in must not
// leak into the order they are published.
func TestSortedIssuesDeterministic(t *testing.T) {
	in := []types.ReviewIssue{
		{File: "docs/notas.md", Line: 99},
		{File: "cmdb/settings.go", Line: 3},
		{File: "cmdb/settings.go", Line: 1},
	}
	got := sortedIssues(in)
	want := []struct {
		file string
		line int
	}{
		{"cmdb/settings.go", 1},
		{"cmdb/settings.go", 3},
		{"docs/notas.md", 99},
	}
	if len(got) != len(want) {
		t.Fatalf("expected %d issues, got %d", len(want), len(got))
	}
	for i, w := range want {
		if got[i].File != w.file || got[i].Line != w.line {
			t.Errorf("position %d: got %s:%d, want %s:%d", i, got[i].File, got[i].Line, w.file, w.line)
		}
	}
	// sortedIssues must not mutate its input's order.
	if in[0].File != "docs/notas.md" || in[0].Line != 99 {
		t.Fatal("sortedIssues mutated the caller's slice order")
	}
}

func TestFormatReviewSummaryIsAReviewNotAnExecutionTranscript(t *testing.T) {
	result := &types.ReviewResult{
		Verdict:   "changes_requested",
		Strengths: []string{"The change keeps the public API stable."},
		Issues: []types.ReviewIssue{{
			File:         "internal/example.go",
			Line:         12,
			Severity:     "error",
			Message:      "The new branch can return a nil dependency.",
			Impact:       "The next call panics instead of returning an error.",
			Evidence:     "The changed branch returns nil without checking the constructor result.",
			Suggestion:   "Propagate the constructor error.",
			Verification: "Run the focused package test.",
		}},
		Suggestions: []types.ReviewSuggestion{{
			Title:        "Add a regression test",
			Description:  "Cover the nil dependency branch.",
			Kind:         "code",
			File:         "internal/example_test.go",
			StartLine:    20,
			EndLine:      22,
			ProposedCode: "func TestNilDependency(t *testing.T) {\n\t// assert the error\n}",
			Rationale:    "The test locks the failure mode at the change site.",
			Verification: "Run the focused package test.",
		}},
		CIAnalysis: []types.CIAnalysis{{
			Check:            "unit-tests",
			Status:           "failure",
			Cause:            "The status alone is insufficient to establish a root cause.",
			Evidence:         "Only the check status was available.",
			Fix:              "Open the failed check details.",
			NextVerification: "Rerun unit-tests after the confirmed fix.",
		}},
		TestPlan:    []string{"Run the focused package test."},
		Limitations: []string{"The failed CI log was unavailable."},
		Summary:     "The change is close, but one correctness issue must be fixed.",
	}

	comment := formatReviewSummary(result)
	t.Logf("generated review comment:\n%s", comment)
	for _, want := range []string{
		"## AurumCode code review",
		"**Verdict:** Changes requested",
		"### Strengths",
		"### Findings",
		"### Suggestions",
		"### CI status",
		"Cause:",
		"Proposed implementation:",
		"Rationale:",
		"### Tests",
		"### Review limits",
	} {
		if !strings.Contains(comment, want) {
			t.Errorf("summary missing %q:\n%s", want, comment)
		}
	}
	for _, forbidden := range []string{
		"go test ./...",
		"stderr",
		"exit code",
		"/tmp/",
	} {
		if strings.Contains(comment, forbidden) {
			t.Errorf("summary contains execution detail %q:\n%s", forbidden, comment)
		}
	}
}

func TestFormatReviewSummaryUsesFilteredResult(t *testing.T) {
	result := &types.ReviewResult{
		Verdict:     "changes_requested",
		Summary:     "A stale model summary claims a credential is hardcoded.",
		Suggestions: []types.ReviewSuggestion{{Title: ""}},
	}
	comment := formatReviewSummary(result)
	if !strings.Contains(comment, "**Verdict:** Approve") {
		t.Fatalf("expected an all-clear result to approve, got:\n%s", comment)
	}
	if strings.Contains(comment, result.Summary) {
		t.Fatalf("published comment copied the untrusted model summary:\n%s", comment)
	}
}

func TestFormatReviewSummaryWithDiffIncludesCodeSummary(t *testing.T) {
	diff := &types.Diff{Files: []types.DiffFile{{
		Path:  "internal/review.go",
		Hunks: []types.DiffHunk{{Lines: []string{"+return err"}}},
	}}}
	result := &types.ReviewResult{Summary: "The changed branch now propagates the constructor error."}

	comment := formatReviewSummaryForLanguageAndDiff(result, diff, "en-US")
	if !strings.Contains(comment, "### Summary") || !strings.Contains(comment, result.Summary) {
		t.Fatalf("code summary missing from PR review:\n%s", comment)
	}

	operational := &types.Diff{Files: []types.DiffFile{{
		Path:  ".github/workflows/review.yml",
		Hunks: []types.DiffHunk{{Lines: []string{"+permissions:"}}},
	}}}
	comment = formatReviewSummaryForLanguageAndDiff(result, operational, "en-US")
	if strings.Contains(comment, result.Summary) {
		t.Fatalf("operational review published code summary:\n%s", comment)
	}
}

func TestFormatReviewSummaryUsesConfiguredLanguage(t *testing.T) {
	result := &types.ReviewResult{
		Issues: []types.ReviewIssue{{
			File:         "internal/exemplo.go",
			Line:         12,
			Severity:     "error",
			Message:      "A dependência pode ser nula.",
			Impact:       "A chamada seguinte pode falhar.",
			Evidence:     "O ramo alterado retorna sem validar o resultado.",
			Suggestion:   "Propague o erro da construção.",
			Verification: "Execute o teste focado.",
		}},
		CIAnalysis: []types.CIAnalysis{{
			Check: "testes", Status: "failure", Cause: "A causa não está no contexto fornecido.",
		}},
	}

	comment := formatReviewSummaryForLanguage(result, "pt-BR")
	for _, want := range []string{
		"## AurumCode revisão de código",
		"**Veredito:** Alterações solicitadas",
		"### Achados",
		"Impacto:",
		"Evidência:",
		"Correção sugerida:",
		"Verificação:",
		"### Status do CI",
		"Causa:",
	} {
		if !strings.Contains(comment, want) {
			t.Errorf("Portuguese summary missing %q:\n%s", want, comment)
		}
	}
	if strings.Contains(comment, "### Findings") || strings.Contains(comment, "**Verdict:**") {
		t.Fatalf("Portuguese summary retained English headings:\n%s", comment)
	}

	inline := formatInlineIssueForLanguage(result.Issues[0], "pt-BR")
	if strings.Contains(inline, "Impact:") || !strings.Contains(inline, "Impacto:") {
		t.Fatalf("inline comment language =\n%s", inline)
	}
}

func TestFilterSuggestionsToChangedLines(t *testing.T) {
	diff := &types.Diff{Files: []types.DiffFile{{
		Path: "config.yml",
		Hunks: []types.DiffHunk{{
			NewStart: 4,
			NewLines: 1,
			Lines:    []string{" review:", "+  language: pt-BR"},
		}},
	}}}
	suggestions := []types.ReviewSuggestion{
		{Title: "General advice", Description: "Keep the configuration documented."},
		{Title: "Changed line", File: "config.yml", Line: 5},
		{Title: "Changed range", Kind: "code", File: "config.yml", StartLine: 5, EndLine: 5, ProposedCode: "language: pt-BR"},
		{Title: "Outside file", File: "HANDOFF.md", Line: 0},
		{Title: "Context line", File: "config.yml", Line: 4},
	}

	got := filterSuggestionsToChangedLines(diff, suggestions)
	if len(got) != 3 {
		t.Fatalf("filtered suggestions = %+v, want general advice and changed lines", got)
	}
	if got[0].Title != "General advice" || got[1].Title != "Changed line" || got[2].Title != "Changed range" {
		t.Fatalf("filtered suggestions = %+v, want stable order and valid locations", got)
	}
	if strings.Contains(formatReviewSummaryForLanguage(&types.ReviewResult{Suggestions: suggestions}, "pt-BR"), "HANDOFF.md:0") {
		t.Fatal("summary rendered invalid line zero")
	}
}

func TestSuppressOperationalStrengths(t *testing.T) {
	diff := &types.Diff{Files: []types.DiffFile{{
		Path:  ".aurumcode/config.yml",
		Hunks: []types.DiffHunk{{Lines: []string{"+review:", "+  language: pt-BR"}}},
	}}}
	result := &types.ReviewResult{Strengths: []string{"The repository is now configured clearly."}}

	suppressOperationalStrengths(diff, result)
	if len(result.Strengths) != 0 {
		t.Fatalf("operational strengths = %+v, want empty", result.Strengths)
	}

	codeDiff := &types.Diff{Files: []types.DiffFile{{
		Path:  "internal/review.go",
		Hunks: []types.DiffHunk{{Lines: []string{"+return err"}}},
	}}}
	result = &types.ReviewResult{Strengths: []string{"The error path is explicit."}}
	suppressOperationalStrengths(codeDiff, result)
	if len(result.Strengths) != 1 {
		t.Fatalf("code strengths = %+v, want preserved", result.Strengths)
	}
}

func TestFilterLimitationsAgainstDiff(t *testing.T) {
	diff := &types.Diff{Files: []types.DiffFile{
		{Path: "lib/pricing.mjs"},
		{Path: "test/unit.mjs"},
	}}
	limitations := []string{
		"The review could not assess lib/pricing.mjs because it was not available.",
		"The test/unit.mjs changes were unavailable for review.",
		"The failed CI log was unavailable.",
		"Repository history was not provided.",
	}

	got := filterLimitationsAgainstDiff(diff, limitations)
	if len(got) != 2 {
		t.Fatalf("filtered limitations = %+v, want only evidence outside the diff", got)
	}
	if got[0] != "The failed CI log was unavailable." || got[1] != "Repository history was not provided." {
		t.Fatalf("filtered limitations = %+v, want stable order", got)
	}
}

// TestReviewFlagsAcceptSingleOrDoubleDash pins that every flag `review`
// registers -- --base and the AUR-438 additions (--pr, --repo, --publicar,
// --na-linha) -- parses identically whether spelled with one leading dash
// or two, per Go's documented flag.FlagSet behavior ("a single dash and
// double dash are equivalent"). AUR-440's own lane guard had a gap here: it
// only recognized "--"-prefixed tokens, so the equally valid "-publish"
// spelling slipped through undetected. This card does not repeat that
// mistake by writing a bespoke "--"-prefix guard at all; flag registration
// and fs.Visit (see runReview's --pr dispatch) go through Go's own
// flag.FlagSet, which already treats both spellings alike for every flag
// below, including the pre-existing --base -- this test is the proof.
func TestReviewFlagsAcceptSingleOrDoubleDash(t *testing.T) {
	newFlagSet := func() *flag.FlagSet {
		fs := flag.NewFlagSet("review", flag.ContinueOnError)
		fs.String("base", "", "")
		fs.String("repo", "", "")
		fs.Int("pr", 0, "")
		fs.Bool("publicar", false, "")
		fs.Bool("na-linha", false, "")
		return fs
	}

	oneDash := []string{"-base", "HEAD~1", "-pr", "42", "-repo", "dono/projeto", "-publicar", "-na-linha"}
	twoDash := []string{"--base", "HEAD~1", "--pr", "42", "--repo", "dono/projeto", "--publicar", "--na-linha"}

	fsOne := newFlagSet()
	if err := fsOne.Parse(oneDash); err != nil {
		t.Fatalf("single-dash parse failed: %v", err)
	}
	fsTwo := newFlagSet()
	if err := fsTwo.Parse(twoDash); err != nil {
		t.Fatalf("double-dash parse failed: %v", err)
	}

	for _, name := range []string{"base", "pr", "repo", "publicar", "na-linha"} {
		one := fsOne.Lookup(name).Value.String()
		two := fsTwo.Lookup(name).Value.String()
		if one != two {
			t.Errorf("flag %q: single-dash parse gave %q, double-dash gave %q", name, one, two)
		}
	}
}
