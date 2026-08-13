package prompt

import (
	"errors"
	"strings"
	"testing"
)

func TestRepairJSON(t *testing.T) {
	parser := NewResponseParser()

	tests := []struct {
		name     string
		input    string
		contains string // What the repaired output should contain
	}{
		{
			name:     "trailing comma in object",
			input:    `{"key": "value",}`,
			contains: `{"key": "value"}`,
		},
		{
			name:     "trailing comma in array",
			input:    `{"items": [1, 2, 3,]}`,
			contains: `{"items": [1, 2, 3]}`,
		},
		{
			name:     "smart quotes",
			input:    `{"message": "Hello "world""}`,
			contains: `"Hello \"world\""`,
		},
		{
			name:     "already valid JSON",
			input:    `{"valid": true}`,
			contains: `{"valid": true}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repaired := parser.repairJSON(tt.input)
			if !strings.Contains(repaired, tt.contains) {
				t.Errorf("Expected repaired JSON to contain %q, got %q", tt.contains, repaired)
			}
		})
	}
}

// TestRepairJSON_TypographicQuotesInsideStringsDoNotCorruptJSON is the case
// the c12d7ab blind-replace implementation got wrong: a typographic quote
// used as real content of a string value must not become a bare, unescaped
// `"` that ends the string early. Both curly-quote forms are checked, plus
// that the result is still valid JSON end to end (not just "contains a
// substring").
func TestRepairJSON_TypographicQuotesInsideStringsDoNotCorruptJSON(t *testing.T) {
	parser := NewResponseParser()

	input := "{\"issues\": [], \"summary\": \"She said “Hi” to me\"}"

	result, err := parser.ParseReviewResponse("```json\n" + input + "\n```")
	if err != nil {
		t.Fatalf("expected the repaired JSON to still parse as a valid review response, got error: %v", err)
	}
	if !strings.Contains(result.Summary, "Hi") {
		t.Errorf("expected the typographic-quoted content to survive, got summary %q", result.Summary)
	}
}

func TestParseReviewResponse(t *testing.T) {
	parser := NewResponseParser()

	validJSON := `{
		"issues": [
			{
				"file": "main.go",
				"line": 42,
				"severity": "error",
				"rule_id": "security/sql-injection",
				"message": "SQL injection vulnerability",
				"suggestion": "Use prepared statements"
			}
		],
		"iso_scores": {
			"functionality": 8,
			"reliability": 7,
			"usability": 9,
			"efficiency": 8,
			"maintainability": 7,
			"portability": 9,
			"security": 6,
			"compatibility": 8
		},
		"summary": "Good code quality overall"
	}`

	result, err := parser.ParseReviewResponse(validJSON)
	if err != nil {
		t.Fatalf("failed to parse valid JSON: %v", err)
	}

	if len(result.Issues) != 1 {
		t.Errorf("expected 1 issue, got %d", len(result.Issues))
	}

	if result.ISOScores == nil {
		t.Fatal("expected ISOScores to be populated")
	}
	if result.ISOScores.Security != 6 {
		t.Errorf("expected security score 6, got %d", result.ISOScores.Security)
	}
}

func TestParseReviewResponse_ISOScoresOptional(t *testing.T) {
	// internal/review/iso25010 is out of AUR-430's scope: this engine's own
	// prompt never requires a model to return iso_scores, so a response
	// that omits the block must still parse successfully with a nil
	// ISOScores rather than fail validation.
	parser := NewResponseParser()

	result, err := parser.ParseReviewResponse(`{"issues": [], "summary": "No scores here"}`)
	if err != nil {
		t.Fatalf("expected a response without iso_scores to parse, got: %v", err)
	}
	if result.ISOScores != nil {
		t.Errorf("expected ISOScores to be nil when omitted, got %+v", result.ISOScores)
	}
}

func TestParseReviewResponse_ISOScoresValidatedWhenPresent(t *testing.T) {
	parser := NewResponseParser()

	_, err := parser.ParseReviewResponse(`{"issues": [], "iso_scores": {"functionality": 99, "reliability": 1, "usability": 1, "efficiency": 1, "maintainability": 1, "portability": 1, "security": 1, "compatibility": 1}, "summary": "bad score"}`)
	if err == nil {
		t.Fatal("expected an out-of-range ISO score to fail validation")
	}
}

func TestParseReviewResponse_WithMarkdown(t *testing.T) {
	parser := NewResponseParser()

	response := "```json\n" + `{
		"issues": [],
		"iso_scores": {
			"functionality": 8, "reliability": 8, "usability": 8,
			"efficiency": 8, "maintainability": 8, "portability": 8,
			"security": 8, "compatibility": 8
		},
		"summary": "Test"
	}` + "\n```"

	result, err := parser.ParseReviewResponse(response)
	if err != nil {
		t.Fatalf("failed to parse markdown JSON: %v", err)
	}

	if result.Summary != "Test" {
		t.Errorf("expected summary 'Test', got %s", result.Summary)
	}
}

// TestParseReviewResponse_DegradedFallback exercises the recovery path this
// card adds: a model that ignores the JSON schema entirely but still lists
// findings using the documented "file:line: severity: message" convention
// must still produce usable issues, not a dead end.
func TestParseReviewResponse_DegradedFallback(t *testing.T) {
	parser := NewResponseParser()

	response := strings.Join([]string{
		"I looked at the change and found the following problems:",
		"- config/demo-tokens.txt:3: error: a plaintext credential was committed",
		"- src/greeter.py:5: missing input validation on name",
		"That's everything I noticed.",
	}, "\n")

	result, err := parser.ParseReviewResponse(response)
	if err != nil {
		t.Fatalf("expected the degraded fallback to recover findings, got error: %v", err)
	}
	if len(result.Issues) != 2 {
		t.Fatalf("expected 2 recovered issues, got %d: %+v", len(result.Issues), result.Issues)
	}
	if result.Issues[0].File != "config/demo-tokens.txt" || result.Issues[0].Line != 3 || result.Issues[0].Severity != "error" {
		t.Errorf("unexpected first issue: %+v", result.Issues[0])
	}
	if result.Issues[1].File != "src/greeter.py" || result.Issues[1].Line != 5 || result.Issues[1].Severity != "warning" {
		t.Errorf("unexpected second issue (severity should default to warning): %+v", result.Issues[1])
	}
	if result.Metadata["parse_mode"] != "degraded" {
		t.Errorf("expected parse_mode=degraded in metadata, got %+v", result.Metadata)
	}
}

// TestParseReviewResponse_TypedErrorWhenNothingRecoverable proves the other
// half of the fallback contract: when there is truly nothing to recover,
// the parser returns a typed, inspectable error instead of panicking or
// silently returning an empty-but-successful result.
func TestParseReviewResponse_TypedErrorWhenNothingRecoverable(t *testing.T) {
	parser := NewResponseParser()

	result, err := parser.ParseReviewResponse("I'm not sure what you're asking me to review here.")
	if err == nil {
		t.Fatal("expected an error for a response with no JSON and no recoverable findings")
	}
	if result != nil {
		t.Errorf("expected a nil result alongside the error, got %+v", result)
	}

	var parseErr *ParseError
	if !errors.As(err, &parseErr) {
		t.Fatalf("expected a *ParseError, got %T: %v", err, err)
	}
	if parseErr.Kind != ParseErrorNoJSON {
		t.Errorf("expected ParseErrorNoJSON, got %s", parseErr.Kind)
	}
}

func TestExtractJSON(t *testing.T) {
	parser := NewResponseParser()

	tests := []struct {
		name    string
		input   string
		hasJSON bool
	}{
		{"markdown block", "```json\n{\"key\":\"value\"}\n```", true},
		{"raw json", "{\"key\":\"value\"}", true},
		{"no json", "just text", false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := parser.extractJSON(test.input)
			if test.hasJSON && result == "" {
				t.Error("expected JSON to be extracted")
			}
			if !test.hasJSON && result != "" {
				t.Error("expected no JSON extraction")
			}
		})
	}
}

func TestParseDocumentationResponse(t *testing.T) {
	parser := NewResponseParser()

	doc := "# API Documentation\n\nThis is a test."
	result, err := parser.ParseDocumentationResponse(doc)
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	if !strings.Contains(result, "API Documentation") {
		t.Error("documentation not preserved")
	}
}

func TestParseTestResponse(t *testing.T) {
	parser := NewResponseParser()

	response := "```go\nfunc TestExample(t *testing.T) {}\n```"
	result, err := parser.ParseTestResponse(response, "go")
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	if !strings.Contains(result, "TestExample") {
		t.Error("test code not extracted")
	}
}

func TestSanitizeResponse(t *testing.T) {
	parser := NewResponseParser()

	inputs := []string{
		"Here is the code:",
		"Here's what you need:",
		"Below is the solution:",
	}

	for _, input := range inputs {
		result := parser.SanitizeResponse(input + " actual content")
		if strings.Contains(result, "Here") || strings.Contains(result, "Below") {
			t.Errorf("failed to sanitize: %s", input)
		}
	}
}
