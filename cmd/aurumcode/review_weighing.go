// The model weighs the deterministic evidence. The evidence phase runs the
// security pass, the embedded analysis and SAST before the model; this file
// turns their findings into the prompt's evidence section, joins the
// model's per-evidence assessments back onto those findings once it has
// answered, and versions the caches by the evidence offered.
package main

import (
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/prompt"
	"github.com/Mpaape/AurumCode/internal/render"
	"github.com/Mpaape/AurumCode/internal/review"
	"github.com/Mpaape/AurumCode/internal/review/cache"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// errEvidenceNotCacheable keeps a review that offered evidence out of the
// per-file model cache (prepareCache).
var errEvidenceNotCacheable = errors.New("the review offered deterministic evidence; per-file cache bypassed")

// evidenceIDPrefix starts every evidence id the prompt shows ("E1", "E2").
const evidenceIDPrefix = "E"

// buildRepositoryContext renders the configured context providers' block
// (the policy's own first) for the prompt's repository-context slot. The
// provider is not wrapped: the block travels inside the budgeted prompt.
func (s *reviewState) buildRepositoryContext(providers []config.ContextProvider) (int, bool) {
	block, warnings, err := config.BuildContextBlockWithWarnings(s.ctx, providers, diffPaths(s.diff), s.filter)
	if err != nil {
		fmt.Fprintf(s.stderr, "aurumcode review: %v\n", err)
		return 1, true
	}
	for _, warning := range warnings {
		fmt.Fprintf(s.stderr, "aurumcode review: context provider %q unavailable: %s; continuing without that context\n", warning.Provider, warning.Reason)
	}
	s.repositoryContext = block
	return 0, false
}

// withOrigin returns issues with Origin set to the engine's own label.
func withOrigin(issues []types.ReviewIssue, origin string) []types.ReviewIssue {
	for i := range issues {
		issues[i].Origin = origin
	}
	return issues
}

// staticAnalysisIssues is the embedded analysis catalog's findings over
// diff, labeled with their origin.
func staticAnalysisIssues(diff *types.Diff) []types.ReviewIssue {
	var result types.ReviewResult
	mergeStaticAnalysis(diff, &result)
	return withOrigin(result.Issues, gateOriginAnalysis)
}

// evidenceKey identifies one deterministic finding across the passes.
func evidenceKey(issue types.ReviewIssue) string {
	return issue.Origin + "|" + findingOriginKey(issue.RuleID, issue.File, issue.Line)
}

// offerEvidence turns every deterministic finding of this run into one
// evidence item of the prompt, in a fixed order (origin, file, line, rule),
// with a short id the model cites back. Every finding is offered: the
// prompt's ceiling may omit some from the text (and says how many), while
// the gate counts all of them regardless.
func (s *reviewState) offerEvidence() {
	var all []types.ReviewIssue
	all = append(all, s.securityFindings...)
	all = append(all, s.analysisIssues...)
	all = append(all, s.sastIssues...)
	sort.SliceStable(all, func(i, j int) bool {
		a, b := all[i], all[j]
		if a.Origin != b.Origin {
			return a.Origin < b.Origin
		}
		if a.File != b.File {
			return a.File < b.File
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		return a.RuleID < b.RuleID
	})
	s.evidence, s.evidenceIDs = nil, map[string]string{}
	for _, issue := range all {
		key := evidenceKey(issue)
		if _, dup := s.evidenceIDs[key]; dup {
			continue
		}
		id := fmt.Sprintf("%s%d", evidenceIDPrefix, len(s.evidence)+1)
		s.evidenceIDs[key] = id
		s.evidence = append(s.evidence, prompt.EvidenceItem{
			ID: id, Origin: issue.Origin, RuleID: issue.RuleID,
			File: issue.File, Line: issue.Line, Side: issue.Side, Severity: issue.Severity,
			Snippet: render.FindingIdentityFor(s.diff, issue, s.filter).Context,
		})
	}
}

// reviewContext completes a source's review context with what every source
// sends the same way: the evidence offered and the repository context block.
func (s *reviewState) reviewContext(base review.ReviewContext) review.ReviewContext {
	base.Evidence = s.evidence
	base.RepositoryContext = s.repositoryContext
	return base
}

// contextCacheKey versions the model cache and the verdict reuse by
// everything the request carries besides the diff, the evidence offered
// included: an answer given before (or without) this evidence is never
// reused for a request that offers it.
func (s *reviewState) contextCacheKey() string {
	legacy := reviewContextCacheKey(s.provider, s.baseModelIdentity, s.reviewLanguage, s.codebaseText, s.memoryNotesText, s.profileIdentity, s.contextBlockDigest, s.ruleCatalogDigest)
	if len(s.evidence) == 0 {
		return legacy
	}
	evidenceDigest, err := cache.DigestOf(s.evidence)
	if err != nil {
		// json.Marshal of plain strings and ints cannot fail; a key that
		// matches nothing is the safe answer if it ever did.
		evidenceDigest = "unavailable:" + err.Error()
	}
	return cache.RequestKey(cache.RequestKeyInput{PromptDigest: legacy, EvidenceDigest: evidenceDigest})
}

// attachAssessments copies the model's assessment of each offered evidence
// item onto the deterministic finding it names. The model never writes the
// finding itself, its origin or its severity.
func (s *reviewState) attachAssessments() {
	if s.result == nil || len(s.evidenceIDs) == 0 {
		return
	}
	byID := make(map[string]types.EvidenceAssessment, len(s.result.EvidenceAssessments))
	for _, a := range s.result.EvidenceAssessments {
		byID[a.EvidenceID] = a
	}
	for _, list := range [][]types.ReviewIssue{s.securityFindings, s.analysisIssues, s.sastIssues} {
		for i := range list {
			if a, ok := byID[s.evidenceIDs[evidenceKey(list[i])]]; ok {
				assessment := a
				list[i].Assessment = &assessment
			}
		}
	}
}

// disputedEvidence is every deterministic finding the model disputed.
func (s *reviewState) disputedEvidence() []types.ReviewIssue {
	var out []types.ReviewIssue
	for _, list := range [][]types.ReviewIssue{s.securityFindings, s.analysisIssues, s.sastIssues} {
		for _, issue := range list {
			if issue.Assessment != nil && issue.Assessment.Status == types.AssessmentDisputed {
				out = append(out, issue)
			}
		}
	}
	return out
}

// printAssessment writes, under a finding of the terminal report, what the
// engine measured (origin) beside what the model concluded (assessment).
func printAssessment(w io.Writer, issue types.ReviewIssue) {
	a := issue.Assessment
	if a == nil {
		return
	}
	fmt.Fprintf(w, "  origem: %s | avaliacao do modelo: %s [%s]", issue.Origin, a.Status, a.EvidenceID)
	if a.Priority != "" {
		fmt.Fprintf(w, " prioridade %s", a.Priority)
	}
	fmt.Fprintf(w, " - %s\n", oneLine(a.Justification))
	if len(a.Correlates) > 0 {
		fmt.Fprintf(w, "  correlacao: %s\n", strings.Join(a.Correlates, ", "))
	}
	if a.Suggestion != "" {
		fmt.Fprintf(w, "  sugestao: %s\n", oneLine(a.Suggestion))
	}
}

// oneLine folds text onto one line for the terminal report.
func oneLine(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

// mergeAssessments adds next's assessments to merged, keeping the first
// profile's assessment of an evidence id (profiles in declaration order).
func mergeAssessments(merged, next []types.EvidenceAssessment) []types.EvidenceAssessment {
	seen := make(map[string]bool, len(merged))
	for _, a := range merged {
		seen[a.EvidenceID] = true
	}
	for _, a := range next {
		if !seen[a.EvidenceID] {
			seen[a.EvidenceID] = true
			merged = append(merged, a)
		}
	}
	return merged
}
