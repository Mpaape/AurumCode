package iso25010

import (
	"aurumcode/internal/analyzer"
	"aurumcode/pkg/types"
	"testing"
)

func TestScorer(t *testing.T) {
	config := &Config{
		Weights: Weights{
			Functionality:   0.15,
			Reliability:     0.15,
			Usability:       0.10,
			Efficiency:      0.12,
			Maintainability: 0.18,
			Portability:     0.08,
			Security:        0.17,
			Compatibility:   0.05,
		},
		Thresholds: Thresholds{
			Excellent:  90,
			Good:       75,
			Acceptable: 60,
			Poor:       40,
			Critical:   0,
		},
	}

	scorer := NewScorer(config)

	// Create test data
	result := &types.ReviewResult{
		Issues: []types.ReviewIssue{
			{
				RuleID:   "security/test",
				Severity: "error",
			},
			{
				RuleID:   "quality/test",
				Severity: "warning",
			},
		},
		ISOScores: types.ISOScores{
			Functionality:   90,
			Reliability:     85,
			Usability:       90,
			Efficiency:      85,
			Maintainability: 80,
			Portability:     90,
			Security:        85,
			Compatibility:   90,
		},
	}

	metrics := &analyzer.DiffMetrics{
		TotalFiles:   1,
		LinesAdded:   10,
		LinesDeleted: 5,
	}

	// Score
	scores := scorer.Score(result, metrics)

	// Verify scores are adjusted
	if scores.Security >= 85 {
		t.Error("Expected security score to be penalized for security issue")
	}

	if scores.Functionality >= 90 {
		t.Error("Expected functionality score to be penalized for error")
	}

	// Verify all scores are clamped
	if scores.Security < 0 || scores.Security > 100 {
		t.Errorf("Security score out of range: %d", scores.Security)
	}
}

func TestClamp(t *testing.T) {
	tests := []struct {
		value    int
		min      int
		max      int
		expected int
	}{
		{50, 0, 100, 50},
		{-10, 0, 100, 0},
		{150, 0, 100, 100},
		{0, 0, 100, 0},
		{100, 0, 100, 100},
	}

	for _, tt := range tests {
		result := clamp(tt.value, tt.min, tt.max)
		if result != tt.expected {
			t.Errorf("clamp(%d, %d, %d) = %d, want %d",
				tt.value, tt.min, tt.max, result, tt.expected)
		}
	}
}

func TestGetQualityLevel(t *testing.T) {
	config := &Config{
		Weights: Weights{
			Functionality:   0.125,
			Reliability:     0.125,
			Usability:       0.125,
			Efficiency:      0.125,
			Maintainability: 0.125,
			Portability:     0.125,
			Security:        0.125,
			Compatibility:   0.125,
		},
		Thresholds: Thresholds{
			Excellent:  90,
			Good:       75,
			Acceptable: 60,
			Poor:       40,
			Critical:   0,
		},
	}

	scorer := NewScorer(config)

	tests := []struct {
		scores types.ISOScores
		want   string
	}{
		{
			scores: types.ISOScores{
				Functionality: 95, Reliability: 95, Usability: 95, Efficiency: 95,
				Maintainability: 95, Portability: 95, Security: 95, Compatibility: 95,
			},
			want: "excellent",
		},
		{
			scores: types.ISOScores{
				Functionality: 80, Reliability: 80, Usability: 80, Efficiency: 80,
				Maintainability: 80, Portability: 80, Security: 80, Compatibility: 80,
			},
			want: "good",
		},
		{
			scores: types.ISOScores{
				Functionality: 30, Reliability: 30, Usability: 30, Efficiency: 30,
				Maintainability: 30, Portability: 30, Security: 30, Compatibility: 30,
			},
			want: "critical",
		},
	}

	for _, tt := range tests {
		got := scorer.GetQualityLevel(tt.scores)
		if got != tt.want {
			t.Errorf("GetQualityLevel() = %s, want %s", got, tt.want)
		}
	}
}
