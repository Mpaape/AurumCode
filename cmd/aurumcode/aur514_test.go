package main

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/prompt"
	"github.com/Mpaape/AurumCode/internal/review"
	"github.com/Mpaape/AurumCode/internal/review/blocking"
	"github.com/Mpaape/AurumCode/internal/reviewprofile"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// aur514Diff removes a guard (old line 11, LEFT) and adds a call (new line
// 12, RIGHT): both findings below sit on changed lines.
func aur514Diff() *types.Diff {
	return &types.Diff{Files: []types.DiffFile{{Path: "handler.go", Hunks: []types.DiffHunk{{
		OldStart: 10, NewStart: 10,
		Lines: []string{" if user == nil {", "-  return errMissingUser", " }", "+persist(user.ID)"},
	}}}}}
}

// aur514Field is one detail field the merge must keep, as the model sent it.
type aur514Field struct{ name, value string }

var aur514Left = []aur514Field{
	{"impact", "O handler entra em panico sem usuario."},
	{"evidence", "Sem o retorno, user == nil chega a user.ID."},
	{"suggestion", "Restaure o retorno antecipado."},
	{"verification", "Chame o handler sem usuario e confira o erro."},
}

const aur514Response = `{"summary":"ok","verdict":"comment","issues":[` +
	`{"file":"handler.go","line":11,"side":"LEFT","severity":"error","rule_id":"quality/poor-naming",` +
	`"message":"Remover o retorno permite nil","impact":"O handler entra em panico sem usuario.",` +
	`"evidence":"Sem o retorno, user == nil chega a user.ID.","suggestion":"Restaure o retorno antecipado.",` +
	`"verification":"Chame o handler sem usuario e confira o erro."},` +
	`{"file":"handler.go","line":12,"side":"RIGHT","severity":"warning","rule_id":"quality/magic-numbers",` +
	`"message":"Persistir sem validar","impact":"Grava id invalido.","evidence":"persist(user.ID) sem checagem.",` +
	`"suggestion":"Valide antes de persistir.","verification":"Teste com id vazio."}]}`

// aur514Merged runs two real profile passes (the same model answer for
// both, so every finding is a duplicate the merge must collapse) and
// returns the merged result.
func aur514Merged(t *testing.T) *types.ReviewResult {
	t.Helper()
	provider := &review.FakeProvider{Response: aur514Response}
	profiles := []reviewprofile.Profile{{Name: "solid"}, {Name: "release"}}
	merged, err := runProfilePasses(context.Background(), provider, nil, profiles, aur514Diff(), review.ReviewContext{}, nil, prompt.DefaultRuleCatalog)
	if err != nil {
		t.Fatalf("runProfilePasses: %v", err)
	}
	if len(merged.Issues) != 2 {
		t.Fatalf("merged %d issues, want 2 (both findings sit on changed lines): %+v", len(merged.Issues), merged.Issues)
	}
	return merged
}

func aur514LeftIssue(t *testing.T, issues []types.ReviewIssue) types.ReviewIssue {
	t.Helper()
	for _, issue := range issues {
		if issue.Line == 11 {
			return issue
		}
	}
	t.Fatalf("AUR-514 field lost: the LEFT finding at handler.go:11 is gone: %+v", issues)
	return types.ReviewIssue{}
}

func aur514Value(issue types.ReviewIssue, name string) string {
	switch name {
	case "impact":
		return issue.Impact
	case "evidence":
		return issue.Evidence
	case "suggestion":
		return issue.Suggestion
	default:
		return issue.Verification
	}
}

// TestAUR514AC001ProfileMergeKeepsEveryField: a finding with every field
// keeps them after two profiles merge it, and the collapsed duplicate names
// both profiles.
func TestAUR514AC001ProfileMergeKeepsEveryField(t *testing.T) {
	left := aur514LeftIssue(t, aur514Merged(t).Issues)
	if left.Side != "LEFT" {
		t.Fatalf("AUR-514 field lost: side = %q, want LEFT", left.Side)
	}
	for _, f := range aur514Left {
		if got := aur514Value(left, f.name); got != f.value {
			t.Fatalf("AUR-514 field lost: %s = %q, want %q", f.name, got, f.value)
		}
	}
	if !strings.Contains(left.Message, "[perfil solid; tambem: release]") {
		t.Fatalf("AUR-514 attribution lost: the duplicate must name both profiles: %q", left.Message)
	}
}

