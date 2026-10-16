package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
)

// #201: publication: status posts nothing on the pull request. The parecer
// goes to AURUMCODE_PARECER_FILE (the reusable workflow's job summary), or to
// stdout when that is unset.
func TestPublicationStatusPostsNothingOnThePullRequest(t *testing.T) {
	for _, toFile := range []bool{true, false} {
		t.Run(fmt.Sprintf("file=%v", toFile), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == "GET" && r.URL.Path == "/repos/team/project/pulls/7":
					fmt.Fprint(w, "diff --git a/config.go b/config.go\n@@ -1,1 +1,1 @@\n-foo := 1\n+foo := 2\n")
				case r.Method == "GET" && strings.Contains(r.URL.Path, "/contents/"):
					w.WriteHeader(404)
				case r.Method == "GET":
					fmt.Fprint(w, "[]")
				default:
					t.Errorf("status mode must not write to the pull request: %s %s", r.Method, r.URL)
					w.WriteHeader(400)
				}
			}))
			defer server.Close()

			dir := t.TempDir()
			fixture := filepath.Join(dir, "response.json")
			if err := os.WriteFile(fixture, []byte(`{"issues":[],"summary":"Nothing to report."}`), 0o600); err != nil {
				t.Fatal(err)
			}
			parecer := filepath.Join(dir, "parecer.md")
			if !toFile {
				parecer = ""
			}
			t.Setenv("AURUMCODE_LLM_FIXTURE", fixture)
			t.Setenv("AURUMCODE_GITHUB_API_URL", server.URL)
			t.Setenv("AURUMCODE_PR_PERMISSION_MODE", "endpoint")
			t.Setenv("GITHUB_SHA", "new-head")
			t.Setenv("AURUMCODE_BASE_SHA", "base")
			t.Setenv("AURUMCODE_CI_CONTEXT_FILE", "")
			t.Setenv("AURUMCODE_PARECER_FILE", parecer)

			var stdout, stderr strings.Builder
			if code := runReview([]string{"--pr", "7", "--repo", "team/project", "--publicar", "--na-linha", "--modo-publicacao", "status"}, &stdout, &stderr, redaction.NewFilter()); code != 0 {
				t.Fatalf("exit=%d stderr=%s", code, stderr.String())
			}
			if !strings.Contains(stdout.String(), "publication: status") {
				t.Fatalf("stdout lacks the status-only line:\n%s", stdout.String())
			}
			body := stdout.String()
			if toFile {
				raw, err := os.ReadFile(parecer)
				if err != nil {
					t.Fatalf("parecer file not written: %v", err)
				}
				body = string(raw)
			}
			if !strings.Contains(body, "<!-- aurumcode-review -->") {
				t.Fatalf("parecer missing:\n%s", body)
			}
		})
	}
}

func TestNormalizeReviewPublicationStatus(t *testing.T) {
	for _, raw := range []string{"status", "none", " Nenhuma "} {
		got, err := config.NormalizeReviewPublication(raw)
		if err != nil || got != "status" {
			t.Fatalf("%q -> %q, %v; want status", raw, got, err)
		}
	}
	if _, err := config.NormalizeReviewPublication("silent"); err == nil || !strings.Contains(err.Error(), "status") {
		t.Fatalf("unknown mode error must name status: %v", err)
	}
}
