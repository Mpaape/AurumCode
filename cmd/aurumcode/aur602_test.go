package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/changelog"
	"github.com/Mpaape/AurumCode/internal/git/githubclient"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
	"github.com/Mpaape/AurumCode/pkg/types"
	"gopkg.in/yaml.v3"
)

// aur602ModelLine is the entry the model fixture answers with.
const aur602ModelLine = "O check de changelog agora oferece a entrada pronta para colar."

// aur602Canary builds a secret-shaped value at run time, so no literal
// secret lives in the repository.
func aur602Canary(t *testing.T) string {
	t.Helper()
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		t.Fatal(err)
	}
	return "aurum" + "canary" + hex.EncodeToString(buf)
}

// aur602NoModel clears every provider variable of the environment.
func aur602NoModel(t *testing.T) {
	t.Helper()
	for _, k := range []string{"AURUMCODE_LLM_FIXTURE", "LLM_PROVIDER", "LLM_API_KEY", "LLM_BASE_URL", "LLM_MODEL"} {
		t.Setenv(k, "")
	}
}

func aur602ModelFixture(t *testing.T, body string) {
	t.Helper()
	aur602NoModel(t)
	path := filepath.Join(t.TempDir(), "model.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AURUMCODE_LLM_FIXTURE", path)
}

// aur602Run runs the check on a required repository whose PR changes only
// app.go, with the given commits and filter; it returns the job summary too.
func aur602Run(t *testing.T, commits []changelog.Commit, filter *redaction.Filter) (int, string, string, string) {
	t.Helper()
	t.Setenv("AURUMCODE_POLICY", "")
	summary := filepath.Join(t.TempDir(), "step-summary.md")
	t.Setenv(stepSummaryEnv, summary)
	root := aur509Repo(t, aur509Required)
	deps := changelogDeps{
		differ:   aur509Differ([]types.DiffFile{{Path: "app.go"}}, nil, nil),
		commits:  func(string, string, string) ([]changelog.Commit, error) { return commits, nil },
		provider: changelogProviderFromEnv,
		filter:   filter,
	}
	var stdout, stderr bytes.Buffer
	code := runChangelogWith([]string{"--base", "base-sha", "--repo", root}, &stdout, &stderr, deps)
	data, err := os.ReadFile(summary)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return code, stdout.String(), stderr.String(), string(data)
}

// aur602Pasted proves a suggestion is ready to paste: inserted under the
// required section, the check itself accepts it.
func aur602Pasted(t *testing.T, out string) {
	t.Helper()
	i := strings.Index(out, "## Unreleased\n\n")
	if i < 0 {
		t.Fatalf("no pasteable block in output:\n%s", out)
	}
	req, err := changelog.DefaultRequirement()
	if err != nil {
		t.Fatal(err)
	}
	pasted := strings.Replace(aur509Changelog, "## Unreleased\n\n", out[i:]+"\n", 1)
	if v := req.Verify(changelog.Change{State: changelog.FileChanged, Old: aur509Changelog, New: pasted}); !v.OK {
		t.Fatalf("the suggestion pasted into the changelog is refused (%s): %s\n%s", v.Reason, v.Detail, pasted)
	}
}

// AC-001: a PR without an entry still fails, and the output carries the
// entry the model suggested, ready to paste; the job summary gets it too.
func TestAUR602AC001SuggestionFromModel(t *testing.T) {
	aur602ModelFixture(t, `{"entry":["`+aur602ModelLine+`"]}`)
	code, out, errOut, summary := aur602Run(t, []changelog.Commit{{Subject: "feat: sugere entrada no check de changelog"}}, redaction.NewFilter())
	if code == 0 {
		t.Fatalf("a PR without an entry passed with a suggestion: out %q stderr %q", out, errOut)
	}
	for _, want := range []string{"reprovado (entrada_ausente)", "entrada sugerida (fonte: modelo)", "- " + aur602ModelLine} {
		if !strings.Contains(out, want) {
			t.Fatalf("output lacks %q:\n%s\nstderr: %s", want, out, errOut)
		}
	}
	if !strings.Contains(summary, "Entrada de changelog sugerida") || !strings.Contains(summary, aur602ModelLine) {
		t.Fatalf("job summary lacks the suggestion:\n%s", summary)
	}
	aur602Pasted(t, out)
}

