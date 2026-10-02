package main

// AUR-524 behavior proof: the gate verdict for the same reviewed SHA/diff,
// the same central policy, the same repo context/skills and the same
// model must not flip between two runs -- including between a --base run
// and a --pr run (aur524.go, wired into runReview and runPRReview). Each
// test here fails if the behavior it names is removed.
//
// AC-001 needs a provider whose ANSWER changes per call while its
// IDENTITY stays fixed -- unlike AURUMCODE_LLM_FIXTURE, whose file content
// is itself folded into modelCacheKey (two different canned responses are,
// by that function's own doc, two different "models"), so overwriting a
// fixture between rounds would always look like a model change and could
// never isolate this card's own reuse decision. aur524AlternatingLLMServer
// stands in for a LiteLLM-compatible endpoint (LLM_BASE_URL/LLM_API_KEY/
// LLM_MODEL, exactly like TestAUR513ModelIdentitySurvivesContextWrapping's
// own second half) and answers call N with bodies[N-1], clamped to the
// last body once calls exceed len(bodies) -- same identity, different
// answer per call, exactly what AC-001 needs.

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Mpaape/AurumCode/internal/review"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
)

const (
	aur524SkillBody  = "## No Hardcoded Secrets\n\nNever commit a literal credential.\n"
	aur524CleanResp  = `{"summary":"ok","verdict":"approve","issues":[]}`
	aur524BreachResp = `{"summary":"ok","verdict":"approve","issues":[{"file":"app.go","line":3,"severity":"error","rule_id":"security#no-hardcoded-secrets","message":"Hardcoded secret","evidence":"dbPassword := \"hunter2-super-secret\"","impact":"Credential leak","verification":"Remove the literal secret"}]}`
	aur524GateConfig = "review:\n  context:\n    skills:\n      - skills/security.md\ngate:\n  fail_on: [high]\n"
)

func aur524WriteFixture(t *testing.T, body string) string {
	t.Helper()
	fixture := filepath.Join(t.TempDir(), "response.json")
	if err := os.WriteFile(fixture, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	return fixture
}

func aur524WriteSkill(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, "skills"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "skills", "security.md"), []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
}