// TestAUR514AC001DuplicateKeepsTheLaterEvidence: when the first profile's
// copy lacks a field the later one carries, the merge keeps the later
// value; distinct evidence from both is kept; the original issue's ID and
// assessment survive (the merge rebuilds from the whole original).
func TestAUR514AC001DuplicateKeepsTheLaterEvidence(t *testing.T) {
	base := types.ReviewIssue{ID: "f-1", File: "a.go", Line: 3, Side: "LEFT", Severity: "error", RuleID: "r", Message: "m",
		Assessment: &types.EvidenceAssessment{EvidenceID: "e-1", Status: types.AssessmentConfirmed}}
	first := base
	first.Evidence = "evidencia do primeiro"
	second := base
	second.Impact, second.Evidence, second.Verification = "impacto", "evidencia do segundo", "verificacao"
	originals := []types.ReviewIssue{first, second}
	merged := reviewprofile.MergeFindings([]reviewprofile.Finding{
		profileFinding("solid", 0, first), profileFinding("release", 1, second),
	})
	got := attributedIssues(merged, originals)
	if len(got) != 1 {
		t.Fatalf("duplicate not collapsed: %+v", got)
	}
	issue := got[0]
	if issue.Impact != "impacto" || issue.Verification != "verificacao" || issue.Side != "LEFT" {
		t.Fatalf("AUR-514 field lost: the later duplicate's detail did not fill the kept one: %+v", issue)
	}
	if !strings.Contains(issue.Evidence, "evidencia do primeiro") || !strings.Contains(issue.Evidence, "evidencia do segundo") {
		t.Fatalf("AUR-514 field lost: evidence of one profile dropped: %q", issue.Evidence)
	}
	if issue.ID != "f-1" || issue.Assessment == nil || issue.Assessment.EvidenceID != "e-1" {
		t.Fatalf("AUR-514 field lost: id/assessment of the original dropped: %+v", issue)
	}
	if issue.Message != "m [perfil solid; tambem: release]" {
		t.Fatalf("attribution = %q", issue.Message)
	}
}

// TestAUR514AC001SidesAreDistinctFindings: the same line number on the two
// sides of the diff is two lines, never one collapsed finding.
func TestAUR514AC001SidesAreDistinctFindings(t *testing.T) {
	left := reviewprofile.Finding{Profile: "a", RuleID: "r", File: "a.go", Line: 3, Message: "m", Side: "LEFT"}
	right := left
	right.Side = ""
	if got := reviewprofile.MergeFindings([]reviewprofile.Finding{left, right}); len(got) != 2 {
		t.Fatalf("LEFT and RIGHT findings collapsed: %+v", got)
	}
}

// TestAUR514AC002TerminalAndPRShowTheSameFields: the merged finding shows
// impact, evidence, suggested fix and verification in the terminal report,
// in the published review document and in the inline comment, labeled in
// the chosen language.
func TestAUR514AC002TerminalAndPRShowTheSameFields(t *testing.T) {
	merged := aur514Merged(t)
	left := aur514LeftIssue(t, merged.Issues)
	for _, language := range []string{"pt-BR", "en-US"} {
		copy := reviewCopyFor(language)
		labels := map[string]string{"impact": copy.impact, "evidence": copy.evidence, "suggestion": copy.suggestedFix, "verification": copy.verify}
		var terminal strings.Builder
		printFindings(&terminal, merged, "", language)
		document := formatReviewDocument(merged, aur514Diff(), language, blocking.Ungated())
		inline := formatInlineIssueForLanguage(left, language)
		for _, f := range aur514Left {
			plain := fmt.Sprintf("  - %s: %s", labels[f.name], f.value)
			if !strings.Contains(terminal.String(), plain) {
				t.Fatalf("AUR-514 field lost: terminal (%s) lacks %q:\n%s", language, plain, terminal.String())
			}
			if !strings.Contains(document, plain) {
				t.Fatalf("AUR-514 field lost: review document (%s) lacks %q:\n%s", language, plain, document)
			}
			bold := fmt.Sprintf("**%s:** %s", labels[f.name], f.value)
			if !strings.Contains(inline, bold) {
				t.Fatalf("AUR-514 field lost: inline comment (%s) lacks %q:\n%s", language, bold, inline)
			}
		}
		if !strings.Contains(terminal.String(), "Location: LEFT") {
			t.Fatalf("AUR-514 field lost: terminal (%s) lost the LEFT side:\n%s", language, terminal.String())
		}
	}
}
