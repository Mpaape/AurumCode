// Dependency reachability (AUR-531): with deliberation.dependency_reachability,
// each advisory of the dependency check gets the model's explanation of
// whether the reviewed code uses the vulnerable part, found with the
// repository tools and grounded in the reviewed revision. The explanation is
// a line of the review beside the finding: the report the gate judges is
// never touched, so no explanation can lower, remove or re-rate a finding.
package main

import (
	"fmt"

	"github.com/Mpaape/AurumCode/internal/deliberation"
	"github.com/Mpaape/AurumCode/internal/dependencies"
	"github.com/Mpaape/AurumCode/internal/grammar"
	"github.com/Mpaape/AurumCode/internal/llm"
	"github.com/Mpaape/AurumCode/internal/review/reach"
	reviewtools "github.com/Mpaape/AurumCode/internal/review/tools"
)

// maxReachExplanations bounds the deliberations one review spends on
// reachability; the rest are declared as not explained.
const maxReachExplanations = 5

// explainDependencyReach adds one explanation line per advisory of the
// dependency report, or says why none could be given.
func (s *reviewState) explainDependencyReach() {
	if s.depReport == nil || s.cfg == nil || !s.cfg.Deliberation.Active() || !s.cfg.Deliberation.DependencyReachability || s.result == nil {
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
			x = reach.Explanation{Request: req, Reason: fmt.Sprintf("limite de %d explicações por revisão", maxReachExplanations)}
		default:
			x = explainer.Explain(s.ctx, req)
		}
		line := s.redactText(reach.Line(x))
		fmt.Fprintf(s.stderr, "aurumcode review: %s\n", line)
		s.result.Limitations = append(s.result.Limitations, line)
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
		return reach.Explainer{}, "sem provedor de modelo"
	}
	orchestrator := llm.NewOrchestrator(s.provider, nil, s.tracker)
	if !orchestrator.SupportsTools() {
		return reach.Explainer{}, "o provedor não chama ferramentas"
	}
	if s.scanRoot == "" || s.scanBlocked != "" {
		return reach.Explainer{}, "checkout não verificado como a revisão revisada"
	}
	rev, err := s.reviewedRevision()
	if err != nil {
		return reach.Explainer{}, "revisão revisada ilegível"
	}
	offers := reviewtools.RepositoryOffers(rev, s.diff, grammar.Default(), s.redactText)
	maxRounds, maxCost, perTool := s.cfg.Deliberation.EffectiveLimits()
	return reach.Explainer{
		Caller: orchestrator,
		Tools:  reviewtools.Tools(offers),
		Limits: deliberation.Limits{MaxRounds: maxRounds, MaxCostTokens: maxCost, PerToolTimeout: perTool},
		Redact: s.redactText,
		Read:   rev.Read,
	}, ""
}
