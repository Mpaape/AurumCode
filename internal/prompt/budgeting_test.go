package prompt

import (
	"aurumcode/internal/analyzer"
	"aurumcode/pkg/types"
	"testing"
)

func TestNewTokenBudget(t *testing.T) {
	estimator := &StubEstimator{fixedCount: 10}
	budget := NewTokenBudget(estimator, 1000, 200)

	if budget.maxTokens != 1000 {
		t.Errorf("Expected maxTokens 1000, got %d", budget.maxTokens)
	}

	if budget.reserved != 200 {
		t.Errorf("Expected reserved 200, got %d", budget.reserved)
	}
}

func TestAvailable(t *testing.T) {
	estimator := &StubEstimator{}
	budget := NewTokenBudget(estimator, 1000, 200)

	available := budget.Available()
	if available != 800 {
		t.Errorf("Expected 800 available tokens, got %d", available)
	}
}

func TestBuildContextSegments(t *testing.T) {
	estimator := &StubEstimator{fixedCount: 50}
	budget := NewTokenBudget(estimator, 1000, 200)
	detector := analyzer.NewLanguageDetector()

	diff := &types.Diff{
		Files: []types.DiffFile{
			{
				Path: "main.go",
				Hunks: []types.DiffHunk{
					{
						Lines: []string{"+func main() {", "+}", ""},
					},
				},
			},
		},
	}

	segments := budget.BuildContextSegments(diff, detector)

	if len(segments) != 1 {
		t.Fatalf("Expected 1 segment, got %d", len(segments))
	}

	if segments[0].Priority != PriorityHigh {
		t.Errorf("Expected PriorityHigh for source file, got %d", segments[0].Priority)
	}

	if segments[0].Tokens != 50 {
		t.Errorf("Expected 50 tokens, got %d", segments[0].Tokens)
	}
}

func TestDetermineFilePriority(t *testing.T) {
	estimator := &StubEstimator{}
	budget := NewTokenBudget(estimator, 1000, 200)
	detector := analyzer.NewLanguageDetector()

	tests := []struct {
		path     string
		expected PriorityTier
	}{
		{"main.go", PriorityHigh},
		{"main_test.go", PriorityLow},
		{"config.yml", PriorityMedium},
		{"src/handler.go", PriorityHigh},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			priority := budget.determineFilePriority(tt.path, detector)
			if priority != tt.expected {
				t.Errorf("Expected priority %d for %s, got %d", tt.expected, tt.path, priority)
			}
		})
	}
}

func TestTrimToFit(t *testing.T) {
	estimator := &StubEstimator{}
	budget := NewTokenBudget(estimator, 1000, 200)

	segments := []ContextSegment{
		{Content: "high1", Priority: PriorityHigh, SortKey: "a", Tokens: 100},
		{Content: "high2", Priority: PriorityHigh, SortKey: "b", Tokens: 100},
		{Content: "medium", Priority: PriorityMedium, SortKey: "c", Tokens: 300},
		{Content: "low", Priority: PriorityLow, SortKey: "d", Tokens: 500},
	}

	// Budget: 1000 max - 200 reserved - 100 base = 700 available
	trimmed := budget.TrimToFit(segments, 100)

	// Should include: high1 (100) + high2 (100) + medium (300) = 500 tokens
	// low (500) would exceed budget
	if len(trimmed) != 3 {
		t.Errorf("Expected 3 segments, got %d", len(trimmed))
	}

	// Verify order: high priority first
	if trimmed[0].Priority != PriorityHigh {
		t.Errorf("Expected first segment to be PriorityHigh, got %d", trimmed[0].Priority)
	}
}

func TestTrimToFit_Deterministic(t *testing.T) {
	estimator := &StubEstimator{}
	budget := NewTokenBudget(estimator, 1000, 200)

	segments := []ContextSegment{
		{Content: "seg1", Priority: PriorityHigh, SortKey: "file2.go:0", Tokens: 100},
		{Content: "seg2", Priority: PriorityHigh, SortKey: "file1.go:0", Tokens: 100},
		{Content: "seg3", Priority: PriorityHigh, SortKey: "file3.go:0", Tokens: 100},
	}

	// Run multiple times to ensure deterministic ordering
	for i := 0; i < 5; i++ {
		trimmed := budget.TrimToFit(segments, 100)

		if len(trimmed) != 3 {
			t.Errorf("Run %d: Expected 3 segments, got %d", i, len(trimmed))
		}

		// Verify stable sort by SortKey
		if trimmed[0].SortKey != "file1.go:0" {
			t.Errorf("Run %d: Expected first segment file1.go:0, got %s", i, trimmed[0].SortKey)
		}
	}
}

func TestTruncateSegment(t *testing.T) {
	estimator := &StubEstimator{}
	budget := NewTokenBudget(estimator, 1000, 200)

	segment := ContextSegment{
		Content:  "This is a very long content that needs to be truncated to fit within token limits",
		Priority: PriorityHigh,
		SortKey:  "test",
		Tokens:   100,
	}

	truncated := budget.truncateSegment(segment, 10)

	if truncated.Tokens != 10 {
		t.Errorf("Expected 10 tokens after truncation, got %d", truncated.Tokens)
	}

	// Content should be truncated
	if len(truncated.Content) >= len(segment.Content) {
		t.Error("Content was not truncated")
	}
}

func TestEstimateTotal(t *testing.T) {
	estimator := &StubEstimator{}
	budget := NewTokenBudget(estimator, 1000, 200)

	segments := []ContextSegment{
		{Tokens: 100},
		{Tokens: 200},
		{Tokens: 300},
	}

	total := budget.EstimateTotal(segments)
	if total != 600 {
		t.Errorf("Expected 600 total tokens, got %d", total)
	}
}
