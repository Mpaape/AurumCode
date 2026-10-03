package prompt

import (
	"fmt"
	"strings"
)

// ParseDocumentationResponse parses documentation from LLM response
func (p *ResponseParser) ParseDocumentationResponse(response string) (string, error) {
	// Documentation is typically markdown, so just clean it up
	content := strings.TrimSpace(response)

	if content == "" {
		return "", fmt.Errorf("empty documentation response")
	}

	// Remove markdown code block markers if present
	matches := markdownFencePattern.FindStringSubmatch(content)

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
	matches := languageFencePattern(language).FindStringSubmatch(content)

	if len(matches) > 1 {
		return strings.TrimSpace(matches[1]), nil
	}

	// If no code block, look for any code block
	matches = anyFencePattern.FindStringSubmatch(content)

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
	text = boldPattern.ReplaceAllString(text, "$1")
	text = italicPattern.ReplaceAllString(text, "$1")

	// Remove headers
	text = headerPattern.ReplaceAllString(text, "")

	// Remove code blocks
	text = anyFencePattern.ReplaceAllString(text, "$1")

	return strings.TrimSpace(text)
}

// ExtractCodeBlocks extracts all code blocks from response
func (p *ResponseParser) ExtractCodeBlocks(response string) []string {
	var blocks []string

	matches := anyFencePattern.FindAllStringSubmatch(response, -1)

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
	for _, re := range preamblePatterns {
		response = re.ReplaceAllString(response, "")
	}

	return strings.TrimSpace(response)
}
