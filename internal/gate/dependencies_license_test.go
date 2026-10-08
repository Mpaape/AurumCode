package gate

import (
	"testing"

	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/dependencies"
)

func deniedLicense() dependencies.Report {
	return dependencies.Report{Licenses: []dependencies.License{{
		Change:     dependencies.Change{Manifest: "web/package.json", Ecosystem: "npm", Name: "copyleft-lib", Head: "2.0.0"},
		Expression: "AGPL-3.0-only",
		Verdict:    dependencies.LicenseDenied,
	}}}
}

// AUR-529 AC-001: a denied license fails the check with package, version and
// license in the gate line.
func TestAUR529AC001DeniedLicenseFails(t *testing.T) {
	res := applyDeps(t, depRun(&config.DependenciesConfig{LicensesDenied: []string{"AGPL-3.0-only"}}), deniedLicense())
	if !res.Fail || !hasLine(res, "copyleft-lib 2.0.0") || !hasLine(res, `"AGPL-3.0-only"`) || res.BlockingFindings[0].RuleID != "license/copyleft-lib" {
		t.Fatalf("denied license = %+v", res)
	}
}

// AUR-529 AC-003: an unknown license is inconclusive, never a pass.
func TestAUR529AC003UnknownLicenseInconclusive(t *testing.T) {
	report := dependencies.Report{Reason: dependencies.ReasonLicenseUnknown, Licenses: []dependencies.License{{Change: dependencies.Change{Name: "x", Head: "1.0.0"}, Verdict: dependencies.LicenseUnknown, Reason: "non-standard"}}}
	res := applyDeps(t, depRun(&config.DependenciesConfig{LicensesDenied: []string{"AGPL-3.0-only"}}), report)
	if !res.Inconclusive || !res.Fail {
		t.Fatalf("unknown license = %+v", res)
	}
}

// AUR-529 AC-004: an exception for the package, with owner and expiry,
// applies as in AUR-520; expired, it does not.
func TestAUR529AC004PackageException(t *testing.T) {
	exc := config.ExceptionConfig{Repo: "acme/app", Rule: "license/copyleft-lib", Path: "web/package.json", Owner: "juridico", Reason: "uso interno aprovado", Expires: "2026-12-31"}
	run := depRun(&config.DependenciesConfig{LicensesDenied: []string{"AGPL-3.0-only"}})
	run.Cfg.Exceptions = []config.ExceptionConfig{exc}
	if res := applyDeps(t, run, deniedLicense()); res.Fail || len(res.AppliedExceptions) != 1 {
		t.Fatalf("license exception = %+v", res)
	}
	exc.Expires = "2026-01-01"
	run = depRun(&config.DependenciesConfig{LicensesDenied: []string{"AGPL-3.0-only"}})
	run.Cfg.Exceptions = []config.ExceptionConfig{exc}
	if res := applyDeps(t, run, deniedLicense()); !res.Fail || !hasLine(res, ExpiredExceptionMarker) {
		t.Fatalf("expired license exception = %+v", res)
	}
}
