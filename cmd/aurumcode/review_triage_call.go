// The model always decides: after the review, every deterministic finding
// of a source under gate.triage: model that the review left unassessed is
// sent to the model in a dedicated triage call (review.EvidenceTriager).
// When that call does not answer, the findings wait for the model
// (review_triage_awaiting.go); they never count as a static error.
package main

import (
	"errors"
	"fmt"

	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/llm"
	"github.com/Mpaape/AurumCode/internal/prompt"
	"github.com/Mpaape/AurumCode/internal/render"
	"github.com/Mpaape/AurumCode/internal/review"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// triageIDPrefix starts the id of a finding the review never offered (a
// scanner that settled after the prompt was assembled).
const triageIDPrefix = "T"

// triageUnassessed runs the dedicated triage over the unassessed evidence
// and attaches the model's assessments to the findings the gate reads. It
// runs only when the review's model answered cleanly: a model that did not
// already leaves the evidence awaiting it.
func (s *reviewState) triageUnassessed() {
	if s.result == nil || s.modelReason() != "" {
		return
	}
	if s.verifyCaller == nil {
		// No caller for the triage: the evidence waits for the model rather
		// than counting as a static error.
		s.triageFailure = triageReasonUnanswered
		return
	}
	sources := s.triageSources()
	items, keys := s.unassessedEvidence(sources)
	if len(items) == 0 {
		return
	}
	triager := &review.EvidenceTriager{Caller: s.verifyCaller, Language: s.reviewLanguage, Redact: s.redactText}
	assessments, err := triager.Assess(s.ctx, items)
	if err != nil {
		s.triageFailure = triageFailureReason(err)
		fmt.Fprintf(s.stderr, "aurumcode review: gate.triage: %s\n", s.redactText(err.Error()))
		return
	}
	byKey := make(map[string]types.EvidenceAssessment, len(assessments))
	for _, a := range assessments {
		byKey[keys[a.EvidenceID]] = a
	}
	for _, list := range s.evidenceLists() {
		for i := range list {
			if a, ok := byKey[evidenceKey(list[i])]; ok && list[i].Assessment == nil {
				assessment := a
				list[i].Assessment = &assessment
			}
		}
	}
	s.result.EvidenceAssessments = mergeAssessments(s.result.EvidenceAssessments, assessments)
	if len(assessments) < len(items) {
		// Asked twice and still silent on some: they wait for the model.
		s.triageFailure = triageReasonUnanswered
	}
}

// triageReasonUnanswered is the reason of a triage that left findings
// without an assessment even when asked again about them alone.
const triageReasonUnanswered = "triage_unanswered"

// unassessedEvidence is every finding of a source the model may triage
// that carries no assessment, as one evidence item per finding (the id the
// review offered it under, or a new one), with the key each id names.
func (s *reviewState) unassessedEvidence(sources map[string]bool) ([]prompt.EvidenceItem, map[string]string) {
	var items []prompt.EvidenceItem
	keys := map[string]string{}
	seen := map[string]bool{}
	add := func(source string, issues []types.ReviewIssue) {
		if !sources[source] {
			return
		}
		for _, issue := range issues {
			key := evidenceKey(issue)
			if issue.Assessment != nil || seen[key] {
				continue
			}
			seen[key] = true
			id := s.evidenceIDs[key]
			if id == "" {
				id = fmt.Sprintf("%s%d", triageIDPrefix, len(items)+1)
			}
			keys[id] = key
			items = append(items, prompt.EvidenceItem{
				ID: id, Origin: issue.Origin, RuleID: issue.RuleID,
				File: issue.File, Line: issue.Line, Side: issue.Side, Severity: issue.Severity,
				Snippet: render.FindingIdentityFor(s.diff, issue, s.filter).Context,
			})
		}
	}
	add(config.GateSourceAnalysis, s.securityFindings)
	add(config.GateSourceAnalysis, s.analysisIssues)
	for _, scan := range s.scans {
		if scan.Section != gateOriginPolicy {
			add(scan.Source(), scan.Issues)
		}
	}
	return items, keys
}

// triageFailureReason is the model's own reason for a triage without an
// answer: the provider that never answered, or a reply that was not the
// JSON asked for.
func triageFailureReason(err error) string {
	if errors.Is(err, llm.ErrAllProvidersFailed) {
		return gateReasonProviderFailure
	}
	return gateReasonModelParse
}
