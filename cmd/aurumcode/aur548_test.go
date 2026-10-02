package main

// AUR-548 end-to-end behavior proof: Semgrep, driven through a fake
// executable on PATH (no network, no real Semgrep binary), feeding
// quality_gates.sast's own, independent gate decision -- exactly like
// aur519_e2e_test.go/aur537_test.go already prove AUR-519/537's own gate
// behavior through the real `aurumcode review` command.
//
// AC-001: a Semgrep finding at or above fail_on_severity fails the gate,
// naming the check_id and line in the published output and the audit/
// SARIF artifacts.
// AC-002: a finding below the threshold is published but does not fail.
// AC-003 (and MUT-001's anchor): Semgrep absent, erroring, or returning
// invalid JSON all become inconclusive, never a clean pass -- the exact
// defect MUT-001 names ("treat an execution error as zero findings") is
// what distinguishes a real clean scan (zero findings, exit 0) from every
// one of these three failure modes (inconclusive, exitQualityNotReviewed
// under gate.inconclusive: block).
// AC-004: rule packs come from the yml (or the RFC's own documented
// defaults); with no quality_gates.sast section, Semgrep is never
// invoked at all.
// AC-005: a model reply that tries to recite away or downgrade the
// Semgrep finding has no effect on the gate -- the decision is computed
// from Semgrep's own report alone, never from result.Issues/the model's
// JSON.

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/security/redaction"
)

// semgrepFake writes a fake "semgrep" executable at dir/semgrep that logs
// its own argv (one line per invocation) to argvLog when argvLog is
// non-empty, then prints body to stdout and exits 0 -- or, when
// exitNonZero is true, exits 2 after printing body to stderr instead. It
// returns the directory to prepend to PATH.
func semgrepFake(t *testing.T, body string, exitNonZero bool, argvLog string) string {
	t.Helper()
	bin := t.TempDir()
	stream := "stdout"
	exitCode := "0"
	if exitNonZero {
		stream = "stderr"
		exitCode = "2"
	}
	redirect := ">&1"
	if stream == "stderr" {
		redirect = ">&2"
	}
	script := "#!/usr/bin/env bash\n"
	if argvLog != "" {
		script += "printf '%s\\n' \"$*\" >> " + shellQuote(argvLog) + "\n"
	}
	script += "cat <<'AUR548EOF' " + redirect + "\n" + body + "\nAUR548EOF\n"
	script += "exit " + exitCode + "\n"
	writeExec(t, filepath.Join(bin, "semgrep"), script)
	return bin
}

// shellQuote wraps path in single quotes for safe embedding in the fake
// script's own source (test-controlled paths only, never untrusted input).
func shellQuote(path string) string {
	return "'" + strings.ReplaceAll(path, "'", `'\''`) + "'"
}

// semgrepMissingPATH returns an empty, real directory with no "semgrep"
// executable in it, for the "absent" branch of AC-003.
func semgrepMissingPATH(t *testing.T) string {
	t.Helper()
	return t.TempDir()
}

const semgrepErrorFinding = `{"results":[{"check_id":"python.lang.security.audit.hardcoded-secret","path":"app.go","start":{"line":3},"extra":{"severity":"ERROR","message":"Hardcoded secret detected"}}]}`
const semgrepWarningFinding = `{"results":[{"check_id":"generic.secrets.insufficient-entropy","path":"app.go","start":{"line":3},"extra":{"severity":"WARNING","message":"Possible secret"}}]}`
const semgrepClean = `{"results":[]}`
const semgrepNoResultsKey = `{}`
const semgrepGarbage = `not json at all`

