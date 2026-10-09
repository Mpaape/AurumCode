package review

import (
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/pkg/types"
)

func TestLocalizeSecurityFindings(t *testing.T) {
	rules, err := sharedRules()
	if err != nil {
		t.Fatal(err)
	}
	rule, _ := rules.Get("security/command-injection")
	in := func() []types.ReviewIssue {
		return []types.ReviewIssue{{RuleID: rule.ID, Message: rule.Description + " [standards/security-review SCR-001]", Suggestion: rule.Fix}}
	}
	pt := LocalizeSecurityFindings("pt-BR", in())
	if !strings.HasPrefix(pt[0].Message, "Possível injeção de comando [standards/security-review SCR-001]") || !strings.HasPrefix(pt[0].Suggestion, "Passe os argumentos") {
		t.Fatalf("pt-BR not localized: %+v", pt[0])
	}
	en := LocalizeSecurityFindings("en", in())
	if en[0].Message != rule.Description+" [standards/security-review SCR-001]" || en[0].Suggestion != rule.Fix {
		t.Fatalf("en must stay the catalog text: %+v", en[0])
	}
	other := LocalizeSecurityFindings("pt-BR", []types.ReviewIssue{{RuleID: "quality/x", Message: "texto"}})
	if other[0].Message != "texto" {
		t.Fatalf("a finding of another rule changed: %+v", other[0])
	}
}
