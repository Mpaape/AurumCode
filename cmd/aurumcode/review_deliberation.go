// Deliberation in the model phase: when the configuration enables it and the
// provider can call tools, the scanners the configuration does not require
// and the codebase context are offered to the model, which decides what to
// ask for. A scanner the model asked for joins the session's scans like any
// other (its findings count in the gate with their origin, its failure is
// the scan's inconclusive reason); one it did not ask for does not run, and
// the transcript records both. When tools cannot be offered, the deferred
// scanners run exactly as they did before the model existed.
package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/deliberation"
	"github.com/Mpaape/AurumCode/internal/llm"
	"github.com/Mpaape/AurumCode/internal/prompt"
	"github.com/Mpaape/AurumCode/internal/review"
	"github.com/Mpaape/AurumCode/internal/review/cache"
	reviewtools "github.com/Mpaape/AurumCode/internal/review/tools"
	"github.com/Mpaape/AurumCode/internal/scanner"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// errDeliberationNotCacheable keeps a review that offered tools out of the
// per-file cache: its answer depends on tool results the cache never sees.
var errDeliberationNotCacheable = errors.New("the review offered tools to the model; per-file cache bypassed")

// prepareDeliberation installs the deliberation on reviewer and records the
// manifest the prompt's tools slot shows (s.toolManifest), or offers nothing
// when deliberation is off or tools cannot be offered. supportsTools is
// false for a provider without tool calling and for profile passes (each
// profile decorates the request, so its provider is not a plain caller).
func (s *reviewState) prepareDeliberation(caller deliberation.Caller, supportsTools bool, reviewer *review.Reviewer) {
	s.toolManifest, s.toolsOffered = nil, false
	if !s.cfg.Deliberation.Active() {
		return
	}
	if !supportsTools {
		fmt.Fprintf(s.stderr, "aurumcode review: deliberation: o provedor não chama ferramentas; os scanners opcionais rodam sem consulta ao modelo\n")
		return
	}
	offers := s.toolOffers()
	maxRounds, maxCost, perTool := s.cfg.Deliberation.EffectiveLimits()
	reviewer.SetDeliberation(&review.Deliberation{
		Caller: caller,
		Tools:  reviewtools.Tools(offers),
		Limits: deliberation.Limits{MaxRounds: maxRounds, MaxCostTokens: maxCost, PerToolTimeout: perTool},
	})
	s.toolManifest, s.toolsOffered = reviewtools.Manifest(offers), true
}

// toolOffers is every tool this session offers: one per deferred scanner,
// then the codebase context of the changed files.
func (s *reviewState) toolOffers() []reviewtools.Offer {
	_, _, perTool := s.cfg.Deliberation.EffectiveLimits()
	var offers []reviewtools.Offer
	for _, entry := range s.deferredScans {
		engine, ok := entry.Lookup()
		if !ok {
			continue
		}
		offers = append(offers, reviewtools.Offer{
			Tool: reviewtools.NewScannerTool(engine, s.scanOnRequest(entry)),
			Cost: reviewtools.ScannerCost(int(perTool.Seconds())),
		})
	}
	if s.scanRoot != "" {
		offers = append(offers, reviewtools.Offer{Tool: reviewtools.NewContextTool(s.scanRoot, diffPaths(s.diff)), Cost: reviewtools.ContextCost})
	}
	if sections := s.skillSections(); len(sections) > 0 {
		offers = append(offers, reviewtools.Offer{Tool: reviewtools.NewSkillTool(sections), Cost: reviewtools.SkillCost})
	}
	return offers
}

// skillSections is the configured skill sections the model may read on
// demand, keyed by the rule id the catalog lists.
func (s *reviewState) skillSections() map[string]reviewtools.SkillSection {
	out := make(map[string]reviewtools.SkillSection, len(s.dynamicRules))
	for id, rule := range s.dynamicRules {
		out[id] = reviewtools.SkillSection{Title: rule.Title, Text: rule.Description}
	}
	return out
}

// scanOnRequest runs entry when the model asks for it, through the same
// path as the evidence phase (same root, trust and refusal of an
// unverified checkout), and joins the scan to the session's scans.
func (s *reviewState) scanOnRequest(entry config.ScannerConfig) reviewtools.ScanFunc {
	return func(ctx context.Context) scanner.Outcome {
		// scanEntry runs under the session's context; the copy carries the
		// tool's own deadline instead.
		bounded := *s
		bounded.ctx = ctx
		scan := bounded.scanEntry(entry)
		s.scans = append(s.scans, scan)
		return scanOutcome(scan)
	}
}

// scanOutcome is the scan as the tool reports it to the model.
func scanOutcome(g gateScan) scanner.Outcome {
	out := scanner.Outcome{Engine: g.Engine, Reason: g.Reason, Detail: g.Detail}
	for _, issue := range g.Issues {
		out.Findings = append(out.Findings, scanner.Finding{Path: issue.File, Line: issue.Line, RuleID: issue.RuleID, Severity: issue.Severity, Message: issue.Message})
	}
	return out
}

