package review

import (
	"errors"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/llm"
)

// A fixture standing in for a model with a small window refuses a longer
// prompt the way a real provider does: a typed 400 the reviewer recognizes.
func TestFakeProviderWindowRefusesALongerPrompt(t *testing.T) {
	provider := &FakeProvider{Response: approveResponse, Window: 100}

	if _, err := provider.Complete(strings.Repeat("x", 400), llm.Options{}); err != nil {
		t.Fatalf("a prompt of exactly the window was refused: %v", err)
	}
	_, err := provider.Complete(strings.Repeat("x", 800), llm.Options{})
	var api *llm.APIError
	if !errors.As(err, &api) || api.Status != 400 {
		t.Fatalf("error = %v, want a 400 *llm.APIError", err)
	}
	if got, ok := llm.AsPromptTooLong(err); !ok || got.Limit != 100 || got.Requested != 200 {
		t.Fatalf("AsPromptTooLong = %+v, %v; want the window 100 and the 200 tokens received", got, ok)
	}
}

func TestFakeProviderWithoutWindowTakesAnyPrompt(t *testing.T) {
	provider := &FakeProvider{Response: approveResponse}
	if _, err := provider.Complete(strings.Repeat("x", 4_000_000), llm.Options{}); err != nil {
		t.Fatalf("a provider with no window refused a prompt: %v", err)
	}
}
