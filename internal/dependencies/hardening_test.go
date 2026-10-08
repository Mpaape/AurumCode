package dependencies

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// exitErr is a process exit with a code, as exec.ExitError reports it.
type exitErr int

func (e exitErr) Error() string { return "exit status" }
func (e exitErr) ExitCode() int { return int(e) }

func scannerReporting(stdout string, err error) OSVScanner {
	return OSVScanner{Binary: "osv-scanner", Command: func(context.Context, string, string, ...string) (string, string, error) {
		return stdout, "", err
	}}
}

// Only exit 0 and 1 carry a report; 128 is "no packages"; any other exit, a
// timeout or a kill is a failed extraction even with JSON on stdout. A
// relative root still matches the changed paths.
func TestScannerExitCodesAndRelativeRoot(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	report := `{"results":[{"source":{"path":"` + filepath.ToSlash(filepath.Join(cwd, "app", "package-lock.json")) + `"},"packages":[{"package":{"name":"minimist","version":"1.2.5","ecosystem":"npm"}}]}]}`
	paths := []string{"app/package-lock.json"}
	for _, ok := range []error{nil, exitErr(1)} {
		got, err := scannerReporting(report, ok).Extract(context.Background(), ".", paths)
		if err != nil || len(got["app/package-lock.json"]) != 1 {
			t.Fatalf("exit %v: %v %v", ok, got, err)
		}
	}
	if got, err := scannerReporting("", exitErr(128)).Extract(context.Background(), ".", paths); err != nil || len(got) != 0 {
		t.Fatalf("no packages: %v %v", got, err)
	}
	for _, bad := range []error{exitErr(127), exitErr(2), exitErr(-1), context.DeadlineExceeded} {
		if _, err := scannerReporting(report, bad).Extract(context.Background(), ".", paths); !errors.Is(err, ErrScannerFailed) {
			t.Fatalf("exit %v accepted: %v", bad, err)
		}
	}
	if scannerReason(errors.Join(ErrScannerFailed)) != ReasonScannerFailed {
		t.Fatal("a failed extraction must be dependencies_scanner_failed")
	}
}

// The scheduled scan never concludes over fewer packages: a package the
// scanner lists without a version is unresolved.
func TestScanVersionlessPackageIsInconclusive(t *testing.T) {
	osv := newFakeOSV(t, fixture)
	scanner := fakeExtractor{pkgs: map[string][]Package{"go.mod": {{Ecosystem: "Go", Name: "example.org/mod", Version: ""}}}}
	r := Scan(context.Background(), ScanInputs{Paths: []string{"go.mod"}, Model: fakeModel{}, Source: osv.source(), Extractor: scanner})
	if r.Reason != ReasonUnresolved || r.Findings != nil {
		t.Fatalf("versionless package = %q %+v", r.Reason, r.Findings)
	}
}

// When the scanner lists the package, its version and ecosystem are the
// ones asked of the source, and the divergence is declared.
func TestScannerVersionAndEcosystemWin(t *testing.T) {
	osv := newFakeOSV(t, fixture)
	scanner := fakeExtractor{pkgs: map[string][]Package{"app/package-lock.json": {{Ecosystem: "npm", Name: "minimist", Version: "1.2.5"}}}}
	wrong := Change{Manifest: "app/package-lock.json", Ecosystem: "NPM-registry", Name: "minimist", Base: "1.2.6", Head: "1.2.5"}
	r := run(t, osv, Inputs{Diff: npmBump("1.2.6", "1.2.5"), Model: fakeModel{manifests: []string{"app/package-lock.json"}, changes: []Change{wrong}}, Extractor: scanner})
	if r.Inconclusive() || len(findingsOf(r, StatusIntroduced)) != 1 || r.Changes[0].Ecosystem != "npm" {
		t.Fatalf("scanner ecosystem not used: %+v", r)
	}
	if !strings.Contains(strings.Join(r.Divergences, "\n"), "ecossistema") {
		t.Fatalf("ecosystem divergence not declared: %v", r.Divergences)
	}

	diff := diffOf(fileDiff("app/package-lock.json", `     "node_modules/minimist": {`, `-      "version": "1.2.6",`, `+      "version": "1.2.5",`, `+      "resolved": "minimist-1.2.4.tgz",`))
	model := fakeModel{manifests: []string{"app/package-lock.json"}, changes: []Change{minimist("1.2.6", "1.2.4")}}
	r = run(t, newFakeOSV(t, fixture), Inputs{Diff: diff, Model: model, Extractor: scanner})
	if r.Inconclusive() || r.Changes[0].Head != "1.2.5" || len(findingsOf(r, StatusIntroduced)) != 1 {
		t.Fatalf("scanner version not used: %+v", r)
	}
}

