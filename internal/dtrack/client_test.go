package dtrack

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func fakeClock(start time.Time) (now func() time.Time, sleep func(time.Duration)) {
	cur := start
	return func() time.Time { return cur },
		func(d time.Duration) { cur = cur.Add(d) }
}

func TestValidateHostAcceptsHTTPSAndLoopbackHTTPOnly(t *testing.T) {
	cases := []struct {
		host string
		ok   bool
	}{
		{"https://dtrack.example.invalid", true},
		{"http://127.0.0.1:8080", true},
		{"http://[::1]:8080", true},
		{"http://dtrack.example.invalid", false},
		{"http://localhost:8080", false},
		{"https://user:pass@dtrack.example.invalid", false},
		{"https://dtrack.example.invalid?x=1", false},
		{"", false},
		{"not-a-url", false},
	}
	for _, c := range cases {
		err := ValidateHost(c.host)
		if (err == nil) != c.ok {
			t.Errorf("ValidateHost(%q) err=%v, want ok=%v", c.host, err, c.ok)
		}
	}
}

// fakeServer builds an httptest Dependency-Track v5 fake: upload always
// succeeds with token "tok", processing returns true for the first
// pendingPolls calls then false, and metrics returns the given values.
type fakeServer struct {
	uploadCalls  atomic.Int32
	pollCalls    atomic.Int32
	metricsCalls atomic.Int32
	seenAPIKeys  []string
	pendingPolls int32

	critical, high, violations int
	metricsIncomplete          bool

	// AUR-570: when script is non-nil, the n-th metrics read returns
	// script[n] (the last entry repeats) instead of the fixed numbers.
	script       []reading
	refreshCode  int // non-zero: the refresh endpoint answers this status
	refreshCalls atomic.Int32
}

// reading is one scripted metrics response. stale = lastOccurrence before
// the upload instant (the previous revision's numbers).
type reading struct {
	critical, high, violations int
	stale                      bool
}

const farFutureMillis = int64(4102444800000)

func (f *fakeServer) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		f.seenAPIKeys = append(f.seenAPIKeys, r.Header.Get("X-Api-Key"))
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/bom":
			f.uploadCalls.Add(1)
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if r.FormValue("project") == "" || r.MultipartForm.File["bom"] == nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"token": "tok"})
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v1/bom/token/"):
			n := f.pollCalls.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]bool{"processing": n <= f.pendingPolls})
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/refresh"):
			f.refreshCalls.Add(1)
			w.WriteHeader(max(f.refreshCode, http.StatusOK))
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v1/metrics/project/"):
			n := int(f.metricsCalls.Add(1))
			if f.metricsIncomplete {
				_ = json.NewEncoder(w).Encode(map[string]int64{"lastOccurrence": farFutureMillis})
				return
			}
			rd := reading{f.critical, f.high, f.violations, false}
			if f.script != nil {
				rd = f.script[min(n, len(f.script))-1]
			}
			last := farFutureMillis
			if rd.stale {
				last = 1000
			}
			_ = json.NewEncoder(w).Encode(map[string]int64{
				"critical": int64(rd.critical), "high": int64(rd.high), "policyViolationsTotal": int64(rd.violations), "lastOccurrence": last,
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}
}

func TestRunApprovesWithinThresholds(t *testing.T) {
	fs := &fakeServer{pendingPolls: 2}
	srv := httptest.NewServer(fs.handler())
	defer srv.Close()

	client, err := NewClient(srv.URL, "secret-key")
	if err != nil {
		t.Fatal(err)
	}
	now, sleep := fakeClock(time.Unix(0, 0))
	client = client.WithClock(now, sleep)

	out := Run(context.Background(), client, "proj", []byte(`{"bomFormat":"CycloneDX"}`), Thresholds{}, time.Second, time.Minute)
	if out.Breach || out.Inconclusive {
		t.Fatalf("expected approval, got %+v", out)
	}
	if fs.uploadCalls.Load() != 1 {
		t.Fatalf("upload calls=%d, want 1", fs.uploadCalls.Load())
	}
	if fs.pollCalls.Load() < 2 {
		t.Fatalf("poll calls=%d, want >= 2", fs.pollCalls.Load())
	}
	if fs.metricsCalls.Load() != 2 {
		t.Fatalf("metrics calls=%d, want 2 (two coinciding readings)", fs.metricsCalls.Load())
	}
	for _, k := range fs.seenAPIKeys {
		if k != "secret-key" {
			t.Fatalf("server saw X-Api-Key=%q, want secret-key", k)
		}
	}
}

func TestRunBreachesOverThresholds(t *testing.T) {
	fs := &fakeServer{pendingPolls: 0, critical: 3, high: 1, violations: 2}
	srv := httptest.NewServer(fs.handler())
	defer srv.Close()
	client, _ := NewClient(srv.URL, "k")
	client = client.WithClock(fakeClock(time.Unix(0, 0)))

	out := Run(context.Background(), client, "proj", []byte("{}"), Thresholds{MaxCritical: 0, MaxHigh: 0, PolicyViolations: 0}, time.Millisecond, time.Second)
	if !out.Breach {
		t.Fatalf("expected breach, got %+v", out)
	}
	if out.Critical != 3 || out.High != 1 || out.PolicyViolationsTotal != 2 {
		t.Fatalf("unexpected metric numbers: %+v", out)
	}
	if len(out.Reasons) != 3 {
		t.Fatalf("expected 3 breach reasons, got %v", out.Reasons)
	}
}

