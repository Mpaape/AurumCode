package review

import (
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/scanner/gitleaks"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// TestAUR607IgnoreFileFindingKeepsItsOwnLabel: the finding about a
// .gitleaksignore is not a leaked secret, so in pt-BR it says what the file
// does instead of borrowing the hardcoded-secret label; English is untouched.
func TestAUR607IgnoreFileFindingKeepsItsOwnLabel(t *testing.T) {
	issue := types.ReviewIssue{
		File:    ".gitleaksignore",
		RuleID:  gitleaks.RuleIgnoreFilePresent,
		Message: "a .gitleaksignore can hide gitleaks findings and no gitleaks flag disables it; under a central policy its presence is a finding (rule " + gitleaks.RuleIgnoreFilePresent + ")",
	}
	pt := LocalizeSecretFinding("pt-BR", secretsCategory, "gitleaks", issue)
	if strings.Contains(pt.Message, "Segredo ou credencial") {
		t.Fatalf("ignore-file finding must not carry the secret label: %q", pt.Message)
	}
	if !strings.Contains(pt.Message, "O arquivo .gitleaksignore pode esconder achados do Gitleaks") || !strings.Contains(pt.Message, "`"+gitleaks.RuleIgnoreFilePresent+"`") {
		t.Fatalf("pt-BR ignore-file finding: %q", pt.Message)
	}
	if en := LocalizeSecretFinding("en", secretsCategory, "gitleaks", issue); en.Message != issue.Message {
		t.Fatalf("English must keep the engine text: %q", en.Message)
	}
}
