// AUR-519: the policy's gate (internal/config.GateConfig) over AUR-519's
// dynamic, skill-section rules (internal/review.ParseSkillSections). This
// file is the pure decision core -- evaluateGate takes the already
// precedence-resolved GateConfig (config.ApplyCentralPolicy already picked
// policy-over-repo) and the finished review's issues, and returns whether
// the check fails and the lines naming why, never touching stdout/stderr,
// the GitHub client, or the process exit code itself. Wiring this into
// runReview's (--base) and runPRReview's (--pr) own result/exit-code
// plumbing is tracked separately; see docs/specs/AUR-519.md's own status
// note for exactly what is and is not wired yet.
package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/review"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// policyGateContext is AC-006's stable commit-status context name for this
// gate: distinct from AUR-439's own checkContext ("aurumcode/review"), so
// an org's branch-protection ruleset can require the skill-driven
// compliance gate independently of --check's own grave-finding gate.
const policyGateContext = "aurumcode/policy-gate"

// gateOriginPolicy and gateOriginRepo are the two values AUR-519's dynamic
// rules (review.Rule.Origin) ever carry. Only one of them counts toward the
// gate on a given run -- see evaluateGate's acceptedOrigin parameter and
// AC-005.
const (
	gateOriginPolicy = "policy"
	gateOriginRepo   = "repo"
)

// gateDecision is one run's AUR-519 gate outcome. Active is false when no
// gate was declared at all (GateConfig.Declared() == false): every other
// field is then meaningless and the caller changes nothing about today's
// behavior. Fail means the check must report failure (AC-001's severity
// breach, or an inconclusive run under gate.inconclusive: block). Lines
// names why, for the published summary/limitations and for stderr -- never
// empty when Fail or Inconclusive is true.
type gateDecision struct {
	Active       bool
	Fail         bool
	Inconclusive bool
	Lines        []string
}

// mergedRuleCatalogIDs returns builtin plus every id of dynamic, sorted and
// deduplicated. It is the exact list a Reviewer must be taught
// (Reviewer.SetRuleCatalog) so a model can cite a skill section's rule id
// at all; builtin is expected to be prompt.DefaultRuleCatalog, already
// sorted, so a caller that passes anything else still gets a correctly
// sorted result.
func mergedRuleCatalogIDs(builtin []string, dynamic map[string]review.Rule) []string {
	seen := make(map[string]bool, len(builtin)+len(dynamic))
	ids := make([]string, 0, len(builtin)+len(dynamic))
	for _, id := range builtin {
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	for id := range dynamic {
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

// severityRankOf maps an issue's own severity word (error/warning/info --
// the only three validateReviewResult ever admits) onto GateSeverityRank.
// Anything else (including an issue whose severity failed to normalize
// upstream) is not comparable to a gate threshold at all.
func severityRankOf(s string) (config.GateSeverityRank, bool) {
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

// evaluateGate applies AUR-519's gate to one finished review.
//
//   - gate is the effective, already precedence-resolved GateConfig --
//     config.ApplyCentralPolicy's own return value already decided whether
//     the policy's or the repository's gate applies (AC-005); this function
//     never re-derives that.
//   - acceptedOrigin is gateOriginPolicy when a central policy is active (so
//     only its own skill sections ever count toward the gate) or
//     gateOriginRepo when the repository opted in with no policy in play.
//   - dynamic is AUR-519's full dynamic rule set, both origins, so a lookup
//     here can tell a policy-origin citation apart from a repo-origin one
//     for the SAME run (a repo's own skill finding never fails the gate,
//     AC-005, even though it is still a perfectly citable finding).
//   - inconclusiveReason is "" for a conclusive review, or a short, stable,
//     non-model-authored reason token (see cmd/aurumcode's own inconclusive
//     detection: provider failure, partial coverage, or
//     prompt.IsDegradedParse) otherwise. Checked BEFORE the severity
//     threshold below, mirroring AUR-458's existing "did not review outranks
//     reviewed and found things": an inconclusive run is never also graded
//     on findings it should not be trusted to have produced in full.
func evaluateGate(gate config.GateConfig, acceptedOrigin string, dynamic map[string]review.Rule, issues []types.ReviewIssue, inconclusiveReason string) (gateDecision, error) {
	var d gateDecision
	if !gate.Declared() {
		return d, nil
	}
	d.Active = true

	if inconclusiveReason != "" {
		mode, err := gate.InconclusiveMode()
		if err != nil {
			return d, err
		}
		if mode != "" {
			d.Inconclusive = true
			d.Lines = []string{fmt.Sprintf("review inconclusive (%s)", inconclusiveReason)}
			d.Fail = mode == "block"
			return d, nil
		}
	}

	rank, name, ok, err := gate.Threshold()
	if err != nil {
		return d, err
	}
	if !ok {
		return d, nil
	}
	for _, issue := range issues {
		rule, found := dynamic[issue.RuleID]
		if !found || rule.Origin != acceptedOrigin {
			continue
		}
		issueRank, rankOK := severityRankOf(issue.Severity)
		if !rankOK || issueRank < rank {
			continue
		}
		d.Fail = true
		d.Lines = append(d.Lines, fmt.Sprintf("%s: %s (severidade %s, limiar %s)", rule.ID, rule.Title, issue.Severity, name))
	}
	return d, nil
}
