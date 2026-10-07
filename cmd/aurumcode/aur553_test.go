package main

// Proofs that every term of the gate-verdict key is load-bearing on its own,
// and that a finding read back from the cache never reaches a sink without
// the model-output redaction. Each test isolates ONE term: the provider
// keeps a fixed identity (an alternating LiteLLM-compatible server, never a
// fixture file whose content is itself a key term) and only the term under
// test moves between rounds.

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	igate "github.com/Mpaape/AurumCode/internal/gate"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
)

const (
	// aur553RegisteredCanary is a secret the filter knows by value (the
	// AURUM_SECRET_CANARY route); aur553ShapeCanary is one it knows by
	// shape. Neither may ever leave the process.
	aur553RegisteredCanary = "aur553-registered-canary-7f3e1d"
	aur553ShapeCanary      = "ghp_AUR553shapeCanary0123456789abcdefXYZ"
	aur553HeaderCanary     = "aur553-header-canary-b41c"
	// aur553ForgedMarker is ordinary text of the forged finding: it must
	// reach the sink, proving the forged entry was really reused.
	aur553ForgedMarker = "aur553-forged-finding-marker"

	aur553PRDiff      = "diff --git a/app.go b/app.go\n@@ -1,2 +1,4 @@\n package demo\n+func Change() {\n+ total := 41\n+ _ = total\n+}\n"
	aur553PRDiffOther = "diff --git a/app.go b/app.go\n@@ -1,2 +1,4 @@\n package demo\n+func Change() {\n+ total := 42\n+ _ = total\n+}\n"
)

// aur553ForgedVerdict is a stored verdict another workflow of the same
// cache scope could have written: one finding whose prose carries every
// canary, one of them as a quoted diff line with its marker.
func aur553ForgedVerdict(path string) []byte {
	return []byte(fmt.Sprintf(`{"path":%q,"issues":[{"file":"app.go","line":3,"severity":"error","rule_id":"security#no-hardcoded-secrets","message":"%s token %s and %s","evidence":"+Authorization: Bearer %s","impact":"leak %s","verification":"check"}]}`,
		path, aur553ForgedMarker, aur553RegisteredCanary, aur553ShapeCanary, aur553HeaderCanary, aur553ShapeCanary))
}

// aur553ForgeEntries overwrites every stored entry whose Path satisfies
// match with the forged verdict.
func aur553ForgeEntries(t *testing.T, cacheDir string, match func(string) bool) int {
	t.Helper()
	forged := 0
	for name, path := range aur524CacheEntryPaths(t, cacheDir) {
		if !match(path) {
			continue
		}
		if err := os.WriteFile(filepath.Join(cacheDir, name), aur553ForgedVerdict(path), 0600); err != nil {
			t.Fatal(err)
		}
		forged++
	}
	if forged == 0 {
		t.Fatal("no stored entry to forge: the first round stored nothing")
	}
	return forged
}

// aur553AssertNoCanary fails when any canary appears in text.
func aur553AssertNoCanary(t *testing.T, sink, text string) {
	t.Helper()
	for _, canary := range []string{aur553RegisteredCanary, aur553ShapeCanary, aur553HeaderCanary} {
		if strings.Contains(text, canary) {
			t.Errorf("%s leaked the cached canary %q:\n%s", sink, canary, text)
		}
	}
}

// aur553GitHub is a GitHub stub for --pr whose diff can change between
// rounds and which records every published body.
type aur553GitHub struct {
	*httptest.Server
	mu        sync.Mutex
	diff      string
	published []string
}

func (g *aur553GitHub) setDiff(diff string) { g.mu.Lock(); g.diff = diff; g.mu.Unlock() }

func (g *aur553GitHub) bodies() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return strings.Join(g.published, "\n")
}