// settleDeferredScans runs, as before, every deferred scanner the model did
// not decide about: a scanner is only ever skipped by the model's own
// decision. When tools were offered and the model answered, or its
// deliberation hit a limit (already inconclusive), the transcript stands.
// When tools were offered but the model never answered (provider failure,
// a USD ceiling), nothing was decided: the deferred scanners it had not
// already run now run, and the transcript says why instead of calling them
// "not requested".
func (s *reviewState) settleDeferredScans() {
	if s.toolsOffered && s.modelDecided() {
		s.reportDeliberation()
		return
	}
	ran := map[string]bool{}
	for _, scan := range s.scans {
		ran[scan.Config.Name()] = true
	}
	for _, entry := range s.deferredScans {
		if !ran[entry.Name()] {
			s.scans = append(s.scans, s.scanEntry(entry))
		}
	}
	if s.toolsOffered && s.transcript != nil {
		s.transcript.MarkUndecided(undecidedReason)
		s.reportDeliberation()
	}
}

// undecidedReason is the transcript's account of a deliberation the model
// never finished for a reason other than a limit.
const undecidedReason = "o modelo não respondeu (falha do provedor ou teto de custo); os scanners opcionais rodaram sem decisão do modelo"

// modelDecided reports a deliberation that ended in the model's answer or
// in a limit.
func (s *reviewState) modelDecided() bool {
	if s.model == modelDeliberationLimit {
		return true
	}
	return s.transcript != nil && s.transcript.Outcome == deliberation.OutcomeAnswered && s.model != modelProviderFailed
}

// reportDeliberation states on stderr what the model was offered, what it
// asked for and how the conversation ended.
func (s *reviewState) reportDeliberation() {
	t := s.transcript
	if t == nil {
		return
	}
	fmt.Fprintf(s.stderr, "aurumcode review: deliberation: oferecidas [%s]; pedidas [%s]; não pedidas [%s]; rodadas %d; desfecho %s\n",
		strings.Join(t.Offered, ", "), strings.Join(t.Requested, ", "), strings.Join(t.NotRequested, ", "), t.Rounds, t.Outcome)
	if t.Undecided != "" {
		fmt.Fprintf(s.stderr, "aurumcode review: deliberation: sem decisão do modelo: %s\n", t.Undecided)
	}
	for _, c := range t.Calls {
		fmt.Fprintf(s.stderr, "aurumcode review: deliberation: rodada %d %s(%s) %s: %s\n", c.Round, c.Tool, c.Arguments, c.Status, c.Result)
	}
}

// toolResultsDigest is the digest of the tool results the model consumed,
// "" when none was offered.
func (s *reviewState) toolResultsDigest() string {
	if !s.toolsOffered || s.transcript == nil {
		return ""
	}
	digest, err := cache.DigestOf(s.transcript.ResultDigests())
	if err != nil {
		return "unavailable:" + err.Error()
	}
	return digest
}

// noteDeliberationLimit handles a deliberation stopped by a limit: the
// model outcome becomes ModelDeliberationLimit (the gate's inconclusive
// motive deliberation_limit:<limit>, never reviewed in either source), the
// result is replaced by the one limitation that says so, and nothing the
// model wrote is kept. The session goes on to the gate, so the audit, the
// SARIF and the --pr statuses are still written. False for any other error.
func (s *reviewState) noteDeliberationLimit(err error) bool {
	var limit *deliberation.LimitError
	if !errors.As(err, &limit) {
		return false
	}
	notice := fmt.Sprintf("inconclusivo: limite de deliberação (%s); nenhum parecer do modelo foi publicado", limit.Reason())
	fmt.Fprintf(s.stderr, "aurumcode review: %s\n", notice)
	s.model = modelDeliberationLimit
	s.result = &types.ReviewResult{Metadata: map[string]string{"quality_degraded": "true"}, Limitations: []string{notice}}
	return true
}

// deliberationLimit is the exceeded limit, "" when none was.
func (s *reviewState) deliberationLimit() string {
	if s.transcript == nil {
		return ""
	}
	return s.transcript.Limit
}

// toolsCapable reports whether the orchestrator can deliberate.
func toolsCapable(o *llm.Orchestrator, profilesApplied bool) bool {
	return !profilesApplied && o.SupportsTools()
}

// manifestOrNil keeps the prompt's tools slot empty unless tools were
// offered.
func (s *reviewState) manifestOrNil() []prompt.ToolOffer {
	if !s.toolsOffered {
		return nil
	}
	return s.toolManifest
}

// offeredTranscript is the transcript for the audit, nil unless tools were
// offered.
func (s *reviewState) offeredTranscript() *deliberation.Transcript {
	if !s.toolsOffered {
		return nil
	}
	return s.transcript
}