// aur524AlternatingLLMServer answers call N with bodies[N-1] (clamped to
// the last body), with a FIXED model identity across every call -- only
// the content field (the review JSON the prompt layer parses) changes.
func aur524AlternatingLLMServer(t *testing.T, bodies []string) (*httptest.Server, *int32) {
	t.Helper()
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := int(atomic.AddInt32(&calls, 1))
		idx := n - 1
		if idx >= len(bodies) {
			idx = len(bodies) - 1
		}
		content, err := json.Marshal(bodies[idx])
		if err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"model":"m","choices":[{"finish_reason":"stop","message":{"role":"assistant","content":%s}}]}`, content)
	}))
	return srv, &calls
}

// TestAUR524AC001VerdictReusedAcrossRunsSameInputs covers AC-001: a
// provider that alternates its answer (clean, then a real breach) under
// the identical reviewed SHA, policy, context and model must still have
// its SECOND run's gate verdict read exactly like the first's -- the
// cached, clean verdict, not the alternated breach the provider actually
// returned on call 2.
func TestAUR524AC001VerdictReusedAcrossRunsSameInputs(t *testing.T) {
	dir := cleanFixture(t, aur524GateConfig)
	aur524WriteSkill(t, dir, aur524SkillBody)
	srv, calls := aur524AlternatingLLMServer(t, []string{aur524CleanResp, aur524BreachResp})
	defer srv.Close()
	t.Setenv("AURUMCODE_LLM_FIXTURE", "")
	t.Setenv("LLM_API_KEY", "test-key")
	t.Setenv("LLM_BASE_URL", srv.URL)
	t.Setenv("LLM_MODEL", "model-x")

	var out1, err1 strings.Builder
	code1 := runReview([]string{"--base", "HEAD~1"}, &out1, &err1, redaction.NewFilter())
	if code1 != 0 {
		t.Fatalf("round1 exit=%d, want 0 (clean); stdout=%s stderr=%s", code1, out1.String(), err1.String())
	}
	if atomic.LoadInt32(calls) != 1 {
		t.Fatalf("round1 must call the provider once, got %d calls", atomic.LoadInt32(calls))
	}

	var out2, err2 strings.Builder
	code2 := runReview([]string{"--base", "HEAD~1"}, &out2, &err2, redaction.NewFilter())
	if code2 != 0 {
		t.Fatalf("round2 exit=%d, want 0: the gate verdict must reuse round1's clean result, never round2's alternated breach; stdout=%s stderr=%s", code2, out2.String(), err2.String())
	}
	if !strings.Contains(err2.String(), "gate verdict reused from cache") {
		t.Fatalf("expected round2 to announce the reused verdict:\n%s", err2.String())
	}
	if strings.Contains(out2.String(), "security#no-hardcoded-secrets") {
		t.Fatalf("round2's reused verdict must not surface the alternated breach:\n%s", out2.String())
	}
}

// TestAUR524AC002SkillChangeInvalidatesReuse covers AC-002: changing a
// repo skill's content between two otherwise identical runs invalidates
// reuse -- round2 must compute its own fresh (breached) verdict instead of
// reusing round1's clean one.
func TestAUR524AC002SkillChangeInvalidatesReuse(t *testing.T) {
	dir := cleanFixture(t, aur524GateConfig)
	aur524WriteSkill(t, dir, aur524SkillBody)
	srv, _ := aur524AlternatingLLMServer(t, []string{aur524CleanResp, aur524BreachResp})
	defer srv.Close()
	t.Setenv("AURUMCODE_LLM_FIXTURE", "")
	t.Setenv("LLM_API_KEY", "test-key")
	t.Setenv("LLM_BASE_URL", srv.URL)
	t.Setenv("LLM_MODEL", "model-x")

	var out1, err1 strings.Builder
	if code := runReview([]string{"--base", "HEAD~1"}, &out1, &err1, redaction.NewFilter()); code != 0 {
		t.Fatalf("round1 exit=%d, want 0; stdout=%s stderr=%s", code, out1.String(), err1.String())
	}

	// Visibly different skill content (AC-002's "uma skill do repo"):
	// contextBlockDigest changes, so the key changes even though the
	// SHA/diff, policy and model did not.
	aur524WriteSkill(t, dir, aur524SkillBody+"\nguideline-zz-marker-71c3\n")

	var out2, err2 strings.Builder
	code2 := runReview([]string{"--base", "HEAD~1"}, &out2, &err2, redaction.NewFilter())
	if code2 != exitFindings {
		t.Fatalf("round2 exit=%d, want exitFindings(%d): a changed skill must never reuse round1's clean verdict; stdout=%s stderr=%s", code2, exitFindings, out2.String(), err2.String())
	}
	if strings.Contains(err2.String(), "gate verdict reused from cache") {
		t.Fatalf("round2 must not claim a reused verdict after the skill changed:\n%s", err2.String())
	}
}

// TestAUR524AC002ModelChangeInvalidatesReuse is AC-002's "o modelo"
// counterpart: switching LLM_MODEL between two otherwise identical runs
// invalidates reuse.
func TestAUR524AC002ModelChangeInvalidatesReuse(t *testing.T) {
	cleanFixture(t, aur524GateConfig)
	// No skill declared for this one: fail_on alone with zero dynamic
	// rules never breaches, so the only thing this test needs to prove is
	// that the SECOND model never prints the reused-verdict line.
	t.Setenv("AURUMCODE_LLM_FIXTURE", aur524WriteFixture(t, aur524CleanResp))
	t.Setenv("LLM_API_KEY", "")
	t.Setenv("LLM_BASE_URL", "")
	t.Setenv("LLM_MODEL", "")

	var out1, err1 strings.Builder
	if code := runReview([]string{"--base", "HEAD~1", "--modelo", "model-a"}, &out1, &err1, redaction.NewFilter()); code != 0 {
		t.Fatalf("round1 exit=%d, want 0; stdout=%s stderr=%s", code, out1.String(), err1.String())
	}

	var out2, err2 strings.Builder
	code2 := runReview([]string{"--base", "HEAD~1", "--modelo", "model-b"}, &out2, &err2, redaction.NewFilter())
	if code2 != 0 {
		t.Fatalf("round2 exit=%d, want 0; stdout=%s stderr=%s", code2, out2.String(), err2.String())
	}
	if strings.Contains(err2.String(), "gate verdict reused from cache") {
		t.Fatalf("a different --modelo must never reuse the previous model's verdict:\n%s", err2.String())
	}
}

// TestAUR524AC003InconclusiveNeverStoredOrReused covers AC-003: a provider
// that fails at the transport level (an unreadable, configured fixture --
// AUR-458's qualityFailed, not qualitySkipped) under gate.inconclusive:
// block must never claim a reused verdict, and must never leave behind a
// stored one either -- proven by a second, fully working round under the
// identical key finding nothing to reuse.
func TestAUR524AC003InconclusiveNeverStoredOrReused(t *testing.T) {
	cleanFixture(t, "gate:\n  inconclusive: block\n")
	t.Setenv("AURUMCODE_LLM_FIXTURE", filepath.Join(t.TempDir(), "missing-response.json"))

	var out1, err1 strings.Builder
	code1 := runReview([]string{"--base", "HEAD~1", "--seguranca"}, &out1, &err1, redaction.NewFilter())
	if code1 != exitQualityNotReviewed {
		t.Fatalf("round1 exit=%d, want exitQualityNotReviewed(%d); stdout=%s stderr=%s", code1, exitQualityNotReviewed, out1.String(), err1.String())
	}
	if strings.Contains(err1.String(), "gate verdict reused from cache") {
		t.Fatalf("an inconclusive run must never claim a reused verdict:\n%s", err1.String())
	}

	cacheDir := os.Getenv("AURUMCODE_CACHE_DIR")
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		t.Fatalf("reading cache dir: %v", err)
	}
	for _, e := range entries {
		data, rerr := os.ReadFile(filepath.Join(cacheDir, e.Name()))
		if rerr != nil {
			continue
		}
		if strings.Contains(string(data), gateVerdictCachePath) {
			t.Fatalf("an inconclusive review must never be persisted as a reusable gate verdict (entry %s): %s", e.Name(), data)
		}
	}

	t.Setenv("AURUMCODE_LLM_FIXTURE", aur524WriteFixture(t, aur524CleanResp))
	var out2, err2 strings.Builder
	code2 := runReview([]string{"--base", "HEAD~1", "--seguranca"}, &out2, &err2, redaction.NewFilter())
	if code2 != 0 {
		t.Fatalf("round2 exit=%d, want 0; stdout=%s stderr=%s", code2, out2.String(), err2.String())
	}
	if strings.Contains(err2.String(), "gate verdict reused from cache") {
		t.Fatalf("round2 must compute its own fresh verdict: round1 stored nothing to reuse:\n%s", err2.String())
	}
}

// TestAUR524AC004NoCacheDirDeclaresUnavailable covers AC-004: without
// AURUMCODE_CACHE_DIR set, a run with a gate declared says plainly, on
// stderr and in the published limitations, that gate-verdict reuse is
// unavailable -- never silently skipped with no trace.
func TestAUR524AC004NoCacheDirDeclaresUnavailable(t *testing.T) {
	cleanFixture(t, "gate:\n  fail_on: [error]\n")
	t.Setenv("AURUMCODE_CACHE_DIR", "")
	t.Setenv("AURUMCODE_LLM_FIXTURE", aur524WriteFixture(t, aur524CleanResp))

	var out, errOut strings.Builder
	code := runReview([]string{"--base", "HEAD~1"}, &out, &errOut, redaction.NewFilter())
	if code != 0 {
		t.Fatalf("exit=%d, want 0; stdout=%s stderr=%s", code, out.String(), errOut.String())
	}
	if !strings.Contains(errOut.String(), gateVerdictCacheUnavailableNotice) {
		t.Fatalf("expected AC-004's own declaration that reuse is unavailable:\n%s", errOut.String())
	}
}

// TestAUR524KeyChangesWithPolicyDigest is MUT-001's own unit anchor: two
// otherwise identical gateVerdictCacheKey calls that differ ONLY in the
// policy digest argument must produce different keys. A mutation that
// drops the policyDigest term from the combination (MUT-001) makes this
// assertion -- and AC-002's own behavioral test under a central policy --
// fail.
func TestAUR524KeyChangesWithPolicyDigest(t *testing.T) {
	p := &review.FakeProvider{NameStr: "fixture-identity"}
	k1 := gateVerdictCacheKey(p, "base-id", "pt-BR", "codebase", "notes", "", "context-digest", "catalog-digest", "policy-A", "sha:abc123")
	k2 := gateVerdictCacheKey(p, "base-id", "pt-BR", "codebase", "notes", "", "context-digest", "catalog-digest", "policy-B", "sha:abc123")
	if k1 == k2 {
		t.Fatal("gateVerdictCacheKey must change when the policy digest changes (AC-002/MUT-001)")
	}
	k3 := gateVerdictCacheKey(p, "base-id", "pt-BR", "codebase", "notes", "", "context-digest", "catalog-digest", "policy-A", "sha:def456")
	if k1 == k3 {
		t.Fatal("gateVerdictCacheKey must change when the reviewed identity changes")
	}
}

// aur524PRGateServer is this card's own GitHub httptest stub, mirroring
// aur519_e2e_test.go's runPRGateMockServer: a diff, one repo config.yml
// (base64, via the contents API), one configured skill, no other context
// files (404, zero-config), and an accepted formal review POST. The LLM
// side is NOT this server -- see aur524AlternatingLLMServer -- so the
// provider's identity (LLM_BASE_URL) stays fixed across calls while only
// its answer alternates, exactly like AC-001's --base test above.
func aur524PRGateServer(t *testing.T, diffBody, repoConfig, skillBody string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/owner/repo":
			_, _ = w.Write([]byte(`{"permissions":{"push":true}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/repos/owner/repo/pulls/48" && r.Header.Get("Accept") == "application/vnd.github.v3.diff":
			_, _ = w.Write([]byte(diffBody))
		case r.Method == http.MethodGet && r.URL.Path == "/repos/owner/repo/pulls/48":
			_, _ = fmt.Fprint(w, `{"head":{"sha":"head-sha"}}`)
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/contents/") && strings.Contains(r.URL.Path, "config.yml"):
			_, _ = fmt.Fprintf(w, `{"content":%q,"encoding":"base64"}`, base64.StdEncoding.EncodeToString([]byte(repoConfig)))
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/contents/") && strings.Contains(r.URL.Path, "security.md"):
			_, _ = fmt.Fprintf(w, `{"content":%q,"encoding":"base64"}`, base64.StdEncoding.EncodeToString([]byte(skillBody)))
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/contents/"):
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodGet && (strings.HasSuffix(r.URL.Path, "/reviews") || strings.HasSuffix(r.URL.Path, "/comments") || strings.HasSuffix(r.URL.Path, "/commits")):
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/reviews"):
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":1}`))
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/repos/owner/repo/statuses/"):
			w.WriteHeader(http.StatusCreated)
		default:
			t.Errorf("unexpected GitHub request: %s %s", r.Method, r.URL.Path)
		}
	}))
}

