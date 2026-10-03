package gate

import (
	"fmt"
	"strings"
	"time"

	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/render"
	"github.com/Mpaape/AurumCode/internal/review"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// OriginPolicy and OriginRepo are the two values AUR-519's dynamic
// rules (review.Rule.Origin) ever carry. Only one of them counts toward the
// gate on a given run -- see evaluateGate's acceptedOrigin parameter and
// AC-005.
const (
	OriginPolicy = "policy"
	OriginRepo   = "repo"
)

// SeverityRankOf maps an issue's own severity word (error/warning/info --
// the only three validateReviewResult ever admits) onto GateSeverityRank.
// Anything else (including an issue whose severity failed to normalize
// upstream) is not comparable to a gate threshold at all.
func SeverityRankOf(s string) (config.GateSeverityRank, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "error":
		return config.GateSeverityError, true
	case "warning":
		return config.GateSeverityWarning, true
	case "info":
		return config.GateSeverityInfo, true
	default:
		return 0, false
	}
}

// GateRankOf is the rank a deterministic finding's severity counts with at
// the gate. A severity the normalizer does not recognize counts as error:
// a finding whose severity could not be read is never dropped from the gate.
func GateRankOf(s string) config.GateSeverityRank {
	if rank, ok := SeverityRankOf(s); ok {
		return rank
	}
	return config.GateSeverityError
}

// EffectiveSeverityRank is the rank EvaluateGate compares against the
// gate's threshold for one matched finding: the HIGHER of the model's own
// issue.Severity and the cited dynamic rule's own declared severity
// (review.Rule.Severity, from the skill section's optional "severity:"
// line -- see ParseSkillSections). The model's text is untrusted input the
// diff it reviewed could have tried to steer (a prompt-injection line
// asking the model to under-report); the rule's own severity is the
// policy/repo author's own, trusted declaration and must act as a floor
// the model cannot talk its way under. Either side missing/unrecognized
// falls back to the other; both missing is not comparable at all.
func EffectiveSeverityRank(issueSeverity, ruleSeverity string) (config.GateSeverityRank, bool) {
	issueRank, issueOK := SeverityRankOf(issueSeverity)
	ruleRank, ruleOK := SeverityRankOf(ruleSeverity)
	switch {
	case issueOK && ruleOK:
		if ruleRank > issueRank {
			return ruleRank, true
		}
		return issueRank, true
	case issueOK:
		return issueRank, true
	case ruleOK:
		return ruleRank, true
	default:
		return 0, false
	}
}

