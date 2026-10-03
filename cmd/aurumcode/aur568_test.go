package main

// AUR-568: a requested --auditoria/--sarif that cannot be written never ends
// as success, on --base and on --pr, with and without a declared gate.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/git/githubclient"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
)

const aur568CleanReply = `{"summary":"ok","verdict":"approve","issues":[]}`

// aur568BadPaths returns the two unwritable shapes: a parent that is a
// regular file (ENOTDIR, independent of the uid) and a directory that does
// not exist (nothing creates it).
func aur568BadPaths(t *testing.T) map[string]string {
	t.Helper()
	dir := t.TempDir()
	file := filepath.Join(dir, "arquivo-regular")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	return map[string]string{
		"pai-e-arquivo":   filepath.Join(file, "saida.json"),
		"dir-inexistente": filepath.Join(dir, "nao-existe", "saida.json"),
	}
}

func aur568Fixture(t *testing.T) {
	t.Helper()
	fixture := filepath.Join(t.TempDir(), "response.json")
	if err := os.WriteFile(fixture, []byte(aur568CleanReply), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AURUMCODE_LLM_FIXTURE", fixture)
}

func TestAUR568BasePathUnwritableNeverSucceeds(t *testing.T) {
	cases := []struct {
		name, gate string
		wantCode   int
		wantReason string
		wantLine   bool
	}{
		{"block", "gate:\n  inconclusive: block\n", exitQualityNotReviewed, "inconclusive", true},
		{"warn", "gate:\n  inconclusive: warn\n", exitArtifactNotWritten, "inconclusive", true},
		{"sem-gate", "", exitArtifactNotWritten, "", false},
	}
	flags := map[string]string{"--auditoria": "audit_write_failed", "--sarif": "sarif_write_failed"}
	for flag, reason := range flags {
		for shape, bad := range aur568BadPaths(t) {
			for _, tc := range cases {
				t.Run(flag+"/"+shape+"/"+tc.name, func(t *testing.T) {
					coverageFixture(t, tc.gate)
					aur568Fixture(t)
					var out, errOut strings.Builder
					code := runReview([]string{"--base", "HEAD~1", flag, bad}, &out, &errOut, redaction.NewFilter())
					if code != tc.wantCode || code == 0 {
						t.Fatalf("exit=%d, want %d; stdout=%s stderr=%s", code, tc.wantCode, out.String(), errOut.String())
					}
					if !strings.Contains(errOut.String(), bad) || !strings.Contains(errOut.String(), reason) {
						t.Fatalf("stderr must name the path %q and %s: %s", bad, reason, errOut.String())
					}
					if tc.wantLine && !strings.Contains(errOut.String(), "policy gate:") {
						t.Fatalf("a declared gate must publish its decision line: %s", errOut.String())
					}
					if strings.Contains(out.String(), "No issues found") && tc.gate != "" {
						t.Fatalf("approval must be withheld under a declared gate: %s", out.String())
					}
				})
			}
		}
	}
}

// The other artifact, when writable, is still written and agrees with the
// final decision (inconclusive, with the failed artifact's reason).
func TestAUR568BaseWritableSiblingAgreesWithDecision(t *testing.T) {
	coverageFixture(t, "gate:\n  inconclusive: block\n")
	aur568Fixture(t)
	good := filepath.Join(t.TempDir(), "audit.json")
	bad := aur568BadPaths(t)["dir-inexistente"]
	var out, errOut strings.Builder
	code := runReview([]string{"--base", "HEAD~1", "--auditoria", good, "--sarif", bad}, &out, &errOut, redaction.NewFilter())
	if code != exitQualityNotReviewed {
		t.Fatalf("exit=%d; stderr=%s", code, errOut.String())
	}
	raw, err := os.ReadFile(good)
	if err != nil {
		t.Fatal(err)
	}
	var audit auditFile
	if err := json.Unmarshal(raw, &audit); err != nil {
		t.Fatal(err)
	}
	if audit.Gate.Decision != "fail" || !strings.Contains(audit.Gate.Reason, "sarif_write_failed") && !strings.Contains(audit.Gate.Reason, "SARIF") && !strings.Contains(audit.Gate.Reason, "sarif") {
		t.Fatalf("audit must record the final decision naming the failed artifact: %+v", audit.Gate)
	}
}

// AC-003: writable paths behave as before.
func TestAUR568BaseWritablePathsUnchanged(t *testing.T) {
	coverageFixture(t, "gate:\n  inconclusive: block\n")
	aur568Fixture(t)
	dir := t.TempDir()
	audit, sarif := filepath.Join(dir, "a.json"), filepath.Join(dir, "s.sarif")
	var out, errOut strings.Builder
	code := runReview([]string{"--base", "HEAD~1", "--auditoria", audit, "--sarif", sarif}, &out, &errOut, redaction.NewFilter())
	if code != 0 {
		t.Fatalf("exit=%d; stderr=%s", code, errOut.String())
	}
	for _, p := range []string{audit, sarif} {
		if st, err := os.Stat(p); err != nil || st.Size() == 0 {
			t.Fatalf("%s must be written: %v", p, err)
		}
	}
	if strings.Contains(errOut.String(), "write_failed") {
		t.Fatalf("no failure expected: %s", errOut.String())
	}
}

// aur568PRServer fakes the GitHub endpoints a --pr --check run touches and
// records the published policy-gate status and formal review event.
func aur568PRServer(t *testing.T, status *githubclient.CommitStatus, event *string) *httptest.Server {
	t.Helper()
	diffBody := "diff --git a/app.go b/app.go\n@@ -1,2 +1,3 @@\n package demo\n+func Change() {}\n"
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/owner/repo":
			_, _ = w.Write([]byte(`{"permissions":{"push":true}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/repos/owner/repo/pulls/48" && r.Header.Get("Accept") == "application/vnd.github.v3.diff":
			_, _ = w.Write([]byte(diffBody))
		case r.Method == http.MethodGet && r.URL.Path == "/repos/owner/repo/pulls/48":
			_, _ = w.Write([]byte(`{"head":{"sha":"head-sha"}}`))
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/contents/") && strings.Contains(r.URL.Path, "config.yml"):
			_, _ = w.Write([]byte(`{"content":"` + b64(aur568PRConfig) + `","encoding":"base64"}`))
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/contents/"):
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/reviews"):
			var body struct {
				Event string `json:"event"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			*event = body.Event
			_, _ = w.Write([]byte(`{"id":1}`))
		case r.Method == http.MethodPost && r.URL.Path == "/repos/owner/repo/statuses/head-sha":
			_ = json.NewDecoder(r.Body).Decode(status)
			w.WriteHeader(http.StatusCreated)
		default:
			t.Errorf("unexpected GitHub request: %s %s", r.Method, r.URL.Path)
		}
	}))
}

