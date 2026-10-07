// The agent server's gateway: every review and gate answer is one --base
// review session (newBaseReview + session.Run), the same the review command
// runs, read back from the session's own state. The server adds no gate.
package main

import (
	"bytes"
	"context"
	"fmt"
	"os"

	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/mcpserver"
	"github.com/Mpaape/AurumCode/internal/review/session"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// reasonNotReviewed is the inconclusive motive of a session whose model
// review did not happen and whose gate declared none.
const reasonNotReviewed = "not_reviewed"

// sessionGateway implements mcpserver.Gateway with the review session.
type sessionGateway struct {
	filter *redaction.Filter
	limite string
}

// mcpReviewArgs is the review command line an agent's request becomes. The
// security pass always runs and a review the model did not do is never a
// clean one, so an agent can only get a stricter answer than a bare
// `review --base`, never a looser one. The client chooses only the ref.
func (g *sessionGateway) mcpReviewArgs(base string) []string {
	args := []string{"--base", base, "--seguranca", "--exigir-qualidade"}
	if g.limite != "" {
		args = append(args, "--limite", g.limite)
	}
	return args
}

// Review runs one --base session and reads its decision.
func (g *sessionGateway) Review(ctx context.Context, req mcpserver.ReviewRequest) (mcpserver.SessionOutcome, error) {
	var stdout, stderr bytes.Buffer
	f, exit, ok := parseReviewFlags(g.mcpReviewArgs(req.Base), &stdout, &stderr)
	if !ok {
		return mcpserver.SessionOutcome{Exit: exit, Diagnostics: stderr.String()}, nil
	}
	policyDir, exit, ok := f.resolvePolicyDir(&stderr)
	if !ok {
		return mcpserver.SessionOutcome{Exit: exit, Diagnostics: stderr.String()}, nil
	}
	b := newBaseReview(reviewIO{stdout: &stdout, stderr: &stderr, filter: g.filter}, f, policyDir)
	b.ctx = ctx
	exit = session.Run(b)
	b.flush()
	return sessionOutcome(b, exit, stdout.String(), stderr.String()), nil
}

// sessionOutcome reads what the finished session decided.
func sessionOutcome(b *baseReview, exit int, report, diagnostics string) mcpserver.SessionOutcome {
	o := mcpserver.SessionOutcome{Exit: exit, ChangedFiles: b.rawDiffFileCount, Report: report, Diagnostics: diagnostics, Redactor: b.filter}
	var issues []types.ReviewIssue
	if b.result != nil {
		issues = append(issues, b.result.Issues...)
	}
	issues = append(issues, b.securityApart()...)
	for _, issue := range issues {
		o.Findings = append(o.Findings, findingOf(issue))
	}
	if b.gateRes != nil {
		o.InconclusiveReason = b.gateRes.Reason
		o.GateLines = b.gateRes.Lines
		for _, bf := range b.gateRes.BlockingFindings {
			o.Blocking = append(o.Blocking, blockingFinding(issues, bf.RuleID, bf.Path, bf.Line, bf.Severity, bf.Origin))
		}
	}
	if o.InconclusiveReason == "" && b.notReviewed() {
		o.InconclusiveReason = reasonNotReviewed
	}
	return o
}

// findingOf is the structured form of one review issue.
func findingOf(issue types.ReviewIssue) mcpserver.Finding {
	return mcpserver.Finding{
		File: issue.File, Line: issue.Line, Severity: issue.Severity, RuleID: issue.RuleID,
		Origin: issue.Origin, Message: issue.Message, Impact: issue.Impact,
		Evidence: issue.Evidence, Suggestion: issue.Suggestion,
	}
}

// blockingFinding is a finding the gate matched, with the text of the
// review issue it came from.
func blockingFinding(issues []types.ReviewIssue, rule, path string, line int, severity, origin string) mcpserver.Finding {
	for _, issue := range issues {
		if issue.RuleID == rule && issue.File == path && issue.Line == line {
			f := findingOf(issue)
			f.Severity, f.Origin = severity, origin
			return f
		}
	}
	return mcpserver.Finding{File: path, Line: line, Severity: severity, RuleID: rule, Origin: origin}
}

// Rules lists the skills and skill rules that apply to paths, from the same
// resolution the review session uses (resolveSkillRules).
func (g *sessionGateway) Rules(_ context.Context, paths []string) (mcpserver.RuleSet, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return mcpserver.RuleSet{}, err
	}
	var stderr bytes.Buffer
	f, _, ok := parseReviewFlags(nil, &stderr, &stderr)
	if !ok {
		return mcpserver.RuleSet{}, fmt.Errorf("%s", stderr.String())
	}
	policyDir, _, ok := f.resolvePolicyDir(&stderr)
	if !ok {
		return mcpserver.RuleSet{}, fmt.Errorf("%s", stderr.String())
	}
	layers, err := loadSkillLayers(cwd, policyDir)
	if err != nil {
		return mcpserver.RuleSet{}, err
	}
	catalog, rules, err := resolveSkillRules(layers, paths)
	if err != nil {
		return mcpserver.RuleSet{}, err
	}
	return ruleSetOf(catalog, rules, paths, policyDir), nil
}

// loadSkillLayers loads the repository configuration and, when a central
// policy is set, the policy over it, exactly as the review's resolve phase.
func loadSkillLayers(cwd, policyDir string) (skillLayers, error) {
	cfg, err := config.Load(cwd)
	if err != nil {
		return skillLayers{}, err
	}
	l := skillLayers{cwd: cwd, policyDir: policyDir, cfg: cfg}
	if policyDir == "" {
		return l, nil
	}
	if err := config.ValidatePolicyOutsideReviewedTree(policyDir, cwd); err != nil {
		return skillLayers{}, err
	}
	if l.centralCfg, err = config.LoadCentralPolicy(policyDir); err != nil {
		return skillLayers{}, err
	}
	l.cfg, _ = config.ApplyCentralPolicy(cfg, l.centralCfg)
	return l, nil
}
