package prompt

import (
	"testing"
)

func TestPromptParts(t *testing.T) {
	parts := PromptParts{
		System: "You are a code reviewer",
		User:   "Review this code",
		Meta:   map[string]string{"task": "review"},
	}

	if parts.System == "" {
		t.Error("System should not be empty")
	}

	if parts.User == "" {
		t.Error("User should not be empty")
	}

	if parts.Meta["task"] != "review" {
		t.Errorf("Expected meta task 'review', got %s", parts.Meta["task"])
	}
}

func TestBuildOptions(t *testing.T) {
	opts := BuildOptions{
		MaxTokens:    4000,
		SchemaKind:   "review",
		Role:         "reviewer",
		ReserveReply: 1000,
	}

	if opts.MaxTokens != 4000 {
		t.Errorf("Expected MaxTokens 4000, got %d", opts.MaxTokens)
	}

	if opts.ReserveReply != 1000 {
		t.Errorf("Expected ReserveReply 1000, got %d", opts.ReserveReply)
	}
}

func TestPriorityTier(t *testing.T) {
	if PriorityHigh != 1 {
		t.Errorf("Expected PriorityHigh to be 1, got %d", PriorityHigh)
	}

	if PriorityMedium != 2 {
		t.Errorf("Expected PriorityMedium to be 2, got %d", PriorityMedium)
	}

	if PriorityLow != 3 {
		t.Errorf("Expected PriorityLow to be 3, got %d", PriorityLow)
	}
}

func TestContextSegment(t *testing.T) {
	segment := ContextSegment{
		Content:  "func main() {}",
		Priority: PriorityHigh,
		SortKey:  "file.go:1",
		Tokens:   10,
	}

	if segment.Priority != PriorityHigh {
		t.Errorf("Expected PriorityHigh, got %d", segment.Priority)
	}

	if segment.Tokens != 10 {
		t.Errorf("Expected 10 tokens, got %d", segment.Tokens)
	}
}

// StubEstimator is a test implementation of TokenEstimator
type StubEstimator struct {
	fixedCount int
}

func (s *StubEstimator) Estimate(text string) int {
	if s.fixedCount > 0 {
		return s.fixedCount
	}
	// Default: 1 token per 4 characters
	return len(text) / 4
}

func TestStubEstimator(t *testing.T) {
	estimator := &StubEstimator{fixedCount: 100}

	count := estimator.Estimate("any text")
	if count != 100 {
		t.Errorf("Expected 100 tokens, got %d", count)
	}

	// Test default heuristic
	estimator2 := &StubEstimator{}
	count2 := estimator2.Estimate("1234567890123456") // 16 chars = 4 tokens
	if count2 != 4 {
		t.Errorf("Expected 4 tokens, got %d", count2)
	}
}
