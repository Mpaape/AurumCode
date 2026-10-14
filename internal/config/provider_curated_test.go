package config

import (
	"context"
	"strings"
	"testing"
)

// curatedTextProvider is a text provider that declares itself the policy's
// own file.
type curatedTextProvider struct{ pendingTextProvider }

func (curatedTextProvider) Curated() {}

// A policy of any size reaches the model (issue #179): the repository's and
// the policy's own files are exempt from the ceiling that guards the prompt
// from an external source, and an external source is not.
func TestCuratedProviderIsNotBoundedByTheContributionCeiling(t *testing.T) {
	big := strings.Repeat("x", MaxProviderContributionBytes+1)

	block, _, err := BuildContextBlockWithWarnings(context.Background(),
		[]ContextProvider{curatedTextProvider{pendingTextProvider{name: "skills", text: big}}}, []string{"a.go"}, nil)
	if err != nil {
		t.Fatalf("a curated provider over the external ceiling was refused: %v", err)
	}
	if !strings.Contains(block, big) {
		t.Fatalf("the curated contribution was cut: the block has %d bytes", len(block))
	}

	_, _, err = BuildContextBlockWithWarnings(context.Background(),
		[]ContextProvider{pendingTextProvider{name: "mcp", text: big}}, []string{"a.go"}, nil)
	if err == nil || !isContributionLimitError(err) {
		t.Fatalf("an external provider over the ceiling must stay a loud error, got %v", err)
	}
}
