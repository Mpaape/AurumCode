package dependencies

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

const fixture = "testdata/osv-fake.json"

func run(t *testing.T, osv *fakeOSV, in Inputs) Report {
	t.Helper()
	if in.Source == nil {
		in.Source = osv.source()
	}
	if in.Extractor == nil {
		in.Extractor = fakeExtractor{}
	}
	return Check(context.Background(), in)
}

// AC-001: a bump to a version with an advisory is introduced by the change,
// with identifiers, fixed version and link.
func TestAUR495AC001IntroducedAdvisory(t *testing.T) {
	osv := newFakeOSV(t, fixture)
	model := fakeModel{manifests: []string{"app/package-lock.json"}, changes: []Change{minimist("1.2.6", "1.2.5")}}
	r := run(t, osv, Inputs{Diff: npmBump("1.2.6", "1.2.5"), Model: model})
	if r.Inconclusive() {
		t.Fatalf("inconclusive: %s %s", r.Reason, r.Detail)
	}
	got := findingsOf(r, StatusIntroduced)
	if len(got) != 1 {
		t.Fatalf("introduced = %+v, want one", r.Findings)
	}
	f := got[0]
	if f.Vuln.ID != "GHSA-xvch-5gv4-984h" || f.Vuln.Aliases[0] != "CVE-2021-44906" || f.Vuln.Severity != "critical" {
		t.Errorf("advisory = %+v", f.Vuln)
	}
	if len(f.Vuln.Fixed) != 1 || f.Vuln.Fixed[0] != "1.2.6" || !strings.HasPrefix(f.Vuln.Link, "https://") {
		t.Errorf("fixed/link = %v %q", f.Vuln.Fixed, f.Vuln.Link)
	}
	if f.Change.Head != "1.2.5" || f.Change.Manifest != "app/package-lock.json" {
		t.Errorf("change = %+v", f.Change)
	}
}

// AC-002 / MUT-002: an advisory on both sides is pre-existing, never
// introduced.
func TestAUR495AC002Preexisting(t *testing.T) {
	osv := newFakeOSV(t, fixture)
	model := fakeModel{manifests: []string{"app/package-lock.json"}, changes: []Change{minimist("1.2.0", "1.2.5")}}
	r := run(t, osv, Inputs{Diff: npmBump("1.2.0", "1.2.5"), Model: model})
	if r.Inconclusive() {
		t.Fatalf("inconclusive: %s", r.Reason)
	}
	if n := len(findingsOf(r, StatusIntroduced)); n != 0 {
		t.Fatalf("introduced = %d, want 0: %+v", n, r.Findings)
	}
	if n := len(findingsOf(r, StatusPreexisting)); n != 1 {
		t.Fatalf("preexisting = %d, want 1: %+v", n, r.Findings)
	}
}

// AC-003: a change that fixes the version removes the finding and records
// the fix.
func TestAUR495AC003Fixed(t *testing.T) {
	osv := newFakeOSV(t, fixture)
	model := fakeModel{manifests: []string{"app/package-lock.json"}, changes: []Change{minimist("1.2.5", "1.2.6")}}
	r := run(t, osv, Inputs{Diff: npmBump("1.2.5", "1.2.6"), Model: model})
	if r.Inconclusive() {
		t.Fatalf("inconclusive: %s", r.Reason)
	}
	if len(findingsOf(r, StatusIntroduced))+len(findingsOf(r, StatusPreexisting)) != 0 {
		t.Fatalf("vulnerable findings remain: %+v", r.Findings)
	}
	fixed := findingsOf(r, StatusFixed)
	if len(fixed) != 1 || fixed[0].Vuln.ID != "GHSA-xvch-5gv4-984h" {
		t.Fatalf("fixed = %+v", fixed)
	}
}

