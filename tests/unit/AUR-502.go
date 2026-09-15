package unit

// Unit program for card AUR-502, selector TestAUR502.
//
// Proves internal/reviewprofile's TEAM profiles and MULTI-AGENT merge in
// isolation: a versioned team file declares named presets; missing, empty,
// duplicate and unknown names fail high naming the problem; two or more
// profiles run in one review and their findings merge deterministically with
// per-profile attribution; no profile can move a safety boundary; and the
// zero-config/single-profile path is unchanged.

import (
	"errors"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/reviewprofile"
)

func TestAUR502(t *testing.T) {
	t.Run("TeamFileDeclaresNamedProfiles", testAUR502TeamFileDeclaresNamedProfiles)
	t.Run("MissingNameFailsHigh", testAUR502MissingNameFailsHigh)
	t.Run("EmptyProfileFailsHigh", testAUR502EmptyProfileFailsHigh)
	t.Run("DuplicateNameFailsHigh", testAUR502DuplicateNameFailsHigh)
	t.Run("UnknownNameFailsHigh", testAUR502UnknownNameFailsHigh)
	t.Run("MultiProfileDeclaresEveryProfile", testAUR502MultiDeclares)
	t.Run("MergeIsDeterministicAndAttributed", testAUR502MergeDeterministic)
	t.Run("NoProfileMovesASafetyBoundary", testAUR502BoundariesFixed)
	t.Run("TeamClauseIsRefusedNamingIt", testAUR502ClauseRefused)
	t.Run("ZeroConfigAndSingleProfileUnchanged", testAUR502Compat)
	t.Run("ProductOwnerIsBuiltIn", testAUR502ProductOwner)
}

const teamDoc = `
profiles:
  - name: release
    version: "2"
    emphasis: upstream compatibility
    families: [quality, performance]
    instructions: Confira compatibilidade de contrato e custo de rollout.
  - name: cliente_x
    emphasis: client-specific rules
    families: [security]
    instructions: Confira os requisitos do cliente.
`

func mustTeam(t *testing.T) *reviewprofile.Team {
	t.Helper()
	team, err := reviewprofile.LoadTeam([]byte(teamDoc))
	if err != nil {
		t.Fatalf("LoadTeam: %v", err)
	}
	return team
}

func testAUR502TeamFileDeclaresNamedProfiles(t *testing.T) {
	team := mustTeam(t)
	names := team.Names()
	if len(names) != 2 || names[0] != "release" || names[1] != "cliente_x" {
		t.Fatalf("team names = %v", names)
	}
	p, ok := team.Lookup("RELEASE")
	if !ok || p.Version != "2" || p.Effective.Emphasis != "upstream compatibility" {
		t.Fatalf("lookup release = %+v ok=%v", p, ok)
	}
}

func testAUR502MissingNameFailsHigh(t *testing.T) {
	_, err := reviewprofile.LoadTeam([]byte("profiles:\n  - emphasis: e\n    families: [quality]\n    instructions: i\n"))
	if !errors.Is(err, reviewprofile.ErrMissingName) {
		t.Fatalf("missing name error = %v", err)
	}
}

func testAUR502EmptyProfileFailsHigh(t *testing.T) {
	_, err := reviewprofile.LoadTeam([]byte("profiles:\n  - name: x\n    families: [quality]\n"))
	if !errors.Is(err, reviewprofile.ErrEmptyProfile) {
		t.Fatalf("empty profile error = %v", err)
	}
	if _, err := reviewprofile.LoadTeam([]byte("profiles: []\n")); !errors.Is(err, reviewprofile.ErrEmptyProfile) {
		t.Fatalf("empty file error = %v", err)
	}
}

func testAUR502DuplicateNameFailsHigh(t *testing.T) {
	dup := "profiles:\n  - name: x\n    emphasis: a\n    families: [quality]\n    instructions: i\n  - name: X\n    emphasis: b\n    families: [security]\n    instructions: j\n"
	if _, err := reviewprofile.LoadTeam([]byte(dup)); !errors.Is(err, reviewprofile.ErrDuplicateProfile) {
		t.Fatalf("duplicate error = %v", err)
	}
	shadow := "profiles:\n  - name: solid\n    emphasis: a\n    families: [quality]\n    instructions: i\n"
	if _, err := reviewprofile.LoadTeam([]byte(shadow)); !errors.Is(err, reviewprofile.ErrDuplicateProfile) {
		t.Fatalf("built-in shadow error = %v", err)
	}
}

func testAUR502UnknownNameFailsHigh(t *testing.T) {
	_, err := reviewprofile.ResolveAll(reviewprofile.Selection{Names: []string{"release", "nao-existe"}}, mustTeam(t))
	if !errors.Is(err, reviewprofile.ErrUnknownProfile) || !strings.Contains(err.Error(), "nao-existe") {
		t.Fatalf("unknown selection error = %v", err)
	}
}

func testAUR502MultiDeclares(t *testing.T) {
	res, err := reviewprofile.ResolveAll(reviewprofile.Selection{Names: []string{"solid", "release"}}, mustTeam(t))
	if err != nil {
		t.Fatalf("ResolveAll: %v", err)
	}
	if !res.Applied || len(res.Profiles) != 2 {
		t.Fatalf("multi result = %+v", res)
	}
	for _, name := range []string{"solid", "release"} {
		if !strings.Contains(res.Declared, name) {
			t.Fatalf("declaration %q does not name %s", res.Declared, name)
		}
	}
	if _, err := reviewprofile.ResolveAll(reviewprofile.Selection{Names: []string{"solid", "solid"}}, mustTeam(t)); !errors.Is(err, reviewprofile.ErrDuplicateProfile) {
		t.Fatalf("duplicate selection error = %v", err)
	}
}

