package prompt

import (
	"fmt"
	"strings"
	"testing"
)

// A rule catalog is rendered whole at any size (issue #179): a refused one
// aborted the review of every policy that outgrew a fixed ceiling, and a cut
// one made the model cite a rule that existed but was gone.
func TestSetRuleCatalogAcceptsACatalogOfAnySize(t *testing.T) {
	ids := append([]string(nil), DefaultRuleCatalog...)
	for i := 0; i < 5000; i++ {
		ids = append(ids, fmt.Sprintf("policy#reg-%04d-a-value-of-this-kind-never-goes-back-to-the-agent-without-review", i))
	}
	b := NewPromptBuilder()
	if err := b.SetRuleCatalog(ids); err != nil {
		t.Fatalf("a large catalog was refused: %v", err)
	}
	section, err := b.ruleCatalogSection()
	if err != nil {
		t.Fatalf("a large catalog does not render: %v", err)
	}
	if got := strings.Count(section, "- `"); got != len(ids) {
		t.Fatalf("the rendered list has %d ids, want all %d: it was cut", got, len(ids))
	}
}

func TestEmptyRuleCatalogIsStillRefused(t *testing.T) {
	if err := NewPromptBuilder().SetRuleCatalog(nil); err == nil {
		t.Fatal("an empty catalog would tell the model to choose from nothing, and was accepted")
	}
}