// AC-004: a manifest format neither side knows is read by the model alone;
// an extraction that diverges from the scanner is declared.
func TestAUR495AC004UnknownFormatAndDivergence(t *testing.T) {
	osv := newFakeOSV(t, fixture)
	diff := diffOf(fileDiff("deps.lock.custom", "-pkg jinja2 == 2.11.2 (PyPI)", "+pkg jinja2 == 2.11.3 (PyPI)"))
	model := fakeModel{manifests: []string{"deps.lock.custom"}, changes: []Change{{Manifest: "deps.lock.custom", Ecosystem: "PyPI", Name: "jinja2", Base: "2.11.2", Head: "2.11.3"}}}
	r := run(t, osv, Inputs{Diff: diff, Model: model, Extractor: fakeExtractor{}})
	if r.Inconclusive() || len(findingsOf(r, StatusIntroduced)) != 1 {
		t.Fatalf("unknown format not read by the model: %+v", r)
	}
	if len(r.Divergences) != 0 {
		t.Fatalf("a file the scanner does not know has no divergence: %v", r.Divergences)
	}
	scanner := fakeExtractor{pkgs: map[string][]Package{"deps.lock.custom": {{Ecosystem: "PyPI", Name: "jinja2", Version: "2.11.2"}}}}
	r = run(t, osv, Inputs{Diff: diff, Model: model, Extractor: scanner})
	if len(r.Divergences) != 1 || !strings.Contains(r.Divergences[0], "jinja2 2.11.3") {
		t.Fatalf("divergence not declared: %v", r.Divergences)
	}
}

// AC-005: a range is consulted as the whole package with the model's
// explanation; a side without a resolvable version is inconclusive.
func TestAUR495AC005Range(t *testing.T) {
	osv := newFakeOSV(t, fixture)
	diff := diffOf(fileDiff("Cargo.toml", `+smallvec = "^0.6.10"`))
	change := Change{Manifest: "Cargo.toml", Ecosystem: "crates.io", Name: "smallvec", HeadRange: "^0.6.10", Explanation: "admite 0.6.10 ate antes de 0.7.0"}
	r := run(t, osv, Inputs{Diff: diff, Model: fakeModel{manifests: []string{"Cargo.toml"}, changes: []Change{change}}})
	got := findingsOf(r, StatusIntroduced)
	if r.Inconclusive() || len(got) != 1 || !got[0].ByRange || got[0].Change.Explanation == "" {
		t.Fatalf("range not consulted: %+v", r)
	}
	if asked := osv.asked(); len(asked) != 1 || asked[0] != "crates.io/smallvec" {
		t.Fatalf("queries = %v", asked)
	}
	unresolved := Change{Manifest: "Cargo.toml", Ecosystem: "crates.io", Name: "smallvec", Unresolved: true}
	r = run(t, osv, Inputs{Diff: diff, Model: fakeModel{manifests: []string{"Cargo.toml"}, changes: []Change{unresolved}}})
	if r.Reason != ReasonUnresolved || r.Findings != nil {
		t.Fatalf("unresolved = %q %+v", r.Reason, r.Findings)
	}
}

