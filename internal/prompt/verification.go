package prompt

import (
	"bytes"
	"fmt"
	"text/template"
)

// verificationTemplateName is the embedded template of the verification
// of one model finding.
const verificationTemplateName = "verify.md"

// VerificationMarker is the template's fixed first line: an offline
// fixture provider answers verification prompts on it.
const VerificationMarker = "AURUMCODE-VERIFICACAO-DE-ACHADO"

// VerificationExcerpt is one window of the reviewed revision shown to the
// verifier.
type VerificationExcerpt struct {
	Path      string
	StartLine int
	EndLine   int
	// Label says why the window is shown (the cited line or a symbol the
	// finding names).
	Label string
	Text  string
}

// VerificationInput is the finding and the code the verifier judges it on.
type VerificationInput struct {
	Language string
	RuleID   string
	File     string
	Line     int
	Severity string
	Message  string
	Evidence string
	Excerpts []VerificationExcerpt
}

// BuildVerificationPrompt renders the verification prompt of one finding.
func BuildVerificationPrompt(in VerificationInput) (string, error) {
	content, err := templateFS.ReadFile("templates/" + verificationTemplateName)
	if err != nil {
		return "", fmt.Errorf("verification template: %w", err)
	}
	tmpl, err := template.New(verificationTemplateName).Parse(string(content))
	if err != nil {
		return "", fmt.Errorf("verification template: %w", err)
	}
	var out bytes.Buffer
	if err := tmpl.Execute(&out, in); err != nil {
		return "", fmt.Errorf("verification template: %w", err)
	}
	return out.String(), nil
}