func TestRunTimesOutWithoutApproving(t *testing.T) {
	fs := &fakeServer{pendingPolls: 1 << 20} // never finishes
	srv := httptest.NewServer(fs.handler())
	defer srv.Close()
	client, _ := NewClient(srv.URL, "k")
	client = client.WithClock(fakeClock(time.Unix(0, 0)))

	out := Run(context.Background(), client, "proj", []byte("{}"), Thresholds{}, time.Second, 5*time.Second)
	if !out.Inconclusive || out.InconclusiveReason != ReasonTimeout {
		t.Fatalf("expected timeout-inconclusive, got %+v", out)
	}
	if out.Breach {
		t.Fatalf("a timeout must never report a breach either: %+v", out)
	}
}

func TestRunHTTPErrorIsInconclusive(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	client, _ := NewClient(srv.URL, "k")
	client = client.WithClock(fakeClock(time.Unix(0, 0)))

	out := Run(context.Background(), client, "proj", []byte("{}"), Thresholds{}, time.Millisecond, time.Second)
	if !out.Inconclusive || out.InconclusiveReason != ReasonHTTPError {
		t.Fatalf("expected http-error-inconclusive, got %+v", out)
	}
}

func TestRunUnreachableServerIsInconclusive(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close() // closed before any call: connection refused
	client, _ := NewClient(srv.URL, "k")
	client = client.WithClock(fakeClock(time.Unix(0, 0)))

	out := Run(context.Background(), client, "proj", []byte("{}"), Thresholds{}, time.Millisecond, time.Second)
	if !out.Inconclusive || out.InconclusiveReason != ReasonUnreachable {
		t.Fatalf("expected unreachable-inconclusive, got %+v", out)
	}
}

func TestRunIncompleteMetricsIsInconclusiveNeverZero(t *testing.T) {
	fs := &fakeServer{pendingPolls: 0, metricsIncomplete: true}
	srv := httptest.NewServer(fs.handler())
	defer srv.Close()
	client, _ := NewClient(srv.URL, "k")
	client = client.WithClock(fakeClock(time.Unix(0, 0)))

	out := Run(context.Background(), client, "proj", []byte("{}"), Thresholds{}, time.Millisecond, time.Second)
	if !out.Inconclusive || out.InconclusiveReason != ReasonMetricsIncomplete {
		t.Fatalf("expected metrics-incomplete-inconclusive, got %+v", out)
	}
	if out.Breach {
		t.Fatalf("an incomplete metrics response must never read as a breach, and never as approved either: %+v", out)
	}
}

// TestEvaluateMetricsNegativeCountIsIncompleteNeverClean: a negative count
// is not a real Dependency-Track value. Grading it normally would be
// dangerous (a negative can never exceed a non-negative threshold, so it
// would always read as "clean" no matter how wrong the response actually
// is) -- EvaluateMetrics must refuse it exactly like a missing field.
func TestEvaluateMetricsNegativeCountIsIncompleteNeverClean(t *testing.T) {
	neg, zero := -1, 0
	cases := []ProjectMetrics{
		{Critical: &neg, High: &zero, PolicyViolationsTotal: &zero},
		{Critical: &zero, High: &neg, PolicyViolationsTotal: &zero},
		{Critical: &zero, High: &zero, PolicyViolationsTotal: &neg},
	}
	for i, m := range cases {
		eval, err := EvaluateMetrics(m, Thresholds{})
		if err != ErrMetricsIncomplete {
			t.Fatalf("case %d: err=%v, want ErrMetricsIncomplete", i, err)
		}
		if eval.Breach {
			t.Fatalf("case %d: a negative count must never read as a breach either: %+v", i, eval)
		}
	}
}

// TestRunMissingProcessingFieldNeverReadsAsDone: a /bom/token/{token}
// response with no "processing" key at all (not even "processing":false)
// must never be read as finished -- it keeps polling until the timeout,
// exactly like "processing":true would.
func TestRunMissingProcessingFieldNeverReadsAsDone(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/bom":
			_ = json.NewEncoder(w).Encode(map[string]string{"token": "tok"})
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v1/bom/token/"):
			_, _ = w.Write([]byte(`{}`)) // no "processing" key at all
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	client, _ := NewClient(srv.URL, "k")
	client = client.WithClock(fakeClock(time.Unix(0, 0)))

	out := Run(context.Background(), client, "proj", []byte("{}"), Thresholds{}, time.Second, 5*time.Second)
	if !out.Inconclusive || out.InconclusiveReason != ReasonTimeout {
		t.Fatalf("expected timeout-inconclusive (missing processing field never reads as done), got %+v", out)
	}
}

// TestUploadNeverForwardsAPIKeyAcrossARedirect: a server that answers the
// upload with a 302 to a second, different server must never have that
// second server see X-Api-Key -- Go's http.Client strips Authorization/
// Cookie on a cross-host redirect but NOT a custom header, so the only
// safe rule (enforced by NewClient's CheckRedirect) is to never follow a
// redirect at all.
func TestUploadNeverForwardsAPIKeyAcrossARedirect(t *testing.T) {
	var redirectTargetSawKey bool
	var redirectTargetCalled bool
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirectTargetCalled = true
		if r.Header.Get("X-Api-Key") != "" {
			redirectTargetSawKey = true
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"token": "tok"})
	}))
	defer target.Close()

	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/api/v1/bom", http.StatusFound)
	}))
	defer origin.Close()

	client, _ := NewClient(origin.URL, "secret-key")
	_, err := client.Upload(context.Background(), "proj", []byte("{}"))
	if err == nil {
		t.Fatal("expected Upload to fail on an unfollowed redirect, got nil error")
	}
	if redirectTargetCalled {
		t.Fatalf("the redirect target must never be contacted at all (no-follow policy); X-Api-Key seen=%v", redirectTargetSawKey)
	}
	if redirectTargetSawKey {
		t.Fatal("the redirect target saw X-Api-Key -- the no-follow policy failed")
	}
}
