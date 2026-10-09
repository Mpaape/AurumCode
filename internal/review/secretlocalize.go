package review

import (
	"regexp"
	"strings"

	"github.com/Mpaape/AurumCode/internal/i18n"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// secretsCategory is the scanner category whose findings are leaked secrets.
const secretsCategory = "secrets"

// secretRuleKey is the catalog label every secret finding leads with,
// whichever source found it: the hardcoded-secret rule's description.
const secretRuleKey = "security.hardcoded-secret.description"

// commitTail matches the end of a secrets engine's message: the commit that
// introduced the leak and the engine's identity, "<text> in commit <id>
// (<engine> <version>)".
var commitTail = regexp.MustCompile(`^(.*) in commit (\S+) \((\S+ \S+)\)$`)

// LocalizeSecretFinding shows a finding of a secrets engine (category
// "secrets") in the review's language: the catalog's hardcoded-secret label
// first, then the rule, then the engine's own text as a detail, with the
// commit and the engine version in parentheses instead of in the middle of
// the sentence. English, and a finding of any other category, keep the
// engine's message as before; the rule id, the place and the severity never
// change.
func LocalizeSecretFinding(language, category, engine string, issue types.ReviewIssue) types.ReviewIssue {
	if i18n.LocaleOf(language) == i18n.English || category != secretsCategory {
		return issue
	}
	detail := strings.TrimSuffix(issue.Message, " (rule "+issue.RuleID+")")
	if m := commitTail.FindStringSubmatch(detail); m != nil {
		detail = i18n.Format(language, "scanner.commit_detail", strings.TrimRight(strings.TrimSpace(m[1]), "."), m[2], m[3])
	}
	issue.Message = i18n.Format(language, "scanner.secret_finding", i18n.Text(language, secretRuleKey), issue.RuleID, displayName(engine), detail)
	return issue
}

// displayName is an engine name as a sentence names it: "gitleaks" reads
// "Gitleaks".
func displayName(engine string) string {
	if engine == "" {
		return engine
	}
	return strings.ToUpper(engine[:1]) + engine[1:]
}