var aur568PRConfig string

func TestAUR568PRPathUnwritableNeverSucceeds(t *testing.T) {
	cases := []struct {
		name, cfg, wantState string
		wantCode             int
	}{
		{"block", "gate:\n  inconclusive: block\n", "failure", exitQualityNotReviewed},
		{"warn", "gate:\n  inconclusive: warn\n", "success", exitArtifactNotWritten},
		// Without a gate the --check status stays the grave-finding one; only the exit changes.
		{"sem-gate", "review: {}\n", "success", exitArtifactNotWritten},
	}
	for _, flag := range []string{"auditoria", "sarif"} {
		for shape, bad := range aur568BadPaths(t) {
			for _, tc := range cases {
				t.Run(flag+"/"+shape+"/"+tc.name, func(t *testing.T) {
					aur568PRConfig = tc.cfg
					var status githubclient.CommitStatus
					var event string
					server := aur568PRServer(t, &status, &event)
					defer server.Close()
					aur568Fixture(t)
					setPRGateEnv(t, server, os.Getenv("AURUMCODE_LLM_FIXTURE"))
					opts := prReviewOptions{publicationSet: true, publication: "review"}
					if flag == "auditoria" {
						opts.auditoriaPath = bad
					} else {
						opts.sarifPath = bad
					}
					var stdout, stderr strings.Builder
					code := runPRReview(&stdout, &stderr, 48, "owner/repo", true, true, true, redaction.NewFilter(), opts)
					if code != tc.wantCode {
						t.Fatalf("exit=%d, want %d; stdout=%s stderr=%s", code, tc.wantCode, stdout.String(), stderr.String())
					}
					if !strings.Contains(stderr.String(), bad) {
						t.Fatalf("stderr must name %q: %s", bad, stderr.String())
					}
					if status.State != tc.wantState {
						t.Fatalf("policy-gate status=%q, want %q (%s) stdout=%s stderr=%s", status.State, tc.wantState, status.Description, stdout.String(), stderr.String())
					}
					if tc.wantState != "" && strings.Contains(strings.ToLower(status.Description), "aprovado") {
						t.Fatalf("status must never say approved: %s", status.Description)
					}
					if tc.cfg != "review: {}\n" && event == "APPROVE" {
						t.Fatalf("formal review must not approve under a declared gate")
					}
				})
			}
		}
	}
}
