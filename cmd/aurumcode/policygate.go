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
	"time"

	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/git/githubclient"
	"github.com/Mpaape/AurumCode/internal/render"
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
	// Breach is true only when an actual severity-threshold breach was
	// found in the issues (the threshold loop below), as opposed to Fail
	// being set purely because gate.inconclusive: block fired with no
	// breach ever checked. publishPolicyGateStatus and the exit-code
	// sections of runReview/runPRReview use this to tell "a real finding
	// closed the gate" (exitFindings) apart from "the review itself was
	// never trustworthy enough to grade" (exitQualityNotReviewed) even
	// when both end up with Fail == true at once (inconclusive: warn/""
	// plus a breach).
	Breach bool
	Lines  []string
	// AUR-521: BlockingFindings and AppliedExceptions are the SAME
	// decisions above (Breach's threshold match, and AUR-520's exception
	// match), captured as structured data instead of printable lines, so
	// the compliance audit record/SARIF document never re-derive the
	// gate's own matching logic a second time -- there is exactly one
	// place a finding is decided to block or be excepted, and this struct
	// is its only output. BlockingFindings is appended to ONLY at the
	// exact point Breach is set below, so it is never populated by a run
	// that never reached (or never passed) the threshold loop at all
	// (e.g. gate.inconclusive: block, Fail without Breach).
	BlockingFindings  []render.AuditFinding
	AppliedExceptions []render.AuditException
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

// effectiveSeverityRank is the rank evaluateGate compares against the
// gate's threshold for one matched finding: the HIGHER of the model's own
// issue.Severity and the cited dynamic rule's own declared severity
// (review.Rule.Severity, from the skill section's optional "severity:"
// line -- see ParseSkillSections). The model's text is untrusted input the
// diff it reviewed could have tried to steer (a prompt-injection line
// asking the model to under-report); the rule's own severity is the
// policy/repo author's own, trusted declaration and must act as a floor
// the model cannot talk its way under. Either side missing/unrecognized
// falls back to the other; both missing is not comparable at all.
func effectiveSeverityRank(issueSeverity, ruleSeverity string) (config.GateSeverityRank, bool) {
	issueRank, issueOK := severityRankOf(issueSeverity)
	ruleRank, ruleOK := severityRankOf(ruleSeverity)
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
//     prompt.IsDegradedParse) otherwise.
//   - exceptions is AUR-520's approved, time-bounded exception list --
//     already precedence-resolved by config.ApplyCentralPolicy exactly
//     like gate itself, so this function never re-derives policy-over-repo
//     here either. repoIdentity is this run's own verified "owner/repo"
//     (localRepoIdentity for --base, the already-authenticated owner/
//     repoName for --pr) or "" when it could not be confirmed -- matched
//     against each issue's own RuleID/File by matchException (aur520.go),
//     never against any other, model-authored field. now is the
//     injectable clock the expiry comparison uses, always in UTC
//     (truncateToUTCDate): production callers pass time.Now(), a test
//     passes a fixed instant (AC-002/MUT-001).
//
// Inconclusive handling and the severity threshold are NOT mutually
// exclusive (fixed from an earlier, incorrect draft that returned early on
// any inconclusive reason): only gate.inconclusive: block skips the
// threshold loop outright -- a blocked run is never trusted enough to be
// graded on its own findings at all, mirroring AUR-458's "did not review
// outranks reviewed and found things". Both "warn" and "" (no
// gate.inconclusive key declared at all -- AC-005's own non-default
// silence) mark the review Inconclusive but still run the threshold loop:
// an inconclusive review that ALSO contains a real severity breach must
// still fail the gate (exitFindings, never silently waved through because
// the review happened to also be degraded or partially covered), and an
// inconclusive review with gate.fail_on declared but no breach must never
// publish as approved either -- it stays Inconclusive with no Fail, so the
// caller's own verdict/status logic can say so without claiming success.
func evaluateGate(gate config.GateConfig, acceptedOrigin string, dynamic map[string]review.Rule, issues []types.ReviewIssue, inconclusiveReason string, exceptions []config.ExceptionConfig, repoIdentity string, now time.Time) (gateDecision, error) {
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
		d.Inconclusive = true
		d.Lines = append(d.Lines, fmt.Sprintf("review inconclusive (%s)", inconclusiveReason))
		if mode == "block" {
			d.Fail = true
			return d, nil
		}
		// "warn", or "" (undeclared): fall through to the threshold loop.
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
		if exc, status := matchException(exceptions, repoIdentity, issue.RuleID, issue.File, now); status != exceptionNone {
			switch status {
			case exceptionActive:
				// AC-001: tirado do gate por completo -- nunca chega ao
				// loop de limiar/severidade abaixo, e aparece como
				// aceito com dono e validade.
				d.Lines = append(d.Lines, acceptedExceptionLine(exc, issue))
				d.AppliedExceptions = append(d.AppliedExceptions, render.AuditException{
					RuleID:        issue.RuleID,
					Path:          issue.File,
					Justification: fmt.Sprintf("dono: %s, motivo: %s, validade: %s", exc.Owner, exc.Reason, exc.Expires),
				})
				continue
			case exceptionExpired:
				// AC-002: a exceção venceu e não vale mais -- o achado
				// segue para a avaliação normal abaixo, exatamente como
				// se nenhuma exceção tivesse sido configurada para ele.
				d.Lines = append(d.Lines, expiredExceptionLine(exc, issue))
			}
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
		effective, comparable := effectiveSeverityRank(issue.Severity, rule.Severity)
		if !comparable || effective < rank {
			continue
		}
		d.Fail = true
		d.Breach = true
		d.Lines = append(d.Lines, fmt.Sprintf("%s: %s (severidade %s, limiar %s)", rule.ID, rule.Title, issue.Severity, name))
		d.BlockingFindings = append(d.BlockingFindings, render.AuditFinding{
			RuleID:   issue.RuleID,
			Path:     issue.File,
			Line:     issue.Line,
			Severity: issue.Severity,
		})
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
