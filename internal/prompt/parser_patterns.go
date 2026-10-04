package prompt

import (
	"fmt"
	"regexp"
	"sync"
)

// The response parser's patterns, compiled once for the package.
var (
	// jsonFencePattern is a ```json (or bare) fenced block.
	jsonFencePattern = regexp.MustCompile("```(?:json)?\\s*\\n?([\\s\\S]*?)\\n?```")
	// rawJSONPattern is the widest {...} span of a response.
	rawJSONPattern = regexp.MustCompile(`\{[\s\S]*\}`)
	// trailingCommaPattern is a comma before a closing bracket or brace.
	trailingCommaPattern = regexp.MustCompile(`,\s*([}\]])`)
	// markdownFencePattern is a ```markdown, ```md or bare fenced block.
	markdownFencePattern = regexp.MustCompile("```(?:markdown|md)?\\s*\\n?([\\s\\S]*?)\\n?```")
	// anyFencePattern is a fenced block with any language tag.
	anyFencePattern = regexp.MustCompile("```[\\w]*\\s*\\n?([\\s\\S]*?)\\n?```")
	boldPattern     = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	italicPattern   = regexp.MustCompile(`\*([^*]+)\*`)
	headerPattern   = regexp.MustCompile(`^#{1,6}\s+`)
	// preamblePatterns are the "Here is..." openings a model puts before
	// the content it was asked for.
	preamblePatterns = []*regexp.Regexp{
		regexp.MustCompile(`^Here is.*?:\s*`),
		regexp.MustCompile(`^Here's.*?:\s*`),
		regexp.MustCompile(`^I'll.*?:\s*`),
		regexp.MustCompile(`^I've.*?:\s*`),
		regexp.MustCompile(`^The.*?is:\s*`),
		regexp.MustCompile(`^Below is.*?:\s*`),
	}
)

// languageFences caches the fenced-block pattern of each test language, so
// each one is compiled once per process.
var languageFences sync.Map

// languageFencePattern is a fenced block optionally tagged with language.
func languageFencePattern(language string) *regexp.Regexp {
	if re, ok := languageFences.Load(language); ok {
		return re.(*regexp.Regexp)
	}
	re := regexp.MustCompile(fmt.Sprintf("```(?:%s)?\\s*\\n?([\\s\\S]*?)\\n?```", language))
	languageFences.Store(language, re)
	return re
}
