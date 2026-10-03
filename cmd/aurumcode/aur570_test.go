package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Mpaape/AurumCode/internal/security/redaction"
)

// settlingDTrack is a Dependency-Track double whose metrics change AFTER
// bom/token already said processing:false (AUR-570): reads[n] is the
// n-th metrics response's policyViolationsTotal (the last one repeats).
// A nil reads slice makes every reading differ, so it never stabilises.
type settlingDTrack struct {
	reads []int
	n     atomic.Int32
}

func (f *settlingDTrack) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/bom":
			_ = json.NewEncoder(w).Encode(map[string]string{"token": "tok"})
		case strings.HasPrefix(r.URL.Path, "/api/v1/bom/token/"):
			_ = json.NewEncoder(w).Encode(map[string]bool{"processing": false})
		case strings.HasSuffix(r.URL.Path, "/refresh"):
			w.WriteHeader(http.StatusForbidden)
		case strings.HasPrefix(r.URL.Path, "/api/v1/metrics/project/"):
			n := int(f.n.Add(1))
			v := n
			if f.reads != nil {
				v = f.reads[min(n, len(f.reads))-1]
			}
			_ = json.NewEncoder(w).Encode(map[string]int64{
				"critical": 0, "high": 0, "policyViolationsTotal": int64(v), "lastOccurrence": 4102444800000,
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}
}

func runSettling(t *testing.T, fs *settlingDTrack, mode string) (int, string) {
	t.Helper()
	srv := httptest.NewServer(fs.handler())
	t.Cleanup(srv.Close)
	useFakeDTrackClock(t)
	cleanFixture(t, dtrackConfigYAML(srv.URL, writeSBOMFixture(t), mode))
	t.Setenv("DTRACK_API_KEY", "settle-test-key")
	t.Setenv("DTRACK_PROJECT_ID", "proj-123")
	t.Setenv("AURUMCODE_LLM_FIXTURE", approveFixture(t))
	var out, errOut strings.Builder
	code := runReview([]string{"--base", "HEAD~1"}, &out, &errOut, redaction.NewFilter())
	return code, out.String() + errOut.String()
}

// AC-001: processing:false, then the policy evaluation lands (0 -> 1).
func TestAUR570GateReadsSettledViolation(t *testing.T) {
	code, out := runSettling(t, &settlingDTrack{reads: []int{0, 1}}, "block")
	if code != exitFindings || !strings.Contains(out, "policy_violations 1 > policy_violations 0") {
		t.Fatalf("exit=%d, want exitFindings with the settled violation:\n%s", code, out)
	}
	if strings.Contains(out, "ssor_dtrack: aprovado") {
		t.Fatalf("approved a policy violation:\n%s", out)
	}
	if !strings.Contains(out, "dtrack_refresh_unavailable") {
		t.Fatalf("a 403 on refresh must be recorded:\n%s", out)
	}
}

// AC-002: metrics that never settle are inconclusive; block fails, warn
// does not approve.
func TestAUR570UnsettledIsInconclusivePerMode(t *testing.T) {
	code, out := runSettling(t, &settlingDTrack{}, "block")
	if code != exitQualityNotReviewed || !strings.Contains(out, "dtrack_metrics_unsettled") {
		t.Fatalf("block: exit=%d:\n%s", code, out)
	}
	code, out = runSettling(t, &settlingDTrack{}, "warn")
	if code != 0 || !strings.Contains(out, "dtrack_metrics_unsettled") || strings.Contains(out, "ssor_dtrack: aprovado") {
		t.Fatalf("warn: exit=%d (must not approve):\n%s", code, out)
	}
}
