package main

// Central policy containment in the pull request (--pr) path, the one the
// direct Docker Action (action.yml + scripts/action-entrypoint.sh) runs. In
// that container the reviewed checkout is /github/workspace (the process's
// working directory, the entrypoint cd's into it) and the only other
// directory the job author can fill before the container starts is
// /github/home, bind-mounted from $RUNNER_TEMP/_github_home. These tests
// reproduce that layout under a temporary root and drive the real
// runPRReview against a fake GitHub API and the offline model fixture.

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/Mpaape/AurumCode/internal/security/redaction"
)

// actionPolicyRun is the observable outcome of one runPRReview call: its
// exit code and stderr, whether the model was called (the offline provider
// wrote its prompt capture), whether a review was posted, and any request
// the fake GitHub API did not expect.
type actionPolicyRun struct {
	code       int
	stderr     string
	modelCalls bool
	postedBody string
	unexpected []string
}

// actionContainerLayout creates <root>/github/workspace (the reviewed
// checkout, made the working directory) and <root>/github/home (the
// runner-temp mount the job author controls), mirroring the paths a
// `using: docker` action sees.
func actionContainerLayout(t *testing.T) (workspace, home string) {
	t.Helper()
	root := t.TempDir()
	workspace = filepath.Join(root, "github", "workspace")
	home = filepath.Join(root, "github", "home")
	for _, dir := range []string{workspace, home} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(workspace)
	return workspace, home
}

// writePolicyAt writes an empty but valid central policy under dir.
func writePolicyAt(t *testing.T, dir string) {
	t.Helper()
	cfgPath := filepath.Join(dir, ".aurumcode", "config.yml")
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgPath, []byte(""), 0600); err != nil {
		t.Fatal(err)
	}
}

// runActionPolicyReview runs the --pr review of a pull request whose head
// config disables analysis/hardcoded-secret and whose diff carries an
// inline credential, with policyDir as the central policy. The fake API
// records unexpected requests instead of failing from its own goroutine,
// so a regression shows up as a behavioral assertion, never a panic.
func runActionPolicyReview(t *testing.T, policyDir string) actionPolicyRun {
	t.Helper()
	diffBody := "diff --git a/app.go b/app.go\n@@ -1,2 +1,4 @@\n package demo\n+func Change() {\n+ dbPassword := \"hunter2-super-secret\"\n+ _ = dbPassword\n+}\n"
	encodedConfig := base64.StdEncoding.EncodeToString([]byte("rules:\n  analysis/hardcoded-secret:\n    enabled: false\n"))

	var mu sync.Mutex
	run := actionPolicyRun{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/owner/repo/pulls/48":
			_, _ = w.Write([]byte(diffBody))
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/contents/"):
			_, _ = fmt.Fprintf(w, `{"content":%q,"encoding":"base64"}`, encodedConfig)
		case r.Method == http.MethodGet && (strings.HasSuffix(r.URL.Path, "/reviews") || strings.HasSuffix(r.URL.Path, "/comments")):
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/reviews"):
			buf := new(bytes.Buffer)
			_, _ = buf.ReadFrom(r.Body)
			run.postedBody = buf.String()
			_, _ = w.Write([]byte(`{"id":1}`))
		default:
			run.unexpected = append(run.unexpected, r.Method+" "+r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))

	fixture := filepath.Join(t.TempDir(), "response.json")
	if err := os.WriteFile(fixture, []byte(aur518FixtureResponse), 0600); err != nil {
		t.Fatal(err)
	}
	capture := filepath.Join(t.TempDir(), "prompt.txt")
	t.Setenv("AURUMCODE_LLM_FIXTURE", fixture)
	t.Setenv("AURUMCODE_PROMPT_CAPTURE", capture)
	t.Setenv("AURUMCODE_GITHUB_API_URL", server.URL)
	t.Setenv("AURUMCODE_PR_PERMISSION_MODE", "endpoint")
	t.Setenv("GITHUB_SHA", "head-sha")
	t.Setenv("AURUMCODE_BASE_SHA", "base")
	t.Setenv("AURUMCODE_CI_CONTEXT_FILE", "")

	var stdout, stderr strings.Builder
	run.code = runPRReview(reviewIO{stdout: &stdout, stderr: &stderr, filter: redaction.NewFilter()}, prReviewOptions{
		prNumber: 48, repo: "owner/repo", publicar: true,
		publicationSet: true, publication: "review",
		policyDir: policyDir,
	})
	// Close waits for every handler, so the recorded fields are final and
	// safely visible here.
	server.Close()
	run.stderr = stderr.String()
	if _, err := os.Stat(capture); err == nil {
		run.modelCalls = true
	}
	return run
}

// assertRefusedBeforeModel is the fail-closed contract for a policy inside
// the reviewed tree: non-zero exit naming the containment refusal, no model
// call, nothing published.
func assertRefusedBeforeModel(t *testing.T, run actionPolicyRun) {
	t.Helper()
	if run.code == 0 {
		t.Fatalf("expected a non-zero exit for a policy inside the reviewed tree; stderr=%s", run.stderr)
	}
	if !strings.Contains(run.stderr, "inside the reviewed repository") {
		t.Fatalf("expected an error naming the containment refusal:\n%s", run.stderr)
	}
	if run.modelCalls {
		t.Fatal("the model was called although the policy is inside the reviewed tree")
	}
	if run.postedBody != "" {
		t.Fatalf("a review was published although the policy was refused:\n%s", run.postedBody)
	}
}

// TestAUR535ActionPolicyFromGithubHomeApplies covers AC-001: following the
// documented direct-Action recipe (policy copied into
// $RUNNER_TEMP/_github_home, policy_path=/github/home/<dir>), the policy is
// applied -- the head's attempt to disable the rule is ignored, the finding
// and the override warning reach the published review -- while the same
// policy placed in the workspace is refused.
func TestAUR535ActionPolicyFromGithubHomeApplies(t *testing.T) {
	workspace, home := actionContainerLayout(t)
	outside := filepath.Join(home, "aurumcode-policy")
	writePolicyAt(t, outside)
	inside := filepath.Join(workspace, "aurumcode-policy")
	writePolicyAt(t, inside)

	applied := runActionPolicyReview(t, outside)
	if applied.code != 0 {
		t.Fatalf("policy under /github/home was not accepted: exit=%d stderr=%s unexpected=%v", applied.code, applied.stderr, applied.unexpected)
	}
	if !strings.Contains(applied.postedBody, hardcodedSecretMessage) {
		t.Fatalf("published review lost the policy-protected finding:\n%s", applied.postedBody)
	}
	if !strings.Contains(applied.postedBody, policyRuleOverrideWarningSnippet) {
		t.Fatalf("published review does not carry the policy override warning:\n%s", applied.postedBody)
	}

	assertRefusedBeforeModel(t, runActionPolicyReview(t, inside))
}

// TestAUR535PRPolicyInsideReviewedTreeFailsClosed covers AC-002: on the
// --pr path a policy directory inside the working directory (the reviewed
// checkout) fails closed before the model is called, including the form a
// relative policy_path takes once the entrypoint has cd'd into the
// workspace.
func TestAUR535PRPolicyInsideReviewedTreeFailsClosed(t *testing.T) {
	workspace, _ := actionContainerLayout(t)
	writePolicyAt(t, filepath.Join(workspace, "policy"))

	assertRefusedBeforeModel(t, runActionPolicyReview(t, filepath.Join(workspace, "policy")))
	assertRefusedBeforeModel(t, runActionPolicyReview(t, "policy"))
	assertRefusedBeforeModel(t, runActionPolicyReview(t, workspace))
}
