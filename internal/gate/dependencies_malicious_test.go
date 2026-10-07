package gate

import (
	"testing"

	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/dependencies"
)

func malicious() dependencies.Report {
	f := depFinding("MAL-2026-1234", dependencies.SeverityUnknown, dependencies.StatusIntroduced)
	f.Change.Name, f.Change.Base, f.Change.Head = "lodahs", "", "1.0.1"
	return dependencies.Report{Findings: []dependencies.Finding{f}}
}

// AUR-528 AC-001 / MUT-001: a MAL- advisory fails the check even with an
// empty fail_on.
func TestAUR528AC001MaliciousAlwaysFails(t *testing.T) {
	for name, cfg := range map[string]*config.DependenciesConfig{"no fail_on": {}, "fail_on critical": failOn("critical")} {
		res := applyDeps(t, depRun(cfg), malicious())
		if !res.Fail || !res.Breach || !hasLine(res, "pacote malicioso") {
			t.Fatalf("%s: malicious package = %+v", name, res)
		}
	}
}

// AUR-528 AC-002: an exception for a MAL- advisory is refused with a
// warning and the check still fails.
func TestAUR528AC002MaliciousExceptionRefused(t *testing.T) {
	run := depRun(failOn("high"))
	run.Cfg.Exceptions = []config.ExceptionConfig{{Repo: "acme/app", Rule: "cve/MAL-2026-1234", Path: "app/package-lock.json", Owner: "dev", Reason: "confio", Expires: "2026-12-31"}}
	res := applyDeps(t, run, malicious())
	if !res.Fail || len(res.AppliedExceptions) != 0 || !hasLine(res, "excecao recusada para pacote malicioso") {
		t.Fatalf("malicious exception = %+v", res)
	}
}

// AUR-528 AC-003: an unreachable source is inconclusive, never a pass.
func TestAUR528AC003UnreachableIsInconclusive(t *testing.T) {
	res := applyDeps(t, depRun(&config.DependenciesConfig{}), dependencies.Report{Reason: dependencies.ReasonSourceFailed})
	if !res.Inconclusive || !res.Fail {
		t.Fatalf("unreachable source = %+v", res)
	}
}

// AUR-528 AC-004: a grounded suspicion follows fail_on with its evidence in
// the gate line.
func TestAUR528AC004SuspicionFollowsFailOn(t *testing.T) {
	s := dependencies.Suspicion{
		Change:   dependencies.Change{Manifest: "web/package.json", Ecosystem: "npm", Name: "lodahs", Head: "1.0.1"},
		Summary:  "nome a uma letra de lodash",
		Evidence: []dependencies.Evidence{{Field: "first_published_at", Value: "2026-10-03T10:00:00Z"}},
	}
	report := dependencies.Report{Suspicions: []dependencies.Suspicion{s}}
	res := applyDeps(t, depRun(failOn("high")), report)
	if !res.Fail || !hasLine(res, "first_published_at=2026-10-03T10:00:00Z") || res.BlockingFindings[0].RuleID != "suspicion/lodahs" {
		t.Fatalf("suspicion at fail_on = %+v", res)
	}
	if res := applyDeps(t, depRun(failOn("critical")), report); res.Fail {
		t.Fatalf("suspicion below a critical-only fail_on must alert: %+v", res)
	}
}
