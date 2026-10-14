package prompt

import (
	"bytes"
	"fmt"
	"text/template"
)

// triageTemplateName is the embedded template of the dedicated triage of
// deterministic evidence the review left unassessed.
const triageTemplateName = "triage.md"

// TriageMarker is the triage template's fixed first line: an offline
// fixture provider answers triage prompts on it.
const TriageMarker = "AURUMCODE-TRIAGEM-DE-EVIDENCIA"

// TriageInput is the evidence the model is asked to decide.
type TriageInput struct {
	Language string
	Items    []EvidenceItem
}

// BuildTriagePrompt renders the triage prompt of items.
func BuildTriagePrompt(in TriageInput) (string, error) {
	content, err := templateFS.ReadFile("templates/" + triageTemplateName)
	if err != nil {
		return "", fmt.Errorf("triage template: %w", err)
	}
	tmpl, err := template.New(triageTemplateName).Parse(string(content))
	if err != nil {
		return "", fmt.Errorf("triage template: %w", err)
	}
	var out bytes.Buffer
	if err := tmpl.Execute(&out, in); err != nil {
		return "", fmt.Errorf("triage template: %w", err)
	}
	return out.String(), nil
}
