package unit

// Unit program for card AUR-479, selector TestAUR479.
//
// Proves, in isolation (no CLI, no git), the split-secret contract of
// internal/config's context-provider seam:
//
//   - AC-001: a registered secret cut across two DIFFERENT contributions
//     (each half harmless alone) does not reach the assembled prompt
//     verbatim;
//   - AC-002: a whole secret confined to one contribution is still
//     redacted, and the per-contribution pass is load-bearing (removing
//     it changes the assembled output for overlapping registered values);
//   - AC-003: redaction of a secret-shaped span that only appears across
//     the contribution boundary does not destroy the legitimate prose on
//     either side of it.

import (
	"context"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/llm"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
)

type aur479Capture struct{ prompt string }

func (c *aur479Capture) Complete(prompt string, _ llm.Options) (llm.Response, error) {
	c.prompt = prompt
	return llm.Response{Text: `{"issues":[]}`}, nil
}
func (c *aur479Capture) Tokens(input string) (int, error) { return len(input), nil }
func (c *aur479Capture) Name() string                     { return "aur479-capture" }

type aur479TextProvider struct {
	name string
	text string
}

func (p aur479TextProvider) Name() string { return p.name }
func (p aur479TextProvider) Provide(context.Context, []string) (string, error) {
	return p.text, nil
}

// aur479Prompt runs the real WrapProvider/compose path and returns the
// exact prompt the underlying provider received.
func aur479Prompt(t *testing.T, filter *redaction.Filter, providers []config.ContextProvider) string {
	t.Helper()
	base := &aur479Capture{}
	wrapped, err := config.WrapProvider(context.Background(), base, providers, []string{"changed.go"}, filter)
	if err != nil {
		t.Fatalf("WrapProvider: %v", err)
	}
	if _, err := wrapped.Complete("BASE", llm.Options{}); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	return base.prompt
}

func TestAUR479(t *testing.T) {
	t.Run("AC-001-SplitSecretIsNotReconstituted", testAUR479SplitSecret)
	t.Run("AC-002-WholeSecretStillRedacted", testAUR479WholeSecret)
	t.Run("AC-003-NoFalsePositiveAcrossBoundary", testAUR479NoFalsePositive)
}

func testAUR479SplitSecret(t *testing.T) {
	const secret = "AURUM-provider-split-canary-9f3a2b7c"
	filter := redaction.NewFilter(secret)
	prompt := aur479Prompt(t, filter, []config.ContextProvider{
		aur479TextProvider{name: "provider-a", text: secret[:12]},
		aur479TextProvider{name: "provider-b", text: secret[12:]},
	})
	// The two halves are joined by a newline, so the literal secret is not
	// contiguous even when the leak is open; ignoring line breaks is what
	// makes the reconstitution visible.
	if strings.Contains(strings.NewReplacer("\n", "", "\r", "").Replace(prompt), secret) {
		t.Fatalf("a secret split across two contributions was reconstituted in the prompt: %q", prompt)
	}
	if !strings.Contains(prompt, redaction.Marker) {
		t.Fatalf("the assembled redaction pass must leave a marker: %q", prompt)
	}
}

func testAUR479WholeSecret(t *testing.T) {
	const secret = "AURUM-whole-contribution-canary-1a2b3c4d"
	filter := redaction.NewFilter(secret)
	prompt := aur479Prompt(t, filter, []config.ContextProvider{
		aur479TextProvider{name: "provider-a", text: "background before " + secret + " background after"},
	})
	if strings.Contains(prompt, secret) {
		t.Fatalf("a whole secret in one contribution reached the prompt: %q", prompt)
	}
	if !strings.Contains(prompt, redaction.Marker) {
		t.Fatalf("expected the redaction marker: %q", prompt)
	}
	if !strings.Contains(prompt, "background before") || !strings.Contains(prompt, "background after") {
		t.Fatalf("legitimate surrounding text must survive per-contribution redaction: %q", prompt)
	}

	// Per-contribution redaction is load-bearing, not decoration. With
	// two registered secrets that overlap across the contribution
	// boundary, redacting each contribution BEFORE the newline join
	// contains the whole secret inside provider-b; redacting only the
	// assembled block lets the spanning value consume the overlap and
	// leave a fragment. MUT-002 removes the per-contribution pass and
	// this exact assertion goes RED.
	filter2 := redaction.NewFilter("abcdef", "defghi")
	prompt2 := aur479Prompt(t, filter2, []config.ContextProvider{
		aur479TextProvider{name: "provider-a", text: "abc"},
		aur479TextProvider{name: "provider-b", text: "defghi"},
	})
	if !strings.Contains(prompt2, "abc\n"+redaction.Marker) {
		t.Fatalf("per-contribution redaction must contain provider-b's whole secret before assembly: %q", prompt2)
	}
	if strings.Contains(prompt2, "defghi") {
		t.Fatalf("provider-b's whole secret must not survive: %q", prompt2)
	}
}

func testAUR479NoFalsePositive(t *testing.T) {
	filter := redaction.NewFilter("unrelated-registered-value")
	prompt := aur479Prompt(t, filter, []config.ContextProvider{
		aur479TextProvider{name: "prose-a", text: `please read note: secret="`},
		aur479TextProvider{name: "prose-b", text: `not-a-credential" and then continue.`},
	})
	if strings.Contains(prompt, "not-a-credential") {
		t.Fatalf("the secret-shaped span crossing the boundary must be redacted: %q", prompt)
	}
	if !strings.Contains(prompt, "please read note:") || !strings.Contains(prompt, "and then continue.") {
		t.Fatalf("legitimate prose on both sides of the redacted span must survive: %q", prompt)
	}
}