// setSemgrepPATH prepends bin to PATH for the duration of the test.
func setSemgrepPATH(t *testing.T, bin string) {
	t.Helper()
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// TestAUR548SeverityBreachFailsGate covers AC-001: an ERROR-severity
// Semgrep finding (fail_on_severity defaults to ERROR) fails the gate --
// exitFindings (a real breach) -- and names the check_id and line in the
// published output.
func TestAUR548SeverityBreachFailsGate(t *testing.T) {
	cleanFixture(t, "quality_gates:\n  sast:\n    enabled: true\n")
	setSemgrepPATH(t, semgrepFake(t, semgrepErrorFinding, false, ""))

	var out, errOut strings.Builder
	code := runReview([]string{"--base", "HEAD~1"}, &out, &errOut, redaction.NewFilter())
	if code != exitFindings {
		t.Fatalf("exit=%d, want exitFindings(%d); stdout=%s stderr=%s", code, exitFindings, out.String(), errOut.String())
	}
	combined := out.String() + errOut.String()
	if !strings.Contains(combined, "semgrep:python.lang.security.audit.hardcoded-secret") {
		t.Fatalf("expected the semgrep check_id named in the output:\n%s", combined)
	}
	if !strings.Contains(combined, "app.go") || !strings.Contains(combined, "3") {
		t.Fatalf("expected file/line named in the output:\n%s", combined)
	}
}

// TestAUR548BelowThresholdDoesNotFailGate covers AC-002: a WARNING finding
// stays below the default ERROR threshold -- it is still produced (and
// would be published among result.Issues) but the gate does not fail.
func TestAUR548BelowThresholdDoesNotFailGate(t *testing.T) {
	cleanFixture(t, "quality_gates:\n  sast:\n    enabled: true\n    fail_on_severity: ERROR\n")
	setSemgrepPATH(t, semgrepFake(t, semgrepWarningFinding, false, ""))

	var out, errOut strings.Builder
	code := runReview([]string{"--base", "HEAD~1"}, &out, &errOut, redaction.NewFilter())
	if code != 0 {
		t.Fatalf("exit=%d, want 0 (below threshold never fails); stdout=%s stderr=%s", code, out.String(), errOut.String())
	}
	combined := out.String() + errOut.String()
	if strings.Contains(combined, "SAST (semgrep") && strings.Contains(combined, "limiar") {
		t.Fatalf("a below-threshold finding must not read as a gate breach line:\n%s", combined)
	}
}

// TestAUR548AbsentSemgrepIsInconclusiveNeverClean is AC-003's "absent"
// branch and MUT-001's own anchor: with gate.inconclusive: block, a PATH
// with no "semgrep" executable at all must fail the run as inconclusive
// -- never silently read as "zero findings, clean pass". A sibling table
// test below (TestAUR548ExecutionFailureIsInconclusive) covers the
// execution-error and invalid-JSON branches identically, and
// TestAUR548CleanScanPasses proves the contrast: an actual, trustworthy
// zero-finding scan DOES pass, so the inconclusive outcome here can only
// come from the missing binary, not from "no findings".
func TestAUR548AbsentSemgrepIsInconclusiveNeverClean(t *testing.T) {
	cleanFixture(t, "gate:\n  inconclusive: block\nquality_gates:\n  sast:\n    enabled: true\n")
	setSemgrepPATH(t, semgrepMissingPATH(t))

	var out, errOut strings.Builder
	code := runReview([]string{"--base", "HEAD~1"}, &out, &errOut, redaction.NewFilter())
	if code != exitQualityNotReviewed {
		t.Fatalf("exit=%d, want exitQualityNotReviewed(%d); stdout=%s stderr=%s", code, exitQualityNotReviewed, out.String(), errOut.String())
	}
	combined := out.String() + errOut.String()
	if !strings.Contains(combined, "sast_unavailable") {
		t.Fatalf("expected the sast_unavailable reason named in the output:\n%s", combined)
	}
}

// TestAUR548ExecutionFailureIsInconclusive covers AC-003's "com erro" and
// "JSON invalido" branches, and is MUT-001's direct target: a semgrep
// execution error (non-zero exit, no parseable JSON) and a semgrep run
// that printed something that is not a trustworthy report (invalid JSON,
// or valid JSON missing the "results" key entirely) must each be
// inconclusive under gate.inconclusive: block, never a clean pass.
func TestAUR548ExecutionFailureIsInconclusive(t *testing.T) {
	cases := []struct {
		name string
		body string
		fail bool
	}{
		{"ExecutionError", "boom: semgrep crashed", true},
		{"InvalidJSON", semgrepGarbage, false},
		{"NoResultsKey", semgrepNoResultsKey, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cleanFixture(t, "gate:\n  inconclusive: block\nquality_gates:\n  sast:\n    enabled: true\n")
			setSemgrepPATH(t, semgrepFake(t, tc.body, tc.fail, ""))

			var out, errOut strings.Builder
			code := runReview([]string{"--base", "HEAD~1"}, &out, &errOut, redaction.NewFilter())
			if code != exitQualityNotReviewed {
				t.Fatalf("exit=%d, want exitQualityNotReviewed(%d); stdout=%s stderr=%s", code, exitQualityNotReviewed, out.String(), errOut.String())
			}
			combined := out.String() + errOut.String()
			if !strings.Contains(combined, "inconclusivo") && !strings.Contains(combined, "inconclusive") {
				t.Fatalf("expected an inconclusive SAST line in the output:\n%s", combined)
			}
		})
	}
}

