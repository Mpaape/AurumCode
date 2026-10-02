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

	"github.com/Mpaape/AurumCode/internal/git/githubclient"
	"github.com/Mpaape/AurumCode/internal/review"
)

// policyGateContext is AC-006's stable commit-status context name for this
// gate: distinct from AUR-439's own checkContext ("aurumcode/review"), so
// an org's branch-protection ruleset can require the skill-driven
// compliance gate independently of --check's own grave-finding gate.
const policyGateContext = "aurumcode/policy-gate"

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
	// AUR-538 AC-007: reasons here feeds only the CAPPED commit-status
	// description (capStatusDescription below), never gateResult.Lines
	// itself or the review body/stderr limitations list built from it
	// elsewhere (main.go/pr.go) -- those stay the full, untruncated
	// account. orderedGateReasons puts a real finding ahead of the
	// inconclusive-reason line within that cap, so a long rule title
	// still has the best chance of surviving the 140-character cut.
	reasons := orderedGateReasons(gateResult.Lines)
	var word, detail string
	switch {
	case gateResult.Breach && gateResult.Inconclusive:
		status.State = "failure"
		word = gateStatusWordFailure
		detail = fmt.Sprintf("achado(s) reprovam o gate numa revisão também inconclusiva no pull request #%d: %s", prNumber, reasons)
	case gateResult.Breach:
		status.State = "failure"
		word = gateStatusWordFailure
		detail = fmt.Sprintf("achado(s) reprovam o gate no pull request #%d: %s", prNumber, reasons)
	case gateResult.Fail:
		// Fail without Breach: gate.inconclusive: block fired and the
		// threshold loop never ran -- this run was never graded at all.
		status.State = "failure"
		word = gateStatusWordInconclusive
		detail = fmt.Sprintf("revisão inconclusiva (bloqueio) no pull request #%d: %s", prNumber, reasons)
	case gateResult.Inconclusive:
		// B5: fail_on declared (or not) with no breach found, but the
		// review itself was inconclusive -- never "aprovado".
		status.State = "success"
		word = gateStatusWordInconclusive
		detail = fmt.Sprintf("revisão inconclusiva (alerta) no pull request #%d: %s", prNumber, reasons)
	default:
		status.State = "success"
		word = gateStatusWordApproved
		detail = fmt.Sprintf("gate de política aprovado no pull request #%d", prNumber)
	}
	status.Description = capStatusDescription(word, detail, statusDescriptionLimit)

	if err := client.SetStatus(ctx, owner, repoName, commitID, status); err != nil {
		fmt.Fprintf(stderr, "aurumcode review: publishing policy gate status: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "check %q publicado no commit %s: %s (%s)\n", policyGateContext, commitID, status.State, status.Description)

	// B1: a real severity breach (Breach) always closes the gate with
	// exitFindings, the same code --fail-on uses, regardless of whether
	// the review was also inconclusive. Fail without Breach can only come
	// from gate.inconclusive: block, which returns exitQualityNotReviewed
	// -- the same "half a review" signal AUR-458 already uses.
	if gateResult.Breach {
		return exitFindings
	}
	if gateResult.Fail {
		return exitQualityNotReviewed
	}
	return 0
}
