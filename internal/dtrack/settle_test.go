package dtrack

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"
)

func runSettle(t *testing.T, fs *fakeServer, timeout time.Duration) Outcome {
	t.Helper()
	srv := httptest.NewServer(fs.handler())
	t.Cleanup(srv.Close)
	client, _ := NewClient(srv.URL, "k")
	client = client.WithClock(fakeClock(time.Unix(1_000_000, 0)))
	return Run(context.Background(), client, "proj", []byte("{}"), Thresholds{}, time.Second, timeout)
}

// AC-001: the server answers processing:false and only afterwards
// finishes the policy evaluation. Reading at the first response sees zero
// violations (fresh timestamp, wrong numbers); the gate must breach.
func TestSettleReadsAfterPolicyEvaluationLands(t *testing.T) {
	cases := map[string][]reading{
		"zero-then-one":   {{violations: 0}, {violations: 1}},
		"stale-then-real": {{violations: 0, stale: true}, {violations: 0, stale: true}, {violations: 1}},
	}
	for name, script := range cases {
		t.Run(name, func(t *testing.T) {
			out := runSettle(t, &fakeServer{script: script}, time.Minute)
			if out.Inconclusive || !out.Breach || out.PolicyViolationsTotal != 1 {
				t.Fatalf("expected breach on the settled value 1, got %+v", out)
			}
		})
	}
}

// AC-002: numbers that never stabilise are never graded.
func TestSettleNeverStableIsInconclusiveUnsettled(t *testing.T) {
	var script []reading
	for i := 0; i < 100; i++ {
		script = append(script, reading{violations: i})
	}
	out := runSettle(t, &fakeServer{script: script}, 10*time.Second)
	if !out.Inconclusive || out.InconclusiveReason != ReasonMetricsUnsettled || out.Breach {
		t.Fatalf("expected dtrack_metrics_unsettled, got %+v", out)
	}
}

// AC-002: a timestamp that never passes the upload instant is unsettled
// even when the numbers coincide (they may be the previous revision's).
func TestSettleStaleForeverIsInconclusiveUnsettled(t *testing.T) {
	out := runSettle(t, &fakeServer{script: []reading{{stale: true}}}, 10*time.Second)
	if !out.Inconclusive || out.InconclusiveReason != ReasonMetricsUnsettled {
		t.Fatalf("expected dtrack_metrics_unsettled, got %+v", out)
	}
}

// AC-003: an already settled server grades normally.
func TestSettleAlreadySettledGradesAsBefore(t *testing.T) {
	out := runSettle(t, &fakeServer{violations: 2}, time.Minute)
	if out.Inconclusive || !out.Breach || out.PolicyViolationsTotal != 2 {
		t.Fatalf("got %+v", out)
	}
	out = runSettle(t, &fakeServer{}, time.Minute)
	if out.Inconclusive || out.Breach {
		t.Fatalf("got %+v", out)
	}
}

func TestSettleRefreshForbiddenIsToleratedAndRecorded(t *testing.T) {
	fs := &fakeServer{refreshCode: 403, violations: 1}
	out := runSettle(t, fs, time.Minute)
	if fs.refreshCalls.Load() != 1 {
		t.Fatalf("refresh calls=%d, want 1", fs.refreshCalls.Load())
	}
	if out.Inconclusive || !out.Breach || len(out.Notes) != 1 || out.Notes[0] != NoteRefreshUnavailable {
		t.Fatalf("got %+v", out)
	}
	ok := runSettle(t, &fakeServer{}, time.Minute)
	if len(ok.Notes) != 0 {
		t.Fatalf("permitted refresh must add no note: %+v", ok)
	}
}

// Measured on Dependency-Track 5.1.1: re-uploading an unchanged SBOM leaves
// metrics.lastOccurrence frozen. The project's own analysis timestamp,
// plus two coinciding readings, then proves the server finished.
func TestSettleUnchangedSBOMSettlesThroughProjectAnalysis(t *testing.T) {
	out := runSettle(t, &fakeServer{script: []reading{{violations: 1, stale: true}}, analysed: true}, time.Minute)
	if out.Inconclusive || !out.Breach || out.PolicyViolationsTotal != 1 {
		t.Fatalf("got %+v", out)
	}
}
