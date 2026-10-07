package prompt

import (
	"encoding/json"
	"sort"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/pkg/types"
)

// AUR-459. The defect this file exists to keep dead: the review prompt
// template showed the model TWO findings schemas -- "line_comments" first,
// "issues" second -- while ResponseParser read only "issues". The model
// answered with the first one it was shown, so a diff carrying three
// planted secrets printed "No issues found." with exit 0. Nothing failed;
// the tool simply reported the opposite of the truth.
//
// Two independent properties are pinned here, and each one is red on its
// own when its half of the fix is removed:
//
//  1. TestReviewTemplateMatchesParser extracts the REAL top-level fields of
//     the template's JSON example and requires exactly the canonical set.
//     Re-adding a "line_comments"/"file_comments"/"commit_comment" block to
//     the template turns this red.
//  2. TestAcceptedReviewFieldsAreConsumed drives one probe response per
//     accepted field through the real parser. "line_comments" is no longer
//     accepted at all (see parser_line_comments.go): it is not a findings
//     schema, and its entries are announced and dropped, never converted.
//
// The second test is what makes the first one honest: canonicalReviewFields
// is a hand-written list, and a hand-written list is exactly the failure
// class of this card. Every name in it is proven to change a parse result.

// reviewTemplateJSONExample returns the JSON object the embedded review
// template shows the model, taken from the template's ```json fence.
func reviewTemplateJSONExample(t *testing.T) string {
	t.Helper()

	raw, err := templateFS.ReadFile("templates/review.md")
	if err != nil {
		t.Fatalf("reading the embedded review template: %v", err)
	}

	lines := strings.Split(string(raw), "\n")
	start := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == "```json" {
			if start != -1 {
				t.Fatalf("the review template shows more than one JSON example; the model is offered a choice of schemas again")
			}
			start = i
		}
	}
	if start == -1 {
		t.Fatal("the review template shows no ```json example: the model is told nothing about the shape the parser reads")
	}

	for i := start + 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "```" {
			return strings.Join(lines[start+1:i], "\n")
		}
	}
	t.Fatal("the review template's ```json example is never closed")
	return ""
}

// TestReviewTemplateMatchesParser reads the template's own bytes -- not a
// copy of them -- and requires that every top-level field it teaches the
// model is one this parser turns into user-visible output, and that no
// field the parser produces is left untaught.
func TestReviewTemplateMatchesParser(t *testing.T) {
	example := reviewTemplateJSONExample(t)

	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(example), &fields); err != nil {
		t.Fatalf("the review template's JSON example does not parse as JSON: %v\n%s", err, example)
	}

	taught := make([]string, 0, len(fields))
	for name := range fields {
		taught = append(taught, name)
	}
	sort.Strings(taught)

	want := append([]string(nil), canonicalReviewFields...)
	sort.Strings(want)

	if strings.Join(taught, ",") != strings.Join(want, ",") {
		t.Fatalf("the prompt and the parser disagree about the response schema.\n"+
			"template teaches: %v\nparser's canonical set: %v\n"+
			"A field taught but not read is a finding the user never sees (AUR-459); "+
			"a field read but not taught is a field no model will send.",
			taught, want)
	}

	// The canonical set must be a subset of what the parser accepts, or
	// the prompt teaches a shape the parser refuses.
	accepted := make(map[string]bool, len(acceptedReviewFields))
	for _, name := range acceptedReviewFields {
		accepted[name] = true
	}
	for _, name := range canonicalReviewFields {
		if !accepted[name] {
			t.Errorf("canonical field %q is taught by the prompt but is not in acceptedReviewFields", name)
		}
	}
}

