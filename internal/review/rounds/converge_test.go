package rounds

import (
	"strings"
	"testing"
)

const sha1 = "0123456789abcdef0123456789abcdef01234567"

func TestRoundMarkerRoundTrip(t *testing.T) {
	marker := RoundMarker(Round{SHA: strings.ToUpper(sha1), Blocking: []string{fpB, fpA, fpA, "ruim"}, Locations: []Location{{Path: "dir/a b,c.go", Line: 40}}})
	got, ok := ParseRoundMarker("parecer\n\n" + marker)
	if !ok || got.SHA != sha1 || len(got.Blocking) != 2 || got.Blocking[0] != fpA || got.Blocking[1] != fpB {
		t.Fatalf("round trip = %+v %v from %q", got, ok, marker)
	}
	if !got.HasLocations || len(got.Locations) != 1 || got.Locations[0] != (Location{Path: "dir/a b,c.go", Line: 40}) {
		t.Fatalf("locations = %+v from %q", got.Locations, marker)
	}
	old, ok := ParseRoundMarker("<!-- aurumcode:round " + sha1 + " " + fpA + " -->")
	if !ok || old.HasLocations {
		t.Fatalf("a record without locations = %+v", old)
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
	removal := "@@ -20,4 +20,2 @@\n ctx\n-guarda\n-retorno\n ctx\n"
	changes, ok := NewChanges("ahead", []ChangedFile{{Path: "a.go", Patch: patch}, {Path: "bin.dat"}, {Path: "pkg/b.go", Patch: removal}})
	if !ok {
		t.Fatal("ahead comparison refused")
	}
	if !changes.LineChanged("a.go", 11) || !changes.LineChanged("a.go", 12) || changes.LineChanged("a.go", 9) || changes.LineChanged("a.go", 14) {
		t.Fatal("added lines misread")
	}
	if !changes.LineChanged("pkg/b.go", 20) || !changes.LineChanged("pkg/b.go", 21) || !changes.LineChanged("pkg/b.go", 22) || changes.LineChanged("pkg/b.go", 25) {
		t.Fatal("a pure removal does not mark the lines next to it")
	}
	earlierRound := Round{SHA: sha1, HasLocations: true, Locations: []Location{{Path: "app.py", Line: 40}}}
	cases := []struct {
		name    string
		finding Finding
		earlier bool
		round   Round
		late    bool
	}{
		{"novo em linha inalterada vira observação", Finding{Fingerprint: fpA, Path: "Dockerfile", Line: 26, Blocking: true}, false, earlierRound, true},
		{"novo em linha inalterada de arquivo alterado", Finding{Fingerprint: fpA, Path: "a.go", Line: 30, Blocking: true}, false, earlierRound, true},
		{"segurança continua bloqueando", Finding{Fingerprint: fpA, Path: "Dockerfile", Line: 26, Blocking: true, Exempt: true}, false, earlierRound, false},
		{"repetido não resolvido continua bloqueando", Finding{Fingerprint: fpA, Path: "Dockerfile", Line: 26, Blocking: true}, true, earlierRound, false},
		{"bloqueio anterior renomeado ou reancorado continua bloqueando", Finding{Fingerprint: fpB, Path: "app.py", Line: 39, Blocking: true}, false, earlierRound, false},
		{"a mais de 5 linhas do bloqueio anterior", Finding{Fingerprint: fpB, Path: "app.py", Line: 46, Blocking: true}, false, earlierRound, true},
		{"registro antigo sem localizações não rebaixa", Finding{Fingerprint: fpA, Path: "Dockerfile", Line: 26, Blocking: true}, false, Round{SHA: sha1}, false},
		{"linha alterada é tratada normalmente", Finding{Fingerprint: fpA, Path: "a.go", Line: 11, Blocking: true}, false, earlierRound, false},
		{"remoção pura ao lado conta como alteração", Finding{Fingerprint: fpA, Path: "pkg/b.go", Line: 21, Blocking: true}, false, earlierRound, false},
		{"agrupado com referência alterada", Finding{Fingerprint: fpA, Path: "Dockerfile", Line: 26, Blocking: true, Refs: Refs("falta teste: Dockerfile:26, a.go:12")}, false, earlierRound, false},
		{"agrupado sem referência alterada", Finding{Fingerprint: fpA, Path: "Dockerfile", Line: 26, Blocking: true, Refs: Refs("Dockerfile:26 e a.go:30")}, false, earlierRound, true},
		{"arquivo sem patch conta como alterado", Finding{Fingerprint: fpA, Path: "bin.dat", Line: 3, Blocking: true}, false, earlierRound, false},
		{"lado LEFT é tratado normalmente", Finding{Fingerprint: fpA, Path: "Dockerfile", Line: 26, Blocking: true, Left: true}, false, earlierRound, false},
		{"achado de arquivo inteiro é tratado normalmente", Finding{Fingerprint: fpA, Path: "Dockerfile", Blocking: true}, false, earlierRound, false},
		{"sem identidade é tratado normalmente", Finding{Path: "Dockerfile", Line: 26, Blocking: true}, false, earlierRound, false},
		{"observação continua observação", Finding{Fingerprint: fpA, Path: "Dockerfile", Line: 26}, false, earlierRound, false},
	}
	for _, c := range cases {
		earlier := map[string]bool{}
		if c.earlier {
			earlier[fpA] = true
		}
		if got := LateFindings([]Finding{c.finding}, earlier, c.round, changes)[0]; got != c.late {
			t.Errorf("%s: late = %v, want %v", c.name, got, c.late)
		}
	}
}
