// Provider selection: which LLM provider the environment configures for a
// review, and the errors that name what is missing.
package main

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"

	"github.com/Mpaape/AurumCode/internal/llm"
)

func selectProvider() (llm.Provider, error) {
	p, _, err := providerFromEnv("fixture", strings.TrimSpace(os.Getenv("LLM_MODEL")))
	if err != nil {
		return nil, err
	}
	if p == nil {
		// The first-run message names the complete fixture shape, rule_id
		// included, because a finding without a known rule_id is discarded.
		return nil, errNoProviderConfigured
	}
	return p, nil
}

// redactedEndpoint renders a configured endpoint URL for the stderr
// selection note with any userinfo password already masked at the origin
// (url.Redacted, AUR-432): LLM_BASE_URL=http://user:PASS@host must never
// echo PASS back on the success path. The stderr redaction writer
// additionally replaces the entire userinfo component with the stable
// marker, so not even the username reaches the terminal; this origin fix
// exists so no call path -- present or future -- starts from a string
// that still carries the password. An unparseable value is returned as
// given and left to the writer's structural rules.
func redactedEndpoint(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.User == nil {
		return raw
	}
	return u.Redacted()
}

// reportModelUnavailable prints the clear, actionable error the card
// promises when the model chosen with --modelo cannot review: which model,
// why it is unavailable, and how to configure a provider that serves it --
// including a local one. It returns the command's exit code, 1, the same
// "the review itself failed" code AUR-430 already documents; the one thing
// this path must never do is report an empty review with exit 0.
func reportModelUnavailable(stderr io.Writer, model string, reason error) int {
	fmt.Fprintf(stderr, "aurumcode review: model %q is unavailable: %v\n", model, reason)
	fmt.Fprintf(stderr, "aurumcode review: to serve model %q: set AURUMCODE_LLM_FIXTURE=<response-file> for a deterministic offline run -- <response-file> is a JSON file shaped like tests/fixtures/review/known-problem-response.json (an \"issues\" array of file/line/severity/message) -- or set LLM_API_KEY and LLM_BASE_URL to an OpenAI-compatible endpoint that serves it -- a local endpoint works, e.g. LLM_BASE_URL=http://localhost:11434/v1 (ollama) or a litellm proxy in front of any local model -- then re-run with --modelo %s\n", model, model)
	return 1
}
