package main

import (
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/security/redaction"
)

// TestRedactedEndpoint pins the AUR-432 origin fix for the --modelo
// endpoint note: a password carried in LLM_BASE_URL userinfo is masked at
// the source with url.Redacted, the host and path survive, and values
// without userinfo pass through byte for byte.
func TestRedactedEndpoint(t *testing.T) {
	// Synthetic, runtime-assembled; accepted by nothing.
	password := "fake-" + "origin-pass-0517"

	got := redactedEndpoint("http://aurum:" + password + "@127.0.0.1:11434/v1")
	if strings.Contains(got, password) {
		t.Fatal("the endpoint password survived redactedEndpoint")
	}
	if !strings.Contains(got, "127.0.0.1:11434") || !strings.Contains(got, "/v1") {
		t.Fatalf("redactedEndpoint destroyed the endpoint identity: %q", got)
	}

	for _, plain := range []string{
		"http://localhost:11434/v1",
		"https://api.example.invalid",
		"not a url at all",
		"",
	} {
		if got := redactedEndpoint(plain); got != plain {
			t.Errorf("redactedEndpoint(%q) = %q, want the input unchanged", plain, got)
		}
	}
}

// TestStderrLinesSurviveTheRedactionWriter pins that every canonical
// stderr line this command publishes is identity under the AUR-009
// redaction filter, which is what makes wrapping stderr with
// redaction.NewWriter (AUR-432) free on the published contract.
func TestStderrLinesSurviveTheRedactionWriter(t *testing.T) {
	filter := redaction.NewFilter()
	lines := []string{
		"usage: aurumcode <review|docs> [flags]",
		"aurumcode review: --base is required",
		`aurumcode review: reviewing with model "local" (offline fixture provider)`,
		"aurumcode review: 1 finding(s) at severity error or above (--fail-on error)",
		`aurumcode review: model "local" is unavailable: no LLM provider is configured to serve it`,
		"aurumcode review: to serve model \"local\": set AURUMCODE_LLM_FIXTURE=<response-file> for a deterministic offline run, or set LLM_API_KEY and LLM_BASE_URL to an OpenAI-compatible endpoint that serves it -- a local endpoint works, e.g. LLM_BASE_URL=http://localhost:11434/v1 (ollama) or a litellm proxy in front of any local model -- then re-run with --modelo local",
		"aurumcode review: --modelo: model name must not be empty",
		// AUR-426's docs subcommand.
		"aurumcode docs: source directory /abs/missing: stat /abs/missing: no such file or directory",
		"aurumcode docs: source directory /abs/file.txt is not a directory",
		"aurumcode docs: no documentation page was generated",
		"aurumcode docs: partial run: 1 extraction error(s), 1 language(s) skipped",
		// AUR-438's --pr review-and-publish path.
		"aurumcode review: --repo is required with --pr",
		"aurumcode review: --na-linha is required with --pr",
		"aurumcode review: --publicar is required with --pr",
		"aurumcode review: --pr must be a positive pull request number",
		"aurumcode review: refusing to publish: an inline comment requires a commit SHA and none is available; set GITHUB_SHA",
		// AUR-433 (--limite): the two cost lines and the refusal message,
		// all newly written to this same redacted stderr writer.
		"aurumcode review: estimated cost $0.0914, diff-only pre-flight (--limite $0.5000)",
		"aurumcode review: actual cost $0.0181 (--limite $0.5000)",
		`aurumcode review: refusing to call the model: the request's cost would exceed --limite $0.1000; nothing was spent (LLM request failed: budget exceeded: insufficient budget for default (model "default"))`,
		"aurumcode review: --limite: value must not be empty",
		`aurumcode review: --limite: must be a number of US dollars (got "abc")`,
		`aurumcode review: --limite: must be greater than zero (got "0")`,
		"aurumcode review: AURUMCODE_LLM_INPUT_USD_PER_1K is set but AURUMCODE_LLM_OUTPUT_USD_PER_1K is not; set both or neither, because a missing side prices those tokens at $0",
		// AUR-439 (--check): the one new stderr line this card writes --
		// publishCheckStatus's SetStatus transport/API failure. The
		// published "failure"/"success" state itself goes to stdout (see
		// runPRReview), not through this stderr writer.
		"aurumcode review: publishing check status: HTTP 500: synthetic transport failure",
	}
	for _, line := range lines {
		if got := filter.Redact(line); got != line {
			t.Errorf("a canonical stderr line is not filter-identity:\n  in:  %q\n  out: %q", line, got)
		}
	}

	// And the one stderr line that must NOT be identity: an endpoint note
	// whose URL still carried userinfo is fully masked by the writer.
	leaky := `aurumcode review: reviewing with model "m" (litellm endpoint http://user:xxxxx@127.0.0.1:1234/v1)`
	got := filter.Redact(leaky)
	if strings.Contains(got, "user:xxxxx@") {
		t.Fatalf("the stderr filter left URL userinfo in place: %q", got)
	}
	if !strings.Contains(got, redaction.Marker+"@127.0.0.1:1234") {
		t.Fatalf("the stderr filter destroyed the endpoint host: %q", got)
	}
}
