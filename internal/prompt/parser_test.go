package prompt

import (
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

	if result.ISOScores.Security != 6 {
		t.Errorf("expected security score 6, got %d", result.ISOScores.Security)
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

func TestExtractJSON(t *testing.T) {
	parser := NewResponseParser()

	tests := []struct {
		name     string
		input    string
		hasJSON  bool
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