// AC-002: without a model the suggestion comes from the commit subjects,
// and merges, history rewrites and agent-log lines never reach it.
func TestAUR602AC002DeterministicWithoutModel(t *testing.T) {
	aur602NoModel(t)
	commits := []changelog.Commit{
		{Subject: "feat: o review publica a entrada de changelog sugerida", Body: "Co-Authored-By: robo <robo@example.com>"},
		{Subject: "Merge pull request #7 from example/branch"},
		{Subject: "fixup! ajusta o texto do aviso"},
		{Subject: "chore: Generated with a coding agent tool"},
		{Subject: "test: --- PASS: TestChangelog (0.00s)"},
		{Subject: "fix: o verificador declara a primeira introducao na base"},
	}
	code, out, errOut, _ := aur602Run(t, commits, redaction.NewFilter())
	if code != 1 {
		t.Fatalf("exit %d, want 1: %s %s", code, out, errOut)
	}
	for _, want := range []string{"fonte: commits", "- o review publica a entrada de changelog sugerida", "- o verificador declara a primeira introducao na base"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output lacks %q:\n%s", want, out)
		}
	}
	for _, noise := range []string{"Merge pull request", "fixup!", "Generated with", "--- PASS", "Co-Authored-By", "robo@example.com"} {
		if strings.Contains(out, noise) {
			t.Fatalf("agent or merge noise %q reached the suggestion:\n%s", noise, out)
		}
	}
	aur602Pasted(t, out)
	// An unusable model answer falls back to the same deterministic entry.
	aur602ModelFixture(t, `{"issues":[]}`)
	if _, out, errOut, _ := aur602Run(t, commits, redaction.NewFilter()); !strings.Contains(out, "fonte: commits") || !strings.Contains(errOut, "sugestão do modelo descartada") {
		t.Fatalf("unusable model answer did not fall back: out %q stderr %q", out, errOut)
	}
}

// AC-003: the canary, mounted at run time, never reaches the log or the
// job summary, whether it came from a commit or from the model.
func TestAUR602AC003SuggestionIsRedacted(t *testing.T) {
	canary := aur602Canary(t)
	filter := redaction.NewFilter(canary)
	aur602NoModel(t)
	commits := []changelog.Commit{{Subject: "feat: rotaciona a chave " + canary + " do provedor configurado"}}
	_, out, _, summary := aur602Run(t, commits, filter)
	if !strings.Contains(out, "rotaciona a chave") {
		t.Fatalf("commit suggestion missing:\n%s", out)
	}
	if strings.Contains(out, canary) || strings.Contains(summary, canary) {
		t.Fatalf("canary leaked through the commit suggestion:\nout:\n%s\nsummary:\n%s", out, summary)
	}
	aur602ModelFixture(t, `{"entry":["O provedor passa a usar a chave `+canary+` rotacionada."]}`)
	_, out, _, summary = aur602Run(t, commits, filter)
	if !strings.Contains(out, "fonte: modelo") {
		t.Fatalf("model suggestion missing:\n%s", out)
	}
	if strings.Contains(out, canary) || strings.Contains(summary, canary) {
		t.Fatalf("canary leaked through the model suggestion:\nout:\n%s\nsummary:\n%s", out, summary)
	}
}

