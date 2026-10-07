package prompt

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// The "line_comments" decision: it is not a findings schema. The template
// does not teach it, the parser does not accept it, and no entry under it
// ever becomes a finding -- with or without severity and rule_id, because
// it can never carry the evidence the evidence gate requires.

// AC-001: the template and the parser agree that "line_comments" is not
// part of the answer schema.
func TestLineCommentsIsNeitherTaughtNorAccepted(t *testing.T) {
	var taught map[string]json.RawMessage
	if err := json.Unmarshal([]byte(reviewTemplateJSONExample(t)), &taught); err != nil {
		t.Fatalf("template JSON example: %v", err)
	}
	if _, ok := taught["line_comments"]; ok {
		t.Fatal("the template teaches line_comments, but the parser rejects it")
	}
	for _, name := range acceptedReviewFields {
		if name == "line_comments" {
			t.Fatal("acceptedReviewFields lists line_comments, but no finding may come from it")
		}
	}
	if findingsListKey != "issues" {
		t.Fatalf("the findings list is %q, want issues", findingsListKey)
	}
}

// AC-002: a reply whose only findings list is "line_comments" is an
// inconclusive review, never a result -- not an empty one ("No issues
// found.") and not one carrying converted, evidence-free findings.
func TestLineCommentsOnlyReplyIsInconclusive(t *testing.T) {
	for _, reply := range []string{
		`{"line_comments":[{"path":"config/demo-tokens.txt","line":4,"severity":"error","rule_id":"security/hardcoded-secret","body":"credential-shaped value committed"}]}`,
		`{"line_comments":[{"path":"config/demo-tokens.txt","line":4,"body":"credential-shaped value committed"}]}`,
		`{"verdict":"approve","line_comments":[{"file":"a.go","line":1,"message":"x"}]}`,
	} {
		result, err := NewResponseParser().ParseReviewResponse(reply)
		var parseErr *ParseError
		if !errors.As(err, &parseErr) || parseErr.Kind != ParseErrorValidation || parseErr.ValidationCode != "missing_issues" {
			t.Fatalf("%s: a line_comments-only reply must be inconclusive (missing_issues); got result=%+v err=%v", reply, result, err)
		}
	}
}

// AC-002: beside a real "issues" list, "line_comments" entries are
// dropped -- they reach neither Issues nor LineComments -- and the drop is
// announced on the parse discard warning the CLI prints to stderr.
func TestLineCommentsBesideIssuesAreDroppedAndAnnounced(t *testing.T) {
	const reply = `{"issues":[{"file":"src/a.go","line":7,"severity":"warning","rule_id":"quality/naming","message":"unclear name","evidence":"line 7 names it x","impact":"readers guess","verification":"rename and rerun"}],
	  "line_comments":[
	    {"path":"src/a.go","line":7,"severity":"error","rule_id":"security/hardcoded-secret","body":"secret committed"},
	    {"path":"src/b.go","line":3,"body":"another"}
	  ]}`
	result, err := NewResponseParser().ParseReviewResponse(reply)
	if err != nil {
		t.Fatalf("a reply with a valid issues list must parse: %v", err)
	}
	if len(result.Issues) != 1 || result.Issues[0].Message != "unclear name" {
		t.Fatalf("only the issues entry may become a finding, got %+v", result.Issues)
	}
	for _, issue := range result.Issues {
		if strings.TrimSpace(issue.Evidence) == "" {
			t.Fatalf("a finding without evidence reached the result: %+v", issue)
		}
	}
	if len(result.LineComments) != 0 {
		t.Fatalf("line_comments entries must not travel on the result, got %+v", result.LineComments)
	}
	want := `2 line comment(s) ignored: line_comments is not part of the answer schema; findings must be reported under "issues" with evidence`
	if got := result.Metadata[parseDiscardWarningKey]; got != want {
		t.Fatalf("the drop must be announced as %q, got %q", want, got)
	}
}

// A reply without "line_comments" gets no warning: the ordinary run never
// receives a byte on stderr from this path.
func TestNoLineCommentsNoWarning(t *testing.T) {
	result, err := NewResponseParser().ParseReviewResponse(`{"issues":[]}`)
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := result.Metadata[parseDiscardWarningKey]; ok {
		t.Fatalf("unexpected discard warning %q", got)
	}
}
