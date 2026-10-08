package review

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Mpaape/AurumCode/internal/llm"
	"github.com/Mpaape/AurumCode/internal/prompt"
)

// The capture keeps the review's own prompt; a verification prompt is
// captured beside it, so neither hides the other.
func TestCaptureKeepsReviewPromptAndVerificationBeside(t *testing.T) {
	capture := filepath.Join(t.TempDir(), "prompt.txt")
	p := &FakeProvider{Response: "{}", CapturePath: capture}
	for _, text := range []string{"prompt da revisao", prompt.VerificationMarker + "\nprompt da verificacao"} {
		if _, err := p.Complete(text, llm.Options{}); err != nil {
			t.Fatal(err)
		}
	}
	review, _ := os.ReadFile(capture)
	verification, _ := os.ReadFile(capture + VerificationCaptureSuffix)
	if string(review) != "prompt da revisao" || len(verification) == 0 {
		t.Fatalf("review capture %q, verification capture %q", review, verification)
	}
}