// AC-004: the review's PR body carries the suggestion block when the
// (central) policy requires an entry and the PR does not touch the file;
// without the requirement the block never appears.
func TestAUR602AC004ReviewBodyCarriesSuggestion(t *testing.T) {
	policy := aur509Repo(t, aur509Required)
	code, errOut, body := aur602PR(t, "", "--politica", policy)
	if code != 0 {
		t.Fatalf("exit %d stderr %s", code, errOut)
	}
	for _, want := range []string{"Suggested changelog entry", "## Unreleased", "- the review offers the changelog entry", "- the checker declares its first introduction", "```markdown"} {
		if !strings.Contains(body, want) {
			t.Fatalf("review body lacks %q:\n%s\nstderr: %s", want, body, errOut)
		}
	}
	if strings.Contains(body, "Co-Authored-By") || strings.Contains(body, "Merge pull request") {
		t.Fatalf("agent or merge noise reached the review body:\n%s", body)
	}
	code, errOut, body = aur602PR(t, "")
	if code != 0 || strings.Contains(body, "Suggested changelog entry") {
		t.Fatalf("no requirement, yet the block appeared: exit %d stderr %s\n%s", code, errOut, body)
	}
	// A PR that touches the changelog file gets no block, even when the
	// review's ignore patterns hide the file from the reviewed diff.
	touched := "diff --git a/app.go b/app.go\n@@ -1,1 +1,1 @@\n-old := 1\n+new := 2\n" +
		"diff --git a/CHANGELOG.md b/CHANGELOG.md\n@@ -3,1 +3,2 @@\n ## Unreleased\n+- O check sugere a entrada de changelog pronta.\n"
	ignoring := aur509Repo(t, aur509Required+"ignore:\n  - \"*.md\"\n")
	for name, dir := range map[string]string{"touched": policy, "touched and ignored": ignoring} {
		code, errOut, body = aur602PR(t, touched, "--politica", dir)
		if code != 0 || strings.Contains(body, "Suggested changelog entry") {
			t.Fatalf("%s: the PR touches the changelog, yet the block appeared: exit %d stderr %s\n%s", name, code, errOut, body)
		}
	}
}

// aur602Fixture is the AUR-499 GitHub fixture with commit subjects long
// enough to be changelog lines.
func aur602Fixture(t *testing.T, diffBody string, posted *githubclient.PullRequestReview) *httptest.Server {
	t.Helper()
	inner := pr499Fixture(t, true, posted)
	t.Cleanup(inner.Close)
	target, err := url.Parse(inner.URL)
	if err != nil {
		t.Fatal(err)
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case diffBody != "" && r.Method == "GET" && r.URL.Path == "/repos/team/project/pulls/7" && strings.Contains(r.Header.Get("Accept"), "diff"):
			fmt.Fprint(w, diffBody)
		case r.Method == "GET" && r.URL.Path == "/repos/team/project/pulls/7" && !strings.Contains(r.Header.Get("Accept"), "diff"):
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"title":"Merge pull request #7 from example/branch","body":"Implements the feature."}`)
		case r.Method == "GET" && r.URL.Path == "/repos/team/project/pulls/7/commits":
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `[{"sha":"aaa111","commit":{"message":"feat: the review offers the changelog entry\n\nCo-Authored-By: robo <robo@example.com>"}},{"sha":"bbb222","commit":{"message":"fix: the checker declares its first introduction"}}]`)
		default:
			proxy.ServeHTTP(w, r)
		}
	}))
}

// aur602PR is one --pr review against the AUR-499 GitHub fixture.
func aur602PR(t *testing.T, diffBody string, extra ...string) (int, string, string) {
	t.Helper()
	var posted githubclient.PullRequestReview
	server := aur602Fixture(t, diffBody, &posted)
	defer server.Close()
	fixture := filepath.Join(t.TempDir(), "response.json")
	if err := os.WriteFile(fixture, []byte(`{"issues":[],"summary":"Nothing to report."}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AURUMCODE_LLM_FIXTURE", fixture)
	t.Setenv("AURUMCODE_GITHUB_API_URL", server.URL)
	t.Setenv("AURUMCODE_PR_PERMISSION_MODE", "endpoint")
	t.Setenv("GITHUB_SHA", "new-head")
	t.Setenv("AURUMCODE_BASE_SHA", "base")
	t.Setenv("AURUMCODE_CI_CONTEXT_FILE", "")
	t.Setenv("AURUMCODE_OUTPUT_FILE", "")
	t.Setenv("AURUMCODE_POLICY", "")
	args := append([]string{"--pr", "7", "--repo", "team/project", "--publicar", "--modo-publicacao", "review"}, extra...)
	var stdout, stderr strings.Builder
	code := runReviewWith(reviewIO{stdout: &stdout, stderr: &stderr, filter: redaction.NewFilter(), deps: reviewDeps{}}, args)
	return code, stderr.String(), posted.Body
}

