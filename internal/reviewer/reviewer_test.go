package reviewer

import (
	"aurumcode/internal/llm"
	"aurumcode/internal/llm/cost"
	"aurumcode/pkg/types"
	"context"
	"testing"
)

type mockProvider struct {
	response string
}

func (m *mockProvider) Complete(prompt string, opts llm.Options) (llm.Response, error) {
	return llm.Response{
		Text: m.response,
		TokensIn: 50,
		TokensOut: 50,
		Model: "test",
	}, nil
}

func (m *mockProvider) Name() string { return "mock" }
func (m *mockProvider) Tokens(input string) (int, error) { return len(input) / 4, nil }

func TestReview(t *testing.T) {
	mock := &mockProvider{
		response: `{"issues":[],"iso_scores":{"functionality":8,"reliability":8,"usability":8,"efficiency":8,"maintainability":8,"portability":8,"security":8,"compatibility":8},"summary":"Good"}`,
	}

	// Create tracker with test budgets and empty price map
	tracker := cost.NewTracker(100.0, 1000.0, map[string]cost.PriceMap{})
	orchestrator := llm.NewOrchestrator(mock, []llm.Provider{}, tracker)
	reviewer := NewReviewer(orchestrator)

	diff := &types.Diff{
		Files: []types.DiffFile{{Path: "test.go"}},
	}

	result, err := reviewer.Review(context.Background(), diff)
	if err != nil {
		t.Fatalf("Review failed: %v", err)
	}

	if result.Cost.Tokens != 100 {
		t.Errorf("expected 100 tokens, got %d", result.Cost.Tokens)
	}
}