// AC-006 / MUT-001: an unreachable source, a missing scanner and a stale
// answer are inconclusive with the reason, never "no advisories".
func TestAUR495AC006SourceFailures(t *testing.T) {
	model := fakeModel{manifests: []string{"app/package-lock.json"}, changes: []Change{minimist("1.2.6", "1.2.5")}}
	diff := npmBump("1.2.6", "1.2.5")

	down := newFakeOSV(t, fixture)
	down.status = http.StatusServiceUnavailable
	if r := run(t, down, Inputs{Diff: diff, Model: model}); r.Reason != ReasonSourceFailed || r.Findings != nil {
		t.Fatalf("unreachable source = %q %+v", r.Reason, r.Findings)
	}

	osv := newFakeOSV(t, fixture)
	if r := run(t, osv, Inputs{Diff: diff, Model: model, Extractor: fakeExtractor{missing: true}}); r.Reason != ReasonScannerMissing {
		t.Fatalf("missing scanner = %q", r.Reason)
	}

	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	stale := newFakeOSV(t, fixture)
	stale.date = now.Add(-72 * time.Hour)
	src := stale.source()
	src.MaxAge, src.Now = 24*time.Hour, func() time.Time { return now }
	if r := run(t, stale, Inputs{Diff: diff, Model: model, Source: src}); r.Reason != ReasonSourceStale || r.Findings != nil {
		t.Fatalf("stale source = %q %+v", r.Reason, r.Findings)
	}
	stale.date = now.Add(-time.Hour)
	if r := run(t, stale, Inputs{Diff: diff, Model: model, Source: src}); r.Inconclusive() {
		t.Fatalf("fresh answer inconclusive: %q %s", r.Reason, r.Detail)
	}

	if r := Check(context.Background(), Inputs{Diff: diff, Source: osv.source()}); r.Reason != ReasonNoModel {
		t.Fatalf("no model = %q", r.Reason)
	}
	failing := fakeModel{err: errors.New("provider down")}
	if r := run(t, osv, Inputs{Diff: diff, Model: failing}); r.Reason != ReasonModelFailed {
		t.Fatalf("model failure = %q", r.Reason)
	}
}

// AC-007: each lockfile of a monorepo is reported separately.
func TestAUR495AC007Monorepo(t *testing.T) {
	osv := newFakeOSV(t, fixture)
	diff := diffOf(
		fileDiff("svc-a/package-lock.json", `-      "version": "1.2.6",`, `+      "version": "1.2.5",`, `     "node_modules/minimist": {`),
		fileDiff("svc-b/package-lock.json", `-      "version": "1.2.6",`, `+      "version": "1.2.0",`, `     "node_modules/minimist": {`),
	)
	a, b := minimist("1.2.6", "1.2.5"), minimist("1.2.6", "1.2.0")
	a.Manifest, b.Manifest = "svc-a/package-lock.json", "svc-b/package-lock.json"
	r := run(t, osv, Inputs{Diff: diff, Model: fakeModel{manifests: []string{"svc-a/package-lock.json", "svc-b/package-lock.json"}, changes: []Change{a, b}}})
	got := findingsOf(r, StatusIntroduced)
	if len(got) != 2 || got[0].Change.Manifest == got[1].Change.Manifest {
		t.Fatalf("monorepo findings not separated: %+v", r.Findings)
	}
	if len(r.Manifests) != 2 {
		t.Fatalf("manifests = %v", r.Manifests)
	}
}

// AC-008: a package or version the model cites and the diff does not hold
// is discarded; a path that is not a changed file is never a manifest.
func TestAUR495AC008InventedDependencyDiscarded(t *testing.T) {
	osv := newFakeOSV(t, fixture)
	invented := Change{Manifest: "app/package-lock.json", Ecosystem: "npm", Name: "left-pad", Head: "1.3.0"}
	wrongVersion := minimist("1.2.6", "1.2.0")
	model := fakeModel{manifests: []string{"app/package-lock.json", "go.sum"}, changes: []Change{invented, wrongVersion, minimist("1.2.6", "1.2.5")}}
	r := run(t, osv, Inputs{Diff: npmBump("1.2.6", "1.2.5"), Model: model})
	if len(r.Discarded) != 2 {
		t.Fatalf("discarded = %v", r.Discarded)
	}
	if len(r.Changes) != 1 || r.Changes[0].Head != "1.2.5" {
		t.Fatalf("changes = %+v", r.Changes)
	}
	if len(r.Manifests) != 1 || r.Manifests[0] != "app/package-lock.json" {
		t.Fatalf("manifests = %v", r.Manifests)
	}
	for _, q := range osv.asked() {
		if strings.Contains(q, "left-pad") || strings.HasSuffix(q, "@1.2.0") {
			t.Fatalf("invented dependency was consulted: %v", osv.asked())
		}
	}
}
