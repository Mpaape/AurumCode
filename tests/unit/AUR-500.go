package unit

// Unit program for card AUR-500, selector TestAUR500.
//
// Proves internal/reviewprofile's core contracts in isolation -- no CLI, no
// git, no model:
//   - AC-001: a named profile is selected by repository config or review flag,
//     the flag wins, and the resolution DECLARES which profile entered;
//   - AC-002: every built-in carries a version, and the same input yields the
//     same emphasis, enabled families and instructions;
//   - AC-003: switching profiles changes ONLY emphasis/families. Severity,
//     --fail-on, redaction, the cost cap and the deterministic security pass
//     are fixed; a definition naming any of them is REFUSED naming the clause;
//   - AC-004: an unknown profile is a named error before any model call; an
//     absent profile is the zero-config no-op.

import (
	"errors"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/reviewprofile"
)

func TestAUR500(t *testing.T) {
	t.Run("NamedProfileAppliesAndDeclares", testAUR500NamedProfileAppliesAndDeclares)
	t.Run("ConfigSelectedProfileApplies", testAUR500ConfigSelectedProfileApplies)
	t.Run("BuiltinsAreVersionedAndDeterministic", testAUR500BuiltinsAreVersionedAndDeterministic)
	t.Run("SwitchingChangesOnlyEmphasisAndFamilies", testAUR500SwitchingChangesOnlyEmphasisAndFamilies)
	t.Run("WeakeningDefinitionIsRefusedNamingClause", testAUR500WeakeningIsRefused)
	t.Run("UnknownProfileIsNamedError", testAUR500UnknownProfileIsNamedError)
	t.Run("AbsentProfileIsZeroConfig", testAUR500AbsentProfileIsZeroConfig)
}

func resolve(t *testing.T, sel reviewprofile.Selection) *reviewprofile.Result {
	t.Helper()
	res, err := reviewprofile.Resolve(sel)
	if err != nil {
		t.Fatalf("Resolve(%+v): %v", sel, err)
	}
	return res
}

func testAUR500NamedProfileAppliesAndDeclares(t *testing.T) {
	res := resolve(t, reviewprofile.Selection{Flag: "seguranca"})
	if !res.Applied {
		t.Fatal("flag-selected profile did not apply")
	}
	if res.Profile.Name != "seguranca" {
		t.Fatalf("selected profile = %q, want seguranca", res.Profile.Name)
	}
	if !strings.Contains(res.Declared, "seguranca") {
		t.Fatalf("declaration %q does not name the entered profile", res.Declared)
	}
	if len(res.Effective.Families) == 0 {
		t.Fatal("profile enabled no rule families")
	}
}

func testAUR500ConfigSelectedProfileApplies(t *testing.T) {
	res := resolve(t, reviewprofile.Selection{Config: "performance"})
	if !res.Applied || res.Profile.Name != "performance" {
		t.Fatalf("config-selected profile = %+v", res)
	}
	// The review flag overrides repository config.
	over := resolve(t, reviewprofile.Selection{Config: "performance", Flag: "solid"})
	if over.Profile.Name != "solid" {
		t.Fatalf("flag did not override config: %q", over.Profile.Name)
	}
}

func testAUR500BuiltinsAreVersionedAndDeterministic(t *testing.T) {
	names := reviewprofile.Names()
	if len(names) != 3 {
		t.Fatalf("built-ins = %v, want solid, seguranca, performance", names)
	}
	for _, name := range names {
		first := resolve(t, reviewprofile.Selection{Flag: name})
		second := resolve(t, reviewprofile.Selection{Flag: name})
		if first.Profile.Version == "" {
			t.Fatalf("profile %q has no version identifier", name)
		}
		if first.Profile.Signature() != second.Profile.Signature() {
			t.Fatalf("profile %q is not deterministic:\n%s\n%s", name, first.Profile.Signature(), second.Profile.Signature())
		}
		if strings.Join(familiesOf(first.Profile), ",") != strings.Join(familiesOf(second.Profile), ",") {
			t.Fatalf("profile %q families are not deterministic", name)
		}
	}
}

func familiesOf(p reviewprofile.Profile) []string {
	out := make([]string, 0, len(p.Families()))
	for _, f := range p.Families() {
		out = append(out, string(f))
	}
	return out
}

