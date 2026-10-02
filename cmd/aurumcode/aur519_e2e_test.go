package main

// AUR-519 end-to-end behavior proof: the gate wired into runReview (--base)
// and runPRReview's own publishPolicyGateStatus, driven through the real
// `aurumcode review` command with AURUMCODE_LLM_FIXTURE, exactly like
// aur476_test.go/aur518_test.go already do. Each test fails if the gate
// behavior it names is removed.

import (
	"bytes"
	"compress/zlib"
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/git/githubclient"
	"github.com/Mpaape/AurumCode/internal/prompt"
	"github.com/Mpaape/AurumCode/internal/review"
	"github.com/Mpaape/AurumCode/internal/reviewprofile"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// cleanFixture builds a minimal git repository whose HEAD~1..HEAD diff
// changes app.go with no secret-shaped content at all -- unlike
// coverageFixture/localPassFixture (which both deliberately embed a
// hardcoded-secret line for their own cards), so a test here is never
// confused by the unrelated, pre-existing deterministic analysis pass.
func cleanFixture(t *testing.T, configYAML string) string {
	t.Helper()
	dir := t.TempDir()
	write := func(name string, data []byte) {
		t.Helper()
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	object := func(kind string, body []byte) string {
		t.Helper()
		payload := append([]byte(fmt.Sprintf("%s %d\x00", kind, len(body))), body...)
		sum := sha1.Sum(payload)
		id := hex.EncodeToString(sum[:])
		var compressed bytes.Buffer
		w := zlib.NewWriter(&compressed)
		if _, err := w.Write(payload); err != nil {
			t.Fatal(err)
		}
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
		write(".git/objects/"+id[:2]+"/"+id[2:], compressed.Bytes())
		return id
	}
	commit := func(source, parent string) string {
		blob := object("blob", []byte(source))
		raw, _ := hex.DecodeString(blob)
		tree := object("tree", append([]byte("100644 app.go\x00"), raw...))
		body := "tree " + tree + "\n"
		if parent != "" {
			body += "parent " + parent + "\n"
		}
		body += "author Fixture <fixture@example.invalid> 1 +0000\ncommitter Fixture <fixture@example.invalid> 1 +0000\n\nfixture\n"
		return object("commit", []byte(body))
	}
	base := commit("package demo\nfunc Change() int {\n return 1\n}\n", "")
	source := "package demo\nfunc Change() int {\n return 42\n}\n"
	head := commit(source, base)
	write(".git/HEAD", []byte("ref: refs/heads/main\n"))
	write(".git/refs/heads/main", []byte(head+"\n"))
	write(".git/config", []byte("[core]\nrepositoryformatversion = 0\nbare = false\n"))
	write("app.go", []byte(source))
	if configYAML != "" {
		write(".aurumcode/config.yml", []byte(configYAML))
	}
	for _, key := range []string{"LLM_API_KEY", "LLM_BASE_URL", "LLM_MODEL", "AURUMCODE_LLM_FIXTURE", "AURUMCODE_LLM_INPUT_USD_PER_1K", "AURUMCODE_LLM_OUTPUT_USD_PER_1K"} {
		t.Setenv(key, "")
	}
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("AURUMCODE_CACHE_DIR", t.TempDir())
	restore := chdir(t, dir)
	t.Cleanup(restore)
	return dir
}

// TestAUR519GateSeverityBreachFailsCheck covers AC-001: a finding citing a
// repo-opted-in skill section at or above gate.fail_on's threshold fails
// the check (exitFindings) and names the skill and section in the output.
func TestAUR519GateSeverityBreachFailsCheck(t *testing.T) {
	dir := coverageFixture(t, "review:\n  context:\n    skills:\n      - skills/security.md\ngate:\n  fail_on: [high]\n")
	if err := os.MkdirAll(filepath.Join(dir, "skills"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "skills", "security.md"), []byte("## No Hardcoded Secrets\n\nNever commit a literal credential.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	fixture := filepath.Join(t.TempDir(), "response.json")
	resp := `{"summary":"ok","verdict":"approve","issues":[{"file":"app.go","line":3,"severity":"error","rule_id":"security#no-hardcoded-secrets","message":"Hardcoded secret","evidence":"dbPassword := \"hunter2-super-secret\"","impact":"Credential leak","verification":"Remove the literal secret"}]}`
	if err := os.WriteFile(fixture, []byte(resp), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AURUMCODE_LLM_FIXTURE", fixture)

	var out, errOut strings.Builder
	code := runReview([]string{"--base", "HEAD~1"}, &out, &errOut, redaction.NewFilter())
	if code != exitFindings {
		t.Fatalf("exit=%d, want exitFindings(%d); stdout=%s stderr=%s", code, exitFindings, out.String(), errOut.String())
	}
	combined := out.String() + errOut.String()
	if !strings.Contains(combined, "security#no-hardcoded-secrets") {
		t.Fatalf("expected the skill/section id named in the output:\n%s", combined)
	}
	if !strings.Contains(combined, "No Hardcoded Secrets") {
		t.Fatalf("expected the section title named in the output:\n%s", combined)
	}
}

// TestAUR519GateInconclusiveProviderFailureBlocks covers AC-003 (and is
// MUT-001's anchor): with gate.inconclusive: block, a provider that was
// configured and failed makes the check fail, with the policy-gate's own
// line naming "provider_failure" reaching the output, and the verdict
// never reads as approved.
func TestAUR519GateInconclusiveProviderFailureBlocks(t *testing.T) {
	coverageFixture(t, "gate:\n  inconclusive: block\n")
	// A configured-but-unreadable fixture is a provider that was selected
	// and then failed -- distinct from "nothing configured at all" -- so
	// qualityFailed, not qualitySkipped, drives this scenario (AUR-458).
	t.Setenv("AURUMCODE_LLM_FIXTURE", filepath.Join(t.TempDir(), "missing-response.json"))

	var out, errOut strings.Builder
	code := runReview([]string{"--base", "HEAD~1", "--seguranca"}, &out, &errOut, redaction.NewFilter())
	if code != exitQualityNotReviewed {
		t.Fatalf("exit=%d, want exitQualityNotReviewed(%d); stdout=%s stderr=%s", code, exitQualityNotReviewed, out.String(), errOut.String())
	}
	combined := out.String() + errOut.String()
	if !strings.Contains(combined, "policy gate: review inconclusive (provider_failure)") {
		t.Fatalf("expected the policy gate's own inconclusive line naming provider_failure:\n%s", combined)
	}
	if strings.Contains(out.String(), "**Verdict:** Approve") {
		t.Fatalf("verdict must never read as approved under a blocking, inconclusive gate:\n%s", out.String())
	}
}

// TestAUR519GatePartialCoverageInconclusiveWarns covers AC-004: AUR-476's
// own partial-coverage detection feeds the same gate.inconclusive
// configuration. With "warn", the check still passes (exit 0) but the
// alert is visible and the verdict is pulled off "approve" -- neither of
// which existed before this card (a clean, 0-issue "approve" response with
// a config-hidden file used to publish untouched).
func TestAUR519GatePartialCoverageInconclusiveWarns(t *testing.T) {
	coverageFixture(t, "ignore:\n  - \"tests/**\"\ngate:\n  inconclusive: warn\n")
	fixture := filepath.Join(t.TempDir(), "response.json")
	if err := os.WriteFile(fixture, []byte(`{"summary":"ok","verdict":"approve","issues":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AURUMCODE_LLM_FIXTURE", fixture)

	var out, errOut strings.Builder
	code := runReview([]string{"--base", "HEAD~1"}, &out, &errOut, redaction.NewFilter())
	if code != 0 {
		t.Fatalf("exit=%d, want 0 (warn never blocks); stdout=%s stderr=%s", code, out.String(), errOut.String())
	}
	combined := out.String() + errOut.String()
	if !strings.Contains(combined, "policy gate: review inconclusive (partial_coverage)") {
		t.Fatalf("expected a visible policy-gate alert naming partial_coverage:\n%s", combined)
	}
	if strings.Contains(out.String(), "**Verdict:** Approve") {
		t.Fatalf("verdict must not read as approved once the gate flags the review inconclusive, even under warn:\n%s", out.String())
	}
}

// TestAUR519PolicyGateStatusContextIsStable covers AC-006: the policy
// gate's own commit status uses a fixed, documented context name
// ("aurumcode/policy-gate"), distinct from --check's own "aurumcode/review"
// (checkContext), so an org's branch-protection ruleset can require it on
// its own. Mirrors TestRequiredPRQualityFailurePublishesFailingStatus's own
// pattern for publishCheckStatus.
func TestAUR519PolicyGateStatusContextIsStable(t *testing.T) {
	if policyGateContext != "aurumcode/policy-gate" {
		t.Fatalf("policyGateContext changed to %q -- this is a published, stable commit-status contract (AC-006)", policyGateContext)
	}
	var published []githubclient.CommitStatus
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/owner/repo":
			_, _ = w.Write([]byte(`{"permissions":{"push":true}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/repos/owner/repo/statuses/head":
			var status githubclient.CommitStatus
			if err := json.NewDecoder(r.Body).Decode(&status); err != nil {
				t.Error(err)
			}
			published = append(published, status)
			w.WriteHeader(http.StatusCreated)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()
	client := githubclient.NewClientWithBaseURL("test-token", server.URL)

	var out, errOut strings.Builder
	noGate := publishPolicyGateStatus(context.Background(), client, &out, &errOut, "owner", "repo", "head", gateDecision{Active: false}, 42)
	if noGate != 0 || len(published) != 0 {
		t.Fatalf("an undeclared gate must publish nothing at all: exit=%d published=%+v", noGate, published)
	}

	breach := gateDecision{Active: true, Fail: true, Breach: true, Lines: []string{"security#no-hardcoded-secrets: No Hardcoded Secrets (severidade error, limiar error)"}}
	code := publishPolicyGateStatus(context.Background(), client, &out, &errOut, "owner", "repo", "head", breach, 42)
	if code != exitFindings {
		t.Fatalf("exit=%d, want exitFindings; stderr=%s", code, errOut.String())
	}
	if len(published) != 1 || published[0].Context != policyGateContext || published[0].State != "failure" {
		t.Fatalf("published=%+v", published)
	}
}

// TestAUR519NoGateConfiguredStaysUntouched proves that without any `gate:`
// key anywhere, the review carries no trace of this card at all: no
// policy-gate line, and a clean response's own "approve" verdict still
// publishes exactly as before AUR-519.
func TestAUR519NoGateConfiguredStaysUntouched(t *testing.T) {
	cleanFixture(t, "")
	fixture := filepath.Join(t.TempDir(), "response.json")
	if err := os.WriteFile(fixture, []byte(`{"summary":"ok","verdict":"approve","issues":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AURUMCODE_LLM_FIXTURE", fixture)

	var out, errOut strings.Builder
	code := runReview([]string{"--base", "HEAD~1"}, &out, &errOut, redaction.NewFilter())
	if code != 0 {
		t.Fatalf("exit=%d stdout=%s stderr=%s", code, out.String(), errOut.String())
	}
	if strings.Contains(out.String()+errOut.String(), "policy gate") {
		t.Fatalf("no gate was configured; the review must carry no policy-gate trace at all:\n%s", out.String()+errOut.String())
	}
	if !strings.Contains(out.String(), "**Verdict:** Approve") {
		t.Fatalf("without a gate, a clean approve verdict must still publish unchanged:\n%s", out.String())
	}
}

// TestAUR519GateSeverityBreachFailsCheckWithProfiles proves the gate is
// identical with and without --perfis: a finding citing a repo-opted-in
// skill section at gate.fail_on's threshold must still fail the check
// (exitFindings) when --perfis selects more than one profile, exactly as
// TestAUR519GateSeverityBreachFailsCheck already proves for the
// single-reviewer path. Before this fix, each profile's own Reviewer never
// learned the dynamic rule set, so the citation was discarded as an unknown
// rule_id and the gate saw no finding at all -- "approved with a defect
// present" purely because --perfis was selected.
func TestAUR519GateSeverityBreachFailsCheckWithProfiles(t *testing.T) {
	dir := coverageFixture(t, "review:\n  context:\n    skills:\n      - skills/security.md\ngate:\n  fail_on: [high]\n")
	if err := os.MkdirAll(filepath.Join(dir, "skills"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "skills", "security.md"), []byte("## No Hardcoded Secrets\n\nNever commit a literal credential.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	fixture := filepath.Join(t.TempDir(), "response.json")
	resp := `{"summary":"ok","verdict":"approve","issues":[{"file":"app.go","line":3,"severity":"error","rule_id":"security#no-hardcoded-secrets","message":"Hardcoded secret","evidence":"dbPassword := \"hunter2-super-secret\"","impact":"Credential leak","verification":"Remove the literal secret"}]}`
	if err := os.WriteFile(fixture, []byte(resp), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AURUMCODE_LLM_FIXTURE", fixture)

	var out, errOut strings.Builder
	code := runReview([]string{"--base", "HEAD~1", "--perfis", "seguranca,solid"}, &out, &errOut, redaction.NewFilter())
	if code != exitFindings {
		t.Fatalf("exit=%d, want exitFindings(%d) with --perfis exactly as without it; stdout=%s stderr=%s", code, exitFindings, out.String(), errOut.String())
	}
	combined := out.String() + errOut.String()
	if !strings.Contains(combined, "security#no-hardcoded-secrets") {
		t.Fatalf("expected the skill/section id named in the output under --perfis:\n%s", combined)
	}
	if !strings.Contains(combined, "No Hardcoded Secrets") {
		t.Fatalf("expected the section title named in the output under --perfis:\n%s", combined)
	}
}

// TestAUR519ProfilePassesCarryDegradedMetadata is B2's regression:
// runProfilePasses never copied a profile pass's result.Metadata into the
// merged result at all, so prompt.IsDegradedParse(merged) was always false
// under --perfis no matter how badly a profile's own pass degraded. Every
// profile here shares the same underlying FakeProvider (profileProvider
// only prefixes the prompt), so a free-form, non-JSON response degrades
// every pass identically; the merged result must still carry the engine's
// own degraded marker.
func TestAUR519ProfilePassesCarryDegradedMetadata(t *testing.T) {
	provider := &review.FakeProvider{Response: "app.go:3: error: looks suspicious\n"}
	diff := &types.Diff{Files: []types.DiffFile{{
		Path:  "app.go",
		Hunks: []types.DiffHunk{{NewStart: 3, Lines: []string{"+ suspicious"}}},
	}}}
	profiles := []reviewprofile.Profile{{Name: "a"}, {Name: "b"}}
	merged, err := runProfilePasses(context.Background(), provider, nil, profiles, diff, review.ReviewContext{}, nil, prompt.DefaultRuleCatalog)
	if err != nil {
		t.Fatalf("runProfilePasses() error = %v", err)
	}
	if !prompt.IsDegradedParse(merged) {
		t.Fatalf("prompt.IsDegradedParse(merged) = false, want true: every profile's own pass degraded, so the merged result must say so (B2)")
	}
}

// TestAUR519DegradedParseNeverCached is B3's regression. The review cache
// key already incorporates the AURUMCODE_LLM_FIXTURE file's own content
// (modelCacheKey, review_cache.go), so this test keeps the SAME fixture
// across both runs -- the bug is not "a different answer got masked", it
// is "a degraded answer's own classification gets lost on a cache hit":
// mergeCacheHits (review_cache.go) rebuilds result.Issues from the cached
// entry alone, with no parse ever happening and therefore no
// prompt.IsDegradedParse signal at all. With gate.inconclusive: block
// configured, the FIRST (fresh) run correctly fails the gate because the
// parse degraded; persistFreshResults used to cache that run's (filtered
// down to zero) issues anyway, so the SECOND run -- same fixture, a cache
// hit -- saw zero issues and no degraded signal, and exited 0: a policy
// gate silently defeated by its own cache. After the fix, a degraded run
// is never persisted, so the second run calls the model again and fails
// the gate exactly like the first.
func TestAUR519DegradedParseNeverCached(t *testing.T) {
	cleanFixture(t, "gate:\n  inconclusive: block\n")

	degraded := filepath.Join(t.TempDir(), "degraded.txt")
	if err := os.WriteFile(degraded, []byte("app.go:3: error: looks suspicious\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AURUMCODE_LLM_FIXTURE", degraded)

	var out1, errOut1 strings.Builder
	code1 := runReview([]string{"--base", "HEAD~1"}, &out1, &errOut1, redaction.NewFilter())
	if code1 != exitQualityNotReviewed {
		t.Fatalf("first run exit=%d, want exitQualityNotReviewed(%d); stdout=%s stderr=%s", code1, exitQualityNotReviewed, out1.String(), errOut1.String())
	}

	// Same fixture, same cache key: without the fix this is a cache hit
	// with zero issues and no degraded signal, and the gate (wrongly)
	// passes.
	var out2, errOut2 strings.Builder
	code2 := runReview([]string{"--base", "HEAD~1"}, &out2, &errOut2, redaction.NewFilter())
	if code2 != exitQualityNotReviewed {
		t.Fatalf("second run (same fixture) exit=%d, want exitQualityNotReviewed(%d) -- a degraded first run must never be cached, letting the gate pass on replay:\nstdout=%s\nstderr=%s", code2, exitQualityNotReviewed, out2.String(), errOut2.String())
	}
	if strings.Contains(errOut2.String(), "reused") {
		t.Fatalf("second run reused a cached result for the degraded file instead of calling the model again:\n%s", errOut2.String())
	}
}

// TestAUR519SkillsConfiguredNoGateStaysSafe verifies the coordinator's own
// concern directly: configuring review.context.skills WITHOUT any `gate:`
// key must behave exactly like today -- the prompt catalog grows (the
// skill's sections become citable, per AC-007/AC-002) but nothing about
// the exit code changes, and in particular SetRuleCatalog's own token-
// budget validation must not turn a configured skill into a surprise exit
// 2 for a repository that never asked for a gate at all.
func TestAUR519SkillsConfiguredNoGateStaysSafe(t *testing.T) {
	dir := coverageFixture(t, "review:\n  context:\n    skills:\n      - skills/security.md\n")
	if err := os.MkdirAll(filepath.Join(dir, "skills"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "skills", "security.md"), []byte("## No Hardcoded Secrets\n\nNever commit a literal credential.\n\n## Prefer Parameterized Queries\n\nBuild SQL with bound parameters.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	setAUR518LLMFixture(t)

	var out, errOut strings.Builder
	code := runReview([]string{"--base", "HEAD~1"}, &out, &errOut, redaction.NewFilter())
	if code != 0 {
		t.Fatalf("exit=%d, want 0: configuring skills with no gate must never change the exit code (and must never exit 2 on a catalog budget check); stdout=%s stderr=%s", code, out.String(), errOut.String())
	}
}

// TestAUR519PRGateWarnStillFailsOnBreach is B1's regression at the --pr
// level: a pull request whose diff both (a) triggers AUR-476 partial
// coverage (a second changed file the repo's own `ignore` hides) and (b)
// carries a real finding citing a repo-opted-in skill section at the
// gate's threshold, under gate.inconclusive: warn, must still fail the
// check -- exitFindings, and the published "aurumcode/policy-gate" status
// must read "failure" naming the breach, never "success" just because the
// review was also inconclusive.
func TestAUR519PRGateWarnStillFailsOnBreach(t *testing.T) {
	diffBody := "diff --git a/app.go b/app.go\n@@ -1,2 +1,4 @@\n package demo\n+func Change() {\n+ dbPassword := \"hunter2-super-secret\"\n+ _ = dbPassword\n+}\n" +
		"diff --git a/tests/change_test.go b/tests/change_test.go\n@@ -1,1 +1,2 @@\n package demo\n+func TestChange() {}\n"
	headConfig := "review:\n  context:\n    skills:\n      - skills/security.md\ngate:\n  fail_on: [high]\n  inconclusive: warn\nignore:\n  - \"tests/**\"\n"
	skillBody := "## No Hardcoded Secrets\n\nNever commit a literal credential.\n"
	resp := `{"summary":"ok","verdict":"approve","issues":[{"file":"app.go","line":3,"severity":"error","rule_id":"security#no-hardcoded-secrets","message":"Hardcoded secret","evidence":"dbPassword := \"hunter2-super-secret\"","impact":"Credential leak","verification":"Remove the literal secret"}]}`

	var publishedStatus githubclient.CommitStatus
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/owner/repo":
			_, _ = w.Write([]byte(`{"permissions":{"push":true}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/repos/owner/repo/pulls/48" && r.Header.Get("Accept") == "application/vnd.github.v3.diff":
			_, _ = w.Write([]byte(diffBody))
		case r.Method == http.MethodGet && r.URL.Path == "/repos/owner/repo/pulls/48":
			_, _ = fmt.Fprint(w, `{"head":{"sha":"head-sha"}}`)
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/contents/") && strings.Contains(r.URL.Path, "config.yml"):
			_, _ = fmt.Fprintf(w, `{"content":%q,"encoding":"base64"}`, base64.StdEncoding.EncodeToString([]byte(headConfig)))
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/contents/") && strings.Contains(r.URL.Path, "security.md"):
			_, _ = fmt.Fprintf(w, `{"content":%q,"encoding":"base64"}`, base64.StdEncoding.EncodeToString([]byte(skillBody)))
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/contents/"):
			// Every other configured/optional context file (the default
			// .aurumcode/prompt.md, instructions, docs): not found, exactly
			// the zero-config case.
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodGet && (strings.HasSuffix(r.URL.Path, "/reviews") || strings.HasSuffix(r.URL.Path, "/comments") || strings.HasSuffix(r.URL.Path, "/commits")):
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/reviews"):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":1}`))
		case r.Method == http.MethodPost && r.URL.Path == "/repos/owner/repo/statuses/head-sha":
			_ = json.NewDecoder(r.Body).Decode(&publishedStatus)
			w.WriteHeader(http.StatusCreated)
		default:
			t.Errorf("unexpected GitHub request: %s %s (Accept=%s)", r.Method, r.URL.Path, r.Header.Get("Accept"))
		}
	}))
	defer server.Close()

	fixture := filepath.Join(t.TempDir(), "response.json")
	if err := os.WriteFile(fixture, []byte(resp), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AURUMCODE_LLM_FIXTURE", fixture)
	t.Setenv("AURUMCODE_GITHUB_API_URL", server.URL)
	t.Setenv("AURUMCODE_PR_PERMISSION_MODE", "endpoint")
	t.Setenv("GITHUB_SHA", "head-sha")
	t.Setenv("AURUMCODE_BASE_SHA", "base")
	t.Setenv("AURUMCODE_CI_CONTEXT_FILE", "")

	var stdout, stderr strings.Builder
	code := runPRReview(&stdout, &stderr, 48, "owner/repo", true, true, true, redaction.NewFilter(), prReviewOptions{
		publicationSet: true,
		publication:    "review",
	})
	if code != exitFindings {
		t.Fatalf("exit=%d, want exitFindings(%d): a real breach must fail the check even though the review is also inconclusive (partial coverage, warn); stdout=%s stderr=%s", code, exitFindings, stdout.String(), stderr.String())
	}
	if publishedStatus.Context != policyGateContext || publishedStatus.State != "failure" {
		t.Fatalf("published status = %+v, want context %q and state failure", publishedStatus, policyGateContext)
	}
	if !strings.Contains(publishedStatus.Description, "security#no-hardcoded-secrets") {
		t.Fatalf("published status description = %q, want it naming the breach, not just the inconclusive alert", publishedStatus.Description)
	}
}

// runPRGateMockServer is the shared httptest GitHub mock for the --pr gate
// table below: a diff, one repo config.yml (base64, via contents API), no
// other context files (404, the zero-config case), an accepted formal
// review POST, and a captured policy-gate status POST. publishedStatus is
// written into *published.
func runPRGateMockServer(t *testing.T, diffBody, repoConfig string, published *githubclient.CommitStatus, postedBody *string) *httptest.Server {
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
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/contents/"):
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodGet && (strings.HasSuffix(r.URL.Path, "/reviews") || strings.HasSuffix(r.URL.Path, "/comments") || strings.HasSuffix(r.URL.Path, "/commits")):
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/reviews"):
			if postedBody != nil {
				var payload struct {
					Body string `json:"body"`
				}
				buf := new(bytes.Buffer)
				_, _ = buf.ReadFrom(r.Body)
				_ = json.Unmarshal(buf.Bytes(), &payload)
				*postedBody = payload.Body
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":1}`))
		case r.Method == http.MethodPost && r.URL.Path == "/repos/owner/repo/statuses/head-sha":
			_ = json.NewDecoder(r.Body).Decode(published)
			w.WriteHeader(http.StatusCreated)
		default:
			t.Errorf("unexpected GitHub request: %s %s (Accept=%s)", r.Method, r.URL.Path, r.Header.Get("Accept"))
		}
	}))
}

func setPRGateEnv(t *testing.T, server *httptest.Server, fixturePath string) {
	t.Helper()
	t.Setenv("AURUMCODE_LLM_FIXTURE", fixturePath)
	t.Setenv("AURUMCODE_GITHUB_API_URL", server.URL)
	t.Setenv("AURUMCODE_PR_PERMISSION_MODE", "endpoint")
	t.Setenv("GITHUB_SHA", "head-sha")
	t.Setenv("AURUMCODE_BASE_SHA", "base")
	t.Setenv("AURUMCODE_CI_CONTEXT_FILE", "")
}

// TestAUR519PRGateInconclusiveBlockTable covers gate.inconclusive: block on
// --pr for the three distinct inconclusive reasons runPRReview computes
// (pr.go's gateInconclusiveReason switch): a free-form degraded reply
// (prompt.IsDegradedParse, "degraded_parse"), a reply the parser could not
// use at all (qualityDegraded, "model_parse_failure" -- renamed from the
// B-fix round's mislabeled "provider_failure"), and AUR-476 partial
// coverage ("partial_coverage"). Each must fail the check
// (exitQualityNotReviewed) and publish aurumcode/policy-gate as "failure".
// A fourth, check-less run isolates gateResult.Fail's own exit-code branch
// from --check's independent checkExit/gateCheckExit path entirely.
func TestAUR519PRGateInconclusiveBlockTable(t *testing.T) {
	simpleDiff := "diff --git a/app.go b/app.go\n@@ -1,2 +1,4 @@\n package demo\n+func Change() {\n+ _ = 1\n+}\n"
	twoFileDiff := simpleDiff + "diff --git a/tests/change_test.go b/tests/change_test.go\n@@ -1,1 +1,2 @@\n package demo\n+func TestChange() {}\n"
	blockConfig := "gate:\n  inconclusive: block\n"
	partialConfig := "gate:\n  inconclusive: block\nignore:\n  - \"tests/**\"\n"

	cases := []struct {
		name       string
		diffBody   string
		repoConfig string
		resp       string
	}{
		{"degraded_free_text", simpleDiff, blockConfig, "app.go:3: error: looks suspicious\n"},
		{"model_parse_failure", simpleDiff, blockConfig, "this reply has no json and no file:line pattern at all"},
		{"partial_coverage", twoFileDiff, partialConfig, `{"summary":"ok","verdict":"approve","issues":[]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var published githubclient.CommitStatus
			var postedBody string
			server := runPRGateMockServer(t, tc.diffBody, tc.repoConfig, &published, &postedBody)
			defer server.Close()
			fixture := filepath.Join(t.TempDir(), "response.txt")
			if err := os.WriteFile(fixture, []byte(tc.resp), 0600); err != nil {
				t.Fatal(err)
			}
			setPRGateEnv(t, server, fixture)

			var stdout, stderr strings.Builder
			code := runPRReview(&stdout, &stderr, 48, "owner/repo", true, true, true, redaction.NewFilter(), prReviewOptions{
				publicationSet: true,
				publication:    "review",
			})
			if code != exitQualityNotReviewed {
				t.Fatalf("exit=%d, want exitQualityNotReviewed(%d); stdout=%s stderr=%s", code, exitQualityNotReviewed, stdout.String(), stderr.String())
			}
			if published.Context != policyGateContext || published.State != "failure" {
				t.Fatalf("published status = %+v, want context %q state failure", published, policyGateContext)
			}
			if tc.name == "partial_coverage" {
				// AC-003/M6b: the fixture said "approve"; neither the
				// published review body's own Verdict line nor GitHub's
				// own formal review action may read Approve once the gate
				// pulls it off under a block.
				if strings.Contains(postedBody, "**Verdict:** Approve") {
					t.Fatalf("published review body verdict read Approve under a blocking gate: %s", postedBody)
				}
				if strings.Contains(stdout.String(), `"APPROVE"`) {
					t.Fatalf("published GitHub review action read APPROVE under a blocking gate: %s", stdout.String())
				}
			}
		})
	}

	// M9b: without --check, checkExit/gateCheckExit never run at all, so
	// this isolates the standalone "if gateResult.Fail { return
	// exitQualityNotReviewed }" branch.
	t.Run("fail_exit_without_check", func(t *testing.T) {
		var published githubclient.CommitStatus
		server := runPRGateMockServer(t, twoFileDiff, partialConfig, &published, nil)
		defer server.Close()
		fixture := filepath.Join(t.TempDir(), "response.json")
		if err := os.WriteFile(fixture, []byte(`{"summary":"ok","verdict":"approve","issues":[]}`), 0600); err != nil {
			t.Fatal(err)
		}
		setPRGateEnv(t, server, fixture)

		var stdout, stderr strings.Builder
		code := runPRReview(&stdout, &stderr, 48, "owner/repo", true, true, false, redaction.NewFilter(), prReviewOptions{
			publicationSet: true,
			publication:    "review",
		})
		if code != exitQualityNotReviewed {
			t.Fatalf("without --check: exit=%d, want exitQualityNotReviewed(%d); stdout=%s stderr=%s", code, exitQualityNotReviewed, stdout.String(), stderr.String())
		}
	})
}

// TestAUR519PartialCoverageNeverCachedUnderBlock is item 2 of the latest
// review round (kills M13b: neutralizing the !cachePartial guard at
// main.go ~981). AUR-476 partial coverage (a file the repo's own `ignore`
// hides) must never be papered over by a cache hit either, the same way a
// degraded parse must not (TestAUR519DegradedParseNeverCached): the same
// fixture runs twice against the same cache dir under gate.inconclusive:
// block, and BOTH runs must fail the gate -- a second run that read a
// cached "clean" result for the same diff would silently defeat the block.
func TestAUR519PartialCoverageNeverCachedUnderBlock(t *testing.T) {
	coverageFixture(t, "ignore:\n  - \"tests/**\"\ngate:\n  inconclusive: block\n")

	fixture := filepath.Join(t.TempDir(), "response.json")
	if err := os.WriteFile(fixture, []byte(`{"summary":"ok","verdict":"approve","issues":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AURUMCODE_LLM_FIXTURE", fixture)

	var out1, errOut1 strings.Builder
	code1 := runReview([]string{"--base", "HEAD~1"}, &out1, &errOut1, redaction.NewFilter())
	if code1 != exitQualityNotReviewed {
		t.Fatalf("first run exit=%d, want exitQualityNotReviewed(%d); stdout=%s stderr=%s", code1, exitQualityNotReviewed, out1.String(), errOut1.String())
	}

	// Same fixture, same cache key: without the fix this is a cache hit
	// with no coverage metadata at all, and the gate (wrongly) passes.
	var out2, errOut2 strings.Builder
	code2 := runReview([]string{"--base", "HEAD~1"}, &out2, &errOut2, redaction.NewFilter())
	if code2 != exitQualityNotReviewed {
		t.Fatalf("second run (same fixture) exit=%d, want exitQualityNotReviewed(%d) -- partial coverage must never be cached, letting the gate pass on replay:\nstdout=%s\nstderr=%s", code2, exitQualityNotReviewed, out2.String(), errOut2.String())
	}
	// AC-003/M6: the fixture said "approve"; a blocked, inconclusive gate
	// must still pull the verdict off it, on both runs.
	if strings.Contains(out1.String(), "**Verdict:** Approve") || strings.Contains(out2.String(), "**Verdict:** Approve") {
		t.Fatalf("verdict read as approved under a blocking gate: run1=%s run2=%s", out1.String(), out2.String())
	}
}

// TestAUR519ProfilePassesCoverageTakesWorstCase is item 4's regression
// (kills M16): each profile pass can truncate the SAME diff differently --
// its own prefix (emphasis/instructions) eats a different slice of the
// shared token budget -- so runProfilePasses must keep the WORST
// (largest) code_files_partial/code_files_omitted across profiles, never
// just the first one's, or a gate relying on this count could under-report
// how much of the diff went unseen merely because the first profile in the
// list happened to fit.
func TestAUR519ProfilePassesCoverageTakesWorstCase(t *testing.T) {
	provider := &review.FakeProvider{Response: `{"summary":"ok","issues":[]}`}
	diff := &types.Diff{Files: []types.DiffFile{{Path: "app.go", Hunks: []types.DiffHunk{{NewStart: 1, Lines: []string{"+x"}}}}}}
	profiles := []reviewprofile.Profile{{Name: "a"}, {Name: "b"}}

	merged, err := runProfilePasses(context.Background(), provider, nil, profiles, diff, review.ReviewContext{}, nil, prompt.DefaultRuleCatalog)
	if err != nil {
		t.Fatalf("runProfilePasses() error = %v", err)
	}
	// Reach past the exported seam to simulate what two DIFFERENT budget
	// outcomes would merge to, exactly as runProfilePasses' own loop does
	// internally -- the real GenerateReviewWithContext path cannot easily
	// be driven to two different per-profile budget outcomes from one
	// shared FakeProvider, so this test exercises the merge helper
	// directly against the two keys item 4 names.
	first := map[string]string{"code_files_partial": "1", "code_files_omitted": "0"}
	second := map[string]string{"code_files_partial": "0", "code_files_omitted": "3"}
	got := mergeWorstCaseCoverage(merged.Metadata, first)
	got = mergeWorstCaseCoverage(got, second)
	if got["code_files_partial"] != "1" {
		t.Fatalf("code_files_partial = %q, want the worst case \"1\" across profiles", got["code_files_partial"])
	}
	if got["code_files_omitted"] != "3" {
		t.Fatalf("code_files_omitted = %q, want the worst case \"3\" across profiles", got["code_files_omitted"])
	}
}
