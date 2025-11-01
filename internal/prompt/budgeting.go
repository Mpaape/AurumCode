package prompt

import (
	"aurumcode/internal/analyzer"
	"aurumcode/pkg/types"
	"fmt"
	"sort"
)

// TokenBudget manages token allocation and context trimming
type TokenBudget struct {
	estimator TokenEstimator
	maxTokens int
	reserved  int // Tokens reserved for reply
}

// NewTokenBudget creates a new token budget manager
func NewTokenBudget(estimator TokenEstimator, maxTokens, reserveReply int) *TokenBudget {
	return &TokenBudget{
		estimator: estimator,
		maxTokens: maxTokens,
		reserved:  reserveReply,
	}
}

// Available returns tokens available for context
func (b *TokenBudget) Available() int {
	return b.maxTokens - b.reserved
}

// BuildContextSegments creates prioritized segments from diff
func (b *TokenBudget) BuildContextSegments(diff *types.Diff, detector *analyzer.LanguageDetector) []ContextSegment {
	segments := []ContextSegment{}

	for _, file := range diff.Files {
		// Determine file priority based on type
		filePriority := b.determineFilePriority(file.Path, detector)

		for hunkIdx, hunk := range file.Hunks {
			// Create segment for each hunk
			content := b.formatHunk(&file, &hunk)
			tokens := b.estimator.Estimate(content)

			segment := ContextSegment{
				Content:  content,
				Priority: filePriority,
				SortKey:  fmt.Sprintf("%s:%d", file.Path, hunkIdx),
				Tokens:   tokens,
			}

			segments = append(segments, segment)
		}
	}

	return segments
}

// determineFilePriority assigns priority based on file type
func (b *TokenBudget) determineFilePriority(path string, detector *analyzer.LanguageDetector) PriorityTier {
	// Test files are lower priority
	if detector.IsTestFile(path) {
		return PriorityLow
	}

	// Config files are medium priority
	if detector.IsConfigFile(path) {
		return PriorityMedium
	}

	// Source code is high priority
	return PriorityHigh
}

// formatHunk formats a hunk for context
func (b *TokenBudget) formatHunk(file *types.DiffFile, hunk *types.DiffHunk) string {
	result := fmt.Sprintf("### File: %s\n", file.Path)
	result += "```diff\n"
	for _, line := range hunk.Lines {
		result += line + "\n"
	}
	result += "```\n"
	return result
}

// TrimToFit trims segments to fit within available tokens
func (b *TokenBudget) TrimToFit(segments []ContextSegment, baseTokens int) []ContextSegment {
	available := b.Available() - baseTokens
	if available <= 0 {
		return []ContextSegment{}
	}

	// Sort by priority (high to low), then by sort key for determinism
	sorted := make([]ContextSegment, len(segments))
	copy(sorted, segments)

	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Priority != sorted[j].Priority {
			return sorted[i].Priority < sorted[j].Priority
		}
		return sorted[i].SortKey < sorted[j].SortKey
	})

	// Accumulate segments until budget exhausted
	result := []ContextSegment{}
	currentTokens := 0

	for _, segment := range sorted {
		if currentTokens+segment.Tokens <= available {
			result = append(result, segment)
			currentTokens += segment.Tokens
		} else if len(result) == 0 {
			// If even the first high-priority segment doesn't fit, truncate it
			truncated := b.truncateSegment(segment, available)
			result = append(result, truncated)
			break
		} else {
			// Stop adding segments
			break
		}
	}

	return result
}

// truncateSegment truncates a segment to fit within maxTokens
func (b *TokenBudget) truncateSegment(segment ContextSegment, maxTokens int) ContextSegment {
	if segment.Tokens <= maxTokens {
		return segment
	}

	// Rough approximation: 1 token ~= 4 characters
	maxChars := maxTokens * 4
	if len(segment.Content) <= maxChars {
		return segment
	}

	truncated := segment
	truncated.Content = segment.Content[:maxChars] + "\n... (truncated) ...\n"
	truncated.Tokens = maxTokens

	return truncated
}

// EstimateTotal estimates total tokens for segments
func (b *TokenBudget) EstimateTotal(segments []ContextSegment) int {
	total := 0
	for _, segment := range segments {
		total += segment.Tokens
	}
	return total
}
