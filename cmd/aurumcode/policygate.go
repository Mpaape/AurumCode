// AUR-519: the policy's gate (internal/config.GateConfig) over AUR-519's
// dynamic, skill-section rules (internal/review.ParseSkillSections).
// evaluateGate is the pure decision core: it takes the already
// precedence-resolved GateConfig (config.ApplyCentralPolicy already picked
// policy-over-repo) and the finished review's issues, and returns whether
// the check fails and the lines naming why, never touching stdout/stderr or
// the GitHub client itself. dynamicRulesFromLocalSkills/
// dynamicRulesFromRemoteSkills/mergeDynamicRules are the loading half:
// runReview (--base, main.go) and runPRReview (--pr, pr.go) call them to
// build the per-run dynamic rule set and the merged prompt catalog before
// GenerateReviewWithContext, then call evaluateGate after the result is
// final to decide the exit code and the published decision lines.
package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/git/githubclient"
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

// dynamicRulesFromLocalSkills reads every skill at root/<path> (paths come
// from Config.Review.Context.Skills, exactly as review.context.skills
// already names them) from the local filesystem and turns each into
// AUR-519's dynamic rules (review.ParseSkillSections), tagged origin. A
// skill that cannot be read (missing, unreadable) is skipped here rather
// than reported: config.FileContextProvider is the place that already
// turns a missing, EXPLICITLY configured skill into a loud provider
// warning for the context block itself (see WrapProviderWithWarnings in
// runReview/runPRReview) -- this function only adds the citable-rule layer
// on top of whatever skill content successfully reached the model, and
// must not duplicate or race that decision.
func dynamicRulesFromLocalSkills(root string, paths []string, origin string) map[string]review.Rule {
	out := map[string]review.Rule{}
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(p)))
		if err != nil {
			continue
		}
		for _, rule := range review.ParseSkillSections(p, string(data), origin) {
			out[rule.ID] = rule
		}
	}
	return out
}

// dynamicRulesFromRemoteSkills is dynamicRulesFromLocalSkills' --pr
// equivalent: each path is read through the restored GitHub client's
// contents API at ref, exactly as loadPullRequestContext already does for
// the context block itself -- the repository under review never supplies
// its own skill content by any other route.
func dynamicRulesFromRemoteSkills(ctx context.Context, client *githubclient.Client, owner, repo string, paths []string, ref, origin string) map[string]review.Rule {
	out := map[string]review.Rule{}
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		data, found, err := client.GetRepositoryFile(ctx, owner, repo, p, ref)
		if err != nil || !found {
			continue
		}
		for _, rule := range review.ParseSkillSections(p, string(data), origin) {
			out[rule.ID] = rule
		}
	}
	return out
}

// mergeDynamicRules combines policy and repo dynamic rule sets into one
// map, keyed by id. On an id collision (the policy and the repository both
// declare a skill section that slugs to the same id) the POLICY's rule
// always wins: a repository must never be able to shadow a policy's rule
// id with its own, lower-trust, repo-origin copy and quietly remove it
// from the gate's accepted set (CR-TRUST-001).
func mergeDynamicRules(policy, repo map[string]review.Rule) map[string]review.Rule {
	out := make(map[string]review.Rule, len(policy)+len(repo))
	for id, rule := range repo {
		out[id] = rule
	}
	for id, rule := range policy {
		out[id] = rule
	}
	return out
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

// publishPolicyGateStatus publishes AC-006's own commit status
// (policyGateContext) through the same restored client's SetStatus
// publishCheckStatus already uses, independently of --check's own
// grave-finding status: an org's branch-protection ruleset can require
// either, both, or neither. It is a complete no-op -- no API call at all
// -- when gateResult.Active is false (no `gate:` was ever declared), so a
// review that never configured a gate never grows a second required
// status. The returned int mirrors publishCheckStatus's own contribution
// to the exit code: 1 when SetStatus itself could not be published, 0 or
// exitFindings/exitQualityNotReviewed otherwise, driven by gateResult
// exactly as runReview's own exit-code section is.
func publishPolicyGateStatus(ctx context.Context, client *githubclient.Client, stdout, stderr io.Writer, owner, repoName, commitID string, gateResult gateDecision, prNumber int) int {
	if !gateResult.Active {
		return 0
	}
	status := githubclient.CommitStatus{Context: policyGateContext}
	reasons := strings.Join(gateResult.Lines, "; ")
	switch {
	case gateResult.Fail && gateResult.Inconclusive:
		status.State = "failure"
		status.Description = fmt.Sprintf("revisão inconclusiva (bloqueio) no pull request #%d: %s", prNumber, reasons)
	case gateResult.Fail:
		status.State = "failure"
		status.Description = fmt.Sprintf("achado(s) reprovam o gate no pull request #%d: %s", prNumber, reasons)
	case gateResult.Inconclusive:
		status.State = "success"
		status.Description = fmt.Sprintf("revisão inconclusiva (alerta) no pull request #%d: %s", prNumber, reasons)
	default:
		status.State = "success"
		status.Description = fmt.Sprintf("gate de política aprovado no pull request #%d", prNumber)
	}

	if err := client.SetStatus(ctx, owner, repoName, commitID, status); err != nil {
		fmt.Fprintf(stderr, "aurumcode review: publishing policy gate status: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "check %q publicado no commit %s: %s (%s)\n", policyGateContext, commitID, status.State, status.Description)

	if gateResult.Fail {
		if gateResult.Inconclusive {
			return exitQualityNotReviewed
		}
		return exitFindings
	}
	return 0
}
