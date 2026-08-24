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
//     accepted field through the real parser. Removing adoptLineComments
//     turns this red, as does removing the conversion's field mapping.
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
		"line_comments": {
			response: `{"line_comments":[{"path":"src/a.go","line":12,"body":"hardcoded AWS key on this line"}]}`,
			verify: func(t *testing.T, got parsedProbe) {
				if len(got.Issues) != 1 {
					t.Fatalf("a response whose only findings are in line_comments produced %d issues: the user is being told nothing (AUR-459)", len(got.Issues))
				}
				issue := got.Issues[0]
				if issue.File != "src/a.go" {
					t.Errorf("line_comments path must map to the issue file, got %q", issue.File)
				}
				if issue.Line != 12 {
					t.Errorf("line_comments line must map to the issue line, got %d", issue.Line)
				}
				if issue.Message != "hardcoded AWS key on this line" {
					t.Errorf("line_comments body must map to the issue message, got %q", issue.Message)
				}
				if issue.Severity != "warning" {
					t.Errorf("a comment with no severity must be reported as a warning, got %q", issue.Severity)
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
			response: `{"issues":[],"suggestions":[{"title":"Add a test","description":"cover the branch"}]}`,
			verify: func(t *testing.T, got parsedProbe) {
				if len(got.Suggestions) != 1 || got.Suggestions[0].Title != "Add a test" {
					t.Fatalf("suggestions were not carried through: %+v", got.Suggestions)
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

// TestLineCommentsDoNotDisplaceIssues pins the merge rule: a model that
// answers with both arrays loses nothing. The SAME finding restated in the
// other vocabulary (same file, same line, same text up to case and
// whitespace) collapses onto the "issues" record, which is the richer one
// -- it carries the rule citation -- and the collapse is announced;
// anything reported only under "line_comments" is added.
func TestLineCommentsDoNotDisplaceIssues(t *testing.T) {
	const response = `{
	  "issues":[{"file":"src/a.go","line":7,"severity":"error","rule_id":"security/hardcoded-secret","message":"from issues"}],
	  "line_comments":[
	    {"path":"src/a.go","line":7,"body":"From   Issues"},
	    {"path":"src/b.go","line":3,"body":"only reported as a comment","rule_id":"security/hardcoded-secret","severity":"error"}
	  ]
	}`

	result, err := NewResponseParser().ParseReviewResponse(response)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if len(result.Issues) != 2 {
		t.Fatalf("expected 2 issues (the issues entry plus the comment at a location it did not cover), got %d: %+v", len(result.Issues), result.Issues)
	}
	if result.Issues[0].Message != "from issues" || result.Issues[0].RuleID != "security/hardcoded-secret" {
		t.Errorf("the issues entry must win at a location both arrays report: %+v", result.Issues[0])
	}
	second := result.Issues[1]
	if second.File != "src/b.go" || second.Line != 3 || second.RuleID != "security/hardcoded-secret" || second.Severity != "error" {
		t.Errorf("a comment-only finding must be adopted with the rule_id and severity it carried: %+v", second)
	}
	if result.Metadata["line_comments_converted"] != "1" {
		t.Errorf("the conversion must be recorded in metadata, got %q", result.Metadata["line_comments_converted"])
	}
	if want := "1 line comment(s) not converted: 1 already reported as an issue"; result.Metadata["parse_discard_warning"] != want {
		t.Errorf("the collapse must be announced as %q, got %q", want, result.Metadata["parse_discard_warning"])
	}
}

// TestLineCommentsAtTheSameLineSurvive is the counterexample that broke
// the first version of this card's merge rule. That version keyed the
// merge on (file, line) alone and justified it as "the issues entry is the
// richer record". Executed, the claim was false: a quality nit reported
// under "issues" swallowed a SECRET LEAK reported at the same line under
// "line_comments", and two DISTINCT comments on one line collapsed into
// one -- silently, because the counter only counted survivors. A card
// whose whole point is that no finding disappears without a word cannot
// hide findings in its own merge.
func TestLineCommentsAtTheSameLineSurvive(t *testing.T) {
	const response = `{
	  "issues":[{"file":"a.go","line":42,"severity":"info","rule_id":"quality/naming","message":"style nit"}],
	  "line_comments":[
	    {"path":"a.go","line":42,"severity":"error","rule_id":"security/hardcoded-secret","body":"SECRET LEAK on this line"},
	    {"path":"a.go","line":42,"severity":"error","rule_id":"security/sql-injection","body":"SQL injection on the same line"}
	  ]
	}`

	result, err := NewResponseParser().ParseReviewResponse(response)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if len(result.Issues) != 3 {
		t.Fatalf("expected all 3 distinct findings (1 nit + 2 security), got %d: %+v", len(result.Issues), result.Issues)
	}

	var sawSecret, sawInjection bool
	for _, issue := range result.Issues {
		if strings.Contains(issue.Message, "SECRET LEAK") {
			sawSecret = true
			if issue.RuleID != "security/hardcoded-secret" || issue.Severity != "error" {
				t.Errorf("the secret finding lost its citation or severity: %+v", issue)
			}
		}
		if strings.Contains(issue.Message, "SQL injection") {
			sawInjection = true
		}
	}
	if !sawSecret {
		t.Error("a security finding reported at a line already carrying a style nit was dropped: the user is told about the nit and not the secret")
	}
	if !sawInjection {
		t.Error("two distinct comments at the same line collapsed into one")
	}
}

// TestLineCommentDiscardsAreAnnounced pins the principle this card chose:
// EVERY discard is announced. adoptLineComments has three of them --
// a comment that repeats a finding already reported, a comment with no
// path, and a comment with a blank body -- and all three must be counted
// and named, never dropped in silence.
func TestLineCommentDiscardsAreAnnounced(t *testing.T) {
	const response = `{
	  "issues":[{"file":"a.go","line":7,"severity":"error","rule_id":"security/hardcoded-secret","message":"same finding"}],
	  "line_comments":[
	    {"path":"a.go","line":7,"body":"Same Finding"},
	    {"path":"","line":1,"body":"nowhere to point"},
	    {"path":"b.go","line":2,"body":"   "},
	    {"path":"b.go","line":9,"body":"a real one"}
	  ]
	}`

	result, err := NewResponseParser().ParseReviewResponse(response)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if len(result.Issues) != 2 {
		t.Fatalf("expected the reported finding plus the one usable comment, got %d: %+v", len(result.Issues), result.Issues)
	}
	if result.Metadata["line_comments_converted"] != "1" {
		t.Errorf("converted count wrong: %q", result.Metadata["line_comments_converted"])
	}
	if result.Metadata["line_comments_discarded"] != "3" {
		t.Errorf("discarded count must cover the repeat, the missing path and the blank body, got %q", result.Metadata["line_comments_discarded"])
	}
	warning := result.Metadata["parse_discard_warning"]
	if warning == "" {
		t.Fatal("a discard with no warning is exactly the silence this card exists to remove")
	}
	for _, want := range []string{"3", "already reported", "no path", "empty body"} {
		if !strings.Contains(warning, want) {
			t.Errorf("the warning must name why; %q is missing from %q", want, warning)
		}
	}
}

// TestLineCommentsJunkIsSkippedNotFatal keeps the fix from trading a false
// negative for a hard failure: a comment with no path or no body is
// skipped, never handed to validateReviewResult, which would reject the
// whole response over one junk entry.
func TestLineCommentsJunkIsSkippedNotFatal(t *testing.T) {
	const response = `{"line_comments":[{"path":"","line":1,"body":"no path"},{"path":"src/a.go","line":2,"body":"   "},{"path":"src/a.go","line":9,"body":"real finding"}]}`

	result, err := NewResponseParser().ParseReviewResponse(response)
	if err != nil {
		t.Fatalf("a partly junk line_comments block must not kill the parse: %v", err)
	}
	if len(result.Issues) != 1 || result.Issues[0].Line != 9 {
		t.Fatalf("expected only the usable comment to survive, got %+v", result.Issues)
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