// AC-005: on the repository's own PR whose base predates the checker, the
// job declares the first introduction and ends without a missing-file
// error; a consumer whose pinned tool lacks it still fails.
func TestAUR602AC005FirstIntroductionOfTheChecker(t *testing.T) {
	run := aur602WorkflowRun(t)
	cases := []struct {
		name     string
		same     string
		withTool bool
		wantCode int
		wantOut  string
		wantDock bool
	}{
		{"own repository, base without checker", "true", false, 0, "::notice", false},
		{"consumer, pinned tool without checker", "false", false, 1, "::error", false},
		{"own repository, base with checker", "true", true, 0, "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ws := t.TempDir()
			if c.withTool {
				if err := os.MkdirAll(filepath.Join(ws, ".aurumcode-tool", "scripts", "ci"), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(ws, ".aurumcode-tool", "scripts", "ci", "changelog-check.sh"), []byte("exit 0\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			bin := t.TempDir()
			marker := filepath.Join(ws, "docker-called")
			if err := os.WriteFile(filepath.Join(bin, "docker"), []byte("#!/bin/sh\ntouch "+marker+"\n"), 0o755); err != nil {
				t.Fatal(err)
			}
			script := filepath.Join(t.TempDir(), "step.sh")
			if err := os.WriteFile(script, []byte(run), 0o644); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command("sh", "-e", script)
			cmd.Dir = ws
			cmd.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
				"SAME_REPOSITORY="+c.same, "GITHUB_WORKSPACE="+ws, "GITHUB_STEP_SUMMARY="+filepath.Join(ws, "summary.md"),
				"BASE_SHA=base", "HEAD_SHA=head", "POLICY_DIR=")
			out, err := cmd.CombinedOutput()
			code := 0
			if ee, ok := err.(*exec.ExitError); ok {
				code = ee.ExitCode()
			} else if err != nil {
				t.Fatal(err)
			}
			if code != c.wantCode || !strings.Contains(string(out), c.wantOut) {
				t.Fatalf("exit %d (want %d), output lacks %q:\n%s", code, c.wantCode, c.wantOut, out)
			}
			if strings.Contains(string(out), "No such file or directory") {
				t.Fatalf("missing-file error reached the job:\n%s", out)
			}
			if _, err := os.Stat(marker); (err == nil) != c.wantDock {
				t.Fatalf("docker called=%v, want %v:\n%s", err == nil, c.wantDock, out)
			}
			if c.wantOut == "::notice" && !strings.Contains(string(out), "primeira introdução") {
				t.Fatalf("notice does not explain the first introduction:\n%s", out)
			}
		})
	}
}

// aur602WorkflowRun extracts the run script of the check step from the
// changelog workflow. A missing workflow fails the test; only an explicit
// module-only acceptance (AURUMCODE_MODULE_ONLY=1) skips it.
func aur602WorkflowRun(t *testing.T) string {
	t.Helper()
	root := filepath.Join("..", "..")
	skip, err := manifestCheckSkipped(root, os.Getenv)
	if err != nil {
		t.Fatal(err)
	}
	if skip {
		t.Skip("AURUMCODE_MODULE_ONLY=1: the acceptance staged only the Go module")
	}
	data, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "changelog.yml"))
	if err != nil {
		t.Fatalf("changelog workflow unreadable: %v", err)
	}
	var wf struct {
		Jobs map[string]struct {
			Steps []struct {
				Name string `yaml:"name"`
				Uses string `yaml:"uses"`
				Run  string `yaml:"run"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(data, &wf); err != nil {
		t.Fatal(err)
	}
	const pin = "actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1"
	var run string
	for _, s := range wf.Jobs["changelog"].Steps {
		if strings.HasPrefix(s.Uses, "actions/checkout@") && s.Uses != pin {
			t.Fatalf("checkout %q is not the pinned v7.0.1 %s", s.Uses, pin)
		}
		if s.Name == "Check the changelog entry" {
			run = s.Run
		}
	}
	if run == "" {
		t.Fatal("check step not found in the changelog workflow")
	}
	return run
}
