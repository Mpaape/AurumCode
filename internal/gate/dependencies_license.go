package gate

import (
	"github.com/Mpaape/AurumCode/internal/dependencies"
	"github.com/Mpaape/AurumCode/internal/gate/facts"
	"github.com/Mpaape/AurumCode/internal/i18n"
)

// DependencyLicensePrefix is the rule prefix of a denied license, as an
// exception names it (`rule: license/<package>` on the manifest).
const DependencyLicensePrefix = "license/"

// depLicenseSeverity is the severity a denied license is recorded with: a
// denied license fails whatever fail_on says, the policy listed it.
const depLicenseSeverity = "high"

// judgeLicense: a new or updated package whose registered license the
// policy denies fails the check, unless an active exception names the
// package on its manifest. An unknown license is stated; the report's
// reason already made the run inconclusive.
func (p dependencyPolicy) judgeLicense(part *Result, l dependencies.License) {
	switch l.Verdict {
	case dependencies.LicenseDenied:
	case dependencies.LicenseUnknown:
		part.Lines = append(part.Lines, DependencyLicenseLine(p.lang, l, i18n.Format(p.lang, "deps.decision.inconclusive", l.Reason)))
		return
	default:
		return
	}
	rule := DependencyLicensePrefix + l.Change.Name
	if p.exception(part, rule, l.Change.Manifest) {
		part.Lines = append(part.Lines, DependencyLicenseLine(p.lang, l, i18n.Text(p.lang, depDecisionExcused)))
		return
	}
	part.Fail, part.Breach = true, true
	part.BlockingFindings = append(part.BlockingFindings, facts.AuditFinding{RuleID: rule, Path: l.Change.Manifest, Severity: depLicenseSeverity, Origin: OriginDependencies})
	part.Lines = append(part.Lines, DependencyLicenseLine(p.lang, l, i18n.Text(p.lang, depDecisionBlock)))
}

// DependencyLicenseLine states the package, its version, its license and
// the decision text, in language.
func DependencyLicenseLine(language string, l dependencies.License, decision string) string {
	how := i18n.Text(language, "deps.license.registry")
	if l.ByModel {
		how = i18n.Text(language, "deps.license.model")
	}
	return i18n.Format(language, "deps.gate.license", i18n.Text(language, "deps.license."+string(l.Verdict)),
		l.Change.Name, l.Change.Head, l.Change.Ecosystem, l.Change.Manifest, l.Expression, how, decision)
}
