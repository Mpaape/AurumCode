package unit

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/llm"
	"github.com/Mpaape/AurumCode/internal/review"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// aur432CaptureProvider is a deterministic llm.Provider that records every
// prompt it receives, so the test can inspect exactly what would have left
// the process toward a model. The review engine talks to a model only
// through llm.Provider, so what this fake receives is what a real vendor
// provider would have received.
type aur432CaptureProvider struct {
	response string
	prompts  []string
}

func (p *aur432CaptureProvider) Complete(prompt string, opts llm.Options) (llm.Response, error) {
	p.prompts = append(p.prompts, prompt)
	return llm.Response{Text: p.response, Model: p.Name()}, nil
}

func (p *aur432CaptureProvider) Tokens(input string) (int, error) {
	if input == "" {
		return 0, nil
	}
	if n := len(input) / 4; n > 0 {
		return n, nil
	}
	return 1, nil
}

func (p *aur432CaptureProvider) Name() string { return "aur432-capture" }

// aur432Response builds a deterministic model response with one finding in
// the shape internal/prompt.ResponseParser validates, citing an embedded
// rule so it survives the AUR-434 rule-citation gate.
func aur432Response(file string, line int, message, suggestion string) string {
	return fmt.Sprintf(`{
  "issues": [
    {
      "file": %q,
      "line": %d,
      "severity": "error",
      "rule_id": "security/hardcoded-secret",
      "message": %q,
      "suggestion": %q
    }
  ],
  "summary": "Deterministic response for AUR-432."
}`, file, line, message, suggestion)
}

// aur432Review runs one in-process review over diff with the capturing
// provider and returns the provider (with its captured prompts) and the
// result. The reviewer is constructed after the caller had a chance to set
// AURUM_SECRET_CANARY, because the redaction filter registers that value
// at construction.
func aur432Review(t *testing.T, diff *types.Diff, response string) (*aur432CaptureProvider, *types.ReviewResult) {
	t.Helper()
	provider := &aur432CaptureProvider{response: response}
	reviewer := review.NewReviewer(llm.NewOrchestrator(provider, nil, nil), review.DefaultConfig())
	result, err := reviewer.GenerateReview(context.Background(), diff)
	if err != nil {
		t.Fatalf("GenerateReview failed: %v", err)
	}
	if len(provider.prompts) == 0 {
		t.Fatal("the provider never received a prompt")
	}
	return provider, result
}

