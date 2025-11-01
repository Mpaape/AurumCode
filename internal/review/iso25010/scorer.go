package iso25010

import (
	"aurumcode/internal/analyzer"
	"aurumcode/pkg/types"
	"math"
	"strings"
)

// Scorer computes ISO/IEC 25010 quality scores
type Scorer struct {
	config *Config
}

// NewScorer creates a new ISO/IEC 25010 scorer
func NewScorer(config *Config) *Scorer {
	return &Scorer{
		config: config,
	}
}

// Score computes quality scores for a review result
func (s *Scorer) Score(result *types.ReviewResult, metrics *analyzer.DiffMetrics) types.ISOScores {
	// Start with base scores from LLM (if present)
	scores := result.ISOScores

	// Apply static analysis adjustments
	s.applyStaticSignals(&scores, result, metrics)

	// Clamp all scores to 0-100 range
	s.clampScores(&scores)

	return scores
}

// applyStaticSignals adjusts scores based on static analysis
func (s *Scorer) applyStaticSignals(scores *types.ISOScores, result *types.ReviewResult, metrics *analyzer.DiffMetrics) {
	// Count issue severities
	errorCount := 0
	warningCount := 0
	infoCount := 0

	for _, issue := range result.Issues {
		switch strings.ToLower(issue.Severity) {
		case "error":
			errorCount++
		case "warning":
			warningCount++
		case "info":
			infoCount++
		}
	}

	// Adjust functionality based on errors
	scores.Functionality -= errorCount * 5
	scores.Functionality -= warningCount * 2

	// Adjust reliability based on error handling issues
	scores.Reliability -= errorCount * 4
	scores.Reliability -= warningCount * 3

	// Adjust maintainability based on code quality issues
	scores.Maintainability -= infoCount * 1

	// Adjust security based on security issues
	securityIssues := s.countIssuesByCategory(result.Issues, "security")
	scores.Security -= securityIssues * 10

	// Adjust efficiency based on performance issues
	performanceIssues := s.countIssuesByCategory(result.Issues, "performance")
	scores.Efficiency -= performanceIssues * 8

	// Calculate overall score as weighted average
	overall := s.calculateOverall(*scores)
	_ = overall // For future use
}

// countIssuesByCategory counts issues in a specific category
func (s *Scorer) countIssuesByCategory(issues []types.ReviewIssue, category string) int {
	count := 0
	for _, issue := range issues {
		// Check if rule ID contains category
		if strings.Contains(strings.ToLower(issue.RuleID), category) {
			count++
		}
	}
	return count
}

// calculateOverall computes weighted overall score
func (s *Scorer) calculateOverall(scores types.ISOScores) int {
	w := s.config.Weights

	overall := float64(scores.Functionality)*w.Functionality +
		float64(scores.Reliability)*w.Reliability +
		float64(scores.Usability)*w.Usability +
		float64(scores.Efficiency)*w.Efficiency +
		float64(scores.Maintainability)*w.Maintainability +
		float64(scores.Portability)*w.Portability +
		float64(scores.Security)*w.Security +
		float64(scores.Compatibility)*w.Compatibility

	return int(math.Round(overall))
}

// clampScores ensures all scores are within 0-100 range
func (s *Scorer) clampScores(scores *types.ISOScores) {
	scores.Functionality = clamp(scores.Functionality, 0, 100)
	scores.Reliability = clamp(scores.Reliability, 0, 100)
	scores.Usability = clamp(scores.Usability, 0, 100)
	scores.Efficiency = clamp(scores.Efficiency, 0, 100)
	scores.Maintainability = clamp(scores.Maintainability, 0, 100)
	scores.Portability = clamp(scores.Portability, 0, 100)
	scores.Security = clamp(scores.Security, 0, 100)
	scores.Compatibility = clamp(scores.Compatibility, 0, 100)
}

// clamp limits a value to a range
func clamp(value, min, max int) int {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

// GetQualityLevel returns the quality level for given scores
func (s *Scorer) GetQualityLevel(scores types.ISOScores) string {
	overall := s.calculateOverall(scores)
	return s.config.GetQualityLevel(overall)
}
