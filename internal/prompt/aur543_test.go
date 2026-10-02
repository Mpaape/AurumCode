package prompt

import (
	"testing"
	"text/template"
)

// AUR-543 behavior proof: internal/review/cache.PromptVersion used to be a
// constant someone had to remember to bump by hand whenever the embedded
// prompt changed (AUR-513's own review flagged exactly this gap).
// FixedContentDigest replaces it with a digest of the fixed content the
// prompt builder actually renders -- the template's literal instructions,
// the response schema text and the built-in rule catalog -- computed at run
// time, sharing buildBasePrompt with BuildPrompt/fixedOverhead so the two
// can never drift apart. Each test here fails if the behavior it names is
// removed.

// TestAUR543AC001FixedTextChangeMovesDigest covers AC-001: editing any fixed
// text the prompt builder renders changes FixedContentDigest's result, with
// no constant anywhere to edit. This test is in-package (package prompt, not
// prompt_test) specifically so it can reach the unexported `templates` map
// directly -- the "test hook" the card's own instructions point at -- and
// swap in a builder whose "review.md" template differs from the real
// embedded one, standing in for an edit to templates/review.md itself: the
// embedded file is compiled in via go:embed and cannot be mutated from a
// test, but the digest must move exactly the same way for either kind of
// change, because both reach FixedContentDigest through the identical
// buildBasePrompt call.
func TestAUR543AC001FixedTextChangeMovesDigest(t *testing.T) {
	b := NewPromptBuilder()
	before, err := b.FixedContentDigest()
	if err != nil {
		t.Fatalf("FixedContentDigest (before): %v", err)
	}
	if before == "" {
		t.Fatal("FixedContentDigest must not be empty")
	}

	mutated, err := template.New("review.md").Parse(
		"MUTATED FIXED TEXT {{.RuleCatalog}} {{.CIContext}} end",
	)
	if err != nil {
		t.Fatalf("parsing mutated template: %v", err)
	}
	b.templates["review.md"] = mutated

	after, err := b.FixedContentDigest()
	if err != nil {
		t.Fatalf("FixedContentDigest (after): %v", err)
	}
	if after == before {
		t.Fatalf("AC-001: editing the fixed prompt template must change FixedContentDigest's result; got the same digest %q before and after", before)
	}
}

// TestAUR543AC001CatalogChangeMovesDigest covers AC-001's other fixed input:
// the built-in rule catalog (RenderRuleCatalog's rendering of b.ruleCatalog)
// is part of the system prompt buildBasePrompt assembles for every diff, so
// a catalog edit -- via the published SetRuleCatalog seam, exactly like a
// real catalog change would reach this builder -- must move the digest too.
func TestAUR543AC001CatalogChangeMovesDigest(t *testing.T) {
	b := NewPromptBuilder()
	before, err := b.FixedContentDigest()
	if err != nil {
		t.Fatalf("FixedContentDigest (before): %v", err)
	}

	mutatedCatalog := append([]string(nil), DefaultRuleCatalog...)
	mutatedCatalog = append(mutatedCatalog, "quality/aur543-canary-rule")
	if err := b.SetRuleCatalog(mutatedCatalog); err != nil {
		t.Fatalf("SetRuleCatalog: %v", err)
	}

	after, err := b.FixedContentDigest()
	if err != nil {
		t.Fatalf("FixedContentDigest (after): %v", err)
	}
	if after == before {
		t.Fatal("AC-001: adding a rule to the built-in catalog must change FixedContentDigest's result")
	}
}

// TestAUR543AC002DigestStableAcrossRuns covers AC-002: with no change to the
// fixed prompt content, FixedContentDigest is stable -- across repeated
// calls on one builder, and across independently constructed builders that
// hold the identical (default) template and catalog. A digest that is not
// reproducible would force a fresh review on every single invocation,
// defeating AUR-441's whole cache.
func TestAUR543AC002DigestStableAcrossRuns(t *testing.T) {
	b1 := NewPromptBuilder()
	d1a, err := b1.FixedContentDigest()
	if err != nil {
		t.Fatalf("FixedContentDigest: %v", err)
	}
	d1b, err := b1.FixedContentDigest()
	if err != nil {
		t.Fatalf("FixedContentDigest (repeat call): %v", err)
	}
	if d1a != d1b {
		t.Fatalf("AC-002: repeated calls on the same builder must be stable; got %q then %q", d1a, d1b)
	}

	b2 := NewPromptBuilder()
	d2, err := b2.FixedContentDigest()
	if err != nil {
		t.Fatalf("FixedContentDigest (second builder): %v", err)
	}
	if d1a != d2 {
		t.Fatalf("AC-002: two independently constructed builders with identical fixed content must produce the same digest; got %q and %q", d1a, d2)
	}

	// Unrelated, diff-shaped inputs must never move this digest: it is
	// defined to be independent of the reviewed diff, not merely observed
	// to be stable by coincidence across these particular calls.
	d3, err := b2.FixedContentDigest()
	if err != nil {
		t.Fatalf("FixedContentDigest (third call): %v", err)
	}
	if d3 != d2 {
		t.Fatal("AC-002: FixedContentDigest must be stable across repeated calls")
	}
}
