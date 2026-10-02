package gate

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/pkg/types"
)

type stubContributor struct {
	name string
	fn   func(*Run, *Result) error
}

func (s stubContributor) Name() string   { return s.name }
func (s stubContributor) Origin() string { return "stub-" + s.name }
func (s stubContributor) Apply(_ context.Context, run *Run, res *Result) error {
	if s.fn == nil {
		return nil
	}
	return s.fn(run, res)
}

func newRun(t *testing.T, mode string) *Run {
	t.Helper()
	cfg := &config.Config{}
	cfg.Gate.Inconclusive = mode
	cfg.Gate.FailOn = []string{"error"}
	return &Run{Cfg: cfg, Review: &types.ReviewResult{}}
}

func TestPipelineAppliesContributorsInDeclaredOrder(t *testing.T) {
	var seen []string
	rec := func(n string) stubContributor {
		return stubContributor{name: n, fn: func(_ *Run, res *Result) error {
			seen = append(seen, n)
			res.Lines = append(res.Lines, n)
			return nil
		}}
	}
	p := NewPipeline(rec("c"), rec("a"), rec("b"))
	var res Result
	if err := p.Run(context.Background(), newRun(t, ""), &res); err != nil {
		t.Fatal(err)
	}
	want := []string{"c", "a", "b"}
	if !reflect.DeepEqual(seen, want) || !reflect.DeepEqual(res.Lines, want) || !reflect.DeepEqual(p.Names(), want) {
		t.Fatalf("order seen=%v lines=%v names=%v, want %v", seen, res.Lines, p.Names(), want)
	}
	if len(res.Trail) != 3 || res.Trail[0].Origin != "stub-c" || res.Trail[1].Lines != 1 {
		t.Fatalf("trail did not record each contributor: %+v", res.Trail)
	}
}

func TestPipelineContributorErrorIsInconclusiveNeverApproved(t *testing.T) {
	boom := stubContributor{name: "boom", fn: func(*Run, *Result) error { return errors.New("exploded") }}
	after := stubContributor{name: "after", fn: func(_ *Run, res *Result) error { res.Lines = append(res.Lines, "after"); return nil }}
	for _, tc := range []struct {
		mode     string
		wantFail bool
	}{{"block", true}, {"warn", false}, {"", false}} {
		var res Result
		if err := NewPipeline(boom, after).Run(context.Background(), newRun(t, tc.mode), &res); err != nil {
			t.Fatalf("mode %q: ordinary error must not abort: %v", tc.mode, err)
		}
		if !res.Active || !res.Inconclusive || res.Reason != "contributor_error:boom" {
			t.Fatalf("mode %q: want active inconclusive with reason, got %+v", tc.mode, res)
		}
		if res.Fail != tc.wantFail {
			t.Fatalf("mode %q: Fail=%v, want %v", tc.mode, res.Fail, tc.wantFail)
		}
		if got := res.Lines[len(res.Lines)-1]; got != "after" {
			t.Fatalf("mode %q: later contributors must still run, last line %q", tc.mode, got)
		}
		if res.Trail[0].Err == "" {
			t.Fatalf("mode %q: failure not recorded in trail", tc.mode)
		}
	}
}

func TestPipelineFatalErrorAborts(t *testing.T) {
	cfgErr := errors.New("bad config")
	ran := false
	p := NewPipeline(
		stubContributor{name: "bad", fn: func(*Run, *Result) error { return Fatal(cfgErr) }},
		stubContributor{name: "never", fn: func(*Run, *Result) error { ran = true; return nil }},
	)
	var res Result
	if err := p.Run(context.Background(), newRun(t, "block"), &res); !errors.Is(err, cfgErr) || err.Error() != "bad config" {
		t.Fatalf("want the bare config error, got %v", err)
	}
	if ran {
		t.Fatal("pipeline kept running after a fatal error")
	}
}

func TestResultMergeJoinsReasonsAndIgnoresInactive(t *testing.T) {
	r := Result{Active: true, Reason: "a"}
	r.Merge(Result{})
	if r.Reason != "a" {
		t.Fatalf("inactive merge changed result: %+v", r)
	}
	r.Merge(Result{Active: true, Fail: true, Reason: "b", Lines: []string{"x"}})
	if r.Reason != "a,b" || !r.Fail || len(r.Lines) != 1 {
		t.Fatalf("merge: %+v", r)
	}
}
