// Dependency reachability: with deliberation.dependency_reachability,
// each advisory of the dependency check gets the model's explanation of
// whether the reviewed code uses the vulnerable part, found with the
// repository tools and grounded in the reviewed revision. The explanation is
// a line of the review beside the finding: the report the gate judges is
// never touched, so no explanation can lower, remove or re-rate a finding.
package main

import (
	"fmt"
	"strings"

	"github.com/Mpaape/AurumCode/internal/deliberation"
	"github.com/Mpaape/AurumCode/internal/dependencies"
	"github.com/Mpaape/AurumCode/internal/grammar"
	"github.com/Mpaape/AurumCode/internal/i18n"
	"github.com/Mpaape/AurumCode/internal/llm"
	"github.com/Mpaape/AurumCode/internal/review/reach"
	reviewtools "github.com/Mpaape/AurumCode/internal/review/tools"
)

// maxReachExplanations bounds the deliberations one review spends on
// reachability; the rest are declared as not explained.
const maxReachExplanations = 5

// explainDependencyReach records one explanation line per advisory of the
// dependency report, or says why none could be given; the lines are the
// review's own reachability section (appendReachSection).
func (s *reviewState) explainDependencyReach() {
	s.reachLines = nil
	if s.depReport == nil || s.cfg == nil || !s.cfg.Deliberation.Active() || !s.cfg.Deliberation.DependencyReachability {
		return
	}
	requests := reachRequests(*s.depReport)
	if len(requests) == 0 {
		return
	}
	explainer, reason := s.reachExplainer()
	for i, req := range requests {
		var x reach.Explanation
		switch {
		case reason != "":
			x = reach.Explanation{Request: req, Reason: reason}
		case i >= maxReachExplanations:
			x = reach.Explanation{Request: req, Reason: i18n.Format(s.reviewLanguage, "reach.reason_limit", maxReachExplanations)}
		default:
			x = explainer.Explain(s.ctx, req)
		}
		line := s.redactText(reach.Line(x, s.reviewLanguage))
		fmt.Fprintf(s.stderr, "aurumcode review: %s\n", line)
		s.reachLines = append(s.reachLines, line)
	}
}

// reachRequests is one request per advisory and package the change keeps
// (introduced or pre-existing), in report order, without duplicates.
func reachRequests(r dependencies.Report) []reach.Request {
	seen := map[string]bool{}
	var out []reach.Request
	for _, f := range r.Findings {
		if f.Status == dependencies.StatusFixed {
			continue
		}
		key := f.Change.Key() + "\x00" + f.Vuln.ID
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, reach.Request{Package: f.Change.Name, Ecosystem: f.Change.Ecosystem, Manifest: f.Change.Manifest, AdvisoryID: f.Vuln.ID, Summary: f.Vuln.Summary})
	}
	return out
}

// reachExplainer is the explainer over the reviewed revision, or the reason
// it cannot exist.
func (s *reviewState) reachExplainer() (reach.Explainer, string) {
	if s.provider == nil {
		return reach.Explainer{}, i18n.Text(s.reviewLanguage, "reach.reason_no_provider")
	}
	orchestrator := llm.NewOrchestrator(s.provider, nil, s.tracker)
	if !orchestrator.SupportsTools() {
		return reach.Explainer{}, i18n.Text(s.reviewLanguage, "reach.reason_no_tools")
	}
	if s.scanRoot == "" || s.scanBlocked != "" {
		return reach.Explainer{}, i18n.Text(s.reviewLanguage, "reach.reason_unverified")
	}
	rev, err := s.reviewedRevision()
	if err != nil {
		return reach.Explainer{}, i18n.Text(s.reviewLanguage, "reach.reason_unreadable")
	}
	offers := reviewtools.RepositoryOffers(rev, s.diff, grammar.Default(), s.redactText)
	maxRounds, maxCost, perTool := s.cfg.Deliberation.EffectiveLimits()
	return reach.Explainer{
		Caller:   orchestrator,
		Tools:    reviewtools.Tools(offers),
		Limits:   deliberation.Limits{MaxRounds: maxRounds, MaxCostTokens: maxCost, PerToolTimeout: perTool},
		Redact:   s.redactText,
		Read:     rev.Read,
		Language: s.reviewLanguage,
	}, ""
}

// appendReachSection appends the reachability section to a review body:
// its own heading, one item per explanation, after everything the filters
// of the review touch.
func appendReachSection(body string, lines []string, language string) string {
	if len(lines) == 0 {
		return body
	}
	var b strings.Builder
	b.WriteString(strings.TrimRight(body, "\n"))
	fmt.Fprintf(&b, "\n\n### %s\n\n", i18n.Text(language, "reach.section"))
	for _, line := range lines {
		fmt.Fprintf(&b, "- %s\n", line)
	}
	return b.String()
}
