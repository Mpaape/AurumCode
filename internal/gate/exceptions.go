// AUR-520: an approved exception (config.ExceptionConfig) removes one
// EXACT finding -- repo, rule and path all matching, and not expired --
// from AUR-519's policy gate. The comparison lives here, next to
// EvaluateGate (policygate.go) it modifies, rather than in
// internal/config: config owns the shape and the fail-closed validation
// of what a human wrote (exceptions.go); matching a finding against that
// list is gate-evaluation logic, exactly like the severity-threshold
// comparison it sits beside, and both read the same untrusted
// types.ReviewIssue fields the same careful way (see EffectiveSeverityRank's
// own B4 note in policygate.go).
//
// THE SECURITY BOUNDARY THIS FILE DOES NOT CROSS: MatchException reads
// exactly two fields off an issue -- RuleID and File -- compared against
// the human-authored exception list, never anything else. It never reads
// issue.Message, issue.Evidence, issue.Impact, issue.Suggestion or
// issue.Verification: those are the model's own free text, and a diff it
// reviewed could try to plant a line asking the model to echo back an
// "owner"/"reason"/"expires" that happens to match some exception's
// fields, or simply to assert outright that the finding is excepted.
// Nothing in this file (or in EvaluateGate) ever parses those fields for
// that purpose, so no amount of model-authored text can make a finding
// look excepted on its own -- only a pre-configured, human-authored
// ExceptionConfig entry, matched by the two trusted-shape fields above
// plus a repo identity this program verified independently (see
// localRepoIdentity for --base, or the --pr path's own owner/repoName
// from the already-authenticated GitHub API call), can ever do that.
package gate

import (
	"fmt"
	"strings"
	"time"

	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// ExceptionMatchStatus is MatchException's own outcome for one issue:
// ExceptionNone (no configured exception names this repo/rule/path at
// all), ExceptionActive (one does, and today is on or before its Expires
// date) or ExceptionExpired (one does, but Expires has passed).
type ExceptionMatchStatus int

const (
	ExceptionNone ExceptionMatchStatus = iota
	ExceptionActive
	ExceptionExpired
)

// MatchException returns the first configured exception whose Repo, Rule
// and Path all equal (repoIdentity, ruleID, path) exactly, and whether it
// is still active. Repo compares case-insensitively (owner/repo on GitHub
// is itself case-insensitive, like codebaseContextMismatch's own
// comparison in aur515.go); Rule and Path compare byte-for-byte -- a rule
// id and a repository-relative path are exact identifiers an author
// copy-pastes from the finding, never a pattern (see
// docs/configuration.md's own "Exceções" section for why path stays exact
// rather than a glob).
//
// now is the injectable clock every caller passes as time.Now() in
// production and a fixed instant in a test (AC-002/MUT-001): the expiry
// comparison always normalizes both sides to a UTC calendar date
// (TruncateToUTCDate) before comparing, so neither a caller's local
// timezone nor a stray time-of-day component can move a date across the
// expires boundary.
//
// An empty repoIdentity (the caller could not verify which repository is
// under review at all -- see localRepoIdentity) never matches anything:
// config.ExceptionConfig.Validate already refused to let Repo be blank,
// so "" on this side can only ever compare unequal to every configured
// exception. That is this function's entire fail-closed behavior for an
// unverifiable repo identity -- no separate branch is needed.
//
// AUR-538 AC-006: a config can list more than one exception for the same
// (repo, rule, path) -- a renewal keeps the expired entry for audit
// history and adds a fresh one with a later Expires, in either order. An
// active match always wins over an expired one regardless of list order:
// the loop returns the first ACTIVE match the instant it finds one, and
// only falls back to the first EXPIRED match (also list-order-first) when
// no active match exists at all. This never widens what can match -- the
// same exact (repo, rule, path) triple is still required -- it only
// decides which of several matching entries this call reports.
func MatchException(exceptions []config.ExceptionConfig, repoIdentity, ruleID, path string, now time.Time) (config.ExceptionConfig, ExceptionMatchStatus) {
	repoIdentity = strings.TrimSpace(repoIdentity)
	var firstExpired config.ExceptionConfig
	haveExpired := false
	for _, exc := range exceptions {
		if !strings.EqualFold(strings.TrimSpace(exc.Repo), repoIdentity) {
			continue
		}
		if exc.Rule != ruleID {
			continue
		}
		if exc.Path != path {
			continue
		}
		expires, err := exc.ExpiresOn()
		if err != nil {
			// config.ValidateExceptions already refused a malformed
			// Expires at load time; this is unreachable in practice, but
			// an exception this function cannot date-check must never be
			// treated as active -- it can only ever be this call's
			// fallback expired match, never override a later active one.
			if !haveExpired {
				firstExpired, haveExpired = exc, true
			}
			continue
		}
		if TruncateToUTCDate(now).After(expires) {
			if !haveExpired {
				firstExpired, haveExpired = exc, true
			}
			continue
		}
		return exc, ExceptionActive
	}
	if haveExpired {
		return firstExpired, ExceptionExpired
	}
	return config.ExceptionConfig{}, ExceptionNone
}

// TruncateToUTCDate drops t's time-of-day and converts to UTC first, so
// "today" always means the same calendar date regardless of the caller's
// local timezone -- the exact bypass MUT-001 tries (reading the local
// wall clock, or comparing in a timezone ahead of UTC, to make an expired
// date look still current).
func TruncateToUTCDate(t time.Time) time.Time {
	u := t.UTC()
	return time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC)
}

