// The model decides the deterministic evidence (gate.triage: model). When
// it was to decide and did not answer, the evidence it never assessed is not
// a static error that fails the gate: it waits for the model, the run is
// inconclusive by the model's own reason, and nothing is approved.
package main

import (
	"strings"

	"github.com/Mpaape/AurumCode/internal/gate/reasons"
	"github.com/Mpaape/AurumCode/internal/i18n"
)

// awaitingReason is why the evidence of a triaged source waits for the
// model: the dedicated triage call that failed, or the review's own model
// failure (provider, refusal, unparsable or degraded reply, deliberation
// limit). Empty when the model answered, and when no model was configured
// at all (quality_skipped), which keeps the deterministic gate it always
// was.
func (s *reviewState) awaitingReason() string {
	if s.triageFailure != "" {
		return s.triageFailure
	}
	if s.model == modelSkipped {
		return ""
	}
	return s.modelReason()
}

// settleAwaiting closes a gate that left findings awaiting the model:
// inconclusive by the model's reason and failing whatever the
// gate.inconclusive mode (without the model there is no verdict), with one
// line counting them (only findings that would have blocked wait,
// gate.Run.Awaiting). They never become blocking findings.
func (s *reviewState) settleAwaiting(awaiting []gateDemotion, res *gateDecision) {
	n := len(awaiting)
	if n == 0 || res == nil {
		return
	}
	reason := s.awaitingReason()
	if reason == "" {
		reason = gateReasonProviderFailure
	}
	res.Active, res.Inconclusive, res.Fail = true, true, true
	if !strings.Contains(","+res.Reason+",", ","+reason+",") {
		res.AddReason(reason)
	}
	line := i18n.Format(s.reviewLanguage, "notice.triage_awaiting", n, reasons.Text(s.reviewLanguage, reason))
	res.Lines = append(res.Lines, line)
}
