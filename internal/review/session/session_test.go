package session

import (
	"reflect"
	"testing"

	"github.com/Mpaape/AurumCode/internal/gate"
)

// recorder is a source that records the phases it ran.
type recorder struct {
	ran    []Phase
	stopAt Phase
}

func (r *recorder) Step(p Phase) Step {
	return func() (int, bool) {
		r.ran = append(r.ran, p)
		if p == r.stopAt {
			return 7, true
		}
		return 0, false
	}
}

// TestRunFollowsTheOneOrder: every phase runs once, in Order, and an early
// exit keeps its code and skips the rest.
func TestRunFollowsTheOneOrder(t *testing.T) {
	all := &recorder{}
	if code := Run(all); code != 0 || !reflect.DeepEqual(all.ran, Order) {
		t.Fatalf("Run = %d ran %v, want 0 and %v", code, all.ran, Order)
	}
	early := &recorder{stopAt: PhaseEvidence}
	if code := Run(early); code != 7 || !reflect.DeepEqual(early.ran, Order[:3]) {
		t.Fatalf("early Run = %d ran %v, want 7 and %v", code, early.ran, Order[:3])
	}
}

// gateCase is one row of the AC-002 table: the same inputs fed to both
// sources.
type gateCase struct {
	name       string
	model      gate.ModelOutcome
	quality    gate.QualityRequirement
	sastReason string
	block      bool // gate.inconclusive: block (the default)
	breach     bool // a finding at or above the gate's fail_on
	failOn     int  // findings at or above --fail-on
	artifacts  bool // a requested audit/SARIF was not written
}

// outcome is what a source decides for a case: the gate's reason and the
// exit decision.
func outcome(src Source, c gateCase) (gate.Reason, gate.ExitDecision) {
	reason := gate.RankReason(gate.ReasonInputs{Model: c.model, SASTReason: c.sastReason})
	res := &gate.Result{Active: true, Reason: string(reason), Breach: c.breach}
	res.Inconclusive = reason != gate.ReasonNone
	res.Fail = c.breach || (res.Inconclusive && c.block)
	return reason, gate.ExitPolicy(gate.ExitInputs{
		NotReviewed:         src.NotReviewed.NotReviewed(c.model, c.quality),
		Gate:                res,
		ArtifactsMissing:    c.artifacts,
		FindingsAtThreshold: c.failOn,
	})
}

// TestAUR576SameInputsSameExitOnBothSources is AC-002: the same inputs give
// the same gate reason and exit code on --base and --pr. The rows where the
// two sources' declared NotReviewed tables differ are listed explicitly
// with each source's code.
func TestAUR576SameInputsSameExitOnBothSources(t *testing.T) {
	shared := []struct {
		c    gateCase
		want int
	}{
		{gateCase{name: "clean", block: true}, gate.ExitClean},
		{gateCase{name: "provider failure, block", model: gate.ModelProviderFailed, block: true}, gate.ExitBehavioral},
		{gateCase{name: "provider failure, quality required", model: gate.ModelProviderFailed, quality: gate.QualityRequired}, gate.ExitBehavioral},
		{gateCase{name: "parse failure, quality required", model: gate.ModelParseFailed, quality: gate.QualityRequired}, gate.ExitBehavioral},
		{gateCase{name: "parse failure, block", model: gate.ModelParseFailed, block: true}, gate.ExitBehavioral},
		{gateCase{name: "scanner absent, block", sastReason: "sast_unavailable", block: true}, gate.ExitBehavioral},
		{gateCase{name: "scanner absent, warn", sastReason: "sast_unavailable"}, gate.ExitClean},
		{gateCase{name: "artifact not written", artifacts: true, failOn: 1}, gate.ExitBehavioral},
		{gateCase{name: "artifact not written, gate breach", artifacts: true, breach: true}, gate.ExitFindings},
		{gateCase{name: "finding above fail-on", failOn: 2}, gate.ExitFindings},
		{gateCase{name: "finding above the gate threshold", breach: true, block: true}, gate.ExitFindings},
	}
	for _, row := range shared {
		lr, ld := outcome(LocalDiff, row.c)
		pr, pd := outcome(PullRequest, row.c)
		if lr != pr {
			t.Errorf("%s: gate reason --base=%q --pr=%q", row.c.name, lr, pr)
		}
		if ld.Code != row.want || pd.Code != row.want {
			t.Errorf("%s: exit --base=%d --pr=%d, want %d on both", row.c.name, ld.Code, pd.Code, row.want)
		}
	}
	// Declared divergences: a model failure always closes a local run; on a
	// pull request it degrades to the deterministic half and the gate (or
	// --exigir-qualidade) decides.
	divergent := []struct {
		c            gateCase
		local, prExt int
	}{
		{gateCase{name: "provider failure, warn", model: gate.ModelProviderFailed}, gate.ExitBehavioral, gate.ExitClean},
		{gateCase{name: "provider failure, gate breach", model: gate.ModelProviderFailed, breach: true}, gate.ExitBehavioral, gate.ExitFindings},
		{gateCase{name: "parse failure, warn", model: gate.ModelParseFailed}, gate.ExitBehavioral, gate.ExitClean},
	}
	for _, row := range divergent {
		_, ld := outcome(LocalDiff, row.c)
		_, pd := outcome(PullRequest, row.c)
		if ld.Code != row.local || pd.Code != row.prExt {
			t.Errorf("%s: exit --base=%d --pr=%d, want declared %d/%d", row.c.name, ld.Code, pd.Code, row.local, row.prExt)
		}
	}
}