// EvaluateGate applies AUR-519's gate to one finished review.
//
//   - gate is the effective, already precedence-resolved GateConfig --
//     config.ApplyCentralPolicy's own return value already decided whether
//     the policy's or the repository's gate applies (AC-005); this function
//     never re-derives that.
//   - acceptedOrigin is OriginPolicy when a central policy is active (so
//     only its own skill sections ever count toward the gate) or
//     OriginRepo when the repository opted in with no policy in play.
//   - dynamic is AUR-519's full dynamic rule set, both origins, so a lookup
//     here can tell a policy-origin citation apart from a repo-origin one
//     for the SAME run (a repo's own skill finding never fails the gate,
//     AC-005, even though it is still a perfectly citable finding).
//   - inconclusiveReason is "" for a conclusive review, or a short, stable,
//     non-model-authored reason token (see cmd/aurumcode's own inconclusive
//     detection: provider failure, partial coverage, or
//     prompt.IsDegradedParse) otherwise.
//   - exceptions is AUR-520's approved, time-bounded exception list --
//     already precedence-resolved by config.ApplyCentralPolicy exactly
//     like gate itself, so this function never re-derives policy-over-repo
//     here either. repoIdentity is this run's own verified "owner/repo"
//     (localRepoIdentity for --base, the already-authenticated owner/
//     repoName for --pr) or "" when it could not be confirmed -- matched
//     against each issue's own RuleID/File by MatchException (aur520.go),
//     never against any other, model-authored field. now is the
//     injectable clock the expiry comparison uses, always in UTC
//     (truncateToUTCDate): production callers pass time.Now(), a test
//     passes a fixed instant (AC-002/MUT-001).
//
// Inconclusive handling and the severity threshold are NOT mutually
// exclusive: only the block mode (written, or the default for a declared
// gate) skips the threshold loop -- a blocked run is never trusted enough to
// be graded on its own findings. EvaluateGate only marks it Inconclusive;
// the failure is decided by ApplyInconclusiveMode, the one place every
// source shares. Under "warn" the review is marked Inconclusive and the
// threshold loop still runs: an inconclusive review that ALSO contains a
// real severity breach must still fail the gate, and one with no breach
// must never publish as approved either.
func EvaluateGate(gate config.GateConfig, acceptedOrigin string, dynamic map[string]review.Rule, issues []types.ReviewIssue, inconclusiveReason string, exceptions []config.ExceptionConfig, repoIdentity string, now time.Time) (Result, error) {
	var d Result
	if !gate.Declared() {
		return d, nil
	}
	d.Active = true

	if inconclusiveReason != "" {
		// A declared gate without gate.inconclusive blocks (the same
		// default Config.InconclusiveMode resolves).
		mode, err := gate.InconclusiveMode(config.InconclusiveBlock)
		if err != nil {
			return d, err
		}
		d.Inconclusive = true
		d.Lines = append(d.Lines, fmt.Sprintf("review inconclusive (%s)", inconclusiveReason))
		if mode == config.InconclusiveBlock {
			// Not graded at all; the failure itself is decided once, by
			// ApplyInconclusiveMode.
			return d, nil
		}
		// "warn": fall through to the threshold loop.
	}

	rank, name, ok, err := gate.Threshold()
	if err != nil {
		return d, err
	}
	if !ok {
		return d, nil
	}
	for _, issue := range issues {
		// AUR-520: an approved exception is checked before (and
		// independently of) the dynamic/accepted-origin filter below, so
		// `rule:` in an exception may name either a dynamic skill-section
		// id or any other advisory/built-in rule id the model cited --
		// the exception's own repo+rule+path match is exact regardless of
		// whether that id would ever have been gate-relevant on its own.
		if exc, status := MatchException(exceptions, repoIdentity, issue.RuleID, issue.File, now); status != ExceptionNone {
			switch status {
			case ExceptionActive:
				// AC-001: tirado do gate por completo -- nunca chega ao
				// loop de limiar/severidade abaixo, e aparece como
				// aceito com dono e validade.
				d.Lines = append(d.Lines, AcceptedExceptionLine(exc, issue))
				d.AppliedExceptions = append(d.AppliedExceptions, render.AuditException{
					RuleID:        issue.RuleID,
					Path:          issue.File,
					Justification: fmt.Sprintf("dono: %s, motivo: %s, validade: %s", exc.Owner, exc.Reason, exc.Expires),
				})
				continue
			case ExceptionExpired:
				// AC-002: a exceção venceu e não vale mais -- o achado
				// segue para a avaliação normal abaixo, exatamente como
				// se nenhuma exceção tivesse sido configurada para ele.
				d.Lines = append(d.Lines, ExpiredExceptionLine(exc, issue))
			}
		}
		// AUR-556: gate.sources can leave the policy's skill sections out.
		if !gate.SourceEnabled(config.GateSourceSkills) {
			continue
		}
		rule, found := dynamic[issue.RuleID]
		if !found || rule.Origin != acceptedOrigin {
			continue
		}
		// B4: the model's own issue.Severity is untrusted -- the diff it
		// reviewed could contain a prompt-injection line asking it to
		// under-report. The cited rule's own declared severity floors the
		// comparison, so a policy skill's "severity: error" cannot be
		// talked down to "info" by the model.
		effective, comparable := EffectiveSeverityRank(issue.Severity, rule.Severity)
		if !comparable || effective < rank {
			continue
		}
		d.Fail = true
		d.Breach = true
		d.Lines = append(d.Lines, FindingLine(rule.ID, rule.Title, issue.Severity, name, OriginSkills))
		d.BlockingFindings = append(d.BlockingFindings, render.AuditFinding{
			RuleID:   issue.RuleID,
			Path:     issue.File,
			Line:     issue.Line,
			Severity: issue.Severity,
			Origin:   OriginSkills,
		})
	}
	return d, nil
}
