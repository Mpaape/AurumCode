package render

import (
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/security/redaction"
)

// The catalog's rule title in a finding's citation is product text: the
// redaction filter must not read "hardcoded-secret: Hardcoded Secrets" as a
// `secret: value` pair, and no pass leaves a stray "]" behind.
func TestSARIFKeepsTheRuleTitleOfASecretRule(t *testing.T) {
	findings := []SARIFFinding{{RuleID: "security/hardcoded-secret", RuleTitle: "Hardcoded Secrets", Path: "app.go", Line: 6, Severity: "error",
		Message: "Possível segredo no código (rule security/hardcoded-secret: Hardcoded Secrets)"}}
	out, err := BuildCategorizedSARIF("v1", "aurumcode", findings, redaction.NewFilter())
	if err != nil {
		t.Fatal(err)
	}
	text := string(out)
	if !strings.Contains(text, "(rule security/hardcoded-secret - Hardcoded Secrets)") || strings.Contains(text, "[REDACTED]") {
		t.Fatalf("the rule title was masked:\n%s", text)
	}
}
