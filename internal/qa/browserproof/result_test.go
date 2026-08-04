package browserproof_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/Mpaape/AurumCode/internal/qa/browserproof"
)

func provedResult(t *testing.T) browserproof.BrowserProofResultV1 {
	t.Helper()

	result, err := browserproof.New(browserproof.NewScriptedDriver()).
		Run(context.Background(), scriptedRequest(fixtureDir(t, "nominal")))
	if err != nil {
		t.Fatalf("nominal run: %v", err)
	}

	return result
}

// TestBrowserProofResultV1RejectsUnsubstantiatedProof is MUT-003: a verdict that
// only declares "proved": true, with no observed selector and no extracted text,
// is rejected instead of being accepted as proof of function.
func TestBrowserProofResultV1RejectsUnsubstantiatedProof(t *testing.T) {
	bare := []byte(`{"proved": true}`)
	if _, err := browserproof.ParseResultV1(bare); err == nil {
		t.Fatal("MUT-003: a bare proved flag was accepted as proof")
	}

	shaped := provedResult(t)
	shaped.Routes[0].Selector = ""
	shaped.Routes[0].ObservedText = ""

	encoded, err := json.Marshal(shaped)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	parsed, err := browserproof.ParseResultV1(encoded)
	if err == nil {
		t.Fatalf("MUT-003: proof without selector or extracted text was accepted: %+v", parsed)
	}
	if !errors.Is(err, browserproof.ErrUnsubstantiatedProof) {
		t.Fatalf("MUT-003: want ErrUnsubstantiatedProof, got %v", err)
	}
	if parsed.Proved {
		t.Fatal("MUT-003: a rejected verdict must not come back proved")
	}
}

func TestBrowserProofResultV1RejectsInconsistentVerdicts(t *testing.T) {
	cases := map[string]func(r *browserproof.BrowserProofResultV1){
		"failed assertion under proved": func(r *browserproof.BrowserProofResultV1) {
			r.Routes[0].Assertion = browserproof.AssertionFail
		},
		"proved with a refusal code": func(r *browserproof.BrowserProofResultV1) {
			r.Code = browserproof.CodeTargetAbsent
		},
		"proved without routes": func(r *browserproof.BrowserProofResultV1) {
			r.Routes = nil
		},
		"proved without artifact digest": func(r *browserproof.BrowserProofResultV1) {
			r.ArtifactDigest = ""
		},
		"proved without driver digest": func(r *browserproof.BrowserProofResultV1) {
			r.DriverDigest = ""
		},
		"proved flag contradicting outcome": func(r *browserproof.BrowserProofResultV1) {
			r.Outcome = browserproof.OutcomeInconclusive
		},
		"observed text missing the expected text": func(r *browserproof.BrowserProofResultV1) {
			r.Routes[0].ObservedText = "something else"
		},
		"raw markup smuggled into the evidence": func(r *browserproof.BrowserProofResultV1) {
			r.Routes[0].ObservedText = "<h1 id=\"symbol-name\">ExtractorPipeline</h1>"
		},
		"unproved verdict without a code": func(r *browserproof.BrowserProofResultV1) {
			r.Proved = false
			r.Outcome = browserproof.OutcomeRefused
		},
	}

	for name, mutate := range cases {
		mutate := mutate
		t.Run(name, func(t *testing.T) {
			result := provedResult(t)
			mutate(&result)

			if err := result.Validate(); err == nil {
				t.Fatalf("inconsistent verdict accepted: %+v", result)
			}
		})
	}
}

func TestBrowserProofResultV1AcceptsARealVerdict(t *testing.T) {
	result := provedResult(t)

	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	parsed, err := browserproof.ParseResultV1(encoded)
	if err != nil {
		t.Fatalf("round-trip of a real verdict failed: %v", err)
	}
	if !parsed.Proved || parsed.Schema != browserproof.SchemaBrowserProofResultV1 {
		t.Fatalf("round-trip lost the verdict: %+v", parsed)
	}
	if parsed.Routes[0].ObservedText != result.Routes[0].ObservedText {
		t.Fatalf("round-trip lost the extracted text: %q", parsed.Routes[0].ObservedText)
	}
}

func TestParseResultV1RejectsUnknownFields(t *testing.T) {
	encoded, err := json.Marshal(provedResult(t))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(encoded, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	raw["screenshot"] = "iVBORw0KGgo="

	smuggled, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	if _, err := browserproof.ParseResultV1(smuggled); err == nil {
		t.Fatal("a verdict carrying a screenshot field was accepted")
	}
}
