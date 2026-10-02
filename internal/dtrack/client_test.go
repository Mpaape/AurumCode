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
}

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
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v1/metrics/project/"):
			f.metricsCalls.Add(1)
			if f.metricsIncomplete {
				_ = json.NewEncoder(w).Encode(map[string]int{})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]int{
				"critical": f.critical, "high": f.high, "policyViolationsTotal": f.violations,
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
	if fs.metricsCalls.Load() != 1 {
		t.Fatalf("metrics calls=%d, want 1", fs.metricsCalls.Load())
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
