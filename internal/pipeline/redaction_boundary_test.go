package pipeline

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/documentation/extractors"
	"github.com/Mpaape/AurumCode/internal/documentation/site"
)

// syntheticToolToken builds an obviously fake credential-shaped token at test
// runtime. It is never printed by these tests.
func syntheticToolToken(prefix string, filler byte, length int) string {
	return prefix + strings.Repeat(string(filler), length)
}

// TestLanguageSkipStringRedactsToolDetail covers extractor_pipeline.go:387 ->
// log.Printf: skip.Detail is verbatim external tool output.
func TestLanguageSkipStringRedactsToolDetail(t *testing.T) {
	token := syntheticToolToken("ghp_", 'C', 36)

	skip := LanguageSkip{
		Language: extractors.Language("go"),
		Reason:   SkipValidationFailed,
		Detail:   "gomarkdoc failed (stderr: auth=" + token + ")",
		Files:    2,
	}

	rendered := skip.String()
	if strings.Contains(rendered, token) {
		t.Fatal("credential-shaped token survived into the rendered skip line")
	}

	if !strings.Contains(rendered, site.RedactionMarker) {
		t.Fatal("rendered skip line carries no redaction marker")
	}

	if !strings.Contains(rendered, "2 file(s)") {
		t.Fatalf("rendered skip line lost its diagnostic content: %q", rendered)
	}
}

// TestExtractionErrorRedactsCauses covers the aggregated error that the pipeline
// returns to the Action entrypoint, which prints it.
func TestExtractionErrorRedactsCauses(t *testing.T) {
	token := syntheticToolToken("sk-", 'A', 32)

	err := &ExtractionError{
		SourceFiles:    3,
		FilesProcessed: 1,
		Errors:         []error{fmt.Errorf("go extraction failed: command failed: exit status 2 (stderr: %s)", token)},
	}

	rendered := err.Error()
	if strings.Contains(rendered, token) {
		t.Fatal("credential-shaped token survived into the aggregated extraction error")
	}

	if !strings.Contains(rendered, site.RedactionMarker) {
		t.Fatal("aggregated extraction error carries no redaction marker")
	}

	if !strings.Contains(rendered, "exit status 2") {
		t.Fatalf("aggregated extraction error lost its diagnostic content: %q", rendered)
	}
}
