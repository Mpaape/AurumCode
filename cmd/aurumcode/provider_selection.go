// Provider selection: which LLM provider the environment configures for a
// review, and the errors that name what is missing.
package main

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"

	"github.com/Mpaape/AurumCode/internal/llm"
)

// selectProvider is the one place cmd/aurumcode names a specific LLM
// vendor, and it does so only to satisfy an environment variable the
// operator set -- nothing here is hardwired to one provider. Two modes are
// supported:
//
//   - AURUMCODE_LLM_FIXTURE=<path>: read that file's content and use it
//     verbatim as the model's response, via review.FakeProvider. This is
//     how tests/acceptance/AUR-430.sh runs the real binary fully offline
//     and deterministically (the sandbox this card's acceptance runs under
//     denies network access entirely).
//   - LLM_API_KEY and LLM_BASE_URL: use the existing, already-vendor-neutral
//     internal/llm/provider/litellm.Provider (an OpenAI-compatible endpoint).
//     LLM_MODEL is forwarded when present; when omitted, the endpoint may
//     choose its own configured default.
//
// Neither set: a clear, typed-by-message error, not a panic or a silent
// no-op provider.
// errNoProviderConfigured is selectProvider's error when neither an
// offline fixture (AURUMCODE_LLM_FIXTURE) nor a live endpoint
// (LLM_API_KEY + LLM_BASE_URL) is configured -- as opposed to any other
// provider failure (an AURUMCODE_LLM_FIXTURE path that does not exist, a
// malformed endpoint). AUR-449's --seguranca-only skip (runReview above)
// tests for this exact sentinel with errors.Is: "the caller configured
// nothing at all" is eligible to fall back to the deterministic security
// pass alone, but a caller who attempted configuration and got it wrong is
// still told the review failed, never silently downgraded.
// The message text is AUR-448's: the COMPLETE fixture shape the engine
// accepts, rule_id included, because enforceRuleCitations (AUR-434)
// silently discards a finding whose rule_id is missing, and the
// pre-AUR-448 shape omitted it. See selectProvider's own comment below and
// docs/specs/AUR-448.md.
var errNoProviderConfigured = errors.New(`no LLM provider configured: set AURUMCODE_LLM_FIXTURE=<path> to a JSON file shaped like {"issues":[{"file":"<path>","line":<n>,"severity":"error|warning|info","rule_id":"<id from the embedded rule catalog, e.g. security/hardcoded-secret>","message":"<text>"}]} for offline use -- a finding whose rule_id is missing or unknown is discarded, never shown, so rule_id is not optional -- if you have the AurumCode source checked out, tests/fixtures/review/known-problem-response.json is a worked example -- or set LLM_API_KEY and LLM_BASE_URL for a live provider`)

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