// TestAcceptedReviewFieldsAreConsumed proves, field by field, that every
// name in acceptedReviewFields actually changes what ParseReviewResponse
// produces. A response carrying only that field must be visible in the
// result; a list nobody executes is the thing this card is about.
func TestAcceptedReviewFieldsAreConsumed(t *testing.T) {
	type probe struct {
		response string
		verify   func(t *testing.T, got parsedProbe)
	}

	table := map[string]probe{
		"ci_analysis": {
			response: `{"issues":[],"ci_analysis":[{"check":"unit","status":"failure","cause":"cause","evidence":"evidence","fix":"fix","next_verification":"rerun","confidence":"high"}]}`,
			verify: func(t *testing.T, got parsedProbe) {
				if len(got.CIAnalysis) != 1 || got.CIAnalysis[0].Cause != "cause" {
					t.Fatalf("ci_analysis was not carried through: %+v", got.CIAnalysis)
				}
			},
		},
		"issues": {
			response: `{"issues":[{"file":"src/a.go","line":7,"severity":"error","rule_id":"security/hardcoded-secret","message":"secret committed"}]}`,
			verify: func(t *testing.T, got parsedProbe) {
				if len(got.Issues) != 1 {
					t.Fatalf("expected the issues array to produce 1 issue, got %d", len(got.Issues))
				}
				if got.Issues[0].File != "src/a.go" || got.Issues[0].Line != 7 || got.Issues[0].Message != "secret committed" {
					t.Fatalf("issues entry not carried through: %+v", got.Issues[0])
				}
			},
		},
		"iso_scores": {
			response: `{"issues":[],"iso_scores":{"functionality":8,"reliability":7,"usability":9,"efficiency":7,"maintainability":8,"portability":9,"security":5,"compatibility":8}}`,
			verify: func(t *testing.T, got parsedProbe) {
				if got.ISOScores == nil {
					t.Fatal("iso_scores was taught to the model but dropped by the parser")
				}
				if got.ISOScores.Security != 5 {
					t.Errorf("iso_scores value not carried through: %+v", *got.ISOScores)
				}
			},
		},
		"limitations": {
			response: `{"issues":[],"limitations":["the check log was unavailable"]}`,
			verify: func(t *testing.T, got parsedProbe) {
				if len(got.Limitations) != 1 || got.Limitations[0] == "" {
					t.Fatalf("limitations were not carried through: %+v", got.Limitations)
				}
			},
		},
		"summary": {
			response: `{"issues":[],"summary":"three secrets added"}`,
			verify: func(t *testing.T, got parsedProbe) {
				if got.Summary != "three secrets added" {
					t.Errorf("summary not carried through, got %q", got.Summary)
				}
			},
		},
		"strengths": {
			response: `{"issues":[],"strengths":["clear separation of concerns"]}`,
			verify: func(t *testing.T, got parsedProbe) {
				if len(got.Strengths) != 1 || got.Strengths[0] == "" {
					t.Fatalf("strengths were not carried through: %+v", got.Strengths)
				}
			},
		},
		"suggestions": {
			response: `{"issues":[],"suggestions":[{"title":"Add a test","description":"cover the branch","kind":"code","file":"internal/example_test.go","start_line":22,"end_line":24,"current_code":"old","proposed_code":"new","rationale":"keep the behavior explicit","verification":"run the focused test"}]}`,
			verify: func(t *testing.T, got parsedProbe) {
				if len(got.Suggestions) != 1 || got.Suggestions[0].Title != "Add a test" {
					t.Fatalf("suggestions were not carried through: %+v", got.Suggestions)
				}
				suggestion := got.Suggestions[0]
				if suggestion.Kind != "code" || suggestion.StartLine != 22 || suggestion.EndLine != 24 || suggestion.CurrentCode != "old" || suggestion.ProposedCode != "new" || suggestion.Rationale == "" {
					t.Fatalf("implementation-ready suggestion fields were not carried through: %+v", suggestion)
				}
			},
		},
		"test_plan": {
			response: `{"issues":[],"test_plan":["run the focused package test"]}`,
			verify: func(t *testing.T, got parsedProbe) {
				if len(got.TestPlan) != 1 || got.TestPlan[0] == "" {
					t.Fatalf("test_plan was not carried through: %+v", got.TestPlan)
				}
			},
		},
		"verdict": {
			response: `{"issues":[],"verdict":"approve"}`,
			verify: func(t *testing.T, got parsedProbe) {
				if got.Verdict != "approve" {
					t.Fatalf("verdict was not carried through: %q", got.Verdict)
				}
			},
		},
	}

	// The table must cover exactly acceptedReviewFields: adding a name to
	// the list without proving it is consumed fails here.
	if len(table) != len(acceptedReviewFields) {
		t.Fatalf("acceptedReviewFields has %d names but %d are proven consumed", len(acceptedReviewFields), len(table))
	}
	parser := NewResponseParser()
	for _, name := range acceptedReviewFields {
		p, ok := table[name]
		if !ok {
			t.Fatalf("accepted field %q has no probe proving the parser consumes it", name)
		}
		t.Run(name, func(t *testing.T) {
			result, err := parser.ParseReviewResponse(p.response)
			if err != nil {
				t.Fatalf("parsing a response containing %q failed: %v", name, err)
			}
			p.verify(t, parsedProbe{
				Issues:      result.Issues,
				ISOScores:   result.ISOScores,
				Summary:     result.Summary,
				Verdict:     result.Verdict,
				Strengths:   result.Strengths,
				Suggestions: result.Suggestions,
				CIAnalysis:  result.CIAnalysis,
				TestPlan:    result.TestPlan,
				Limitations: result.Limitations,
			})
		})
	}
}

// parsedProbe is the slice of a parse result the field probes assert on.
type parsedProbe struct {
	Issues      []types.ReviewIssue
	ISOScores   *types.ISOScores
	Summary     string
	Verdict     string
	Strengths   []string
	Suggestions []types.ReviewSuggestion
	CIAnalysis  []types.CIAnalysis
	TestPlan    []string
	Limitations []string
}
