// The policy gate over skill-section rules: evaluateGate decides from the
// precedence-resolved gate configuration, and the command publishes the
// decision as a commit status and in the review text.
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
	"github.com/Mpaape/AurumCode/internal/context/skills"
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
// warning for the context block itself (config.BuildContextBlockWithWarnings,
// called by the evidence step in review_weighing.go, which puts the block in
// the prompt's evidence) -- this function only adds the citable-rule layer
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

// dynamicRulesFromCatalog turns every skill the catalog selected for changed
// into dynamic rules, by layer, each tagged with its layer's gate origin
// (layerRuleOrigin): a repository skill directory is citable exactly like a
// listed skill file (same ids, see review.ParseSkillSections), and never
// with a central layer's origin (CR-TRUST-001). policyActive is whether a
// central policy governs this run.
func dynamicRulesFromCatalog(catalog *skills.Catalog, changed []string, policyActive bool) layeredRules {
	r := layeredRules{}
	if catalog == nil {
		return r
	}
	for _, doc := range catalog.Docs(changed) {
		layer := layerOfDoc(doc)
		for _, rule := range review.ParseSkillSections(doc.Path, doc.Instructions, layerRuleOrigin(layer, policyActive)) {
			r.add(layer, rule)
		}
	}
	return r
}

// layerRuleOrigin is the gate origin of a layer's skill-section rules: the
// policy's own and the repository's own, and for the engineering standards
// layer -- central like the policy, so its findings count in the gate as
// source skills -- the origin the run's gate accepts (acceptedGateOrigin).
func layerRuleOrigin(layer config.LayerName, policyActive bool) string {
	switch layer {
	case config.LayerPolicy:
		return gateOriginPolicy
	case config.LayerRepository:
		return gateOriginRepo
	}
	return acceptedGateOrigin(policyActive)
}

// unionRules adds every rule of extra to base that base does not already
// hold, and returns base.
func unionRules(base, extra map[string]review.Rule) map[string]review.Rule {
	for id, rule := range extra {
		if _, ok := base[id]; !ok {
			base[id] = rule
		}
	}
	return base
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
	return publishPolicyGateStatusIn(ctx, "", client, stdout, stderr, owner, repoName, commitID, gateResult, prNumber)
}

// publishPolicyGateStatusIn is publishPolicyGateStatus with the description
// in the review's language (gateStatusDescription).
func publishPolicyGateStatusIn(ctx context.Context, language string, client *githubclient.Client, stdout, stderr io.Writer, owner, repoName, commitID string, gateResult gateDecision, prNumber int) int {
	if !gateResult.Active {
		return 0
	}
	status := githubclient.CommitStatus{Context: policyGateContext}
	status.State, status.Description = gateStatusDescription(language, gateResult, prNumber)

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
