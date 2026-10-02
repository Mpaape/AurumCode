package main

// AUR-524 v2 behavior proof: a concluded gate verdict is reused
// MONOTONICALLY -- a stored entry can only ADD findings to what a later
// run finds on its own, never replace or suppress them (CR-TRUST-001:
// AURUMCODE_CACHE_DIR is untrusted input in CI). Each test here fails if
// the behavior it names is removed.
//
// AC-001 needs a provider whose ANSWER changes per call while its
// IDENTITY stays fixed -- unlike AURUMCODE_LLM_FIXTURE, whose file content
// is itself folded into modelCacheKey. aur524AlternatingLLMServer stands
// in for a LiteLLM-compatible endpoint and answers call N with
// bodies[N-1] (clamped to the last body) -- same identity, different
// answer per call.
//
// --base tests that repeat the SAME diff across rounds must isolate
// AUR-524's own verdict entry from AUR-513's per-file cache (which lives
// in the SAME directory): a per-file hit would skip the model call
// entirely and confound "the provider was called again". Between such
// rounds, aur524DeleteNonVerdictEntries removes every entry whose stored
// cache.Entry.Path is not "aur524-gate-verdict", forcing a fresh model
// call while leaving the verdict entry itself untouched. --pr tests need
// no such isolation: runPRReview never uses the per-file cache at all.

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

	"github.com/Mpaape/AurumCode/internal/prompt"
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
// the last body), with a FIXED model identity across every call.
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

// aur524CacheEntryPaths maps every *.json entry's filename in cacheDir to
// its stored cache.Entry.Path field (unparseable files map to "").
func aur524CacheEntryPaths(t *testing.T, cacheDir string) map[string]string {
	t.Helper()
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		t.Fatal(err)
	}
	out := make(map[string]string)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(cacheDir, e.Name()))
		if err != nil {
			continue
		}
		var parsed struct {
			Path string `json:"path"`
		}
		_ = json.Unmarshal(data, &parsed)
		out[e.Name()] = parsed.Path
	}
	return out
}

// aur524DeleteNonVerdictEntries removes every AUR-513 per-file entry,
// isolating AUR-524's own verdict entry from that unrelated cache.
func aur524DeleteNonVerdictEntries(t *testing.T, cacheDir string) {
	t.Helper()
	for name, path := range aur524CacheEntryPaths(t, cacheDir) {
		if path != gateVerdictCachePath {
			_ = os.Remove(filepath.Join(cacheDir, name))
		}
	}
}

// aur524DeleteVerdictEntries removes AUR-524's own entries (used as a
// negative control: nothing left to reuse).
func aur524DeleteVerdictEntries(t *testing.T, cacheDir string) {
	t.Helper()
	for name, path := range aur524CacheEntryPaths(t, cacheDir) {
		if path == gateVerdictCachePath {
			_ = os.Remove(filepath.Join(cacheDir, name))
		}
	}
}

