package dependencies

import (
	"context"
	"testing"
)

// licensed is a new npm package whose registered license the test picks.
func licensed(expressions []string) Inputs {
	diff := diffOf(fileDiff("web/package.json", `+    "copyleft-lib": "2.0.0",`))
	change := Change{Manifest: "web/package.json", Ecosystem: "npm", LicenseSystem: "npm", Name: "copyleft-lib", Head: "2.0.0"}
	reg := fakeRegistry{meta: map[string]Metadata{"copyleft-lib": {"version": "2.0.0"}}, licenses: map[string][]string{"copyleft-lib": expressions}}
	return Inputs{Diff: diff, Model: fakeModel{manifests: []string{"web/package.json"}, changes: []Change{change}}, Registry: reg, LicensesDenied: []string{"AGPL-3.0-only", "SSPL-1.0"}}
}

type fakeText string

func (f fakeText) LicenseText(context.Context, Change) (string, error) { return string(f), nil }

// AUR-529 AC-001: a new package with a denied license is recorded with
// package, version and license.
func TestAUR529AC001DeniedLicense(t *testing.T) {
	r := run(t, newFakeOSV(t, fixture), licensed([]string{"AGPL-3.0-only"}))
	if r.Inconclusive() || len(r.Licenses) != 1 {
		t.Fatalf("license not judged: %+v", r)
	}
	l := r.Licenses[0]
	if l.Verdict != LicenseDenied || l.Change.Name != "copyleft-lib" || l.Change.Head != "2.0.0" || l.Expression != "AGPL-3.0-only" {
		t.Fatalf("license = %+v", l)
	}
	in := licensed([]string{"AGPL-3.0-only"})
	in.LicensesDenied = nil
	if r := run(t, newFakeOSV(t, fixture), in); len(r.Licenses) != 0 || r.Inconclusive() {
		t.Fatalf("no licenses_denied must not judge licenses: %+v", r)
	}
}

// AUR-529 AC-002 / MUT-001: an SPDX expression is judged by its structure,
// never by substring.
func TestAUR529AC002ExpressionNotSubstring(t *testing.T) {
	denied := []string{"AGPL-3.0-only", "GPL-3.0-only", "GPL-2.0-only"}
	for expr, want := range map[string]LicenseVerdict{
		"MIT OR AGPL-3.0-only":                      LicenseAllowed,
		"MIT AND AGPL-3.0-only":                     LicenseDenied,
		"LGPL-3.0-only":                             LicenseAllowed,
		"(MIT OR AGPL-3.0-only) AND Apache-2.0":     LicenseAllowed,
		"(AGPL-3.0-only OR GPL-3.0-only) AND MIT":   LicenseDenied,
		"agpl-3.0-only":                             LicenseDenied,
		"GPL-2.0-only WITH Classpath-exception-2.0": LicenseDenied,
		"Apache-2.0 WITH LLVM-exception":            LicenseAllowed,
		"LicenseRef-custom":                         LicenseUnknown,
		"MIT OR LicenseRef-custom":                  LicenseAllowed,
		"AGPL-3.0-only OR LicenseRef-custom":        LicenseUnknown,
		"MIT OR":                                    LicenseUnknown,
		"non-standard":                              LicenseUnknown,
	} {
		if got := EvaluateLicense(expr, denied); got != want {
			t.Errorf("EvaluateLicense(%q) = %s, want %s", expr, got, want)
		}
	}
	r := run(t, newFakeOSV(t, fixture), licensed([]string{"MIT OR AGPL-3.0-only"}))
	if r.Inconclusive() || r.Licenses[0].Verdict != LicenseAllowed {
		t.Fatalf("dual license = %+v", r)
	}
}

// AUR-529 AC-003: an unknown license or an unreachable source is
// inconclusive; the model's confident classification of the license text
// decides when the registry has no SPDX identifier.
func TestAUR529AC003UnknownIsInconclusive(t *testing.T) {
	if r := run(t, newFakeOSV(t, fixture), licensed([]string{"non-standard"})); r.Reason != ReasonLicenseUnknown {
		t.Fatalf("non-standard license = %q", r.Reason)
	}
	down := licensed([]string{"MIT"})
	down.Registry = fakeRegistry{fail: true}
	if r := run(t, newFakeOSV(t, fixture), down); r.Reason != ReasonLicenseUnknown {
		t.Fatalf("unreachable license source = %q", r.Reason)
	}

	classified := licensed([]string{"non-standard"})
	model := classified.Model.(fakeModel)
	model.license = map[string]any{"spdx": "AGPL-3.0-only", "confident": true}
	classified.Model, classified.LicenseText = model, fakeText("GNU AFFERO GENERAL PUBLIC LICENSE Version 3")
	r := run(t, newFakeOSV(t, fixture), classified)
	if r.Inconclusive() || r.Licenses[0].Verdict != LicenseDenied || !r.Licenses[0].ByModel {
		t.Fatalf("classified text = %+v", r)
	}

	model.license = map[string]any{"spdx": "MIT", "confident": false}
	classified.Model = model
	if r := run(t, newFakeOSV(t, fixture), classified); r.Reason != ReasonLicenseUnknown {
		t.Fatalf("unconfident classification = %q", r.Reason)
	}
}
