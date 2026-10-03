// The embedded analysis catalog (analysis/*) counts toward the
// policy gate. Like ApplyScannerGate (scanner.go) it folds its own decision
// into the Result EvaluateGate already returned, so EvaluateGate's
// signature and every AUR-519/520/521/548 consumer stay untouched. The
// findings come from a fresh, deterministic analysis.Runner pass over the
// diff -- never from result.Issues, where a model reply could forge or
// omit an "analysis/*" id -- and are subject to the same evidence gate
// (they are produced from the diff's own added lines), the same rule
// config (config.ApplyRuleConfig) and the same AUR-520 exceptions as any
// other finding. With no gate declared, nothing here runs.
package gate

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/Mpaape/AurumCode/internal/analysis"
	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// OriginAnalysis labels findings of the embedded analysis catalog.
const (
	OriginAnalysis = "analysis"
	OriginSkills   = "skills"
	// OriginSecurity labels findings of the --seguranca pass (AUR-569).
	OriginSecurity = "security"
	// OriginDTrack and OriginAnalysisData label the Dependency-Track and
	// analysis-data contributors (AUR-567).
	OriginDTrack       = "dtrack"
	OriginAnalysisData = "analysis_data"
)

// FindingLine is the one spelling of a gate line for a finding, shared by
// every source (AUR-567): the message, then severity, threshold and the typed
// origin, the same value the audit and the SARIF carry in origin. The rule id
// and the message are joined by " - ", never by a colon: a colon after an id
// that ends in "secret" reads to the redaction filter as a `secret: value`
// pair, and it would replace the first word of the message.
//
// The citation the review appends to a message, "(rule <id>: <title>)", has the
// same shape, so the line spells it "(rule <id> - <title>)". The report keeps
// the legacy citation; the two differ only in that separator.
func FindingLine(ruleID, message, severity, threshold, origin string) string {
	message = ruleCitation.ReplaceAllString(message, "(rule $1 - ")
	return fmt.Sprintf("%s - %s (severidade %s, limiar %s, origem %s)", ruleID, message, severity, threshold, origin)
}

// ruleCitation matches the opening of the review's "(rule <id>: <title>)" citation.
var ruleCitation = regexp.MustCompile(`\(rule ([^\s:()]+): `)

// FindingOriginKey identifies a finding for origin lookup.
func FindingOriginKey(ruleID, path string, line int) string {
	return fmt.Sprintf("%s|%s|%d", ruleID, path, line)
}

// AnalysisIssuesForGate runs the embedded catalog over diff and applies the
// effective rule config, exactly as the review's own merge pass does.
func AnalysisIssuesForGate(diff *types.Diff, cfg *config.Config) []types.ReviewIssue {
	if diff == nil {
		return nil
	}
	var out []types.ReviewIssue
	for _, f := range analysis.NewRunner().Analyze(diff) {
		out = append(out, f.ToIssue(""))
	}
	return config.ApplyRuleConfig(out, cfg)
}

// ApplyAnalysisGate folds the analysis origin into d in place. A complete
// no-op unless the gate is declared, has a severity threshold and does not
// exclude the analysis source via gate.sources.
func ApplyAnalysisGate(d *Result, gate config.GateConfig, issues []types.ReviewIssue, exceptions []config.ExceptionConfig, repoIdentity string, now time.Time) error {
	return applyDeterministic(d, gate, issues, exceptions, repoIdentity, now, OriginAnalysis, func(id string) bool {
		return strings.HasPrefix(id, "analysis/")
	})
}

// ApplySecurityGate folds the --seguranca pass's findings (AUR-569) into d.
// They are deterministic evidence from the embedded security catalog, so a
// finding at or above fail_on counts in every gate.inconclusive mode: the mode
// governs the model's missing opinion, not a finding that is already there.
// They count under the analysis source of gate.sources (the embedded catalog).
// Findings the gate already holds (same rule, path and line) are not counted
// twice.
func ApplySecurityGate(d *Result, gate config.GateConfig, issues []types.ReviewIssue, exceptions []config.ExceptionConfig, repoIdentity string, now time.Time) error {
	known := make(map[string]bool, len(d.BlockingFindings))
	for _, f := range d.BlockingFindings {
		known[FindingOriginKey(f.RuleID, f.Path, f.Line)] = true
	}
	fresh := make([]types.ReviewIssue, 0, len(issues))
	for _, i := range issues {
		if !known[FindingOriginKey(i.RuleID, i.File, i.Line)] {
			fresh = append(fresh, i)
		}
	}
	return applyDeterministic(d, gate, fresh, exceptions, repoIdentity, now, OriginSecurity, func(string) bool { return true })
}

// applyDeterministic is the one loop that counts deterministic findings
// (those a model reply cannot forge or omit) toward the gate, labeled origin.
func applyDeterministic(d *Result, gate config.GateConfig, issues []types.ReviewIssue, exceptions []config.ExceptionConfig, repoIdentity string, now time.Time, origin string, counts func(ruleID string) bool) error {
	if !gate.Declared() || !gate.SourceEnabled(config.GateSourceAnalysis) {
		return nil
	}
	rank, name, ok, err := gate.Threshold()
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	d.Active = true
	for _, issue := range issues {
		if !counts(issue.RuleID) {
			continue
		}
		if exc, status := MatchException(exceptions, repoIdentity, issue.RuleID, issue.File, now); status != ExceptionNone {
			switch status {
			case ExceptionActive:
				d.Lines = append(d.Lines, AcceptedExceptionLine(exc, issue))
				d.AppliedExceptions = append(d.AppliedExceptions, types.AuditException{
					RuleID:        issue.RuleID,
					Path:          issue.File,
					Justification: fmt.Sprintf("dono: %s, motivo: %s, validade: %s", exc.Owner, exc.Reason, exc.Expires),
				})
				continue
			case ExceptionExpired:
				d.Lines = append(d.Lines, ExpiredExceptionLine(exc, issue))
			}
		}
		if GateRankOf(issue.Severity) < rank {
			continue
		}
		d.Fail = true
		d.Breach = true
		d.Lines = append(d.Lines, FindingLine(issue.RuleID, issue.Message, issue.Severity, name, origin))
		d.BlockingFindings = append(d.BlockingFindings, types.AuditFinding{
			RuleID: issue.RuleID, Path: issue.File, Line: issue.Line, Severity: issue.Severity,
			Origin: origin,
		})
	}
	return nil
}
