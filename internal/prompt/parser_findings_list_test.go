package prompt

import (
	"errors"
	"testing"
)

// A reply without a findings list is outside the answer schema: it is a
// parse failure (an inconclusive review), never an empty list of issues.
func TestAnswerWithoutFindingsListIsInconclusive(t *testing.T) {
	for _, reply := range []string{
		`{"answer":"looks fine to me"}`,
		`{"verdict":"approve","summary":"ok"}`,
		`{"issues":null,"verdict":"approve"}`,
		`{}`,
		`{"line_comments":[]}`,
	} {
		result, err := NewResponseParser().ParseReviewResponse(reply)
		var parseErr *ParseError
		if !errors.As(err, &parseErr) || parseErr.Kind != ParseErrorValidation || parseErr.ValidationCode != "missing_issues" {
			t.Fatalf("%s: result=%+v err=%v", reply, result, err)
		}
	}
	for _, reply := range []string{`{"issues":[]}`} {
		if _, err := NewResponseParser().ParseReviewResponse(reply); err != nil {
			t.Fatalf("%s: %v", reply, err)
		}
	}
}