// TestAUR432 proves AUR-432's outcome at the model boundary, in process:
// a secret present in the reviewed diff never reaches the provider (the
// prompt is redacted before it leaves internal/review), a secret echoed
// back by the model never reaches the report (the parsed result is
// redacted at the same boundary), and the redaction replaces values, not
// context, so the review stays useful. Secret values that must not exist
// in a tracked file (credential shapes, private-key banners) are assembled
// at runtime from split literals.
func TestAUR432(t *testing.T) {
	// Runtime-assembled synthetic secrets. None of these values is real.
	kvSecret := "AURUM-FAKE-UNIT-VALUE-0001"
	skSecret := "sk-" + strings.Repeat("a1b2c3d", 4)
	canary := "AURUM-UNIT-REGISTERED-7431"
	t.Setenv(redaction.CanaryEnv, canary)

	plantedDiff := &types.Diff{Files: []types.DiffFile{{
		Path: "config/creds.env",
		Lang: "env",
		Hunks: []types.DiffHunk{{
			OldStart: 0, OldLines: 0, NewStart: 1, NewLines: 4,
			Lines: []string{
				"+# planted synthetic credentials (AUR-432 unit); never real",
				"+DEMO_API_TOKEN=" + kvSecret,
				"+service_password = \"" + skSecret + "\"",
				"+incident marker " + canary,
			},
		}},
	}}}

	cleanDiff := &types.Diff{Files: []types.DiffFile{{
		Path: "src/greeter.py",
		Lang: "python",
		Hunks: []types.DiffHunk{{
			OldStart: 0, OldLines: 0, NewStart: 1, NewLines: 1,
			Lines:    []string{"+print('hello')"},
		}},
	}}}

	t.Run("send-path-redaction", func(t *testing.T) {
		provider, result := aur432Review(t, plantedDiff,
			aur432Response("config/creds.env", 3, "A planted synthetic credential is committed at this line.", ""))
		prompt := provider.prompts[0]

		for name, secret := range map[string]string{
			"kv-value": kvSecret, "credential-shape": skSecret, "registered-canary": canary,
		} {
			if strings.Contains(prompt, secret) {
				t.Errorf("the %s secret reached the provider prompt", name)
			}
		}
		if !strings.Contains(prompt, redaction.Marker) {
			t.Error("the prompt carries no redaction marker where the secrets were")
		}
		// The redaction must not destroy the review: the model still sees
		// which file, which keys, and THAT a credential sits there.
		for _, context := range []string{"config/creds.env", "DEMO_API_TOKEN=", "service_password"} {
			if !strings.Contains(prompt, context) {
				t.Errorf("redaction destroyed review context %q", context)
			}
		}
		// The finding citing the secret line is still delivered.
		if len(result.Issues) != 1 {
			t.Fatalf("expected 1 issue, got %d: %+v", len(result.Issues), result.Issues)
		}
		if result.Issues[0].File != "config/creds.env" || result.Issues[0].Line != 3 {
			t.Errorf("the finding lost its location: %+v", result.Issues[0])
		}
	})

	t.Run("private-key-block", func(t *testing.T) {
		// The banners are assembled at runtime so no tracked file carries a
		// private-key shape.
		begin := "-----BEGIN " + "PRIVATE KEY-----"
		end := "-----END " + "PRIVATE KEY-----"
		body := "AURUM-FAKE-KEY-BODY-0001"
		keyDiff := &types.Diff{Files: []types.DiffFile{{
			Path: "config/service.pem",
			Lang: "text",
			Hunks: []types.DiffHunk{{
				OldStart: 0, OldLines: 0, NewStart: 1, NewLines: 3,
				Lines:    []string{"+" + begin, "+" + body, "+" + end},
			}},
		}}}
		provider, _ := aur432Review(t, keyDiff,
			aur432Response("config/service.pem", 1, "A private key is committed in plain text.", ""))
		if strings.Contains(provider.prompts[0], body) {
			t.Error("the private-key body reached the provider prompt")
		}
	})

	t.Run("output-boundary-redaction", func(t *testing.T) {
		echoedKV := "AURUM-FAKE-UNIT-VALUE-0002"
		message := "Rotate the leaked value DEMO_DB_PASSWORD=" + echoedKV + " and the marker " + canary + " now."
		suggestion := "Never commit DEMO_DB_PASSWORD=" + echoedKV + " again."
		_, result := aur432Review(t, cleanDiff,
			aur432Response("src/greeter.py", 1, message, suggestion))

		if len(result.Issues) != 1 {
			t.Fatalf("expected 1 issue, got %d", len(result.Issues))
		}
		issue := result.Issues[0]
		for _, field := range []string{issue.Message, issue.Suggestion, result.Summary} {
			if strings.Contains(field, echoedKV) || strings.Contains(field, canary) {
				t.Errorf("a model-echoed secret survived into the report: %q", redactionSafe(field))
			}
		}
		if !strings.Contains(issue.Message, redaction.Marker) {
			t.Error("the echoed secret was not replaced with the redaction marker")
		}
		// The redaction happens before the trusted rule citation is
		// appended, so the published citation suffix survives byte for byte.
		if !strings.HasSuffix(issue.Message, "(rule security/hardcoded-secret: Hardcoded Secrets)") {
			t.Errorf("the rule citation suffix was damaged: %q", redactionSafe(issue.Message))
		}
	})

	t.Run("determinism", func(t *testing.T) {
		response := aur432Response("config/creds.env", 3, "A planted synthetic credential is committed at this line.", "")
		firstProvider, firstResult := aur432Review(t, plantedDiff, response)
		secondProvider, secondResult := aur432Review(t, plantedDiff, response)
		if firstProvider.prompts[0] != secondProvider.prompts[0] {
			t.Error("the redacted prompt is not deterministic")
		}
		if len(firstResult.Issues) != len(secondResult.Issues) {
			t.Fatalf("non-deterministic issue count: %d vs %d", len(firstResult.Issues), len(secondResult.Issues))
		}
		for i := range firstResult.Issues {
			if firstResult.Issues[i] != secondResult.Issues[i] {
				t.Errorf("non-deterministic issue at %d", i)
			}
		}
	})
}

// redactionSafe re-redacts a string before it can reach the test log, so a
// failing assertion never prints a leaked value verbatim.
func redactionSafe(s string) string {
	return redaction.FromEnv().Redact(s)
}
