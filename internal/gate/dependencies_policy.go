package gate

import (
	"fmt"
	"strings"
	"time"

	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/dependencies"
	"github.com/Mpaape/AurumCode/internal/gate/facts"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// DependencyRulePrefix is the rule prefix of an advisory finding, as an
// exception names it (`rule: cve/<id>`; any identifier the advisory
// carries: OSV, GHSA, CVE).
const DependencyRulePrefix = "cve/"

// DependencySuspicionPrefix is the rule prefix of a typosquat suspicion.
const DependencySuspicionPrefix = "suspicion/"

// The decisions a dependency gate line ends with.
const (
	depDecisionInfo      = "informativo"
	depDecisionWarn      = "alerta"
	depDecisionBlock     = "reprova"
	depDecisionFixed     = "corrigida"
	depDecisionExcused   = "aceita por excecao"
	depDecisionMalicious = "reprova: pacote malicioso"
)

// dependencyPolicy is the dependencies section applied to one run: which
// severities fail (gated is false without fail_on: findings are
// informative), what a pre-existing advisory does, and the exceptions.
type dependencyPolicy struct {
	cfg        *config.DependenciesConfig
	gated      bool
	exceptions []config.ExceptionConfig
	repo       string
	now        time.Time
}

func newDependencyPolicy(run *Run) (dependencyPolicy, error) {
	cfg := run.Cfg.Dependencies
	if err := cfg.Validate(); err != nil {
		return dependencyPolicy{}, err
	}
	return dependencyPolicy{cfg: cfg, gated: cfg.Gated(), exceptions: run.Cfg.Exceptions, repo: run.RepoIdentity, now: run.Clock()}, nil
}

// judge folds one advisory into part: fixed is stated; an active exception
// for any of its identifiers on this manifest accepts it; otherwise an
// introduced advisory at or above fail_on fails and one below alerts, and a
// pre-existing one follows `preexisting`. The severity is the source's at
// this run, never a stored one.
func (p dependencyPolicy) judge(part *Result, f dependencies.Finding) {
	if f.Status == dependencies.StatusFixed {
		part.Lines = append(part.Lines, DependencyFindingLine(f, depDecisionFixed))
		return
	}
	if f.Vuln.Malicious() {
		p.judgeMalicious(part, f)
		return
	}
	if !p.gated && !(f.Status == dependencies.StatusPreexisting && p.cfg.EffectivePreexisting() == config.PreexistingBlock) {
		part.Lines = append(part.Lines, DependencyFindingLine(f, depDecisionInfo))
		return
	}
	if p.excused(part, f) {
		part.Lines = append(part.Lines, DependencyFindingLine(f, depDecisionExcused))
		return
	}
	if p.blocks(f) {
		p.block(part, f, DependencyRulePrefix+f.Vuln.ID, f.Vuln.Severity)
		part.Lines = append(part.Lines, DependencyFindingLine(f, depDecisionBlock))
		return
	}
	part.Lines = append(part.Lines, DependencyFindingLine(f, depDecisionWarn))
}

// blocks decides an unexcused advisory. Introduced: its severity is in
// fail_on. Pre-existing: only under `preexisting: block`, in fail_on when
// one is declared, at any severity otherwise.
func (p dependencyPolicy) blocks(f dependencies.Finding) bool {
	atThreshold := p.cfg.Fails(f.Vuln.Severity)
	if f.Status == dependencies.StatusIntroduced {
		return atThreshold
	}
	return p.cfg.EffectivePreexisting() == config.PreexistingBlock && (atThreshold || !p.gated)
}

// block records a failing dependency finding as a breach.
func (p dependencyPolicy) block(part *Result, f dependencies.Finding, rule, severity string) {
	part.Fail, part.Breach = true, true
	part.BlockingFindings = append(part.BlockingFindings, facts.AuditFinding{
		RuleID:   rule,
		Path:     f.Change.Manifest,
		Severity: severity,
		Origin:   OriginDependencies,
	})
}

// excused looks for an active exception naming any identifier of the
// advisory on this manifest; an expired one is stated and does not apply.
func (p dependencyPolicy) excused(part *Result, f dependencies.Finding) bool {
	for _, id := range f.Vuln.Identifiers() {
		if p.exception(part, DependencyRulePrefix+id, f.Change.Manifest) {
			return true
		}
	}
	return false
}

// exception matches one rule on one path against the configured exceptions
// (the same exact repo/rule/path match and expiry as every other finding).
func (p dependencyPolicy) exception(part *Result, rule, path string) bool {
	issue := types.ReviewIssue{RuleID: rule, File: path}
	exc, status := MatchException(p.exceptions, p.repo, rule, path, p.now)
	switch status {
	case ExceptionActive:
		part.Lines = append(part.Lines, AcceptedExceptionLine(exc, issue))
		part.AppliedExceptions = append(part.AppliedExceptions, facts.AuditException{
			RuleID:        rule,
			Path:          path,
			Justification: fmt.Sprintf("dono: %s, motivo: %s, validade: %s", exc.Owner, exc.Reason, exc.Expires),
		})
		return true
	case ExceptionExpired:
		part.Lines = append(part.Lines, ExpiredExceptionLine(exc, issue))
	}
	return false
}

// judgeMalicious: a package the source marks malicious (MAL-) fails the
// check whatever fail_on says, and no exception can accept it; a matching
// one is refused with a warning.
func (p dependencyPolicy) judgeMalicious(part *Result, f dependencies.Finding) {
	for _, id := range f.Vuln.Identifiers() {
		exc, status := MatchException(p.exceptions, p.repo, DependencyRulePrefix+id, f.Change.Manifest, p.now)
		if status == ExceptionActive {
			part.Lines = append(part.Lines, fmt.Sprintf("%s%s em %s: excecao recusada para pacote malicioso (dono: %s, validade: %s)", DependencyRulePrefix, id, f.Change.Manifest, exc.Owner, exc.Expires))
		}
	}
	if blocksMalicious(p, f) {
		p.block(part, f, DependencyRulePrefix+f.Vuln.ID, f.Vuln.Severity)
		part.Lines = append(part.Lines, DependencyFindingLine(f, depDecisionMalicious))
		return
	}
	part.Lines = append(part.Lines, DependencyFindingLine(f, depDecisionInfo))
}

// blocksMalicious: a malicious package always fails, independent of fail_on.
func blocksMalicious(p dependencyPolicy, f dependencies.Finding) bool {
	return f.Vuln.Malicious()
}

// judgeSuspicion: a grounded typosquat or malicious-package suspicion
// counts with suspicion_severity against fail_on, as any finding; an
// exception may name it as `rule: suspicion/<name>` on its manifest.
func (p dependencyPolicy) judgeSuspicion(part *Result, s dependencies.Suspicion) {
	severity := p.cfg.EffectiveSuspicionSeverity()
	rule := DependencySuspicionPrefix + s.Change.Name
	line := DependencySuspicionLine(s, severity)
	switch {
	case !p.gated:
		part.Lines = append(part.Lines, line+" ["+depDecisionInfo+"]")
	case p.exception(part, rule, s.Change.Manifest):
		part.Lines = append(part.Lines, line+" ["+depDecisionExcused+"]")
	case p.cfg.Fails(severity):
		part.Fail, part.Breach = true, true
		part.BlockingFindings = append(part.BlockingFindings, facts.AuditFinding{RuleID: rule, Path: s.Change.Manifest, Severity: severity, Origin: OriginDependencies})
		part.Lines = append(part.Lines, line+" ["+depDecisionBlock+"]")
	default:
		part.Lines = append(part.Lines, line+" ["+depDecisionWarn+"]")
	}
}

// DependencySuspicionLine states a suspicion with the metadata it rests on.
func DependencySuspicionLine(s dependencies.Suspicion, severity string) string {
	evidence := make([]string, 0, len(s.Evidence))
	for _, e := range s.Evidence {
		evidence = append(evidence, e.Field+"="+e.Value)
	}
	return fmt.Sprintf("DEPENDENCIAS suspeita de pacote malicioso ou typosquat: %s %s (%s, %s), severidade %s: %s (evidencia: %s)",
		s.Change.Name, s.Change.Head, s.Change.Ecosystem, s.Change.Manifest, severity, s.Summary, strings.Join(evidence, "; "))
}
