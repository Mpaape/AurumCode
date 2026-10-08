package dependencies

import (
	"net/http"
	"strings"
	"testing"
)

// typosquat is a new npm package one letter away from a popular one,
// published days ago with an install script.
func typosquat() (*Inputs, fakeRegistry) {
	diff := diffOf(fileDiff("web/package.json", `+    "lodahs": "1.0.1",`))
	change := Change{Manifest: "web/package.json", Ecosystem: "npm", LicenseSystem: "npm", Name: "lodahs", Head: "1.0.1"}
	reg := fakeRegistry{meta: map[string]Metadata{"lodahs": {
		"version": "1.0.1", "first_published_at": "2026-10-03T10:00:00Z", "versions_count": "1",
		"scripts.install": "node install.js", "links.SOURCE_REPO": "",
	}}}
	return &Inputs{Diff: diff, Model: fakeModel{manifests: []string{"web/package.json"}, changes: []Change{change}}, Registry: reg}, reg
}

// AUR-528 AC-003: an unreachable advisory source is inconclusive and never
// clears a new package; an unreachable registry too.
func TestAUR528AC003UnreachableNeverClears(t *testing.T) {
	in, reg := typosquat()
	down := newFakeOSV(t, fixture)
	down.status = http.StatusBadGateway
	if r := run(t, down, *in); r.Reason != ReasonSourceFailed || r.Findings != nil || r.Suspicions != nil {
		t.Fatalf("unreachable source = %q %+v", r.Reason, r)
	}
	reg.fail = true
	in.Registry = reg
	if r := run(t, newFakeOSV(t, fixture), *in); r.Reason != ReasonMetadataFailed {
		t.Fatalf("unreachable registry = %q", r.Reason)
	}
}

// AUR-528 AC-004: the model points out the suspicion with the evidence of
// the live metadata.
func TestAUR528AC004GroundedSuspicion(t *testing.T) {
	in, _ := typosquat()
	model := in.Model.(fakeModel)
	model.suspicions = []map[string]any{{
		"manifest": "web/package.json", "name": "lodahs", "summary": "nome a uma letra de lodash, publicado ha dias, com script de instalacao",
		"evidence": []map[string]string{{"field": "first_published_at", "value": "2026-10-03T10:00:00Z"}, {"field": "scripts.install", "value": "node install.js"}},
	}}
	in.Model = model
	r := run(t, newFakeOSV(t, fixture), *in)
	if r.Inconclusive() || len(r.Suspicions) != 1 {
		t.Fatalf("suspicion not kept: %+v", r)
	}
	s := r.Suspicions[0]
	if s.Change.Name != "lodahs" || s.Change.Head != "1.0.1" || len(s.Evidence) != 2 || !strings.Contains(s.Summary, "lodash") {
		t.Fatalf("suspicion = %+v", s)
	}
}

// AUR-528 AC-005: a suspicion without evidence from the consulted metadata
// is discarded.
func TestAUR528AC005UngroundedSuspicionDiscarded(t *testing.T) {
	in, _ := typosquat()
	model := in.Model.(fakeModel)
	model.suspicions = []map[string]any{
		{"manifest": "web/package.json", "name": "lodahs", "summary": "autor novo", "evidence": []map[string]string{{"field": "author_created_at", "value": "2026-10-01"}}},
		{"manifest": "web/package.json", "name": "lodahs", "summary": "parece estranho", "evidence": []map[string]string{}},
		{"manifest": "web/package.json", "name": "lodahs", "summary": "versao", "evidence": []map[string]string{{"field": "versions_count", "value": "2"}}},
		{"manifest": "web/package.json", "name": "react", "summary": "outro", "evidence": []map[string]string{{"field": "versions_count", "value": "1"}}},
	}
	in.Model = model
	r := run(t, newFakeOSV(t, fixture), *in)
	if len(r.Suspicions) != 0 || len(r.Discarded) != 4 {
		t.Fatalf("ungrounded suspicions kept: %+v discarded=%v", r.Suspicions, r.Discarded)
	}
}
