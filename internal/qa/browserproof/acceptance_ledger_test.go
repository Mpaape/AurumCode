package browserproof_test

import (
	"context"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/qa/browserproof"
)

// ledgerMarker prefixes the per-variant observation line that
// tests/acceptance/AUR-423.sh reads back. The acceptance program does not trust
// a recorded verdict: it re-executes this run and checks the codes itself, so
// the line has to carry what a verdict is made of and nothing else.
const ledgerMarker = "AUR-423/AC-001/observation|"

// TestAUR423AcceptanceLedger drives every AC-001 variant and emits what each one
// observed. It asserts only what no variant may ever do — carry markup into the
// evidence, or come back without a verdict class — because deciding which code
// each variant deserves is the acceptance program's job, not this test's.
func TestAUR423AcceptanceLedger(t *testing.T) {
	for _, variant := range []string{"nominal", "missing-symbol-page", "unlinked-symbol-page", "empty-render"} {
		result, runErr := browserproof.New(browserproof.NewScriptedDriver()).
			Run(context.Background(), scriptedRequest(fixtureDir(t, variant)))

		if result.Outcome == "" {
			t.Fatalf("variant %q produced no verdict class: %v", variant, runErr)
		}

		route, selector, assertion, observed := "", "", "", ""
		if len(result.Routes) > 0 {
			row := result.Routes[0]
			route, selector, assertion = row.Route, row.Selector, row.Assertion
			observed = strings.ReplaceAll(row.ObservedText, "|", " ")
		}
		if strings.ContainsAny(observed, "<>") {
			t.Fatalf("variant %q carried markup into the evidence: %q", variant, observed)
		}

		t.Logf("%s%s|%s|%v|%s|%s|%s|%s|%s|%s",
			ledgerMarker, variant, result.Outcome, result.Proved, result.Code,
			result.ArtifactDigest, route, selector, assertion, observed)
	}
}
