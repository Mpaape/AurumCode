// The text a person reads for one finding in the parecer and in its line
// comment: a short title in the review's language and who found it, never
// the engine's raw English, the rule citation or an internal code. The
// rule, the place and the severity the gate reads never change here.
package main

import (
	"regexp"
	"strings"

	"github.com/Mpaape/AurumCode/internal/i18n"
	"github.com/Mpaape/AurumCode/internal/review/consolidate"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// ruleCitationTail is the citation the review appends to a finding's
// message, " (rule <id>: <title>)" or " (rule <id>)", and what follows it.
var ruleCitationTail = regexp.MustCompile(`\s*\(rule [^\s()]+(?::[^()]*)?\)`)

// internalCode is a bracketed code the engine writes for support, not for
// the author: a standards citation ("[standards/security-review SCR-003]"),
// a profile ("[perfil qa]") or a reason code ("[provider_failure]").
var internalCode = regexp.MustCompile(`\s*\[(?:standards/[^\]]*|perfil [^\]]*|[a-z]+(?:_[a-z]+)+)\]`)

// readableText is text with the rule citation and the internal codes
// removed, for a person to read.
func readableText(text string) string {
	text = ruleCitationTail.ReplaceAllString(text, "")
	text = internalCode.ReplaceAllString(text, "")
	return strings.TrimSpace(text)
}

// findingTitle is the short title of a finding in language: a leaked secret
// reads the same whichever engine found it; any other finding keeps its
// own message, without citation or codes.
func findingTitle(issue types.ReviewIssue, language string) string {
	if consolidate.IsSecret(issue.RuleID) {
		return i18n.Text(language, "review.secret_title")
	}
	return readableText(issue.Message)
}

// publishedMessage is the finding's title followed by who found it, when an
// engine did or several passes did.
func publishedMessage(issue types.ReviewIssue, alsoFrom []string, language string) string {
	title := findingTitle(issue, language)
	if issue.Origin == "" && len(alsoFrom) == 0 {
		return title
	}
	sources := sourceList(append([]string{issue.Origin}, alsoFrom...), language)
	return title + " — " + i18n.Format(language, "review.presentation_sources", sources)
}

// sourceName is how a person names the pass that found a finding: a
// catalog name when there is one, the origin itself otherwise.
func sourceName(origin, language string) string {
	if origin == "" {
		return i18n.Text(language, "review.presentation_model_source")
	}
	if name, ok := i18n.Lookup(language, "review.source."+origin); ok {
		return name
	}
	return origin
}

// joinNames joins names as a sentence does: "a", "a e b", "a, b e c".
func joinNames(names []string, language string) string {
	switch len(names) {
	case 0:
		return ""
	case 1:
		return names[0]
	}
	and := " " + i18n.Text(language, "review.and") + " "
	return strings.Join(names[:len(names)-1], ", ") + and + names[len(names)-1]
}