// AcceptedExceptionMarker and ExpiredExceptionMarker are the fixed,
// literal substrings of AcceptedExceptionLine/ExpiredExceptionLine that
// never vary with RuleID/File/Owner/Reason/Expires -- the exact text
// between the two dynamic "%s em %s" fields and the dynamic
// owner/reason/expires fields that follow. AUR-538's orderedGateReasons
// (aur538.go) matches on these two constants, never on the single word
// "exceção" alone, so a real severity-breach line (EvaluateGate's own
// "%s: %s (severidade %s, limiar %s)" format, aur519's threshold loop)
// can never be misclassified as an exception line merely because a
// policy/repo author's own rule.Title happens to mention "exceção" --
// only this package's own two exception-line constructors ever produce
// either marker.
const (
	AcceptedExceptionMarker = ": aceito por exceção ("
	ExpiredExceptionMarker  = ": exceção venceu em "
)

// AcceptedExceptionLine is AC-001's own published line for a finding an
// active exception covers: the rule and path identify which exact finding
// (both already redacted/trusted -- RuleID is compared against a known
// catalog elsewhere, File is a repository-relative path), and
// owner/reason/expires are the human accountability the exception itself
// declared.
func AcceptedExceptionLine(exc config.ExceptionConfig, issue types.ReviewIssue) string {
	return fmt.Sprintf(
		"%s em %s"+AcceptedExceptionMarker+"dono: %s, motivo: %s, validade: %s)",
		issue.RuleID, issue.File, exc.Owner, exc.Reason, exc.Expires,
	)
}

// ExpiredExceptionLine is AC-002's own published line: the exception
// named this exact finding, but its Expires date has passed, so it no
// longer applies and the finding is evaluated exactly as if no exception
// had ever been configured for it.
func ExpiredExceptionLine(exc config.ExceptionConfig, issue types.ReviewIssue) string {
	return fmt.Sprintf(
		"%s em %s"+ExpiredExceptionMarker+"%s e não vale mais (dono: %s, motivo: %s)",
		issue.RuleID, issue.File, exc.Expires, exc.Owner, exc.Reason,
	)
}

// ExceptionsConfigured reports whether either side (policy or repo, pre-
// precedence) declared any exception at all -- used only to decide
// whether the --base path's "repo identity unknown" notice is worth
// publishing; a run with zero exceptions configured anywhere must never
// grow a new limitation line just because this card exists (AC-006-style
// byte-identical behavior).
func ExceptionsConfigured(cfg *config.Config) bool {
	return cfg != nil && len(cfg.Exceptions) > 0
}

// RepoIdentityUnavailableNotice is the declared limitation published on
// --base when at least one exception was configured but the local
// checkout's repository identity could not be confirmed (localRepoIdentity
// returned ok=false): every such exception fails closed (never matches),
// and this says why, the same way codebaseContextOmittedNotice already
// explains --pr's own identity-verification failures.
func RepoIdentityUnavailableNotice(language string) string {
	if language == "pt-BR" || language == "pt" {
		return "Exceções desativadas: não foi possível confirmar o repositório revisado a partir do remoto \"origin\"."
	}
	return "Exceptions disabled: the reviewed repository's identity could not be confirmed from the \"origin\" remote."
}
