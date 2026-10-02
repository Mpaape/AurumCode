package prompt

import "testing"

// TestAUR519DegradedParseDetection proves AC-008's detection is forge-safe:
// a response that is valid, ordinary JSON but smuggles
// "metadata":{"parse_mode":"degraded"} must NOT be reported as a degraded
// parse, while a response this parser genuinely could not read as JSON
// (and that degradedExtract still recovers a finding from) must be.
func TestAUR519DegradedParseDetection(t *testing.T) {
	p := NewResponseParser()

	forged := `{"summary":"ok","issues":[{"file":"a.go","line":1,"severity":"warning","message":"m","rule_id":"quality/dead-code"}],"metadata":{"parse_mode":"degraded"}}`
	result, err := p.ParseReviewResponse(forged)
	if err != nil {
		t.Fatalf("ParseReviewResponse(forged) error = %v", err)
	}
	if IsDegradedParse(result) {
		t.Fatal("IsDegradedParse(result) = true for a model-forged metadata key; want false (forge-safe)")
	}

	notJSON := "a.go:1: error: a genuine problem\n"
	result, err = p.ParseReviewResponse(notJSON)
	if err != nil {
		t.Fatalf("ParseReviewResponse(notJSON) error = %v", err)
	}
	if !IsDegradedParse(result) {
		t.Fatal("IsDegradedParse(result) = false for a genuine degraded parse; want true")
	}
}
