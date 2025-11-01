package prompt

import (
	"aurumcode/pkg/types"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// ResponseParser parses LLM responses into structured data
type ResponseParser struct{}

// NewResponseParser creates a new response parser
func NewResponseParser() *ResponseParser {
	return &ResponseParser{}
}

// ParseReviewResponse parses a review response from LLM
func (p *ResponseParser) ParseReviewResponse(response string) (*types.ReviewResult, error) {
	// Extract JSON from response (handle markdown code blocks)
	jsonContent := p.extractJSON(response)

	if jsonContent == "" {
		return nil, fmt.Errorf("no JSON found in response")
	}

	// Parse JSON
	var result types.ReviewResult
	if err := json.Unmarshal([]byte(jsonContent), &result); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	// Validate
	if err := p.validateReviewResult(&result); err != nil {
		return nil, err
	}

	return &result, nil
}

// extractJSON extracts JSON content from response, handling markdown code blocks
func (p *ResponseParser) extractJSON(response string) string {
	// Try to find JSON in markdown code blocks
	codeBlockPattern := regexp.MustCompile("```(?:json)?\\s*\\n?([\\s\\S]*?)\\n?```")
	matches := codeBlockPattern.FindStringSubmatch(response)

	if len(matches) > 1 {
		return p.repairJSON(strings.TrimSpace(matches[1]))
	}

	// Try to find raw JSON (look for { ... })
	jsonPattern := regexp.MustCompile(`\{[\s\S]*\}`)
	matches = jsonPattern.FindStringSubmatch(response)

	if len(matches) > 0 {
		return p.repairJSON(strings.TrimSpace(matches[0]))
	}

	return ""
}

// repairJSON attempts to repair common JSON malformations
func (p *ResponseParser) repairJSON(jsonStr string) string {
	// Remove trailing commas before closing brackets/braces
	repaired := regexp.MustCompile(`,\s*([}\]])`).ReplaceAllString(jsonStr, "$1")

	// Normalize smart quotes to standard quotes
	repaired = strings.ReplaceAll(repaired, """, "\"")
	repaired = strings.ReplaceAll(repaired, """, "\"")
	repaired = strings.ReplaceAll(repaired, "'", "'")
	repaired = strings.ReplaceAll(repaired, "'", "'")

	// Trim whitespace
	repaired = strings.TrimSpace(repaired)

	return repaired
}

// validateReviewResult validates a review result
func (p *ResponseParser) validateReviewResult(result *types.ReviewResult) error {
	// Check if issues is nil (create empty slice if needed)
	if result.Issues == nil {
		result.Issues = []types.ReviewIssue{}
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

	// Validate ISO scores (should be 1-10)
	if err := p.validateISOScores(&result.ISOScores); err != nil {
		return err
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

// ParseDocumentationResponse parses documentation from LLM response
func (p *ResponseParser) ParseDocumentationResponse(response string) (string, error) {
	// Documentation is typically markdown, so just clean it up
	content := strings.TrimSpace(response)

	if content == "" {
		return "", fmt.Errorf("empty documentation response")
	}

	// Remove markdown code block markers if present
	codeBlockPattern := regexp.MustCompile("```(?:markdown|md)?\\s*\\n?([\\s\\S]*?)\\n?```")
	matches := codeBlockPattern.FindStringSubmatch(content)

	if len(matches) > 1 {
		content = strings.TrimSpace(matches[1])
	}

	return content, nil
}

// ParseTestResponse parses generated tests from LLM response
func (p *ResponseParser) ParseTestResponse(response string, language string) (string, error) {
	// Tests are typically in code blocks
	content := strings.TrimSpace(response)

	if content == "" {
		return "", fmt.Errorf("empty test response")
	}

	// Try to extract from code block
	pattern := fmt.Sprintf("```(?:%s)?\\s*\\n?([\\s\\S]*?)\\n?```", language)
	codeBlockPattern := regexp.MustCompile(pattern)
	matches := codeBlockPattern.FindStringSubmatch(content)

	if len(matches) > 1 {
		return strings.TrimSpace(matches[1]), nil
	}

	// If no code block, look for any code block
	anyCodeBlock := regexp.MustCompile("```[\\w]*\\s*\\n?([\\s\\S]*?)\\n?```")
	matches = anyCodeBlock.FindStringSubmatch(content)

	if len(matches) > 1 {
		return strings.TrimSpace(matches[1]), nil
	}

	// Return as-is if no code blocks found
	return content, nil
}

// ParseSummaryResponse parses a summary from LLM response
func (p *ResponseParser) ParseSummaryResponse(response string) (string, error) {
	content := strings.TrimSpace(response)

	if content == "" {
		return "", fmt.Errorf("empty summary response")
	}

	// Remove any markdown formatting
	content = p.cleanMarkdown(content)

	return content, nil
}

// cleanMarkdown removes basic markdown formatting
func (p *ResponseParser) cleanMarkdown(text string) string {
	// Remove bold/italic
	text = regexp.MustCompile(`\*\*([^*]+)\*\*`).ReplaceAllString(text, "$1")
	text = regexp.MustCompile(`\*([^*]+)\*`).ReplaceAllString(text, "$1")

	// Remove headers
	text = regexp.MustCompile(`^#{1,6}\s+`).ReplaceAllString(text, "")

	// Remove code blocks
	text = regexp.MustCompile("```[\\w]*\\s*\\n?([\\s\\S]*?)\\n?```").ReplaceAllString(text, "$1")

	return strings.TrimSpace(text)
}

// ExtractCodeBlocks extracts all code blocks from response
func (p *ResponseParser) ExtractCodeBlocks(response string) []string {
	var blocks []string

	pattern := regexp.MustCompile("```[\\w]*\\s*\\n?([\\s\\S]*?)\\n?```")
	matches := pattern.FindAllStringSubmatch(response, -1)

	for _, match := range matches {
		if len(match) > 1 {
			blocks = append(blocks, strings.TrimSpace(match[1]))
		}
	}

	return blocks
}

// SanitizeResponse removes common LLM artifacts from response
func (p *ResponseParser) SanitizeResponse(response string) string {
	// Remove "Here is..." prefixes
	patterns := []string{
		`^Here is.*?:\s*`,
		`^Here's.*?:\s*`,
		`^I'll.*?:\s*`,
		`^I've.*?:\s*`,
		`^The.*?is:\s*`,
		`^Below is.*?:\s*`,
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		response = re.ReplaceAllString(response, "")
	}

	return strings.TrimSpace(response)
}
