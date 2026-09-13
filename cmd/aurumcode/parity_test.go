package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/git/githubclient"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
)

// TestPRPathMergesDeterministicCapabilities proves the zero-config parity
// wiring: a diff containing a hardcoded secret must produce a deterministic
// static-analysis finding merged into the published formal review, plus the
// rendered summary and Mermaid diagram -- even when the model itself reports
// nothing. It is a transport/publication assertion, not a model-accuracy eval.
func TestPRPathMergesDeterministicCapabilities(t *testing.T) {
	var posted githubclient.PullRequestReview
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/repos/team/project/pulls/7":
			fmt.Fprint(w, "diff --git a/config.go b/config.go\n@@ -1,1 +1,1 @@\n-foo := 1\n+password = \"hunter2-super-secret\"\n")
		case r.Method == "GET" && strings.Contains(r.URL.Path, "/contents/"):
			w.WriteHeader(404)
		case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/reviews"):
			fmt.Fprint(w, "[]")
		case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/comments"):
			fmt.Fprint(w, "[]")
		case r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/reviews"):
			if err := json.NewDecoder(r.Body).Decode(&posted); err != nil {
				t.Error(err)
			}
			w.WriteHeader(201)
			fmt.Fprint(w, `{"id":9}`)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
			w.WriteHeader(400)
		}
	}))
	defer server.Close()

	fixture := filepath.Join(t.TempDir(), "response.json")
	if err := os.WriteFile(fixture, []byte(`{"issues":[],"summary":"Nothing to report."}`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AURUMCODE_LLM_FIXTURE", fixture)
	t.Setenv("AURUMCODE_GITHUB_API_URL", server.URL)
	t.Setenv("AURUMCODE_PR_PERMISSION_MODE", "endpoint")
	t.Setenv("GITHUB_SHA", "new-head")
	t.Setenv("AURUMCODE_BASE_SHA", "base")
	t.Setenv("AURUMCODE_CI_CONTEXT_FILE", "")

	var stdout, stderr strings.Builder
	if code := runReview([]string{"--pr", "7", "--repo", "team/project", "--publicar", "--modo-publicacao", "review"}, &stdout, &stderr, redaction.NewFilter()); code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr.String())
	}

	// The deterministic pass must turn the model's silent approve into a
	// blocking review, and the rendered summary + diagram must be present.
	if posted.Event != "REQUEST_CHANGES" {
		t.Fatalf("event=%q, want REQUEST_CHANGES (analysis finding is error severity)", posted.Event)
	}
	for _, want := range []string{"analysis/hardcoded-secret", "Code Review Summary", "mermaid", "flowchart TD"} {
		if !strings.Contains(posted.Body, want) {
			t.Fatalf("published review missing %q:\n%s", want, posted.Body)
		}
	}
}
