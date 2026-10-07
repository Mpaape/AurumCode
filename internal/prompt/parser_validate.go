package prompt

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Mpaape/AurumCode/pkg/types"
)

// validateReviewResult validates a review result
func (p *ResponseParser) validateReviewResult(result *types.ReviewResult) error {
	// Check if issues is nil (create empty slice if needed)
	if result.Issues == nil {
		result.Issues = []types.ReviewIssue{}
	}
	if result.Strengths == nil {
		result.Strengths = []string{}
	}
	if result.Suggestions == nil {
		result.Suggestions = []types.ReviewSuggestion{}
	}
	if result.CIAnalysis == nil {
		result.CIAnalysis = []types.CIAnalysis{}
	}
	if result.TestPlan == nil {
		result.TestPlan = []string{}
	}
	if result.Limitations == nil {
		result.Limitations = []string{}
	}

	if result.Verdict != "" && result.Verdict != "approve" && result.Verdict != "changes_requested" && result.Verdict != "comment" {
		return fmt.Errorf("invalid verdict %q (must be approve, changes_requested, or comment)", result.Verdict)
	}

	// Validate each issue
	for i, issue := range result.Issues {
		if issue.File == "" {
			return fmt.Errorf("issue %d: missing file path", i)
		}
		if issue.Severity == "" {
			return fmt.Errorf("issue %d: missing severity", i)
		}
		if issue.Message == "" {
			return fmt.Errorf("issue %d: missing message", i)
		}

		// Normalize severity
		severity := strings.ToLower(issue.Severity)
		if severity != "error" && severity != "warning" && severity != "info" {
			return fmt.Errorf("issue %d: invalid severity %s (must be error, warning, or info)", i, issue.Severity)
		}
	}

	// Validate ISO scores (should be 1-10) only when the response supplied
	// them. types.ReviewResult.ISOScores is *ISOScores (unlike c12d7ab's
	// value field -- see AUR-430's restoration audit) precisely so a
	// response can omit them: this card's engine never asks a model to
	// score ISO/IEC 25010 characteristics (that is internal/review/iso25010,
	// explicitly out of scope here), so treating a missing block as
	// mandatory would reject every response this card's own prompt asks
	// for. A block that *is* present is still held to the same 1-10 range.
	if result.ISOScores != nil {
		if err := p.validateISOScores(result.ISOScores); err != nil {
			return err
		}
	}

	return nil
}

// validateISOScores validates ISO/IEC 25010 scores
func (p *ResponseParser) validateISOScores(scores *types.ISOScores) error {
	scoreMap := map[string]int{
		"functionality":   scores.Functionality,
		"reliability":     scores.Reliability,
		"usability":       scores.Usability,
		"efficiency":      scores.Efficiency,
		"maintainability": scores.Maintainability,
		"portability":     scores.Portability,
		"security":        scores.Security,
		"compatibility":   scores.Compatibility,
	}

	for name, score := range scoreMap {
		if score < 1 || score > 10 {
			return fmt.Errorf("ISO score %s must be between 1-10, got %d", name, score)
		}
	}

	return nil
}

// sanitizeModelIssueProvenance clears the engine-owned Origin of every
// model-reported issue and drops an assessment that is not well formed.
func sanitizeModelIssueProvenance(issues []types.ReviewIssue) {
	for i := range issues {
		issues[i].Origin = ""
		a := issues[i].Assessment
		if a == nil {
			continue
		}
		a.Status = strings.ToLower(strings.TrimSpace(a.Status))
		a.EvidenceID = strings.TrimSpace(a.EvidenceID)
		if !types.IsKnownAssessmentStatus(a.Status) || a.EvidenceID == "" {
			issues[i].Assessment = nil
		}
	}
}

// errMissingFindingsList is the validation failure of an answer that names
// no "issues" list. A "line_comments" list does not count: it is not part
// of the answer schema, so a reply carrying only it is inconclusive.
var errMissingFindingsList = errors.New("missing issues list")

// hasFindingsList reports whether the decoded answer object carries a
// findings list. Without one, a reply such as {"answer":"ok"} would decode
// into an empty list of issues -- an approval the model never gave. A
// provider that does not enforce the answer schema (no response_format)
// can return exactly that, so the parser holds every reply to it.
func hasFindingsList(jsonContent string) bool {
	var fields map[string]json.RawMessage
	if json.Unmarshal([]byte(jsonContent), &fields) != nil {
		return false
	}
	raw, ok := fields[findingsListKey]
	return ok && strings.TrimSpace(string(raw)) != "null"
}
