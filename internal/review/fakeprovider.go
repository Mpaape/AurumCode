package review

import (
	"fmt"
	"os"

	"github.com/Mpaape/AurumCode/internal/llm"
)

// FakeProvider is a deterministic llm.Provider that never touches the
// network: Complete always returns the fixed Response it was constructed
// with. AurumCode's review engine talks to a model exclusively through
// llm.Provider (see llm.Orchestrator.Complete in internal/llm, whose
// signature is unchanged from the original engine), so this fake is exactly
// as capable, from the engine's point of view, as any real vendor provider
// under internal/llm/provider/*. That is the proof, not just the claim,
// that nothing in this package is coupled to a specific LLM vendor: the
// review pipeline cannot tell FakeProvider apart from a real one without
// inspecting its Name().
//
// It is used both by this package's own tests and, via
// AURUMCODE_LLM_FIXTURE, by cmd/aurumcode itself so the acceptance test can
// run the real CLI binary fully offline against a deterministic response
// (see tests/fixtures/review and docs/specs/AUR-430.md).
type FakeProvider struct {
	// Response is returned verbatim as the completion text.
	Response string
	// NameStr, if set, is returned by Name(); otherwise Name() returns
	// "fake".
	NameStr string
	// CapturePath, when non-empty, names a file this provider writes the
	// exact prompt it received to (mode 0600, truncating) before
	// answering. It exists so an offline, deterministic run can OBSERVE
	// what would have left the process toward a real model:
	// tests/acceptance/AUR-432.sh proves the AUR-432 redaction guarantee
	// against this capture, and MUT-001 proves the leak through it. The
	// live providers under internal/llm/provider/* have no equivalent --
	// this is a diagnostic property of the offline fixture provider only,
	// and with the AUR-432 wiring in place the captured prompt is already
	// redacted, so the capture file never holds a secret either.
	CapturePath string
}

// Complete implements llm.Provider.
func (f *FakeProvider) Complete(prompt string, opts llm.Options) (llm.Response, error) {
	if f.CapturePath != "" {
		if err := os.WriteFile(f.CapturePath, []byte(prompt), 0o600); err != nil {
			return llm.Response{}, fmt.Errorf("writing prompt capture %s: %w", f.CapturePath, err)
		}
	}
	return llm.Response{
		Text:      f.Response,
		TokensIn:  heuristicTokenCount(prompt),
		TokensOut: heuristicTokenCount(f.Response),
		Model:     f.Name(),
	}, nil
}

// Tokens implements llm.Provider with the same ~4-chars-per-token heuristic
// used elsewhere in this engine (internal/llm/estimator.go,
// internal/prompt/estimator.go) when a provider cannot report an exact
// count.
func (f *FakeProvider) Tokens(input string) (int, error) {
	return heuristicTokenCount(input), nil
}

// Name implements llm.Provider.
func (f *FakeProvider) Name() string {
	if f.NameStr == "" {
		return "fake"
	}
	return f.NameStr
}

func heuristicTokenCount(s string) int {
	if s == "" {
		return 0
	}
	if n := len(s) / 4; n > 0 {
		return n
	}
	return 1
}
