package prompt

import "testing"

func TestHeuristicEstimator_Empty(t *testing.T) {
	e := NewHeuristicEstimator()
	if got := e.Estimate(""); got != 0 {
		t.Errorf("Estimate(\"\") = %d, want 0", got)
	}
}

func TestHeuristicEstimator_NeverZeroForNonEmpty(t *testing.T) {
	e := NewHeuristicEstimator()
	if got := e.Estimate("a"); got != 1 {
		t.Errorf("Estimate(\"a\") = %d, want 1 (never free)", got)
	}
	if got := e.Estimate("abc"); got != 1 {
		t.Errorf("Estimate(\"abc\") = %d, want 1", got)
	}
}

func TestHeuristicEstimator_FourCharsPerToken(t *testing.T) {
	e := NewHeuristicEstimator()
	if got := e.Estimate("12345678"); got != 2 {
		t.Errorf("Estimate(8 chars) = %d, want 2", got)
	}
	if got := e.Estimate("1234567890123456"); got != 4 {
		t.Errorf("Estimate(16 chars) = %d, want 4", got)
	}
}

func TestHeuristicEstimator_ImplementsTokenEstimator(t *testing.T) {
	var _ TokenEstimator = NewHeuristicEstimator()
}
