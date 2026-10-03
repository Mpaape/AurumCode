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
	"io"
	"strings"

	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/deliberation"
	"github.com/Mpaape/AurumCode/internal/llm"
	"github.com/Mpaape/AurumCode/internal/prompt"
	"github.com/Mpaape/AurumCode/internal/review"
	"github.com/Mpaape/AurumCode/internal/review/cache"
	reviewtools "github.com/Mpaape/AurumCode/internal/review/tools"
	"github.com/Mpaape/AurumCode/internal/scanner"
)

// errDeliberationNotCacheable keeps a review that offered tools out of the
// per-file cache: its answer depends on tool results the cache never sees.
var errDeliberationNotCacheable = errors.New("the review offered tools to the model; per-file cache bypassed")

// prepareDeliberation installs the deliberation on reviewer and returns the
// manifest for the prompt, or nil when nothing is offered. supportsTools is
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
	return offers
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
	out := scanner.Outcome{Engine: g.Engine, Reason: g.Reason}
	for _, issue := range g.Issues {
		out.Findings = append(out.Findings, scanner.Finding{Path: issue.File, Line: issue.Line, RuleID: issue.RuleID, Severity: issue.Severity, Message: issue.Message})
	}
	return out
}

// settleDeferredScans runs, as before, every deferred scanner when no tool
// was offered: a scanner is only ever skipped by the model's own decision.
func (s *reviewState) settleDeferredScans() {
	if s.toolsOffered {
		s.reportDeliberation()
		return
	}
	for _, entry := range s.deferredScans {
		s.scans = append(s.scans, s.scanEntry(entry))
	}
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

// reportDeliberationFailure handles a deliberation stopped by a limit: the
// review is inconclusive, nothing the model said is published, exit 1.
func reportDeliberationFailure(stderr io.Writer, err error) (int, bool) {
	var limit *deliberation.LimitError
	if !errors.As(err, &limit) {
		return 0, false
	}
	fmt.Fprintf(stderr, "aurumcode review: inconclusivo (%s): %v; nenhum parecer foi publicado\n", limit.Reason(), limit)
	return 1, true
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