// aur524ForgeVerdictEntriesEmpty simulates AC-005's attack: another
// workflow in the same untrusted cache scope overwrites the verdict
// entry with a forged, empty issue list.
func aur524ForgeVerdictEntriesEmpty(t *testing.T, cacheDir string) {
	t.Helper()
	forged := []byte(`{"path":"` + gateVerdictCachePath + `","issues":[]}`)
	for name, path := range aur524CacheEntryPaths(t, cacheDir) {
		if path == gateVerdictCachePath {
			if err := os.WriteFile(filepath.Join(cacheDir, name), forged, 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
}

// TestAUR524AC001StickyFailAcrossRuns is T1: a provider that FAILS
// (breach) then APPROVES (clean) under the identical reviewed content,
// policy, context and model must still have its SECOND run FAIL --
// proving the stored verdict was actually applied, not merely that the
// second call happened to repeat the first's answer. A third round, with
// the verdict entry also removed, finds nothing to reuse and passes.
func TestAUR524AC001StickyFailAcrossRuns(t *testing.T) {
	dir := cleanFixture(t, aur524GateConfig)
	aur524WriteSkill(t, dir, aur524SkillBody)
	srv, calls := aur524AlternatingLLMServer(t, []string{aur524BreachResp, aur524CleanResp})
	defer srv.Close()
	t.Setenv("AURUMCODE_LLM_FIXTURE", "")
	t.Setenv("LLM_API_KEY", "test-key")
	t.Setenv("LLM_BASE_URL", srv.URL)
	t.Setenv("LLM_MODEL", "model-x")
	cacheDir := os.Getenv("AURUMCODE_CACHE_DIR")

	var out1, err1 strings.Builder
	code1 := runReview([]string{"--base", "HEAD~1"}, &out1, &err1, redaction.NewFilter())
	if code1 != exitFindings {
		t.Fatalf("round1 exit=%d, want exitFindings(%d); stdout=%s stderr=%s", code1, exitFindings, out1.String(), err1.String())
	}
	if atomic.LoadInt32(calls) != 1 {
		t.Fatalf("round1 must call the provider once, got %d calls", atomic.LoadInt32(calls))
	}

	aur524DeleteNonVerdictEntries(t, cacheDir)

	var out2, err2 strings.Builder
	code2 := runReview([]string{"--base", "HEAD~1"}, &out2, &err2, redaction.NewFilter())
	if code2 != exitFindings {
		t.Fatalf("round2 exit=%d, want exitFindings(%d): the sticky FAIL must survive a later, cleaner answer; stdout=%s stderr=%s", code2, exitFindings, out2.String(), err2.String())
	}
	if atomic.LoadInt32(calls) != 2 {
		t.Fatalf("round2 must call the provider again, got %d calls", atomic.LoadInt32(calls))
	}
	if !strings.Contains(out2.String(), "security#no-hardcoded-secrets") {
		t.Fatalf("round2's stdout must still name the reused breach:\n%s", out2.String())
	}
	if !strings.Contains(err2.String(), "reaplicado") {
		t.Fatalf("expected round2 to announce the reapplied finding(s):\n%s", err2.String())
	}

	// Negative control: with the verdict entry ALSO gone, there is
	// nothing left to reuse -- round3 (clamped to the clean answer) must
	// pass.
	aur524DeleteVerdictEntries(t, cacheDir)
	aur524DeleteNonVerdictEntries(t, cacheDir)
	var out3, err3 strings.Builder
	code3 := runReview([]string{"--base", "HEAD~1"}, &out3, &err3, redaction.NewFilter())
	if code3 != 0 {
		t.Fatalf("round3 exit=%d, want 0 (nothing stored to reuse): stdout=%s stderr=%s", code3, out3.String(), err3.String())
	}
}

// aur524PRGateServer is this card's own GitHub httptest stub: a diff, one
// repo config.yml (base64, via the contents API), one configured skill,
// no other context files (404, zero-config), and an accepted formal
// review/status POST.
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

// TestAUR524AC001PRStickyFailInsideHunk is T2: AC-001's --pr counterpart.
// The breach's cited line (app.go:3) is INSIDE the changed hunk (unlike
// an out-of-diff citation, which the scope gate would drop), the same
// reviewed SHA (GITHUB_SHA) and the same fixed-identity, alternating
// provider as T1: round2 must still FAIL even though the provider's own
// second answer is clean.
func TestAUR524AC001PRStickyFailInsideHunk(t *testing.T) {
	const diffBody = "diff --git a/app.go b/app.go\n@@ -1,2 +1,4 @@\n package demo\n+func Change() {\n+ dbPassword := \"hunter2-super-secret\"\n+ _ = dbPassword\n+}\n"
	gh := aur524PRGateServer(t, diffBody, aur524GateConfig, aur524SkillBody)
	defer gh.Close()
	llm, calls := aur524AlternatingLLMServer(t, []string{aur524BreachResp, aur524CleanResp})
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
	if code1 != exitFindings {
		t.Fatalf("round1 exit=%d, want exitFindings(%d); stdout=%s stderr=%s", code1, exitFindings, out1.String(), err1.String())
	}
	if atomic.LoadInt32(calls) != 1 {
		t.Fatalf("round1 must call the provider once, got %d calls", atomic.LoadInt32(calls))
	}

	var out2, err2 strings.Builder
	code2 := runPRReview(&out2, &err2, 48, "owner/repo", true, true, false, redaction.NewFilter(), opts)
	if code2 != exitFindings {
		t.Fatalf("round2 exit=%d, want exitFindings(%d): the --pr gate verdict must stay sticky; stdout=%s stderr=%s", code2, exitFindings, out2.String(), err2.String())
	}
	if atomic.LoadInt32(calls) != 2 {
		t.Fatalf("round2 must call the provider again, got %d calls", atomic.LoadInt32(calls))
	}
	if !strings.Contains(err2.String(), "reaplicado") {
		t.Fatalf("expected round2 to announce the reapplied finding(s):\n%s", err2.String())
	}
}

// TestAUR524AC001CleanThenBreachStillFails is T3: a MISS that stores a
// clean (empty) raw issue set must never suppress a LATER round's own,
// freshly-found breach -- a sanity check that the monotonic union is
// never accidentally inverted into "a stored pass overrides a fresh
// fail".
func TestAUR524AC001CleanThenBreachStillFails(t *testing.T) {
	dir := cleanFixture(t, aur524GateConfig)
	aur524WriteSkill(t, dir, aur524SkillBody)
	srv, calls := aur524AlternatingLLMServer(t, []string{aur524CleanResp, aur524BreachResp})
	defer srv.Close()
	t.Setenv("AURUMCODE_LLM_FIXTURE", "")
	t.Setenv("LLM_API_KEY", "test-key")
	t.Setenv("LLM_BASE_URL", srv.URL)
	t.Setenv("LLM_MODEL", "model-x")
	cacheDir := os.Getenv("AURUMCODE_CACHE_DIR")

	var out1, err1 strings.Builder
	if code := runReview([]string{"--base", "HEAD~1"}, &out1, &err1, redaction.NewFilter()); code != 0 {
		t.Fatalf("round1 exit=%d, want 0; stdout=%s stderr=%s", code, out1.String(), err1.String())
	}

	aur524DeleteNonVerdictEntries(t, cacheDir)

	var out2, err2 strings.Builder
	code2 := runReview([]string{"--base", "HEAD~1"}, &out2, &err2, redaction.NewFilter())
	if code2 != exitFindings {
		t.Fatalf("round2 exit=%d, want exitFindings(%d): a stored clean result must never suppress a fresh breach; stdout=%s stderr=%s", code2, exitFindings, out2.String(), err2.String())
	}
	if atomic.LoadInt32(calls) != 2 {
		t.Fatalf("round2 must call the provider again, got %d calls", atomic.LoadInt32(calls))
	}
}

// TestAUR524AC002SkillChangeInvalidatesReuse covers AC-002: changing a
// repo skill's content between two otherwise identical runs invalidates
// reuse -- round2 (a clean answer) must NOT have round1's stored breach
// reapplied onto it.
func TestAUR524AC002SkillChangeInvalidatesReuse(t *testing.T) {
	dir := cleanFixture(t, aur524GateConfig)
	aur524WriteSkill(t, dir, aur524SkillBody)
	srv, _ := aur524AlternatingLLMServer(t, []string{aur524BreachResp, aur524CleanResp})
	defer srv.Close()
	t.Setenv("AURUMCODE_LLM_FIXTURE", "")
	t.Setenv("LLM_API_KEY", "test-key")
	t.Setenv("LLM_BASE_URL", srv.URL)
	t.Setenv("LLM_MODEL", "model-x")

	var out1, err1 strings.Builder
	if code := runReview([]string{"--base", "HEAD~1"}, &out1, &err1, redaction.NewFilter()); code != exitFindings {
		t.Fatalf("round1 exit=%d, want exitFindings(%d); stdout=%s stderr=%s", code, exitFindings, out1.String(), err1.String())
	}

	// Visibly different skill content: the key (ruleCatalogDigest and
	// contextBlockDigest alike) changes even though the reviewed content,
	// policy and model did not.
	aur524WriteSkill(t, dir, aur524SkillBody+"\nguideline-zz-marker-71c3\n")

	var out2, err2 strings.Builder
	code2 := runReview([]string{"--base", "HEAD~1"}, &out2, &err2, redaction.NewFilter())
	if code2 != 0 {
		t.Fatalf("round2 exit=%d, want 0: a changed skill must never reuse round1's stored breach; stdout=%s stderr=%s", code2, out2.String(), err2.String())
	}
	if strings.Contains(err2.String(), "reaplicado") {
		t.Fatalf("round2 must not claim a reapplied finding after the skill changed:\n%s", err2.String())
	}
}

// TestAUR524AC002ModelChangeInvalidatesReuse is AC-002's "o modelo"
// counterpart: switching --modelo between two otherwise identical runs
// invalidates reuse.
func TestAUR524AC002ModelChangeInvalidatesReuse(t *testing.T) {
	dir := cleanFixture(t, aur524GateConfig)
	aur524WriteSkill(t, dir, aur524SkillBody)
	t.Setenv("LLM_API_KEY", "")
	t.Setenv("LLM_BASE_URL", "")
	t.Setenv("LLM_MODEL", "")

	t.Setenv("AURUMCODE_LLM_FIXTURE", aur524WriteFixture(t, aur524BreachResp))
	var out1, err1 strings.Builder
	if code := runReview([]string{"--base", "HEAD~1", "--modelo", "model-a"}, &out1, &err1, redaction.NewFilter()); code != exitFindings {
		t.Fatalf("round1 exit=%d, want exitFindings(%d); stdout=%s stderr=%s", code, exitFindings, out1.String(), err1.String())
	}

	t.Setenv("AURUMCODE_LLM_FIXTURE", aur524WriteFixture(t, aur524CleanResp))
	var out2, err2 strings.Builder
	code2 := runReview([]string{"--base", "HEAD~1", "--modelo", "model-b"}, &out2, &err2, redaction.NewFilter())
	if code2 != 0 {
		t.Fatalf("round2 exit=%d, want 0; stdout=%s stderr=%s", code2, out2.String(), err2.String())
	}
	if strings.Contains(err2.String(), "reaplicado") {
		t.Fatalf("a different --modelo must never reuse the previous model's verdict:\n%s", err2.String())
	}
}

// TestAUR524AC002PromptVersionChangeInvalidatesReuse covers AC-002's
// prompt-version term: swapping newCacheDigestBuilder (review_cache.go's
// own seam, AUR-543) for one with different fixed content -- as if the
// embedded prompt/catalog changed -- between two otherwise identical runs
// invalidates reuse. Mirrors TestAUR543AC001PromptEditForcesFreshReview's
// own technique of substituting the seam rather than editing a constant.
func TestAUR524AC002PromptVersionChangeInvalidatesReuse(t *testing.T) {
	dir := cleanFixture(t, aur524GateConfig)
	aur524WriteSkill(t, dir, aur524SkillBody)
	t.Setenv("AURUMCODE_LLM_FIXTURE", aur524WriteFixture(t, aur524BreachResp))

	var out1, err1 strings.Builder
	if code := runReview([]string{"--base", "HEAD~1"}, &out1, &err1, redaction.NewFilter()); code != exitFindings {
		t.Fatalf("round1 exit=%d, want exitFindings(%d); stdout=%s stderr=%s", code, exitFindings, out1.String(), err1.String())
	}

	original := newCacheDigestBuilder
	t.Cleanup(func() { newCacheDigestBuilder = original })
	newCacheDigestBuilder = prompt.NewPromptBuilderWithoutTemplates
	t.Setenv("AURUMCODE_LLM_FIXTURE", aur524WriteFixture(t, aur524CleanResp))

	var out2, err2 strings.Builder
	code2 := runReview([]string{"--base", "HEAD~1"}, &out2, &err2, redaction.NewFilter())
	if code2 != 0 {
		t.Fatalf("round2 exit=%d, want 0: a changed prompt-version digest must never reuse round1's stored breach; stdout=%s stderr=%s", code2, out2.String(), err2.String())
	}
	if strings.Contains(err2.String(), "reaplicado") {
		t.Fatalf("round2 must not claim a reapplied finding after the prompt digest changed:\n%s", err2.String())
	}
}

// TestAUR524AC002CentralPolicyChangeInvalidatesReuse is this card's own
// behavioral anchor for MUT-001: a run under a central policy (whose
// digest, render.PolicyDigest, folds into the key) must never share a
// verdict with a repo-only run that happens to be identical in every
// OTHER respect. A mutation dropping the policy digest term from
// gateVerdictCacheKey makes this test fail (not merely a unit test).
func TestAUR524AC002CentralPolicyChangeInvalidatesReuse(t *testing.T) {
	dir := cleanFixture(t, "")
	aur524WriteSkill(t, dir, aur524SkillBody)
	cacheDir := os.Getenv("AURUMCODE_CACHE_DIR")

	policyDir := policyFixture(t, aur524GateConfig)
	aur524WriteSkill(t, policyDir, aur524SkillBody)

	// A fixed-identity, alternating provider (breach, then clean) across
	// BOTH rounds: AURUMCODE_LLM_FIXTURE's own content-hashed identity
	// (modelCacheKey) would otherwise change between a breach and a
	// clean canned response, confounding "only the policy digest
	// differs".
	srv, _ := aur524AlternatingLLMServer(t, []string{aur524BreachResp, aur524CleanResp})
	defer srv.Close()
	t.Setenv("AURUMCODE_LLM_FIXTURE", "")
	t.Setenv("LLM_API_KEY", "test-key")
	t.Setenv("LLM_BASE_URL", srv.URL)
	t.Setenv("LLM_MODEL", "model-x")

	var out1, err1 strings.Builder
	code1 := runReview([]string{"--base", "HEAD~1", "--politica", policyDir}, &out1, &err1, redaction.NewFilter())
	if code1 != exitFindings {
		t.Fatalf("round1 (with policy) exit=%d, want exitFindings(%d); stdout=%s stderr=%s", code1, exitFindings, out1.String(), err1.String())
	}

	aur524DeleteNonVerdictEntries(t, cacheDir)

	// Round2: the SAME repository, the SAME policy directory, skill and
	// gate (so ruleCatalogDigest/contextBlockDigest -- both derived from
	// PARSED content -- stay byte-identical), but the policy's own
	// config.yml gains a trailing comment: render.PolicyDigest hashes the
	// RAW file bytes, so this alone changes PolicyDigest with no other
	// term moving at all. A clean answer must stay clean.
	policyConfigPath := filepath.Join(policyDir, ".aurumcode", "config.yml")
	if err := os.WriteFile(policyConfigPath, []byte(aur524GateConfig+"# aur524-policy-digest-marker\n"), 0600); err != nil {
		t.Fatal(err)
	}

	var out2, err2 strings.Builder
	code2 := runReview([]string{"--base", "HEAD~1", "--politica", policyDir}, &out2, &err2, redaction.NewFilter())
	if code2 != 0 {
		t.Fatalf("round2 (edited policy bytes) exit=%d, want 0: a changed policy digest must never reuse round1's stored breach; stdout=%s stderr=%s", code2, out2.String(), err2.String())
	}
	if strings.Contains(err2.String(), "reaplicado") {
		t.Fatalf("round2 must not claim a reapplied finding after the policy digest changed:\n%s", err2.String())
	}
}

// TestAUR524AC003InconclusiveNeverStoredOrReused covers AC-003: a
// provider that fails at the transport level (an unreadable, configured
// fixture -- AUR-458's qualityFailed) under gate.inconclusive: block must
// never claim a reused/reapplied verdict, and must never leave behind a
// stored one either.
func TestAUR524AC003InconclusiveNeverStoredOrReused(t *testing.T) {
	cleanFixture(t, "gate:\n  inconclusive: block\n")
	t.Setenv("AURUMCODE_LLM_FIXTURE", filepath.Join(t.TempDir(), "missing-response.json"))

	var out1, err1 strings.Builder
	code1 := runReview([]string{"--base", "HEAD~1", "--seguranca"}, &out1, &err1, redaction.NewFilter())
	if code1 != exitQualityNotReviewed {
		t.Fatalf("round1 exit=%d, want exitQualityNotReviewed(%d); stdout=%s stderr=%s", code1, exitQualityNotReviewed, out1.String(), err1.String())
	}
	if strings.Contains(err1.String(), "reaplicado") {
		t.Fatalf("an inconclusive run must never claim a reapplied verdict:\n%s", err1.String())
	}

	cacheDir := os.Getenv("AURUMCODE_CACHE_DIR")
	for name, path := range aur524CacheEntryPaths(t, cacheDir) {
		if path == gateVerdictCachePath {
			t.Fatalf("an inconclusive review must never be persisted as a reusable gate verdict (entry %s)", name)
		}
	}

	t.Setenv("AURUMCODE_LLM_FIXTURE", aur524WriteFixture(t, aur524CleanResp))
	var out2, err2 strings.Builder
	code2 := runReview([]string{"--base", "HEAD~1", "--seguranca"}, &out2, &err2, redaction.NewFilter())
	if code2 != 0 {
		t.Fatalf("round2 exit=%d, want 0; stdout=%s stderr=%s", code2, out2.String(), err2.String())
	}
	if strings.Contains(err2.String(), "reaplicado") {
		t.Fatalf("round2 must compute its own fresh verdict: round1 stored nothing to reuse:\n%s", err2.String())
	}
}

// TestAUR524AC004NoCacheDirDeclaresUnavailable covers AC-004: without
// AURUMCODE_CACHE_DIR set, a run with a gate declared says plainly, on
// stderr and in the published limitations, that gate-verdict reuse is
// unavailable.
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

// TestAUR524AC005ForgedEmptyEntryNeverApproves is T4: a stored verdict
// legitimately written by round1 is then overwritten -- simulating
// another workflow in the same untrusted cache scope -- with a forged,
// empty issue list. Round2's OWN provider call still finds the SAME
// breach fresh (not from cache): the forged entry can only ever add zero
// findings, and must never erase or override what round2 found on its
// own.
func TestAUR524AC005ForgedEmptyEntryNeverApproves(t *testing.T) {
	dir := cleanFixture(t, aur524GateConfig)
	aur524WriteSkill(t, dir, aur524SkillBody)
	t.Setenv("AURUMCODE_LLM_FIXTURE", aur524WriteFixture(t, aur524BreachResp))
	cacheDir := os.Getenv("AURUMCODE_CACHE_DIR")

	var out1, err1 strings.Builder
	if code := runReview([]string{"--base", "HEAD~1"}, &out1, &err1, redaction.NewFilter()); code != exitFindings {
		t.Fatalf("round1 exit=%d, want exitFindings(%d); stdout=%s stderr=%s", code, exitFindings, out1.String(), err1.String())
	}

	aur524DeleteNonVerdictEntries(t, cacheDir)
	aur524ForgeVerdictEntriesEmpty(t, cacheDir)

	var out2, err2 strings.Builder
	code2 := runReview([]string{"--base", "HEAD~1"}, &out2, &err2, redaction.NewFilter())
	if code2 != exitFindings {
		t.Fatalf("round2 exit=%d, want exitFindings(%d): a forged, empty cache entry must never produce an approval; stdout=%s stderr=%s", code2, exitFindings, out2.String(), err2.String())
	}
}

// TestAUR524AC006RuleConfigReappliedOnReuse is T5: a reused entry is
// re-evaluated against the CURRENT run's rule config, never the config
// that was active when it was written -- proven with ONE stored raw
// entry (round1's) reapplied twice, under two opposite configs:
//
//   - round1 (rule disabled, provider breach): the review passes (exit 0
//     -- a disabled rule's finding never gates) but the RAW,
//     pre-rule-config breach is still stored (storeGateVerdict stores
//     BEFORE config.ApplyRuleConfig runs).
//   - round2 (rule re-enabled, provider now clean): must FAIL, because
//     round1's stored raw finding is reapplied against the NOW-enabled
//     rule (forward case).
//   - round3 (rule disabled again, provider still clean): must PASS,
//     because the SAME stored raw finding is reapplied against the
//     NOW-disabled rule and dropped (inverse case) -- the entry written
//     while disabled is just as reusable/sensitive to current config as
//     one written while enabled, since storage never depends on config
//     at all.
func TestAUR524AC006RuleConfigReappliedOnReuse(t *testing.T) {
	const disabledConfig = aur524GateConfig + "rules:\n  security#no-hardcoded-secrets:\n    enabled: false\n"
	dir := cleanFixture(t, disabledConfig)
	aur524WriteSkill(t, dir, aur524SkillBody)
	srv, _ := aur524AlternatingLLMServer(t, []string{aur524BreachResp, aur524CleanResp})
	defer srv.Close()
	t.Setenv("AURUMCODE_LLM_FIXTURE", "")
	t.Setenv("LLM_API_KEY", "test-key")
	t.Setenv("LLM_BASE_URL", srv.URL)
	t.Setenv("LLM_MODEL", "model-x")
	cacheDir := os.Getenv("AURUMCODE_CACHE_DIR")

	var out1, err1 strings.Builder
	code1 := runReview([]string{"--base", "HEAD~1"}, &out1, &err1, redaction.NewFilter())
	if code1 != 0 {
		t.Fatalf("round1 (rule disabled) exit=%d, want 0: a disabled rule's finding never gates; stdout=%s stderr=%s", code1, out1.String(), err1.String())
	}

	aur524DeleteNonVerdictEntries(t, cacheDir)
	if err := os.WriteFile(filepath.Join(dir, ".aurumcode", "config.yml"), []byte(aur524GateConfig), 0600); err != nil {
		t.Fatal(err)
	}

	var out2, err2 strings.Builder
	code2 := runReview([]string{"--base", "HEAD~1"}, &out2, &err2, redaction.NewFilter())
	if code2 != exitFindings {
		t.Fatalf("round2 (rule re-enabled) exit=%d, want exitFindings(%d): the stored raw finding must be reapplied under the NOW-enabled rule; stdout=%s stderr=%s", code2, exitFindings, out2.String(), err2.String())
	}

	aur524DeleteNonVerdictEntries(t, cacheDir)
	if err := os.WriteFile(filepath.Join(dir, ".aurumcode", "config.yml"), []byte(disabledConfig), 0600); err != nil {
		t.Fatal(err)
	}

	var out3, err3 strings.Builder
	code3 := runReview([]string{"--base", "HEAD~1"}, &out3, &err3, redaction.NewFilter())
	if code3 != 0 {
		t.Fatalf("round3 (rule disabled again) exit=%d, want 0: the SAME stored raw finding must be dropped once reapplied under a NOW-disabled rule; stdout=%s stderr=%s", code3, out3.String(), err3.String())
	}
}