func testAUR500SwitchingChangesOnlyEmphasisAndFamilies(t *testing.T) {
	a := resolve(t, reviewprofile.Selection{Flag: "solid"})
	b := resolve(t, reviewprofile.Selection{Flag: "seguranca"})
	if a.Effective.Emphasis == b.Effective.Emphasis {
		t.Fatal("switching profiles did not change emphasis")
	}
	if strings.Join(familiesOf(a.Profile), ",") == strings.Join(familiesOf(b.Profile), ",") {
		t.Fatal("switching profiles did not change enabled families")
	}
	for _, res := range []*reviewprofile.Result{a, b} {
		e := res.Effective
		if !e.SecurityPassEnabled {
			t.Fatalf("%s disabled the deterministic security pass", res.Profile.Name)
		}
		if !e.RedactionEnabled {
			t.Fatalf("%s disabled secret redaction", res.Profile.Name)
		}
		if e.SeverityFloor != "" {
			t.Fatalf("%s changed severity to %q", res.Profile.Name, e.SeverityFloor)
		}
		if e.FailOnThreshold != "" {
			t.Fatalf("%s changed --fail-on to %q", res.Profile.Name, e.FailOnThreshold)
		}
		if e.CostCap != -1 {
			t.Fatalf("%s changed the cost cap to %d", res.Profile.Name, e.CostCap)
		}
	}
}

func testAUR500WeakeningIsRefused(t *testing.T) {
	cases := []struct {
		name   string
		doc    string
		clause string
	}{
		{"severity", "name: x\nemphasis: e\nseverity: info\n", "severity"},
		{"fail-on", "name: x\nemphasis: e\nfail_on: error\n", "fail_on"},
		{"cost", "name: x\nemphasis: e\ncost_cap: 100000\n", "cost_cap"},
		{"redaction", "name: x\nemphasis: e\nredact_secrets: false\n", "redact_secrets"},
		{"security-pass", "name: x\nemphasis: e\nsecurity_pass: false\n", "security_pass"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := reviewprofile.Compile([]byte(tc.doc))
			if err == nil {
				t.Fatalf("definition naming %q was accepted", tc.clause)
			}
			if !errors.Is(err, reviewprofile.ErrRefusedClause) {
				t.Fatalf("error %v is not a refused-clause error", err)
			}
			if !strings.Contains(err.Error(), tc.clause) {
				t.Fatalf("refusal %q does not name the clause %q", err.Error(), tc.clause)
			}
		})
	}

	// A legitimate definition (emphasis + families only) compiles.
	p, err := reviewprofile.Compile([]byte("name: custom\nversion: 7\nemphasis: custom focus\nfamilies: [quality]\n"))
	if err != nil {
		t.Fatalf("legitimate definition refused: %v", err)
	}
	if p.Version != "7" || p.Effective.Emphasis != "custom focus" {
		t.Fatalf("compiled profile = %+v", p)
	}
	if !p.Effective.SecurityPassEnabled || !p.Effective.RedactionEnabled {
		t.Fatal("compiled profile weakened a boundary")
	}

	// An unknown family is its own named error, never silently accepted.
	_, err = reviewprofile.Compile([]byte("name: custom\nfamilies: [bogus]\n"))
	if !errors.Is(err, reviewprofile.ErrUnknownFamily) {
		t.Fatalf("unknown family error = %v", err)
	}
}

func testAUR500UnknownProfileIsNamedError(t *testing.T) {
	_, err := reviewprofile.Resolve(reviewprofile.Selection{Flag: "nao-existe"})
	if err == nil {
		t.Fatal("unknown profile resolved successfully")
	}
	if !errors.Is(err, reviewprofile.ErrUnknownProfile) {
		t.Fatalf("error %v is not unknown-profile", err)
	}
	if !strings.Contains(err.Error(), "nao-existe") {
		t.Fatalf("error %q does not name the unknown profile", err.Error())
	}
}

func testAUR500AbsentProfileIsZeroConfig(t *testing.T) {
	res := resolve(t, reviewprofile.Selection{})
	if res.Applied {
		t.Fatal("empty selection applied a profile")
	}
	if res.Profile.Name != "" {
		t.Fatalf("zero-config resolved profile %q", res.Profile.Name)
	}
	if res.Effective.Families != nil && len(res.Effective.Families) != 0 {
		t.Fatalf("zero-config enabled families: %v", res.Effective.Families)
	}
}
