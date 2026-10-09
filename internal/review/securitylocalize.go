package review

import (
	"strings"

	"github.com/Mpaape/AurumCode/internal/i18n"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// LocalizeSecurityFindings shows the security pass's own text (the rule's
// description at the start of the message, and its fix as the suggestion)
// in the review's language, when the i18n catalog carries it. The rule
// citation, the standard reference and every other finding are unchanged;
// English is the catalog's own text.
func LocalizeSecurityFindings(language string, issues []types.ReviewIssue) []types.ReviewIssue {
	if i18n.LocaleOf(language) == i18n.English {
		return issues
	}
	rules, err := sharedRules()
	if err != nil {
		return issues
	}
	for i := range issues {
		rule, ok := rules.Get(issues[i].RuleID)
		if !ok {
			continue
		}
		base := "security." + strings.TrimPrefix(rule.ID, "security/")
		if text, ok := catalogText(language, base+".description"); ok && rule.Description != "" && strings.HasPrefix(issues[i].Message, rule.Description) {
			issues[i].Message = text + strings.TrimPrefix(issues[i].Message, rule.Description)
		}
		if text, ok := catalogText(language, base+".fix"); ok && rule.Fix != "" && issues[i].Suggestion == rule.Fix {
			issues[i].Suggestion = text
		}
	}
	return issues
}

// catalogText is i18n.Text that reports a missing key instead of echoing it.
func catalogText(language, key string) (string, bool) {
	text := i18n.Text(language, key)
	return text, text != key
}
