package main

// AUR-458 unit proof, in-package: the decision "this run did not review"
// and the diagnosis it prints, tested directly rather than through the
// binary. tests/unit/AUR-458.go proves the observable exit codes,
// tests/integration/AUR-458.go proves the security pass survives, and
// tests/e2e/AUR-458.sh proves the CI-shaped invocation -- this file is the
// only one that can reach the unexported cascade, because it lives in
// package main.

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/llm"
	"github.com/Mpaape/AurumCode/internal/prompt"
)

// TestQualityNotReviewedExitCodeIsOne pins the taxonomy choice this card
// made deliberately: "did not review" reuses the existing behavioral
// failure code 1 and mints nothing new, and it is distinct from exit 3,
// which claims a COMPLETE review found something at the threshold.
func TestQualityNotReviewedExitCodeIsOne(t *testing.T) {
	if exitQualityNotReviewed != 1 {
		t.Fatalf("exit code for a review that did not happen must be 1 (the existing behavioral-failure code), got %d", exitQualityNotReviewed)
	}
	if exitQualityNotReviewed == exitFindings {
		t.Fatal("'did not review' must not share an exit code with 'reviewed and found findings at the threshold'")
	}
}

// TestReportQualityFailureDiagnoses proves the extracted cascade keeps a
// distinct, actionable diagnosis for each way a quality review can fail
// after a provider was selected, and that every one of them is a non-zero
// exit. The extraction exists so the "return immediately" caller and the
// "keep --seguranca alive" caller can never print different text for the
// same failure.
func TestReportQualityFailureDiagnoses(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		modelo string
		want   string
	}{
		{
			name: "budget refused before the model was called",
			err:  fmt.Errorf("LLM request failed: %w: insufficient budget", llm.ErrBudgetExceeded),
			want: "refusing to call the model",
		},
		{
			name: "response does not parse",
			err:  fmt.Errorf("parsing: %w", &prompt.ParseError{Kind: "no_json_found"}),
			want: "could not understand the model's response",
		},
		{
			name:   "chosen model unavailable",
			err:    fmt.Errorf("LLM request failed: %w: dial tcp: connection refused", llm.ErrAllProvidersFailed),
			modelo: "llama3",
			want:   `model "llama3" is unavailable`,
		},
		{
			name: "transport failure with no --modelo",
			err:  fmt.Errorf("LLM request failed: %w: dial tcp: connection refused", llm.ErrAllProvidersFailed),
			want: "all providers failed",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			rc := reportQualityFailure(&buf, tc.err, tc.modelo, 0.5)
			if rc == 0 {
				t.Fatalf("a quality review that failed must never report exit 0, got %d", rc)
			}
			if rc != exitQualityNotReviewed {
				t.Fatalf("want exit %d, got %d", exitQualityNotReviewed, rc)
			}
			if got := buf.String(); !strings.Contains(got, tc.want) {
				t.Fatalf("diagnosis must contain %q, got %q", tc.want, got)
			}
		})
	}
}

// TestReportQualityFailureNeverClaimsCleanliness is the defect this card
// exists to kill, at the unit level: no failure diagnosis may ever contain
// the phrase the tool prints when it reviewed and found nothing.
func TestReportQualityFailureNeverClaimsCleanliness(t *testing.T) {
	var buf bytes.Buffer
	reportQualityFailure(&buf, errors.New("boom"), "", 0)
	if strings.Contains(buf.String(), "No issues found.") {
		t.Fatal("a failed review must never print the clean-review sentence")
	}
}
