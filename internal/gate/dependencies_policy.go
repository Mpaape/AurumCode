package gate

import (
	"fmt"
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

// The decisions a dependency gate line ends with.
const (
	depDecisionInfo    = "informativo"
	depDecisionWarn    = "alerta"
	depDecisionBlock   = "reprova"
	depDecisionFixed   = "corrigida"
	depDecisionExcused = "aceita por excecao"
)

// dependencyPolicy is the dependencies section applied to one run: the
// severity threshold (gated is false without fail_on: findings are
// informative), what a pre-existing advisory does, and the exceptions.
type dependencyPolicy struct {
	cfg        *config.DependenciesConfig
	threshold  config.GateSeverityRank
	gated      bool
	exceptions []config.ExceptionConfig
	repo       string
	now        time.Time
}

func newDependencyPolicy(run *Run) (dependencyPolicy, error) {
	cfg := run.Cfg.Dependencies
	rank, gated, err := cfg.Threshold()
	if err != nil {
		return dependencyPolicy{}, err
	}
	return dependencyPolicy{cfg: cfg, threshold: rank, gated: gated, exceptions: run.Cfg.Exceptions, repo: run.RepoIdentity, now: run.Clock()}, nil
}

// DependencySeverityRank is the gate rank of an advisory severity read from
// the source at this run (never a stored one). An unknown severity counts
// at the top: a severity that cannot be read never lets a finding pass.
func DependencySeverityRank(severity string) config.GateSeverityRank {
	if rank, _, err := config.NormalizeGateSeverity(severity); err == nil {
		return rank
	}
	return config.GateSeverityError
}

// judge folds one advisory into part: fixed is stated; an active exception
// for any of its identifiers on this manifest accepts it; otherwise an
// introduced advisory at or above fail_on fails and one below alerts, and a
// pre-existing one follows `preexisting`.
func (p dependencyPolicy) judge(part *Result, f dependencies.Finding) {
	if f.Status == dependencies.StatusFixed {
		part.Lines = append(part.Lines, DependencyFindingLine(f, depDecisionFixed))
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

// blocks decides an unexcused advisory. Introduced: at or above fail_on.
// Pre-existing: only under `preexisting: block`, at or above fail_on when
// one is declared, at any severity otherwise.
func (p dependencyPolicy) blocks(f dependencies.Finding) bool {
	atThreshold := p.gated && DependencySeverityRank(f.Vuln.Severity) >= p.threshold
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
