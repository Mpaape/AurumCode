package changelog

import (
	"strings"
	"unicode"
)

// botAccountType is the account type GitHub reports for an app or bot
// account in the pull request event (user.type).
const botAccountType = "Bot"

// botLoginSuffix ends every GitHub App account login (dependabot[bot],
// renovate[bot]); a person's login cannot contain brackets.
const botLoginSuffix = "[bot]"

// maxAuthorLabel bounds the login echoed in the check's log line.
const maxAuthorLabel = 64

// Author is the pull request author as the event reports it (AUR-610). Both
// fields come from the event, never from the pull request's content; an
// absent author is a human, so a missing signal never relaxes the check.
type Author struct {
	Login string
	Type  string
}

// IsBot reports whether the event marks the author as a bot account: the
// account type is Bot, or the login carries the reserved [bot] suffix. A
// login that merely contains the word bot is a person.
func (a Author) IsBot() bool {
	return strings.TrimSpace(a.Type) == botAccountType || strings.HasSuffix(strings.TrimSpace(a.Login), botLoginSuffix)
}

// Label is the login for a log line: printable, without spaces or control
// characters, and bounded, so the event's text cannot forge another line.
func (a Author) Label() string {
	var b strings.Builder
	n := 0
	for _, r := range strings.TrimSpace(a.Login) {
		if n == maxAuthorLabel {
			break
		}
		if unicode.IsPrint(r) && !unicode.IsSpace(r) {
			b.WriteRune(r)
			n++
		}
	}
	if b.Len() == 0 {
		return "login ausente"
	}
	return b.String()
}