func newAUR553GitHub(t *testing.T, repoConfig func() string) *aur553GitHub {
	t.Helper()
	g := &aur553GitHub{diff: aur553PRDiff}
	g.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			body, _ := io.ReadAll(r.Body)
			g.mu.Lock()
			g.published = append(g.published, string(body))
			g.mu.Unlock()
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/owner/repo":
			_, _ = w.Write([]byte(`{"permissions":{"push":true}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/repos/owner/repo/pulls/48" && r.Header.Get("Accept") == "application/vnd.github.v3.diff":
			g.mu.Lock()
			_, _ = w.Write([]byte(g.diff))
			g.mu.Unlock()
		case r.Method == http.MethodGet && r.URL.Path == "/repos/owner/repo/pulls/48":
			_, _ = fmt.Fprint(w, `{"head":{"sha":"head-sha"}}`)
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/contents/") && strings.Contains(r.URL.Path, "config.yml"):
			_, _ = fmt.Fprintf(w, `{"content":%q,"encoding":"base64"}`, base64.StdEncoding.EncodeToString([]byte(repoConfig())))
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/contents/") && strings.Contains(r.URL.Path, "security.md"):
			_, _ = fmt.Fprintf(w, `{"content":%q,"encoding":"base64"}`, base64.StdEncoding.EncodeToString([]byte(aur524SkillBody)))
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/contents/"):
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodGet && (strings.HasSuffix(r.URL.Path, "/reviews") || strings.HasSuffix(r.URL.Path, "/comments") || strings.HasSuffix(r.URL.Path, "/commits")):
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/reviews"):
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":1}`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/comments"):
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":2}`))
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/repos/owner/repo/statuses/"):
			w.WriteHeader(http.StatusCreated)
		default:
			t.Errorf("unexpected GitHub request: %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(g.Close)
	return g
}

// aur553PREnv points a --pr run at gh and at a fixed-identity provider
// answering bodies in turn. GITHUB_SHA is left empty: only the diff digest
// names the reviewed content, and the configuration (gate and skill) is the
// local checkout's, returned as dir.
func aur553PREnv(t *testing.T, gh *aur553GitHub, bodies ...string) (dir string, calls *int32) {
	t.Helper()
	dir = cleanFixture(t, aur524GateConfig)
	aur524WriteSkill(t, dir, aur524SkillBody)
	llm, calls := aur524AlternatingLLMServer(t, bodies)
	t.Cleanup(llm.Close)
	t.Setenv("AURUMCODE_LLM_FIXTURE", "")
	t.Setenv("LLM_API_KEY", "test-key")
	t.Setenv("LLM_BASE_URL", llm.URL)
	t.Setenv("LLM_MODEL", "model-x")
	t.Setenv("AURUMCODE_GITHUB_API_URL", gh.URL)
	t.Setenv("AURUMCODE_PR_PERMISSION_MODE", "endpoint")
	t.Setenv("GITHUB_SHA", "")
	t.Setenv("AURUMCODE_BASE_SHA", "base")
	t.Setenv("AURUMCODE_CI_CONTEXT_FILE", "")
	return dir, calls
}

// aur553RunPR runs one publishing --pr review of PR 48 with filter.
func aur553RunPR(t *testing.T, filter *redaction.Filter) (int, string, string) {
	t.Helper()
	var out, errOut strings.Builder
	opts := prReviewOptions{publicationSet: true, publication: "review"}
	opts.prNumber, opts.repo, opts.publicar, opts.naLinha, opts.check = 48, "owner/repo", true, false, false
	code := runPRReview(reviewIO{stdout: &out, stderr: &errOut, filter: filter}, opts)
	return code, out.String(), errOut.String()
}

// aur553RunBase runs one --base HEAD~1 review in the current fixture.
func aur553RunBase(t *testing.T, io reviewIO, extra ...string) (int, string, string) {
	t.Helper()
	var out, errOut strings.Builder
	io.stdout, io.stderr = &out, &errOut
	if io.filter == nil {
		io.filter = redaction.NewFilter()
	}
	code := runReviewWith(io, append([]string{"--base", "HEAD~1"}, extra...))
	return code, out.String(), errOut.String()
}

// aur553BaseEnv prepares a --base fixture with the gate and a fixed-
// identity provider answering bodies in turn.
func aur553BaseEnv(t *testing.T, bodies ...string) (dir, cacheDir string, calls *int32) {
	t.Helper()
	dir = cleanFixture(t, aur524GateConfig)
	aur524WriteSkill(t, dir, aur524SkillBody)
	srv, calls := aur524AlternatingLLMServer(t, bodies)
	t.Cleanup(srv.Close)
	t.Setenv("LLM_API_KEY", "test-key")
	t.Setenv("LLM_BASE_URL", srv.URL)
	t.Setenv("LLM_MODEL", "model-x")
	return dir, os.Getenv("AURUMCODE_CACHE_DIR"), calls
}

// aur553RequireFreshPass is the shared assertion of a round whose key term
// moved: the stored breach must not be reapplied onto a clean answer.
func aur553RequireFreshPass(t *testing.T, term string, code int, stdout, stderr string) {
	t.Helper()
	if code != 0 {
		t.Fatalf("round2 exit=%d, want 0: a changed %s must never reuse round1's stored breach; stdout=%s stderr=%s", code, term, stdout, stderr)
	}
	if strings.Contains(stderr, "reaplicado") {
		t.Fatalf("round2 must not reapply a finding after the %s changed:\n%s", term, stderr)
	}
}

// TestAUR553AC004PRCachedFindingRedacted: a forged verdict entry carrying
// secret canaries is reused on a --pr run; the review published to GitHub
// carries the forged finding (it was reused) but none of the canaries.
func TestAUR553AC004PRCachedFindingRedacted(t *testing.T) {
	gh := newAUR553GitHub(t, func() string { return aur524GateConfig })
	aur553PREnv(t, gh, aur524CleanResp)
	cacheDir := os.Getenv("AURUMCODE_CACHE_DIR")
	filter := redaction.NewFilter(aur553RegisteredCanary)

	if code, out, errOut := aur553RunPR(t, filter); code != 0 {
		t.Fatalf("round1 exit=%d, want 0; stdout=%s stderr=%s", code, out, errOut)
	}
	aur553ForgeEntries(t, cacheDir, func(p string) bool { return p == igate.VerdictCachePath })

	code, out, errOut := aur553RunPR(t, filter)
	if code != exitFindings {
		t.Fatalf("round2 exit=%d, want exitFindings(%d): the forged finding must be reused; stdout=%s stderr=%s", code, exitFindings, out, errOut)
	}
	published := gh.bodies()
	if !strings.Contains(published, aur553ForgedMarker) {
		t.Fatalf("the forged finding never reached the published review, the proof is vacuous:\n%s", published)
	}
	aur553AssertNoCanary(t, "published PR review", published)
	aur553AssertNoCanary(t, "stdout", out)
	aur553AssertNoCanary(t, "stderr", errOut)
	if !strings.Contains(published, redaction.Marker) {
		t.Fatalf("the published review shows no redaction marker:\n%s", published)
	}
}

// TestAUR553AC004BaseCachedFileFindingRedacted: the per-file review cache
// of --base is the same untrusted directory; a forged per-file entry is
// served redacted.
func TestAUR553AC004BaseCachedFileFindingRedacted(t *testing.T) {
	_, cacheDir, calls := aur553BaseEnv(t, aur524CleanResp)
	filter := redaction.NewFilter(aur553RegisteredCanary)
	if code, out, errOut := aur553RunBase(t, reviewIO{filter: filter}); code != 0 {
		t.Fatalf("round1 exit=%d, want 0; stdout=%s stderr=%s", code, out, errOut)
	}
	aur553ForgeEntries(t, cacheDir, func(p string) bool { return p == "app.go" })

	code, out, errOut := aur553RunBase(t, reviewIO{filter: filter})
	if atomic.LoadInt32(calls) != 1 {
		t.Fatalf("round2 must be served from the per-file cache, got %d provider calls", atomic.LoadInt32(calls))
	}
	if !strings.Contains(out, aur553ForgedMarker) {
		t.Fatalf("the forged per-file finding was not served (exit %d), the proof is vacuous:\nstdout=%s\nstderr=%s", code, out, errOut)
	}
	aur553AssertNoCanary(t, "stdout", out)
	aur553AssertNoCanary(t, "stderr", errOut)
}

// TestAUR553AC003RepoIdentityChangeInvalidatesReuse: the same content
// reviewed in a checkout of another repository (origin remote) must not
// reuse the first repository's verdict.
func TestAUR553AC003RepoIdentityChangeInvalidatesReuse(t *testing.T) {
	dir, cacheDir, _ := aur553BaseEnv(t, aur524BreachResp, aur524CleanResp)
	setOrigin := func(repo string) {
		t.Helper()
		cfg := "[core]\nrepositoryformatversion = 0\nbare = false\n[remote \"origin\"]\nurl = https://github.com/owner/" + repo + ".git\n"
		if err := os.WriteFile(filepath.Join(dir, ".git", "config"), []byte(cfg), 0600); err != nil {
			t.Fatal(err)
		}
	}
	setOrigin("repo-a")
	if code, out, errOut := aur553RunBase(t, reviewIO{}); code != exitFindings {
		t.Fatalf("round1 exit=%d, want exitFindings(%d); stdout=%s stderr=%s", code, exitFindings, out, errOut)
	}
	aur524DeleteNonVerdictEntries(t, cacheDir)
	setOrigin("repo-b")
	code, out, errOut := aur553RunBase(t, reviewIO{})
	aur553RequireFreshPass(t, "repository identity", code, out, errOut)
}

// TestAUR553AC003BinaryIdentityChangeInvalidatesReuse: a verdict stored by
// another build of the binary (main.version) is never reused.
func TestAUR553AC003BinaryIdentityChangeInvalidatesReuse(t *testing.T) {
	_, cacheDir, _ := aur553BaseEnv(t, aur524BreachResp, aur524CleanResp)
	saved := version
	t.Cleanup(func() { version = saved })
	version = "aur553-build-a"
	if code, out, errOut := aur553RunBase(t, reviewIO{}); code != exitFindings {
		t.Fatalf("round1 exit=%d, want exitFindings(%d); stdout=%s stderr=%s", code, exitFindings, out, errOut)
	}
	aur524DeleteNonVerdictEntries(t, cacheDir)
	version = "aur553-build-b"
	code, out, errOut := aur553RunBase(t, reviewIO{})
	aur553RequireFreshPass(t, "binary identity", code, out, errOut)
}

// TestAUR553AC002DiffChangeWithoutSHAInvalidatesReuse: without GITHUB_SHA
// the diff digest alone names the reviewed content; another diff of the
// same PR is reviewed fresh, never under the first diff's verdict.
func TestAUR553AC002DiffChangeWithoutSHAInvalidatesReuse(t *testing.T) {
	gh := newAUR553GitHub(t, func() string { return aur524GateConfig })
	_, calls := aur553PREnv(t, gh, aur524BreachResp, aur524CleanResp)
	if code, out, errOut := aur553RunPR(t, redaction.NewFilter()); code != exitFindings {
		t.Fatalf("round1 exit=%d, want exitFindings(%d); stdout=%s stderr=%s", code, exitFindings, out, errOut)
	}
	gh.setDiff(aur553PRDiffOther)
	code, out, errOut := aur553RunPR(t, redaction.NewFilter())
	if atomic.LoadInt32(calls) != 2 {
		t.Fatalf("round2 must call the provider again, got %d calls", atomic.LoadInt32(calls))
	}
	aur553RequireFreshPass(t, "diff", code, out, errOut)
}

// TestAUR553AC003PRStoresRawIssues: on --pr the stored verdict is the RAW
// finding set, before the rule config: a breach found while its rule was
// disabled is stored and fires once the rule is enabled again.
func TestAUR553AC003PRStoresRawIssues(t *testing.T) {
	const disabledConfig = aur524GateConfig + "rules:\n  security#no-hardcoded-secrets:\n    enabled: false\n"
	gh := newAUR553GitHub(t, func() string { return aur524GateConfig })
	dir, _ := aur553PREnv(t, gh, aur524BreachResp, aur524CleanResp)
	setConfig := func(body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, ".aurumcode", "config.yml"), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	setConfig(disabledConfig)
	if code, out, errOut := aur553RunPR(t, redaction.NewFilter()); code != 0 {
		t.Fatalf("round1 (rule disabled) exit=%d, want 0; stdout=%s stderr=%s", code, out, errOut)
	}
	setConfig(aur524GateConfig)
	code, out, errOut := aur553RunPR(t, redaction.NewFilter())
	if code != exitFindings {
		t.Fatalf("round2 (rule enabled) exit=%d, want exitFindings(%d): the raw finding stored by round1 must fire under the enabled rule; stdout=%s stderr=%s", code, exitFindings, out, errOut)
	}
	if !strings.Contains(errOut, "reaplicado") {
		t.Fatalf("round2 must announce the reapplied raw finding:\n%s", errOut)
	}
}
