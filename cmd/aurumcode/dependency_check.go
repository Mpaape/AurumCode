// The dependency check of the evidence phase: with a declared dependencies
// section, the model reads the changed manifests, the advisory source is
// asked about both sides and the report joins the gate. It runs after the
// model's provider is selected, over the same checkout and refusal as the
// scanners.
package main

import (
	"fmt"
	"net/http"
	"time"

	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/dependencies"
	"github.com/Mpaape/AurumCode/internal/llm"
)

// dependencySources builds the advisory source and the extraction scanner
// of one check; nil fields take production's (dependencySourcesFor).
type dependencySources struct {
	source    func(*config.DependenciesConfig, func() time.Time) dependencies.Source
	extractor func(*config.DependenciesConfig) dependencies.Extractor
}

// dependencySourcesFor is production: the configured OSV API and
// osv-scanner through the scanners' executor command.
func (s *reviewState) dependencySourcesFor() dependencySources {
	d := s.deps.dependencies
	if d.source == nil {
		d.source = func(cfg *config.DependenciesConfig, now func() time.Time) dependencies.Source {
			return dependencies.OSV{BaseURL: cfg.EffectiveOSVURL(), Client: &http.Client{Timeout: 60 * time.Second}, MaxAge: sourceMaxAge(cfg), Now: now}
		}
	}
	if d.extractor == nil {
		d.extractor = func(cfg *config.DependenciesConfig) dependencies.Extractor {
			return dependencies.OSVScanner{Binary: cfg.EffectiveScanner(), Command: s.deps.scanners.Command}
		}
	}
	return d
}

func sourceMaxAge(cfg *config.DependenciesConfig) time.Duration {
	if cfg.MaxSourceAgeHours == nil {
		return 0
	}
	return time.Duration(*cfg.MaxSourceAgeHours) * time.Hour
}

// runDependencyCheck runs the check when the section is declared and
// states its outcome as limitations; the gate reads s.depReport.
func (s *reviewState) runDependencyCheck() {
	s.depReport = nil
	cfg := s.cfg.Dependencies
	if !cfg.Declared() {
		return
	}
	var model dependencies.Completer
	if s.provider != nil {
		model = llm.NewOrchestrator(s.provider, nil, s.tracker)
	}
	src := s.dependencySourcesFor()
	report := dependencies.Check(s.ctx, dependencies.Inputs{
		Diff:      s.diff,
		Model:     model,
		Source:    src.source(cfg, s.deps.clock),
		Extractor: src.extractor(cfg),
		Root:      s.scanRoot,
		Blocked:   s.scanBlocked,
	})
	s.depReport = &report
	for _, line := range dependencyNotices(report) {
		fmt.Fprintf(s.stderr, "aurumcode review: %s\n", line)
		s.result.Limitations = append(s.result.Limitations, line)
	}
}

// dependencyNotices is what the review states besides the gate lines: the
// reason of an inconclusive check, the model's discarded answers and the
// manifests read.
func dependencyNotices(r dependencies.Report) []string {
	var out []string
	if r.Inconclusive() {
		out = append(out, fmt.Sprintf("Dependencias: verificacao inconclusiva (%s): %s", r.Reason, r.Detail))
	}
	for _, d := range r.Discarded {
		out = append(out, "Dependencias: extracao do modelo descartada, ausente do diff: "+d)
	}
	for _, d := range r.Divergences {
		out = append(out, "Dependencias: divergencia entre modelo e scanner: "+d)
	}
	for _, f := range r.Findings {
		if f.Status == dependencies.StatusFixed {
			out = append(out, fmt.Sprintf("Dependencias: o PR corrige %s em %s (%s)", f.Vuln.ID, f.Change.Name, f.Change.Manifest))
		}
	}
	return out
}

// dependencyReportForGate is the report the gate judges: nil without the
// section, and an inconclusive report when the section is declared but the
// check never ran (a phase ended before it), never "no findings".
func (s *reviewState) dependencyReportForGate() *dependencies.Report {
	if s.depReport != nil || s.cfg == nil || !s.cfg.Dependencies.Declared() {
		return s.depReport
	}
	return &dependencies.Report{Reason: dependencies.ReasonNotRun, Detail: "a verificacao de dependencias nao rodou nesta revisao"}
}
