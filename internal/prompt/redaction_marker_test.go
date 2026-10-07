package prompt

import (
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/analyzer"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// redactionMarkerInstruction is the sentence of the review template that
// tells the model the redaction marker is the reviewer's own mask over the
// original value, never evidence of a secret in the change.
const redactionMarkerInstruction = "Nunca reporte segredo, literal\n  mascarado ou credencial com base nesse marcador."

// The redactor replaces values such as `APIKey: key` before the prompt; the
// model sees only the marker. The rendered review prompt must say that the
// marker is not a literal of the change and that secret detection belongs to
// the secret scanner over the raw content.
func TestReviewPromptExplainsTheRedactionMarker(t *testing.T) {
	b := NewPromptBuilder()
	diff := &types.Diff{Files: []types.DiffFile{{Path: "resolve.go", Hunks: []types.DiffHunk{{NewStart: 1, Lines: []string{"+\tAPIKey:  " + redaction.Marker + ","}}}}}}
	metrics := analyzer.NewDiffAnalyzer().AnalyzeDiff(diff)
	parts, err := b.BuildPrompt(diff, metrics, BuildOptions{SchemaKind: "review"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"`" + redaction.Marker + "` é a máscara que o próprio Aurum aplica",
		"pode ser só um identificador ou uma expressão",
		"Não é\n  literal da mudança nem evidência de segredo",
		"scanner de segredos sobre o conteúdo bruto",
		redactionMarkerInstruction,
	} {
		if !strings.Contains(parts.System, want) {
			t.Fatalf("the review prompt does not explain the redaction marker: missing %q", want)
		}
	}
}
