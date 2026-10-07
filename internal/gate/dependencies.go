package gate

import (
	"context"
	"strings"

	"github.com/Mpaape/AurumCode/internal/dependencies"
	"github.com/Mpaape/AurumCode/internal/i18n"
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
		detail := report.Detail
		if run.Filter != nil {
			detail = run.Filter.Redact(detail)
		}
		part.Lines = append(part.Lines, i18n.Format(run.Language, "deps.gate.inconclusive", report.Reason, detail))
	}
	policy, err := newDependencyPolicy(run)
	if err != nil {
		return Result{}, Fatal(err)
	}
	for _, d := range report.Divergences {
		part.Lines = append(part.Lines, i18n.Format(run.Language, "deps.gate.divergence", d))
	}
	// Findings exist only when the advisory lookup itself concluded; a later
	// failure (registry) keeps them judged, so a malicious package is never
	// hidden behind an inconclusive mark.
	for _, f := range report.Findings {
		policy.judge(&part, f)
	}
	for _, s := range report.Suspicions {
		policy.judgeSuspicion(&part, s)
	}
	for _, l := range report.Licenses {
		policy.judgeLicense(&part, l)
	}
	return part, nil
}

// DependencyFindingLine is how a gate line states one advisory in language:
// where it stands relative to the change, its identifiers, the package and
// version, the severity, the fixed versions, the advisory link and the
// decision (an internal/i18n key).
func DependencyFindingLine(language string, f dependencies.Finding, decision string) string {
	version := f.Change.Head
	if f.Status == dependencies.StatusFixed || version == "" {
		version = f.Change.Base
	}
	if f.ByRange {
		version = i18n.Format(language, "deps.range", f.Change.HeadRange)
	}
	fixed := i18n.Text(language, "deps.no_fix")
	if len(f.Vuln.Fixed) > 0 {
		fixed = i18n.Format(language, "deps.fixed_in", strings.Join(f.Vuln.Fixed, ", "))
	}
	return i18n.Format(language, "deps.gate.finding",
		i18n.Text(language, "deps.status."+string(f.Status)), strings.Join(f.Vuln.Identifiers(), "/"), f.Change.Name, version,
		f.Change.Ecosystem, f.Change.Manifest, f.Vuln.Severity, fixed, f.Vuln.Link, i18n.Text(language, decision))
}