// TestAUR524AC001PRVerdictReusedAcrossRuns is AC-001's --pr counterpart:
// the same alternating-answer, fixed-identity provider, the same reviewed
// SHA (GITHUB_SHA, read identically to --base's own GITHUB_SHA), the same
// policy-free repo config and skill -- round2 must read exactly round1's
// clean verdict, never the alternated breach.
func TestAUR524AC001PRVerdictReusedAcrossRuns(t *testing.T) {
	const diffBody = "diff --git a/app.go b/app.go\n@@ -1,2 +1,3 @@\n package demo\n+func Change() int { return 1 }\n"
	gh := aur524PRGateServer(t, diffBody, aur524GateConfig, aur524SkillBody)
	defer gh.Close()
	llm, calls := aur524AlternatingLLMServer(t, []string{aur524CleanResp, aur524BreachResp})
	defer llm.Close()

	t.Setenv("AURUMCODE_LLM_FIXTURE", "")
	t.Setenv("LLM_API_KEY", "test-key")
	t.Setenv("LLM_BASE_URL", llm.URL)
	t.Setenv("LLM_MODEL", "model-x")
	t.Setenv("AURUMCODE_GITHUB_API_URL", gh.URL)
	t.Setenv("AURUMCODE_PR_PERMISSION_MODE", "endpoint")
	t.Setenv("GITHUB_SHA", "head-sha")
	t.Setenv("AURUMCODE_BASE_SHA", "base")
	t.Setenv("AURUMCODE_CI_CONTEXT_FILE", "")
	t.Setenv("AURUMCODE_CACHE_DIR", t.TempDir())

	opts := prReviewOptions{publicationSet: true, publication: "review"}

	var out1, err1 strings.Builder
	code1 := runPRReview(&out1, &err1, 48, "owner/repo", true, true, false, redaction.NewFilter(), opts)
	if code1 != 0 {
		t.Fatalf("round1 exit=%d, want 0 (clean); stdout=%s stderr=%s", code1, out1.String(), err1.String())
	}
	if atomic.LoadInt32(calls) != 1 {
		t.Fatalf("round1 must call the provider once, got %d calls", atomic.LoadInt32(calls))
	}

	var out2, err2 strings.Builder
	code2 := runPRReview(&out2, &err2, 48, "owner/repo", true, true, false, redaction.NewFilter(), opts)
	if code2 != 0 {
		t.Fatalf("round2 exit=%d, want 0: the --pr gate verdict must reuse round1's clean result; stdout=%s stderr=%s", code2, out2.String(), err2.String())
	}
	if !strings.Contains(err2.String(), "gate verdict reused from cache") {
		t.Fatalf("expected round2 to announce the reused verdict:\n%s", err2.String())
	}
}