func testAUR502MergeDeterministic(t *testing.T) {
	shared := reviewprofile.Finding{RuleID: "quality/nit", File: "a.go", Line: 10, Message: "same", Severity: "info"}
	onlySec := reviewprofile.Finding{RuleID: "security/x", File: "b.go", Line: 3, Message: "sec", Severity: "error"}
	in := []reviewprofile.Finding{
		withProfile(shared, "solid"),
		withProfile(onlySec, "seguranca"),
		withProfile(shared, "seguranca"),
	}
	got := reviewprofile.MergeFindings(in)
	if len(got) != 2 {
		t.Fatalf("merged %d findings, want 2: %+v", len(got), got)
	}
	seen := map[string]string{}
	for _, f := range got {
		seen[f.RuleID] = f.Profile
	}
	if seen["quality/nit"] != "solid" {
		t.Fatalf("shared finding attributed to %q, want the earlier profile solid", seen["quality/nit"])
	}
	if seen["security/x"] != "seguranca" {
		t.Fatalf("security finding attributed to %q", seen["security/x"])
	}
	if got[0].File != "a.go" || got[1].File != "b.go" {
		t.Fatalf("merge order not stable: %+v", got)
	}
	// Same input twice, same bytes.
	again := reviewprofile.MergeFindings(in)
	if len(again) != len(got) || again[0].Profile != got[0].Profile || again[1].Profile != got[1].Profile {
		t.Fatalf("merge is not deterministic: %+v vs %+v", got, again)
	}
}

func withProfile(f reviewprofile.Finding, name string) reviewprofile.Finding {
	f.Profile = name
	return f
}

func testAUR502BoundariesFixed(t *testing.T) {
	res, err := reviewprofile.ResolveAll(reviewprofile.Selection{Names: []string{"solid", "release", "cliente_x"}}, mustTeam(t))
	if err != nil {
		t.Fatalf("ResolveAll: %v", err)
	}
	if !res.BoundariesFixed() {
		t.Fatalf("a profile moved a boundary: %+v", res.Profiles)
	}
	for _, p := range res.Profiles {
		e := p.Effective
		if !e.SecurityPassEnabled || !e.RedactionEnabled || e.CostCap != -1 || e.SeverityFloor != "" || e.FailOnThreshold != "" {
			t.Fatalf("profile %s changed a boundary: %+v", p.Name, e)
		}
	}
}

func testAUR502ClauseRefused(t *testing.T) {
	cases := []struct {
		name   string
		doc    string
		clause string
	}{
		{"security-pass", "profiles:\n  - name: evil\n    emphasis: e\n    families: [quality]\n    instructions: i\n    security_pass: false\n", "security_pass"},
		{"redact", "profiles:\n  - name: evil\n    emphasis: e\n    families: [quality]\n    instructions: i\n    redact_secrets: false\n", "redact_secrets"},
		{"fail-on", "profiles:\n  - name: evil\n    emphasis: e\n    families: [quality]\n    instructions: i\n    fail_on: error\n", "fail_on"},
		{"severity", "profiles:\n  - name: evil\n    emphasis: e\n    families: [quality]\n    instructions: i\n    severity: info\n", "severity"},
		{"cost", "profiles:\n  - name: evil\n    emphasis: e\n    families: [quality]\n    instructions: i\n    cost_cap: 0\n", "cost_cap"},
		{"merge-key", "profiles:\n  - <<: &m {security_pass: false}\n    name: evil\n    emphasis: e\n    families: [quality]\n    instructions: i\n", "security_pass"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := reviewprofile.LoadTeam([]byte(tc.doc))
			if err == nil {
				t.Fatalf("team clause %q was accepted", tc.clause)
			}
			if !errors.Is(err, reviewprofile.ErrRefusedClause) || !strings.Contains(err.Error(), tc.clause) {
				t.Fatalf("refusal %v does not name %q", err, tc.clause)
			}
		})
	}
}

func testAUR502Compat(t *testing.T) {
	zero, err := reviewprofile.ResolveAll(reviewprofile.Selection{}, reviewprofile.EmptyTeam())
	if err != nil {
		t.Fatalf("zero: %v", err)
	}
	if zero.Applied {
		t.Fatal("zero-config applied a profile")
	}
	single, err := reviewprofile.ResolveAll(reviewprofile.Selection{Names: []string{"seguranca"}}, reviewprofile.EmptyTeam())
	if err != nil {
		t.Fatalf("single: %v", err)
	}
	legacy, err := reviewprofile.Resolve(reviewprofile.Selection{Flag: "seguranca"})
	if err != nil {
		t.Fatalf("legacy: %v", err)
	}
	if !single.Applied || single.Profiles[0].Signature() != legacy.Profile.Signature() {
		t.Fatalf("single-profile result diverged from AUR-500: %+v vs %+v", single, legacy)
	}
}

func testAUR502ProductOwner(t *testing.T) {
	p, ok := reviewprofile.Builtin("product_owner")
	if !ok {
		t.Fatal("product_owner is not a built-in")
	}
	if p.Effective.Emphasis == "" || !p.Effective.SecurityPassEnabled || !p.Effective.RedactionEnabled {
		t.Fatalf("product_owner = %+v", p.Effective)
	}
}
