package analysis

import (
	"strings"

	"github.com/Mpaape/AurumCode/internal/i18n"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// catalogMessages pairs each fixed English message of the embedded catalog
// (or, for a public-base secret rule, the fixed start of its message) with
// the i18n key of the same text in the review's language.
var catalogMessages = []struct{ en, key string }{
	{msgHardcodedSecret, "analysis.hardcoded_secret"},
	{msgCommandInjection, "analysis.command_injection"},
	{msgFilePermissions, "analysis.file_permissions"},
	{msgSQLInjection, "analysis.sql_injection"},
	{strings.SplitN(msgBaseSecretFormat, "%s", 2)[0], "analysis.base_secret_prefix"},
}

// Localize rewrites the fixed catalog text at the start of a finding
// message in language; the rest of the message (a public rule's id and
// description, the rule citation) and every other message stay as they
// are. English is the catalog's own text.
func Localize(language, message string) string {
	if i18n.LocaleOf(language) == i18n.English {
		return message
	}
	for _, m := range catalogMessages {
		if strings.HasPrefix(message, m.en) {
			return i18n.Text(language, m.key) + strings.TrimPrefix(message, m.en)
		}
	}
	return message
}

// LocalizeIssues applies Localize to each issue's message, in place.
func LocalizeIssues(language string, issues []types.ReviewIssue) []types.ReviewIssue {
	for i := range issues {
		issues[i].Message = Localize(language, issues[i].Message)
	}
	return issues
}
