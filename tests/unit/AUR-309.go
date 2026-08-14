//go:build aur309_characterization

package unit

import (
	"errors"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/documentation/extractors"
	legacy "github.com/Mpaape/AurumCode/internal/pipeline"
)

func TestAUR309(t *testing.T) {
	canary := "aur309-unit-canary"
	t.Setenv("AURUM_SECRET_CANARY", canary)
	skip := legacy.LanguageSkip{
		Language: extractors.LanguageJava,
		Reason:   legacy.SkipValidationFailed,
		Detail:   "tool failed with " + canary,
		Files:    1,
	}
	rendered := skip.String()
	if strings.Contains(rendered, canary) || !strings.Contains(rendered, "[REDACTED]") {
		t.Fatalf("LanguageSkip.String did not redact its bounded effect: %q", rendered)
	}

	sentinel := errors.New("nested extractor failure")
	extractionError := &legacy.ExtractionError{
		SourceFiles: 1,
		Errors:      []error{sentinel},
		Skipped:     []legacy.LanguageSkip{skip},
	}
	if !errors.Is(extractionError, sentinel) {
		t.Fatal("ExtractionError.Unwrap did not expose the nested cause")
	}
	message := extractionError.Error()
	if strings.Contains(message, canary) || !strings.Contains(message, "produced no documentation") {
		t.Fatalf("ExtractionError.Error did not preserve its typed, redacted outcome: %q", message)
	}
}
