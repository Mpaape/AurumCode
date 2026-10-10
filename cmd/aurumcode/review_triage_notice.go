// The fail-closed side of gate.triage: when the model was to weigh the
// deterministic evidence and gave no usable answer, nothing was demoted, and
// the run says so instead of leaving the reader to infer it.
package main

import (
	"fmt"

	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/i18n"
)

// reportTriageNotRun states, in stderr and in the review's limitations,
// that the triage did not happen: some evidence belonged to a source the
// model could have demoted (declared gate, no central policy, not opted
// out), yet the model did not answer cleanly, so triage demoted nothing.
// The evidence counted in full; the line says whether the gate kept its
// block.
func (s *reviewState) reportTriageNotRun(res *gateDecision) {
	reason := s.modelReason()
	if reason == "" || !s.triageableEvidence(s.triageSources()) {
		return
	}
	key := "notice.triage_not_run_clear"
	if res != nil && res.Fail {
		key = "notice.triage_not_run"
	}
	line := i18n.Format(s.reviewLanguage, key, reason)
	fmt.Fprintf(s.stderr, "aurumcode review: %s\n", line)
	s.result.Limitations = append(s.result.Limitations, line)
}

// triageableEvidence reports whether this run holds deterministic evidence
// of a source whose disputed findings sources lets the triage demote: the
// embedded analysis and the security pass (both under analysis) or a
// repository-declared scanner.
func (s *reviewState) triageableEvidence(sources map[string]bool) bool {
	if sources[config.GateSourceAnalysis] && len(s.analysisIssues)+len(s.securityFindings) > 0 {
		return true
	}
	for _, scan := range s.scans {
		if scan.Section != gateOriginPolicy && sources[scan.Source()] && len(scan.Issues) > 0 {
			return true
		}
	}
	return false
}
