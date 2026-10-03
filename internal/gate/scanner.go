package gate

import (
	"fmt"
	"strings"

	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/render"
	"github.com/Mpaape/AurumCode/internal/scanner"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// Scan is one scanner engine's outcome as the gate reads it: the effective
// entry, the registered engine, which section declared it (OriginPolicy or
// OriginRepo), its findings as issues, and its inconclusive reason ("" for
// a trustworthy scan, including a clean one).
type Scan struct {
	Config  config.ScannerConfig
	Engine  scanner.Engine
	Section string
	Issues  []types.ReviewIssue
	Reason  string
}

// Origin is the typed origin of the scan's findings.
func (s Scan) Origin() string { return s.Engine.TypedOrigin() }

// Source is the gate.sources/gate.triage key the scan's demotions are
// recorded under: the engine's category, or its name.
func (s Scan) Source() string {
	if c := scanner.Normalize(s.Engine.Category); c != "" {
		return c
	}
	return s.Engine.Name()
}

// label is how a gate line names the scanner: its category (or name) in
// upper case, as "SAST" always read.
func (s Scan) label() string { return strings.ToUpper(s.Source()) }

// countsUnder reports whether the scan's findings count toward a declared
// gate: always when gate.sources is absent, otherwise only when an entry
// names the engine, its category or its origin.
func (s Scan) countsUnder(g config.GateConfig) bool {
	if !g.Declared() || len(g.Sources) == 0 {
		return true
	}
	for _, src := range g.Sources {
		if s.Engine.Answers(src) {
			return true
		}
	}
	return false
}

// ApplyScannerGate folds one scanner's own decision into d in place,
// independently of the gate section: a configuration with a scanner and no
// `gate:` still withholds approval on a breach. issues is the engine's own
// findings only (never the model's), already filtered by gate.sources and
// the triage. A failed scan is inconclusive, never "zero findings"; the
// one rule (ApplyInconclusiveMode) decides what inconclusive does.
func ApplyScannerGate(d *Result, s Scan, issues []types.ReviewIssue) error {
	if !s.Config.IsEnabled() {
		return nil
	}
	d.Active = true
	origin := s.Origin()
	if s.Reason != "" {
		d.Inconclusive = true
		d.Lines = append(d.Lines, fmt.Sprintf("%s (%s, origem %s, secao %s) inconclusivo (%s)", s.label(), s.Engine.Name(), origin, s.Section, s.Reason))
		return nil
	}
	rank, name, err := s.Config.Threshold()
	if err != nil {
		// config.Parse already validates fail_on; this is defensive only.
		return err
	}
	for _, issue := range issues {
		if GateRankOf(issue.Severity) < rank {
			continue
		}
		d.Fail = true
		d.Breach = true
		d.Lines = append(d.Lines, FindingLine(issue.RuleID, issue.Message, issue.Severity, name, origin+", secao "+s.Section))
		d.BlockingFindings = append(d.BlockingFindings, render.AuditFinding{
			RuleID:   issue.RuleID,
			Path:     issue.File,
			Line:     issue.Line,
			Severity: issue.Severity,
			Origin:   origin,
		})
	}
	return nil
}
