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