// TestAUR548CleanScanPasses is the positive contrast MUT-001 needs: a
// genuinely clean Semgrep scan (valid JSON, an explicit empty "results"
// array) passes the gate, so "inconclusive" above can only be explained by
// the execution failure itself, never by "no findings were returned".
func TestAUR548CleanScanPasses(t *testing.T) {
	cleanFixture(t, "gate:\n  inconclusive: block\nquality_gates:\n  sast:\n    enabled: true\n")
	setSemgrepPATH(t, semgrepFake(t, semgrepClean, false, ""))
	// gate.inconclusive: block also fires on an unrelated "no LLM provider
	// configured" skip (AUR-458's own quality_skipped reason) -- a valid
	// fixture keeps this test isolated to SAST's own outcome.
	fixture := filepath.Join(t.TempDir(), "response.json")
	if err := os.WriteFile(fixture, []byte(`{"summary":"ok","verdict":"approve","issues":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AURUMCODE_LLM_FIXTURE", fixture)

	var out, errOut strings.Builder
	code := runReview([]string{"--base", "HEAD~1"}, &out, &errOut, redaction.NewFilter())
	if code != 0 {
		t.Fatalf("exit=%d, want 0 (a real clean scan never fails); stdout=%s stderr=%s", code, out.String(), errOut.String())
	}
}

// TestAUR548NoConfigNeverInvokesSemgrep covers AC-004's own zero-config
// half: with no quality_gates.sast section at all, Semgrep is never
// invoked -- proven here by the fake binary's own argv log file never
// being created, not merely by the exit code.
func TestAUR548NoConfigNeverInvokesSemgrep(t *testing.T) {
	dir := cleanFixture(t, "")
	argvLog := filepath.Join(t.TempDir(), "argv.log")
	setSemgrepPATH(t, semgrepFake(t, semgrepClean, false, argvLog))

	var out, errOut strings.Builder
	code := runReview([]string{"--base", "HEAD~1"}, &out, &errOut, redaction.NewFilter())
	if code != 0 {
		t.Fatalf("exit=%d, want 0; stdout=%s stderr=%s", code, out.String(), errOut.String())
	}
	if _, err := os.Stat(argvLog); err == nil {
		t.Fatalf("semgrep must never be invoked with no quality_gates.sast section (dir=%s)", dir)
	}
}

// TestAUR548RulePacksFromConfig covers AC-004's own configured-packs half:
// quality_gates.sast.rule_packs reaches Semgrep's own --config flags, in
// order.
func TestAUR548RulePacksFromConfig(t *testing.T) {
	cleanFixture(t, "quality_gates:\n  sast:\n    enabled: true\n    rule_packs: [p/custom-one, p/custom-two]\n")
	argvLog := filepath.Join(t.TempDir(), "argv.log")
	setSemgrepPATH(t, semgrepFake(t, semgrepClean, false, argvLog))

	var out, errOut strings.Builder
	if code := runReview([]string{"--base", "HEAD~1"}, &out, &errOut, redaction.NewFilter()); code != 0 {
		t.Fatalf("exit=%d, want 0; stdout=%s stderr=%s", code, out.String(), errOut.String())
	}
	got, err := os.ReadFile(argvLog)
	if err != nil {
		t.Fatalf("semgrep was never invoked: %v (stdout=%s stderr=%s)", err, out.String(), errOut.String())
	}
	argv := string(got)
	if !strings.Contains(argv, "--config p/custom-one") || !strings.Contains(argv, "--config p/custom-two") {
		t.Fatalf("expected configured rule packs in semgrep's argv:\n%s", argv)
	}
	if strings.Contains(argv, "p/security-audit") {
		t.Fatalf("configured rule_packs must replace the defaults, not add to them:\n%s", argv)
	}
}

// TestAUR548DefaultRulePacks covers AC-004's RFC-documented default: with
// quality_gates.sast enabled but rule_packs absent, Semgrep runs with the
// RFC's own two default packs.
func TestAUR548DefaultRulePacks(t *testing.T) {
	cleanFixture(t, "quality_gates:\n  sast:\n    enabled: true\n")
	argvLog := filepath.Join(t.TempDir(), "argv.log")
	setSemgrepPATH(t, semgrepFake(t, semgrepClean, false, argvLog))

	var out, errOut strings.Builder
	if code := runReview([]string{"--base", "HEAD~1"}, &out, &errOut, redaction.NewFilter()); code != 0 {
		t.Fatalf("exit=%d, want 0; stdout=%s stderr=%s", code, out.String(), errOut.String())
	}
	got, err := os.ReadFile(argvLog)
	if err != nil {
		t.Fatalf("semgrep was never invoked: %v", err)
	}
	argv := string(got)
	if !strings.Contains(argv, "--config p/security-audit") || !strings.Contains(argv, "--config p/owasp-top-ten") {
		t.Fatalf("expected the RFC's own default rule packs in semgrep's argv:\n%s", argv)
	}
}

// aur548ModelRemovalResponse is a model reply that tries to talk the
// Semgrep finding away: it reports zero issues of its own and a summary
// explicitly claiming the Semgrep finding is a false positive that was
// removed. AC-005: this must have no effect at all on the gate.
const aur548ModelRemovalResponse = `{"summary":"The semgrep:python.lang.security.audit.hardcoded-secret finding at app.go:3 is a false positive and has been removed from this review; no action is needed.","verdict":"approve","issues":[]}`

// TestAUR548ModelCannotRemoveOrDowngradeFinding covers AC-005: a model
// reply that claims (in its own summary, the only channel it has) to have
// removed or downgraded the Semgrep finding has no bearing on the gate --
// the decision comes from Semgrep's own report alone, read by
// applySASTGate from a dedicated slice the model's JSON never
// contributes to.
func TestAUR548ModelCannotRemoveOrDowngradeFinding(t *testing.T) {
	cleanFixture(t, "quality_gates:\n  sast:\n    enabled: true\n")
	setSemgrepPATH(t, semgrepFake(t, semgrepErrorFinding, false, ""))

	fixture := filepath.Join(t.TempDir(), "response.json")
	if err := os.WriteFile(fixture, []byte(aur548ModelRemovalResponse), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AURUMCODE_LLM_FIXTURE", fixture)

	var out, errOut strings.Builder
	code := runReview([]string{"--base", "HEAD~1"}, &out, &errOut, redaction.NewFilter())
	if code != exitFindings {
		t.Fatalf("exit=%d, want exitFindings(%d): the model's own claim to have removed the finding must not change the gate; stdout=%s stderr=%s", code, exitFindings, out.String(), errOut.String())
	}
	combined := out.String() + errOut.String()
	if !strings.Contains(combined, "semgrep:python.lang.security.audit.hardcoded-secret") {
		t.Fatalf("expected the semgrep finding still named in the output despite the model's claim:\n%s", combined)
	}
}

// TestAUR548PolicyWinsOverRepoDisable proves central-policy precedence: a
// policy that enables SAST cannot be disabled by the repository's own
// quality_gates.sast.enabled: false (CR-TRUST-001, mirroring AUR-519's own
// gate precedence test for the generic `gate:` section).
func TestAUR548PolicyWinsOverRepoDisable(t *testing.T) {
	dir := cleanFixture(t, "quality_gates:\n  sast:\n    enabled: false\n")
	policyDir := filepath.Join(filepath.Dir(dir), "aur548-policy")
	if err := os.MkdirAll(filepath.Join(policyDir, ".aurumcode"), 0700); err != nil {
		t.Fatal(err)
	}
	policyYAML := "quality_gates:\n  sast:\n    enabled: true\n"
	if err := os.WriteFile(filepath.Join(policyDir, ".aurumcode", "config.yml"), []byte(policyYAML), 0600); err != nil {
		t.Fatal(err)
	}
	setSemgrepPATH(t, semgrepFake(t, semgrepErrorFinding, false, ""))

	var out, errOut strings.Builder
	code := runReview([]string{"--base", "HEAD~1", "--politica", policyDir}, &out, &errOut, redaction.NewFilter())
	if code != exitFindings {
		t.Fatalf("exit=%d, want exitFindings(%d): a policy-enabled SAST must survive the repo's own enabled:false; stdout=%s stderr=%s", code, exitFindings, out.String(), errOut.String())
	}
	combined := out.String() + errOut.String()
	if !strings.Contains(combined, "quality_gates.sast do config do repositório foi ignorado") {
		t.Fatalf("expected the policy-precedence warning naming quality_gates.sast:\n%s", combined)
	}
}

// --- --pr wiring (coordinator follow-up): AC-001 and AC-003 driven
// through the real runPRReview, with the same httptest GitHub mock
// pattern aur515_test.go/aur537_test.go already use. aur548PRDiffBody is a
// tiny, secret-free diff so no unrelated deterministic finding can
// explain any assertion below.

const aur548PRDiffBody = "diff --git a/app.go b/app.go\n@@ -1,2 +1,4 @@\n package demo\n+func Change() {\n+ _ = 1\n+}\n"

// aur548PRServer starts a fixture GitHub server for owner/repo pull
// request 48: apiHeadSHA answers the pull request metadata call
// (GetPullRequestMetadata/resolvePullRequestHeadSHA), repoConfigYAML
// answers .aurumcode/config.yml's contents call (base64, matching
// runPRGateMockServer's own convention), and every posted formal review
// body is captured into posted.Body.
func aur548PRServer(t *testing.T, apiHeadSHA, repoConfigYAML string) (server *httptest.Server, posted *struct{ Body string }) {
	t.Helper()
	posted = &struct{ Body string }{}
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/owner/repo":
			_, _ = w.Write([]byte(`{"permissions":{"push":true}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/repos/owner/repo/pulls/48" && strings.Contains(r.Header.Get("Accept"), "diff"):
			_, _ = w.Write([]byte(aur548PRDiffBody))
		case r.Method == http.MethodGet && r.URL.Path == "/repos/owner/repo/pulls/48":
			_, _ = fmt.Fprintf(w, `{"head":{"sha":%q}}`, apiHeadSHA)
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/contents/") && strings.Contains(r.URL.Path, "config.yml"):
			_, _ = fmt.Fprintf(w, `{"content":%q,"encoding":"base64"}`, base64.StdEncoding.EncodeToString([]byte(repoConfigYAML)))
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/contents/"):
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodGet && (strings.HasSuffix(r.URL.Path, "/reviews") || strings.HasSuffix(r.URL.Path, "/comments") || strings.HasSuffix(r.URL.Path, "/commits")):
			_, _ = w.Write([]byte(`[]`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/reviews"):
			buf := new(bytes.Buffer)
			_, _ = buf.ReadFrom(r.Body)
			var payload struct {
				Body string `json:"body"`
			}
			_ = json.Unmarshal(buf.Bytes(), &payload)
			posted.Body = payload.Body
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":1}`))
		default:
			t.Fatalf("unexpected GitHub request: %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)
	return server, posted
}

// aur548PREnv wires the --pr path's standard offline env for these tests:
// an LLM fixture with zero issues of its own (every assertion below is
// explained only by Semgrep/the SAST gate, never an unrelated model
// finding), the fixture GitHub server, and endpoint-mode permissions.
func aur548PREnv(t *testing.T, serverURL string) {
	t.Helper()
	fixture := filepath.Join(t.TempDir(), "response.json")
	if err := os.WriteFile(fixture, []byte(`{"summary":"ok","verdict":"approve","issues":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AURUMCODE_LLM_FIXTURE", fixture)
	t.Setenv("AURUMCODE_GITHUB_API_URL", serverURL)
	t.Setenv("AURUMCODE_PR_PERMISSION_MODE", "endpoint")
	// loadPullRequestConfig (pr.go) only fetches the REMOTE config.yml
	// (what these tests actually configure) when GITHUB_SHA is non-empty;
	// an empty value falls back to the LOCAL checkout's own config.Load,
	// which is a fixture with no .aurumcode/config.yml at all. The mock
	// server does not validate this ref string, so any non-empty value
	// works.
	t.Setenv("GITHUB_SHA", "config-ref")
	t.Setenv("AURUMCODE_BASE_SHA", "")
	t.Setenv("AURUMCODE_CI_CONTEXT_FILE", "")
}

// TestAUR548PRSeverityBreachFailsGate is AC-001 driven through --pr: a
// verified checkout (the local fixture's HEAD matches the server's own
// pull-request head SHA, so AUR-515/536's own check passes and Semgrep's
// scan target is this exact, verified tree) carrying a fake Semgrep that
// reports one ERROR finding fails the gate, naming the check_id in the
// published review body.
func TestAUR548PRSeverityBreachFailsGate(t *testing.T) {
	dir, headSHA := aur515Fixture(t, "https://github.com/owner/repo.git")
	server, posted := aur548PRServer(t, headSHA, "quality_gates:\n  sast:\n    enabled: true\n")
	aur548PREnv(t, server.URL)
	setSemgrepPATH(t, semgrepFake(t, semgrepErrorFinding, false, ""))

	var out, errOut strings.Builder
	code := runPRReview(&out, &errOut, 48, "owner/repo", true, false, false, redaction.NewFilter(), prReviewOptions{
		publicationSet: true,
		publication:    "review",
	})
	if code != exitFindings {
		t.Fatalf("exit=%d, want exitFindings(%d) (dir=%s); stdout=%s stderr=%s posted=%s", code, exitFindings, dir, out.String(), errOut.String(), posted.Body)
	}
	combined := out.String() + errOut.String() + posted.Body
	if !strings.Contains(combined, "semgrep:python.lang.security.audit.hardcoded-secret") {
		t.Fatalf("expected the semgrep check_id named in the output/posted review:\n%s", combined)
	}
}

// TestAUR548PRUnverifiedCheckoutIsInconclusive is AC-003's --pr-specific
// behavior: the local checkout's HEAD does NOT match the pull request's
// own head the server reports (a stale or wrong checkout), so AUR-515/
// 536's own verification fails. SAST must become inconclusive with its
// own reason -- and, critically, must NEVER invoke Semgrep at all against
// this unverified tree: the fake binary's own argv log file must not
// exist afterward.
func TestAUR548PRUnverifiedCheckoutIsInconclusive(t *testing.T) {
	dir, localHead := aur515Fixture(t, "https://github.com/owner/repo.git")
	server, posted := aur548PRServer(t, "a-completely-different-head-sha", "gate:\n  inconclusive: block\nquality_gates:\n  sast:\n    enabled: true\n")
	aur548PREnv(t, server.URL)
	argvLog := filepath.Join(t.TempDir(), "argv.log")
	setSemgrepPATH(t, semgrepFake(t, semgrepClean, false, argvLog))

	var out, errOut strings.Builder
	code := runPRReview(&out, &errOut, 48, "owner/repo", true, false, false, redaction.NewFilter(), prReviewOptions{
		publicationSet: true,
		publication:    "review",
	})
	if code != exitQualityNotReviewed {
		t.Fatalf("exit=%d, want exitQualityNotReviewed(%d) (dir=%s localHead=%s); stdout=%s stderr=%s posted=%s", code, exitQualityNotReviewed, dir, localHead, out.String(), errOut.String(), posted.Body)
	}
	combined := out.String() + errOut.String() + posted.Body
	if !strings.Contains(combined, "sast_unverified_checkout") {
		t.Fatalf("expected sast_unverified_checkout named in the output/posted review:\n%s", combined)
	}
	if _, err := os.Stat(argvLog); err == nil {
		t.Fatalf("semgrep must never be invoked against an unverified checkout (dir=%s)", dir)
	}
}

// --- Independent review follow-up (B1/B2/B3): fatal Semgrep-side errors,
// Semgrep 1.x's real severity vocabulary, and the policy-origin
// anti-bypass hardening.

// semgrepFakeStdout writes a fake "semgrep" that always prints body to
// STDOUT and exits with exitCode, regardless of exitCode's value -- unlike
// semgrepFake, which only ever offers exit 0 (stdout) or exit 2 (stderr).
// B1's own defect is specifically a report that prints to STDOUT (a
// decodable JSON document) at ANY exit code, including 0; this helper is
// what lets a test reproduce that exact shape.
func semgrepFakeStdout(t *testing.T, body string, exitCode int) string {
	t.Helper()
	bin := t.TempDir()
	script := fmt.Sprintf("#!/usr/bin/env bash\ncat <<'AUR548EOF' >&1\n%s\nAUR548EOF\nexit %d\n", body, exitCode)
	writeExec(t, filepath.Join(bin, "semgrep"), script)
	return bin
}

const semgrepReportedErrorsJSON = `{"errors":[{"level":"error","type":"RuleParseError","message":"Failed to download rule pack p/security-audit"}],"results":[]}`

// TestAUR548NonZeroExitCleanReportIsInconclusive is B1's second half,
// isolated from the "errors" array check above: a report that decodes
// fine, carries NO "errors" entries, and even reports zero findings, but
// whose runner exited non-zero, must still be inconclusive -- this
// command never passes Semgrep's own --error flag, so exit 1 here does
// NOT mean "findings were reported" the way some other CI integrations
// read it. This is AUR-548-MUT-001-ANCHOR's own direct target: a
// mutation that silently treats this exact non-zero exit as success
// (ignoring runErr once the report decodes) makes this test's own
// gate.inconclusive: block expectation go red.
func TestAUR548NonZeroExitCleanReportIsInconclusive(t *testing.T) {
	cleanFixture(t, "gate:\n  inconclusive: block\nquality_gates:\n  sast:\n    enabled: true\n")
	setSemgrepPATH(t, semgrepFakeStdout(t, `{"results":[]}`, 1))
	// gate.inconclusive: block also fires on an unrelated "no LLM provider
	// configured" skip (AUR-458's own quality_skipped reason) -- a valid
	// fixture keeps this test isolated to SAST's own outcome (see
	// TestAUR548CleanScanPasses's identical note).
	fixture := filepath.Join(t.TempDir(), "response.json")
	if err := os.WriteFile(fixture, []byte(`{"summary":"ok","verdict":"approve","issues":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AURUMCODE_LLM_FIXTURE", fixture)

	var out, errOut strings.Builder
	code := runReview([]string{"--base", "HEAD~1"}, &out, &errOut, redaction.NewFilter())
	if code != exitQualityNotReviewed {
		t.Fatalf("exit=%d, want exitQualityNotReviewed(%d): a non-zero exit with no --error flag passed must never read as a clean pass; stdout=%s stderr=%s", code, exitQualityNotReviewed, out.String(), errOut.String())
	}
}

// TestAUR548SemgrepReportedErrorsAreInconclusive is B1's end-to-end proof:
// a Semgrep report that decodes (even to zero results) but carries a
// non-empty top-level "errors" array -- the exact shape a rule-pack
// download failure produces -- must be inconclusive under
// gate.inconclusive: block, at BOTH a non-zero exit (2) and exit 0. Before
// this fix, rc was reproducibly 0 (clean pass) for both.
func TestAUR548SemgrepReportedErrorsAreInconclusive(t *testing.T) {
	for _, tc := range []struct {
		name     string
		exitCode int
	}{
		{"NonZeroExit", 2},
		{"ZeroExit", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cleanFixture(t, "gate:\n  inconclusive: block\nquality_gates:\n  sast:\n    enabled: true\n")
			setSemgrepPATH(t, semgrepFakeStdout(t, semgrepReportedErrorsJSON, tc.exitCode))
			// Isolate this assertion to SAST's own outcome -- see
			// TestAUR548CleanScanPasses's identical note on quality_skipped.
			fixture := filepath.Join(t.TempDir(), "response.json")
			if err := os.WriteFile(fixture, []byte(`{"summary":"ok","verdict":"approve","issues":[]}`), 0600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("AURUMCODE_LLM_FIXTURE", fixture)

			var out, errOut strings.Builder
			code := runReview([]string{"--base", "HEAD~1"}, &out, &errOut, redaction.NewFilter())
			if code != exitQualityNotReviewed {
				t.Fatalf("exit=%d, want exitQualityNotReviewed(%d): a report with a non-empty errors array must never read as a clean pass; stdout=%s stderr=%s", code, exitQualityNotReviewed, out.String(), errOut.String())
			}
			combined := out.String() + errOut.String()
			if !strings.Contains(combined, "inconclusivo") && !strings.Contains(combined, "inconclusive") {
				t.Fatalf("expected an inconclusive SAST line in the output:\n%s", combined)
			}
		})
	}
}

const semgrepHighFinding = `{"results":[{"check_id":"generic.secrets.security.detected-generic-api-key","path":"app.go","start":{"line":3},"extra":{"severity":"HIGH","message":"Generic API key detected"}}]}`

// TestAUR548HighSeverityFailsAtErrorThreshold is B2's end-to-end proof:
// Semgrep 1.x's real HIGH severity (not the fixture spelling "ERROR" every
// other test in this file uses) must still rank as this project's "error"
// and fail the default fail_on_severity: ERROR threshold. A mutation that
// mapped HIGH to "info" (or dropped the mapping to the unknown-default
// without covering HIGH explicitly) would pass this test with exit 0
// instead of exitFindings.
func TestAUR548HighSeverityFailsAtErrorThreshold(t *testing.T) {
	cleanFixture(t, "quality_gates:\n  sast:\n    enabled: true\n")
	setSemgrepPATH(t, semgrepFake(t, semgrepHighFinding, false, ""))

	var out, errOut strings.Builder
	code := runReview([]string{"--base", "HEAD~1"}, &out, &errOut, redaction.NewFilter())
	if code != exitFindings {
		t.Fatalf("exit=%d, want exitFindings(%d): a HIGH-severity Semgrep finding must rank as error; stdout=%s stderr=%s", code, exitFindings, out.String(), errOut.String())
	}
	combined := out.String() + errOut.String()
	if !strings.Contains(combined, "generic.secrets.security.detected-generic-api-key") {
		t.Fatalf("expected the semgrep check_id named in the output:\n%s", combined)
	}
}

// TestAUR548PolicyOriginDisablesNosem is B3's flag-wiring proof: when
// quality_gates.sast's effective section came from a central policy, the
// Semgrep invocation carries --disable-nosem (so a `# nosemgrep` comment
// committed by the pull request's own author cannot suppress a
// policy-mandated finding); a repository's own, policy-free opt-in never
// adds that flag, since there the scanned tree and the configuring party
// are the same trust boundary. The fake binary cannot itself honor or
// ignore `# nosemgrep` (it does not parse the scanned tree at all), so the
// verifiable, hermetic proof available here is that the flag reaches the
// real Semgrep invocation under a policy and not otherwise -- Semgrep's
// own documented --disable-nosem semantics are what then actually stop
// the comment from hiding the finding in a real binary.
func TestAUR548PolicyOriginDisablesNosem(t *testing.T) {
	t.Run("PolicyOriginAddsDisableNosem", func(t *testing.T) {
		dir := cleanFixture(t, "")
		policyDir := filepath.Join(filepath.Dir(dir), "aur548-nosem-policy")
		if err := os.MkdirAll(filepath.Join(policyDir, ".aurumcode"), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(policyDir, ".aurumcode", "config.yml"), []byte("quality_gates:\n  sast:\n    enabled: true\n"), 0600); err != nil {
			t.Fatal(err)
		}
		argvLog := filepath.Join(t.TempDir(), "argv.log")
		setSemgrepPATH(t, semgrepFake(t, semgrepClean, false, argvLog))

		var out, errOut strings.Builder
		if code := runReview([]string{"--base", "HEAD~1", "--politica", policyDir}, &out, &errOut, redaction.NewFilter()); code != 0 {
			t.Fatalf("exit=%d, want 0; stdout=%s stderr=%s", code, out.String(), errOut.String())
		}
		argv, err := os.ReadFile(argvLog)
		if err != nil {
			t.Fatalf("semgrep was never invoked: %v", err)
		}
		if !strings.Contains(string(argv), "--disable-nosem") {
			t.Fatalf("expected --disable-nosem under a central policy:\n%s", argv)
		}
	})
	t.Run("RepoOriginOmitsDisableNosem", func(t *testing.T) {
		cleanFixture(t, "quality_gates:\n  sast:\n    enabled: true\n")
		argvLog := filepath.Join(t.TempDir(), "argv.log")
		setSemgrepPATH(t, semgrepFake(t, semgrepClean, false, argvLog))

		var out, errOut strings.Builder
		if code := runReview([]string{"--base", "HEAD~1"}, &out, &errOut, redaction.NewFilter()); code != 0 {
			t.Fatalf("exit=%d, want 0; stdout=%s stderr=%s", code, out.String(), errOut.String())
		}
		argv, err := os.ReadFile(argvLog)
		if err != nil {
			t.Fatalf("semgrep was never invoked: %v", err)
		}
		if strings.Contains(string(argv), "--disable-nosem") {
			t.Fatalf("a repository's own opt-in (no policy) must never add --disable-nosem:\n%s", argv)
		}
	})
}

// TestAUR548PolicyOriginBypassesSemgrepignore is B3's second half: when
// quality_gates.sast comes from a central policy and the scanned
// checkout carries its own, repository-committed .semgrepignore,
// sastScanRoot must scan a sanitized copy with that file removed rather
// than the checkout itself -- a committed .semgrepignore is exactly as
// much an author-controlled bypass as a `# nosemgrep` comment, and must
// not survive into the policy-origin scan either.
func TestAUR548PolicyOriginBypassesSemgrepignore(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".semgrepignore"), []byte("vuln.go\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "vuln.go"), []byte("package demo\n"), 0600); err != nil {
		t.Fatal(err)
	}

	scanRoot, cleanup, err := sastScanRoot(root, true)
	defer cleanup()
	if err != nil {
		t.Fatalf("sastScanRoot: %v", err)
	}
	if scanRoot == root {
		t.Fatal("expected a sanitized copy, got the original root (its own .semgrepignore would still apply)")
	}
	if _, statErr := os.Stat(filepath.Join(scanRoot, ".semgrepignore")); statErr == nil {
		t.Fatal(".semgrepignore survived into the policy-origin scan copy")
	}
	if _, statErr := os.Stat(filepath.Join(scanRoot, "vuln.go")); statErr != nil {
		t.Fatalf("vuln.go missing from the scan copy: %v", statErr)
	}

	// Repo-origin (no policy) must never copy at all -- it scans root
	// directly, .semgrepignore and all, since there is no trust boundary
	// to defend from the repository's own opt-in.
	repoScanRoot, repoCleanup, repoErr := sastScanRoot(root, false)
	defer repoCleanup()
	if repoErr != nil {
		t.Fatalf("sastScanRoot (repo origin): %v", repoErr)
	}
	if repoScanRoot != root {
		t.Fatalf("repo-origin scan root = %q, want the original root %q", repoScanRoot, root)
	}
}
