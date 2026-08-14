package unit

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/prompt"
)

// TestAUR459 proves, at the public boundary of internal/prompt, the two
// halves of AUR-459's fix that used to disagree in silence: the review
// prompt template teaches exactly ONE findings schema, and the parser
// reads the shape a real model actually answers with.
//
// The defect: the template showed "line_comments" first and "issues"
// second while the parser read only "issues", so a model that answered
// with line_comments -- what the real gateway does -- produced
// "No issues found." with exit 0 over a diff carrying planted secrets.
//
// The card's declared selector for this file is "TestAUR435", a typo
// carried from the card template: tests/unit/AUR-435.go already defines
// TestAUR435 in this same package, so redefining it would not compile.
// See docs/specs/AUR-459.md.
func TestAUR459(t *testing.T) {
	t.Run("TemplateTeachesOneSchema", testAUR459TemplateTeachesOneSchema)
	t.Run("LineCommentsReachTheUser", testAUR459LineCommentsReachTheUser)
}

// reviewTemplatePath locates the template from this file's own directory,
// so the test reads the shipped bytes rather than a copy of them.
func reviewTemplatePath(t *testing.T) string {
	t.Helper()
	path := filepath.Join("..", "..", "internal", "prompt", "templates", "review.md")
	// Never a Skip: a template this test cannot read is a test that proves
	// nothing while reporting success, which is the failure mode of the
	// very defect this card fixes.
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("review template not materialized at %s: %v", path, err)
	}
	return path
}

func testAUR459TemplateTeachesOneSchema(t *testing.T) {
	raw, err := os.ReadFile(reviewTemplatePath(t))
	if err != nil {
		t.Fatalf("reading the review template: %v", err)
	}

	lines := strings.Split(string(raw), "\n")
	var starts []int
	for i, line := range lines {
		if strings.TrimSpace(line) == "```json" {
			starts = append(starts, i)
		}
	}
	if len(starts) != 1 {
		t.Fatalf("the review template shows %d JSON examples; the model must be offered exactly one findings schema (AUR-459)", len(starts))
	}

	end := -1
	for i := starts[0] + 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "```" {
			end = i
			break
		}
	}
	if end == -1 {
		t.Fatal("the review template's JSON example is never closed")
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(strings.Join(lines[starts[0]+1:end], "\n")), &fields); err != nil {
		t.Fatalf("the review template's JSON example does not parse as JSON: %v", err)
	}

	taught := make([]string, 0, len(fields))
	for name := range fields {
		taught = append(taught, name)
	}
	sort.Strings(taught)

	want := []string{"iso_scores", "issues", "summary"}
	if strings.Join(taught, ",") != strings.Join(want, ",") {
		t.Fatalf("the template teaches %v; the parser turns %v into user-visible output. A field taught but not read is a finding the user never sees.", taught, want)
	}

	// rule_id is not optional and the template must say why: the AUR-434
	// gate discards a finding that cannot cite a rule, so a prompt that
	// omits it teaches the model to produce findings nobody will see.
	body := string(raw)
	if !strings.Contains(body, `"rule_id"`) {
		t.Error("the template's schema must show rule_id")
	}
	if !strings.Contains(body, "discarded") {
		t.Error("the template must state that a finding without a resolvable rule_id is discarded")
	}
}

func testAUR459LineCommentsReachTheUser(t *testing.T) {
	const response = `{"line_comments":[{"path":"config/demo-tokens.txt","line":4,"severity":"error","rule_id":"security/hardcoded-secret","body":"credential-shaped value committed"}],"summary":"one finding"}`

	result, err := prompt.NewResponseParser().ParseReviewResponse(response)
	if err != nil {
		t.Fatalf("parsing the shape the real gateway answers with failed: %v", err)
	}
	if len(result.Issues) != 1 {
		t.Fatalf("a response whose findings live only in line_comments produced %d issues: this is the confidently-wrong \"No issues found.\" AUR-459 exists to kill", len(result.Issues))
	}
	got := result.Issues[0]
	if got.File != "config/demo-tokens.txt" {
		t.Errorf("path must map to the issue file, got %q", got.File)
	}
	if got.Line != 4 {
		t.Errorf("line must map to the issue line, got %d", got.Line)
	}
	if got.Message != "credential-shaped value committed" {
		t.Errorf("body must map to the issue message, got %q", got.Message)
	}
	if got.RuleID != "security/hardcoded-secret" {
		t.Errorf("a cited rule must survive the conversion, or the AUR-434 gate discards the finding; got %q", got.RuleID)
	}
	if got.Severity != "error" {
		t.Errorf("severity must survive the conversion, got %q", got.Severity)
	}
}
