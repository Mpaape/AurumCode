package gate

import (
	"context"
	"fmt"
	"strings"

	"github.com/Mpaape/AurumCode/internal/dependencies"
)

// ContributorDependencies is the dependency check's contributor name.
const ContributorDependencies = "dependencies"

// OriginDependencies is the typed origin of dependency findings.
const OriginDependencies = "dependencies"

// DependenciesContributor gates the dependency check of the change. Report
// is nil when the configuration declares no dependencies section: the
// contributor then adds nothing. An inconclusive check is inconclusive here
// (the one rule, ApplyInconclusiveMode, decides what that does), never a
// pass. Each advisory is judged by the section's policy at this run's
// severity (dependencies_policy.go).
type DependenciesContributor struct {
	Report *dependencies.Report
}

func (DependenciesContributor) Name() string   { return ContributorDependencies }
func (DependenciesContributor) Origin() string { return OriginDependencies }
func (c DependenciesContributor) Apply(_ context.Context, run *Run, _ Result) (Result, error) {
	if c.Report == nil {
		return Result{}, nil
	}
	part := Result{Active: true}
	report := *c.Report
	if report.Inconclusive() {
		part.Inconclusive = true
		part.AddReason(report.Reason)
		part.Lines = append(part.Lines, fmt.Sprintf("DEPENDENCIAS inconclusivo (%s): %s", report.Reason, report.Detail))
		return part, nil
	}
	policy, err := newDependencyPolicy(run)
	if err != nil {
		return Result{}, Fatal(err)
	}
	for _, d := range report.Divergences {
		part.Lines = append(part.Lines, "DEPENDENCIAS divergencia entre modelo e scanner: "+d)
	}
	for _, f := range report.Findings {
		policy.judge(&part, f)
	}
	return part, nil
}

// DependencyFindingLine is how a gate line states one advisory: where it
// stands relative to the change, its identifiers, the package and version,
// the severity, the fixed versions and the advisory link.
func DependencyFindingLine(f dependencies.Finding, decision string) string {
	version := f.Change.Head
	if f.Status == dependencies.StatusFixed || version == "" {
		version = f.Change.Base
	}
	if f.ByRange {
		version = "faixa " + f.Change.HeadRange
	}
	fixed := "sem versao corrigida"
	if len(f.Vuln.Fixed) > 0 {
		fixed = "corrigida em " + strings.Join(f.Vuln.Fixed, ", ")
	}
	return fmt.Sprintf("DEPENDENCIAS %s %s: %s %s (%s, %s), severidade %s, %s, %s [%s]",
		statusWord(f.Status), strings.Join(f.Vuln.Identifiers(), "/"), f.Change.Name, version,
		f.Change.Ecosystem, f.Change.Manifest, f.Vuln.Severity, fixed, f.Vuln.Link, decision)
}

func statusWord(s dependencies.Status) string {
	switch s {
	case dependencies.StatusIntroduced:
		return "introduzida pelo PR"
	case dependencies.StatusPreexisting:
		return "pre-existente"
	default:
		return "corrigida pelo PR"
	}
}
