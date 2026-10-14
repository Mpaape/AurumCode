// The model always decides: after the review, every deterministic finding
// of a source under gate.triage: model that the review left unassessed is
// sent to the model in a dedicated triage call (review.EvidenceTriager).
// When that call does not answer, the findings wait for the model
// (review_triage_awaiting.go); they never count as a static error.
package main

import (
	"errors"
	"fmt"

	"github.com/Mpaape/AurumCode/internal/analyzer"
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
	triager := &review.EvidenceTriager{Caller: s.verifyCaller, Language: s.reviewLanguage, Redact: s.redactText}
	view := s.evidenceContext()
	items, keys := s.triageEvidence(sources, view, false)
	if !s.triageRound(triager, items, keys) {
		return
	}
	if len(items) > 0 && s.countAssessed(keys) < len(items) {
		// Asked twice and still silent on some: they wait for the model.
		s.triageFailure = triageReasonUnanswered
	}
	// Undecided (needs_context, or a dispute without a reason): once more,
	// with more context; what stays undecided waits for the model.
	items, keys = s.triageEvidence(sources, view, true)
	if !s.triageRound(triager, items, keys) {
		return
	}
	if again, _ := s.triageEvidence(sources, view, true); len(again) > 0 && s.triageFailure == "" {
		s.triageFailure = triageReasonNeedsContext
	}
}

// triageRound asks the model about items and attaches its assessments,
// replacing an undecided one; false when the model did not answer (the
// findings then wait for it).
func (s *reviewState) triageRound(triager *review.EvidenceTriager, items []prompt.EvidenceItem, keys map[string]string) bool {
	if len(items) == 0 {
		return true
	}
	assessments, err := triager.Assess(s.ctx, items)
	if err != nil {
		s.triageFailure = triageFailureReason(err)
		fmt.Fprintf(s.stderr, "aurumcode review: gate.triage: %s\n", s.redactText(err.Error()))
		return false
	}
	byKey := make(map[string]types.EvidenceAssessment, len(assessments))
	for _, a := range assessments {
		byKey[keys[a.EvidenceID]] = a
	}
	for _, list := range s.evidenceLists() {
		for i := range list {
			if a, ok := byKey[evidenceKey(list[i])]; ok && undecided(list[i].Assessment) {
				assessment := a
				list[i].Assessment = &assessment
			}
		}
	}
	s.result.EvidenceAssessments = replaceAssessments(s.result.EvidenceAssessments, assessments)
	return true
}

// countAssessed counts the findings keys names that now carry an
// assessment.
func (s *reviewState) countAssessed(keys map[string]string) int {
	wanted := make(map[string]bool, len(keys))
	for _, key := range keys {
		wanted[key] = true
	}
	n := 0
	for _, list := range s.evidenceLists() {
		for _, issue := range list {
			if key := evidenceKey(issue); wanted[key] && issue.Assessment != nil {
				delete(wanted, key)
				n++
			}
		}
	}
	return n
}

// undecided reports an assessment that decides nothing: none, a request
// for context, or a dispute without a justification. Only a confirmation
// blocks and only a justified dispute demotes.
func undecided(a *types.EvidenceAssessment) bool {
	return a == nil || (a.Status != types.AssessmentConfirmed && !review.DisputeCounts(a))
}

// replaceAssessments merges next into list, next's assessment of an id
// replacing the previous one.
func replaceAssessments(list, next []types.EvidenceAssessment) []types.EvidenceAssessment {
	at := make(map[string]int, len(list))
	for i, a := range list {
		at[a.EvidenceID] = i
	}
	for _, a := range next {
		if i, ok := at[a.EvidenceID]; ok {
			list[i] = a
			continue
		}
		at[a.EvidenceID] = len(list)
		list = append(list, a)
	}
	return list
}

// triageReasonNeedsContext is the reason of findings the model still could
// not decide when shown again with more context.
const triageReasonNeedsContext = "triage_needs_context"

// triageReasonUnanswered is the reason of a triage that left findings
// without an assessment even when asked again about them alone.
const triageReasonUnanswered = "triage_unanswered"

// triageEvidence is every finding of a source the model may triage that
// it has not decided -- without an assessment on the first pass, with an
// undecided one on the wider pass -- as one evidence item per finding (the
// id the review offered it under, or a new one), with the key each id
// names. Each item shows what view finds for it (the commit of a finding
// of the history; with wide, more context).
func (s *reviewState) triageEvidence(sources map[string]bool, view review.EvidenceContext, wide bool) ([]prompt.EvidenceItem, map[string]string) {
	var items []prompt.EvidenceItem
	keys := map[string]string{}
	seen := map[string]bool{}
	add := func(source string, issues []types.ReviewIssue) {
		if !sources[source] {
			return
		}
		for _, issue := range issues {
			key := evidenceKey(issue)
			pending := issue.Assessment == nil
			if wide {
				pending = issue.Assessment != nil && undecided(issue.Assessment)
			}
			if !pending || seen[key] {
				continue
			}
			seen[key] = true
			id := s.evidenceIDs[key]
			if id == "" {
				id = fmt.Sprintf("%s%d", triageIDPrefix, len(items)+1)
			}
			keys[id] = key
			items = append(items, view.Describe(prompt.EvidenceItem{
				ID: id, Origin: issue.Origin, RuleID: issue.RuleID,
				File: issue.File, Line: issue.Line, Side: issue.Side, Severity: issue.Severity,
				Snippet: render.FindingIdentityFor(s.diff, issue, s.filter).Context,
			}, issue.Commit, wide))
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

// evidenceContext reads the reviewed repository for the triage: its object
// database (the commits of the history and HEAD), never a path the
// repository declares secret. Without a verified checkout it reads nothing.
func (s *reviewState) evidenceContext() review.EvidenceContext {
	view := review.EvidenceContext{Redact: s.redactText}
	if s.cfg != nil {
		view.Hidden = s.cfg.IsSecretPath
	}
	if s.scanRoot == "" || s.scanBlocked != "" {
		return view
	}
	if repo, err := analyzer.OpenRepo(s.scanRoot); err == nil {
		view.Source = repo
	}
	return view
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
