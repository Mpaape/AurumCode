package rounds

import (
	"strings"
	"testing"
)

const sha1 = "0123456789abcdef0123456789abcdef01234567"

func TestRoundMarkerRoundTrip(t *testing.T) {
	marker := RoundMarker(Round{SHA: strings.ToUpper(sha1), Blocking: []string{fpB, fpA, fpA, "ruim"}})
	got, ok := ParseRoundMarker("parecer\n\n" + marker)
	if !ok || got.SHA != sha1 || len(got.Blocking) != 2 || got.Blocking[0] != fpA || got.Blocking[1] != fpB {
		t.Fatalf("round trip = %+v %v from %q", got, ok, marker)
	}
	if RoundMarker(Round{SHA: "head-sha"}) != "" {
		t.Fatal("a non-SHA head produced a marker")
	}
	if _, ok := ParseRoundMarker("sem marcador"); ok {
		t.Fatal("a body without a marker was read as a round")
	}
}

func TestNewChangesRefusesUntrustedComparison(t *testing.T) {
	for _, status := range []string{"diverged", "behind", ""} {
		if _, ok := NewChanges(status, nil); ok {
			t.Fatalf("status %q trusted", status)
		}
	}
	if _, ok := NewChanges("ahead", make([]ChangedFile, MaxComparedFiles)); ok {
		t.Fatal("a possibly truncated comparison was trusted")
	}
}

func TestLateFindings(t *testing.T) {
	patch := "@@ -10,3 +10,4 @@ func x() {\n ctx\n-velha\n+nova\n+outra\n ctx\n"
	changes, ok := NewChanges("ahead", []ChangedFile{{Path: "a.go", Patch: patch}, {Path: "bin.dat"}})
	if !ok {
		t.Fatal("ahead comparison refused")
	}
	if !changes.LineChanged("a.go", 11) || !changes.LineChanged("a.go", 12) || changes.LineChanged("a.go", 10) || changes.LineChanged("a.go", 13) {
		t.Fatal("added lines misread")
	}
	cases := []struct {
		name    string
		finding Finding
		earlier bool
		late    bool
	}{
		{"novo em linha inalterada vira observação", Finding{Fingerprint: fpA, Path: "Dockerfile", Line: 26, Blocking: true}, false, true},
		{"novo em linha não alterada de arquivo alterado", Finding{Fingerprint: fpA, Path: "a.go", Line: 10, Blocking: true}, false, true},
		{"segurança continua bloqueando", Finding{Fingerprint: fpA, Path: "Dockerfile", Line: 26, Blocking: true, Exempt: true}, false, false},
		{"repetido não resolvido continua bloqueando", Finding{Fingerprint: fpA, Path: "Dockerfile", Line: 26, Blocking: true}, true, false},
		{"linha alterada é tratada normalmente", Finding{Fingerprint: fpA, Path: "a.go", Line: 11, Blocking: true}, false, false},
		{"arquivo sem patch conta como alterado", Finding{Fingerprint: fpA, Path: "bin.dat", Line: 3, Blocking: true}, false, false},
		{"lado LEFT é tratado normalmente", Finding{Fingerprint: fpA, Path: "Dockerfile", Line: 26, Blocking: true, Left: true}, false, false},
		{"achado de arquivo inteiro é tratado normalmente", Finding{Fingerprint: fpA, Path: "Dockerfile", Blocking: true}, false, false},
		{"sem identidade é tratado normalmente", Finding{Path: "Dockerfile", Line: 26, Blocking: true}, false, false},
		{"observação continua observação", Finding{Fingerprint: fpA, Path: "Dockerfile", Line: 26}, false, false},
	}
	for _, c := range cases {
		earlier := map[string]bool{}
		if c.earlier {
			earlier[fpA] = true
		}
		if got := LateFindings([]Finding{c.finding}, earlier, changes)[0]; got != c.late {
			t.Errorf("%s: late = %v, want %v", c.name, got, c.late)
		}
	}
}
