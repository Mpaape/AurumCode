package analysis_test

import (
	"testing"
	"time"

	"github.com/Mpaape/AurumCode/internal/analysis"
	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/gate"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// aur609GateResult runs the analysis source of the gate over a one-line
// added diff with fail_on: [error], exactly as the review does.
func aur609GateResult(t *testing.T, path, added string) gate.Result {
	t.Helper()
	diff := &types.Diff{Files: []types.DiffFile{{
		Path:  path,
		Hunks: []types.DiffHunk{{OldStart: 1, NewStart: 1, Lines: []string{"+" + added}}},
	}}}
	var res gate.Result
	issues := gate.AnalysisIssuesForGate(diff, &config.Config{})
	if err := gate.ApplyAnalysisGate(&res, config.GateConfig{FailOn: []string{"error"}}, issues, nil, "", time.Now()); err != nil {
		t.Fatalf("ApplyAnalysisGate: %v", err)
	}
	return res
}

// AC-005: the gate does not fail the docstring diff; the real call under the
// same configuration still fails it, so the gate is not simply inert.
func TestAUR609GateDoesNotFailOnDocstring(t *testing.T) {
	doc := aur609GateResult(t, "tool.py", `    """Wrapper seguro; nunca faça subprocess.call("rm " + name)."""`)
	if doc.Fail || len(doc.BlockingFindings) != 0 {
		t.Fatalf("the docstring diff failed the gate: %+v", doc)
	}
	real := aur609GateResult(t, "tool.py", `    subprocess.call("rm " + name)`)
	if !real.Fail || len(real.BlockingFindings) != 1 || real.BlockingFindings[0].RuleID != analysis.RuleCommandInjection {
		t.Fatalf("the real call did not fail the gate with %s: %+v", analysis.RuleCommandInjection, real)
	}
}