// CVSS v3 vectors give the severity when the database names none.
func TestCVSSSeverity(t *testing.T) {
	for vector, want := range map[string]float64{
		"CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H": 9.8,
		"CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:C/C:H/I:H/A:H": 10.0,
		"CVSS:3.1/AV:N/AC:L/PR:N/UI:R/S:U/C:L/I:L/A:N": 5.4,
		"CVSS:3.1/AV:N/AC:L/PR:L/UI:N/S:C/C:L/I:L/A:N": 6.4,
		"CVSS:3.0/AV:N/AC:H/PR:N/UI:N/S:U/C:L/I:N/A:N": 3.7,
	} {
		if got, err := CVSS3Score(vector); err != nil || got != want {
			t.Errorf("CVSS3Score(%s) = %v %v, want %v", vector, got, err, want)
		}
	}
	if _, err := CVSS3Score("CVSS:3.1/AV:N/AC:L"); err == nil {
		t.Error("partial vector scored")
	}
	v := osvVuln{ID: "GO-2024-0001", Severity: []osvSeverity{{Type: "CVSS_V3", Score: "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H"}}}
	if got := (OSV{}).convert(v, Query{Name: "x"}).Severity; got != "critical" {
		t.Fatalf("GO- advisory with CVSS = %q", got)
	}
	v4 := osvVuln{ID: "RUSTSEC-1", Severity: []osvSeverity{{Type: "CVSS_V4", Score: "CVSS:4.0/AV:N/AC:L/AT:N/PR:N/UI:N/VC:H/VI:H/VA:H/SC:N/SI:N/SA:N"}}}
	if got := (OSV{}).convert(v4, Query{Name: "x"}).Severity; got != SeverityUnknown {
		t.Fatalf("v4-only advisory = %q, want unknown", got)
	}
}

// Records of one advisory under several databases are one finding.
func TestAliasesMerged(t *testing.T) {
	got := mergeAliases([]Vulnerability{
		{ID: "GO-2022-0001", Aliases: []string{"CVE-2022-1"}, Severity: SeverityUnknown},
		{ID: "GHSA-aaaa", Aliases: []string{"CVE-2022-1"}, Severity: "high", Fixed: []string{"1.0.1"}},
		{ID: "GHSA-bbbb", Severity: "low"},
	})
	if len(got) != 2 || got[0].Severity != "high" || got[0].CanonicalID() != "CVE-2022-1" || len(got[0].Fixed) != 1 {
		t.Fatalf("merged = %+v", got)
	}
}

// With fail_on declared, a new package no registry could analyse is
// inconclusive, unless the same package was vetted from its lockfile.
func TestUnvettedPackageInconclusiveUnderFailOn(t *testing.T) {
	in, _ := typosquat()
	in.Registry = fakeRegistry{}
	in.VetRequired = true
	if r := run(t, newFakeOSV(t, fixture), *in); r.Reason != ReasonUnvetted || len(r.Unvetted) != 1 {
		t.Fatalf("unvetted under fail_on = %q %v", r.Reason, r.Unvetted)
	}
	in.VetRequired = false
	if r := run(t, newFakeOSV(t, fixture), *in); r.Inconclusive() || len(r.Unvetted) != 1 {
		t.Fatalf("unvetted without fail_on must be declared only: %q %v", r.Reason, r.Unvetted)
	}

	diff := diffOf(
		fileDiff("web/package.json", `+    "lodahs": "^1.0.1",`),
		fileDiff("web/package-lock.json", `+    "node_modules/lodahs": {`, `+      "version": "1.0.1",`),
	)
	ranged := Change{Manifest: "web/package.json", Ecosystem: "npm", Name: "lodahs", HeadRange: "^1.0.1"}
	locked := Change{Manifest: "web/package-lock.json", Ecosystem: "npm", LicenseSystem: "npm", Name: "lodahs", Head: "1.0.1"}
	reg := fakeRegistry{meta: map[string]Metadata{"lodahs": {"version": "1.0.1"}}}
	both := Inputs{Diff: diff, Model: fakeModel{manifests: []string{"web/package.json", "web/package-lock.json"}, changes: []Change{ranged, locked}}, Registry: reg, VetRequired: true}
	if r := run(t, newFakeOSV(t, fixture), both); r.Inconclusive() || len(r.Unvetted) != 0 {
		t.Fatalf("range vetted through its lockfile = %q %v", r.Reason, r.Unvetted)
	}
}
