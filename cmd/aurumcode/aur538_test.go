package main

// AUR-538 behavior proof: closes the non-blocking gaps the AUR-519/520
// reviews left open (see .board/cards/backlog/AUR-538.md, AC-001..AC-008).
// AC-003 (the AUR-519 e2e suite's own decorative assertion) is fixed in
// aur519_e2e_test.go directly; AC-008 is a tests/acceptance/AUR-520.sh
// shell fix. Everything else lives here.

import (
	"bytes"
	"compress/zlib"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	igate "github.com/Mpaape/AurumCode/internal/gate"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/git/githubclient"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
)

// aur538CleanFixture builds a two-file git repository (app.go,
// tests/change_test.go) across HEAD~1..HEAD, the same shape
// coverageFixture (aur476_test.go) uses to trigger AUR-476's own
// partial-coverage detection via a repo-level `ignore: ["tests/**"]`, but
// with NO secret-shaped content anywhere: coverageFixture's app.go
// embeds a literal "hunter2-super-secret" line for AUR-476's own,
// unrelated purposes, which means a verdict assertion driven through
// that fixture can be trivially true for reasons that have nothing to do
// with the policy gate's own verdict-withholding machinery under test
// here -- exactly the "decorative assertion" trap this card closes
// (AC-001/AC-003). A test against THIS fixture can only be explained by
// that machinery.
func aur538CleanFixture(t *testing.T, configYAML string) string {
	t.Helper()
	return aur538CleanFixtureTests(t, configYAML, "")
}

// aur538CleanFixtureTests is aur538CleanFixture with testsPrefix written at
// the top of the head tests/change_test.go (generatedMarker makes the file
// generated, the partial-coverage trigger).
func aur538CleanFixtureTests(t *testing.T, configYAML, testsPrefix string) string {
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
	appBase := []byte("package demo\nfunc Change() int {\n return 1\n}\n")
	appHead := []byte("package demo\nfunc Change() int {\n return 42\n}\n")
	testsBase := []byte("package demo\n\nimport \"testing\"\n\nfunc TestOld(t *testing.T) {}\n")
	testsHead := []byte(testsPrefix + "package demo\n\nimport \"testing\"\n\nfunc TestChange(t *testing.T) {\n if Change() != 42 {\n  t.Fatal(\"boom\")\n }\n}\n")

	appBaseID := gitObject(t, dir, "blob", appBase)
	appHeadID := gitObject(t, dir, "blob", appHead)
	testsBaseID := gitObject(t, dir, "blob", testsBase)
	testsHeadID := gitObject(t, dir, "blob", testsHead)

	testsTreeBase := gitObject(t, dir, "tree", treeEntry(t, "100644", "change_test.go", testsBaseID))
	testsTreeHead := gitObject(t, dir, "tree", treeEntry(t, "100644", "change_test.go", testsHeadID))

	rootBaseBody := append(treeEntry(t, "100644", "app.go", appBaseID), treeEntry(t, "40000", "tests", testsTreeBase)...)
	rootHeadBody := append(treeEntry(t, "100644", "app.go", appHeadID), treeEntry(t, "40000", "tests", testsTreeHead)...)
	rootBase := gitObject(t, dir, "tree", rootBaseBody)
	rootHead := gitObject(t, dir, "tree", rootHeadBody)

	commit := func(tree, parent string) string {
		body := "tree " + tree + "\n"
		if parent != "" {
			body += "parent " + parent + "\n"
		}
		body += "author Fixture <fixture@example.invalid> 1 +0000\ncommitter Fixture <fixture@example.invalid> 1 +0000\n\nfixture\n"
		return gitObject(t, dir, "commit", []byte(body))
	}
	base := commit(rootBase, "")
	head := commit(rootHead, base)

	write(".git/HEAD", []byte("ref: refs/heads/main\n"))
	write(".git/refs/heads/main", []byte(head+"\n"))
	write(".git/config", []byte("[core]\nrepositoryformatversion = 0\nbare = false\n"))
	write("app.go", appHead)
	write("tests/change_test.go", testsHead)
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

// TestAUR538BaseCleanFixtureVerdictWithheldUnderBlock covers AC-001: a
// fixture with NO findings at all (no hardcoded secret, zero model
// issues) under gate.inconclusive: block (triggered by AUR-476's own
// partial-coverage detection, via a repo-ignored second file) must never
// let the --base report read "**Verdict:** Approve" for a model verdict
// of "" or "changes_requested". Removing main.go's own verdict
// pull-down/withheld marker (the
// `result.Metadata[prompt.PolicyGateWithheldKey] = "true"` line inside
// "if gateResult.Fail || gateResult.Inconclusive") must fail this test
// (AC-001/MUT-001).
func TestAUR538BaseCleanFixtureVerdictWithheldUnderBlock(t *testing.T) {
	for _, modelVerdict := range []string{"", "changes_requested"} {
		t.Run("model_verdict_"+modelVerdict, func(t *testing.T) {
			aur538CleanFixtureTests(t, "gate:\n  inconclusive: block\n", generatedMarker)
			resp := `{"summary":"ok","issues":[]}`
			if modelVerdict != "" {
				resp = fmt.Sprintf(`{"summary":"ok","verdict":%q,"issues":[]}`, modelVerdict)
			}
			fixture := filepath.Join(t.TempDir(), "response.json")
			if err := os.WriteFile(fixture, []byte(resp), 0600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("AURUMCODE_LLM_FIXTURE", fixture)

			var out, errOut strings.Builder
			code := runReview([]string{"--base", "HEAD~1"}, &out, &errOut, redaction.NewFilter())
			if code != exitQualityNotReviewed {
				t.Fatalf("exit=%d, want exitQualityNotReviewed(%d); stdout=%s stderr=%s", code, exitQualityNotReviewed, out.String(), errOut.String())
			}
			combined := out.String() + errOut.String()
			if !strings.Contains(combined, "policy gate: review inconclusive (partial_coverage)") {
				t.Fatalf("expected the policy-gate inconclusive line naming partial_coverage:\n%s", combined)
			}
			if strings.Contains(out.String(), "**Verdict:** Approve") {
				t.Fatalf("model verdict %q: verdict read as Approve under a blocking gate with NO findings at all:\n%s", modelVerdict, out.String())
			}
		})
	}
}

// TestAUR538BaseCleanFixtureApprovesWithoutGate is AC-001's own positive
// control: the identical fixture and a legitimate "approve" model
// response, with NO `gate:` declared at all, must still publish
// "**Verdict:** Approve" -- proving the withheld assertion above actually
// exercises the gate's own pull-down and is not trivially true for some
// other, unrelated property of the fixture (the exact trap AC-003 closes
// for a different, pre-existing test).
func TestAUR538BaseCleanFixtureApprovesWithoutGate(t *testing.T) {
	aur538CleanFixture(t, "")
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
	if !strings.Contains(out.String(), "**Verdict:** Approve") {
		t.Fatalf("without a gate, a clean approve verdict must still publish unchanged:\n%s", out.String())
	}
}

// TestAUR538MissingTreeObjectIsUnverifiable covers AC-004: a checkout
// verified as having real git metadata, whose HEAD tree object has been
// deleted from the (loose-object-only) store -- the sealed acceptance
// profile's own shape, never a packed repository -- must be declared
// unverifiable, both with and without a `git` binary on PATH. This calls
// verifiedCleanCheckoutReason (aur536.go) directly, bypassing
// codebaseContextMismatch's own, earlier unverifiable checks entirely, so
// the assertion can only be explained by THIS function's own
// `if err != nil { return codebaseContextReasonUnverifiable, nil }`
// branch around repo.TrackedFiles -- never by some earlier check that
// happens to use the same wording for an unrelated reason. Treating that
// TrackedFiles error as a clean tree (returning "", files instead) must
// fail this test.
func TestAUR538MissingTreeObjectIsUnverifiable(t *testing.T) {
	dir, _ := aur515Fixture(t, "https://github.com/owner/repo.git")

	// Recompute the exact root tree id aur515Fixture already built, from
	// a throwaway scratch store -- gitObject is a pure hash+write, so
	// hashing the identical content here never disturbs dir's own
	// objects -- then delete that one object from dir's real store.
	appLocal := []byte("package demo\n\nfunc " + aur515LocalMarker + "() {}\n")
	scratch := t.TempDir()
	appBlob := gitObject(t, scratch, "blob", appLocal)
	rootTree := gitObject(t, scratch, "tree", treeEntry(t, "100644", "app.go", appBlob))
	treeObjectPath := filepath.Join(dir, ".git", "objects", rootTree[:2], rootTree[2:])
	if _, err := os.Stat(treeObjectPath); err != nil {
		t.Fatalf("precondition: tree object missing before deletion: %v", err)
	}
	if err := os.Remove(treeObjectPath); err != nil {
		t.Fatalf("removing HEAD tree object: %v", err)
	}

	t.Run("git_less", func(t *testing.T) {
		t.Setenv("PATH", "")
		reason, files := verifiedCleanCheckoutReason(dir)
		if reason != codebaseContextReasonUnverifiable || files != nil {
			t.Fatalf("reason=%q files=%v, want (%q, nil): a missing HEAD tree object must be unverifiable, never clean, with no git binary available", reason, files, codebaseContextReasonUnverifiable)
		}
	})

	if _, err := exec.LookPath("git"); err == nil {
		t.Run("with_git", func(t *testing.T) {
			reason, files := verifiedCleanCheckoutReason(dir)
			if reason != codebaseContextReasonUnverifiable || files != nil {
				t.Fatalf("reason=%q files=%v, want (%q, nil): a missing HEAD tree object must be unverifiable even with a git binary present", reason, files, codebaseContextReasonUnverifiable)
			}
		})
	} else {
		t.Log("git not available on this host; git_less leg already covers the sealed acceptance profile's own shape")
	}
}

// aur538ExceptionFixtureNoOrigin mirrors aur520_test.go's own
// exceptionFixture (a local checkout with a configured `exceptions:`
// section), except .git/config here declares NO "origin" remote at all
// (AC-005): localRepoIdentity (aur520.go) can then never resolve a repo
// identity, so every configured exception must fail closed no matter how
// exactly it would otherwise have matched.
func aur538ExceptionFixtureNoOrigin(t *testing.T, configYAML string) string {
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
	source := "package demo\nfunc Change() int {\n dbPassword := \"hunter2-super-secret\"\n _ = dbPassword\n return 42\n}\n"
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

// TestAUR538BaseNoOriginExceptionNeverMatches covers AC-005's first case:
// with no "origin" remote configured, localRepoIdentity cannot resolve
// which repository is under review, so a configured exception that would
// otherwise match the exact finding (repo/rule/path all equal) must fail
// closed -- the gate still fails on the breach, and the identity-
// unavailable notice is published.
func TestAUR538BaseNoOriginExceptionNeverMatches(t *testing.T) {
	cfg := "review:\n  context:\n    skills:\n      - skills/security.md\n" +
		"gate:\n  fail_on: [high]\n  sources: [skills]\n" +
		"exceptions:\n  - repo: org/repo\n    rule: security#no-hardcoded-secrets\n    path: app.go\n" +
		"    owner: time-seguranca\n    reason: consulta fixa, sem entrada do usuario\n    expires: 2099-12-31\n"
	dir := aur538ExceptionFixtureNoOrigin(t, cfg)
	if err := os.MkdirAll(filepath.Join(dir, "skills"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "skills", "security.md"), []byte("## No Hardcoded Secrets\n\nNever commit a literal credential.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AURUMCODE_LLM_FIXTURE", excResponseFixture(t, "security#no-hardcoded-secrets", "app.go"))

	var out, errOut strings.Builder
	code := runReview([]string{"--base", "HEAD~1"}, &out, &errOut, redaction.NewFilter())
	if code != exitFindings {
		t.Fatalf("exit=%d, want exitFindings(%d): with no origin remote, the configured exception must fail closed and the breach must still fail the gate; stdout=%s stderr=%s", code, exitFindings, out.String(), errOut.String())
	}
	combined := out.String() + errOut.String()
	if !strings.Contains(combined, igate.RepoIdentityUnavailableNotice("en-US")) {
		t.Fatalf("expected the identity-unavailable notice:\n%s", combined)
	}
	if strings.Contains(combined, "aceito por exceção") {
		t.Fatalf("an exception must never be shown as accepted when the repo identity could not be confirmed:\n%s", combined)
	}
}

// TestAUR538BaseNoOriginNoExceptionsNoNotice covers AC-005's second case:
// with no "origin" remote AND no exceptions configured anywhere, the
// identity-unavailable notice must never appear at all -- a run that
// never uses this feature carries no new line (exceptionsConfigured's own
// guard, aur520.go).
func TestAUR538BaseNoOriginNoExceptionsNoNotice(t *testing.T) {
	aur538ExceptionFixtureNoOrigin(t, "gate:\n  fail_on: [high]\n  sources: [skills]\n")
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
	combined := out.String() + errOut.String()
	if strings.Contains(combined, "Exceptions disabled") || strings.Contains(combined, "Exceções desativadas") {
		t.Fatalf("no exceptions configured anywhere: the identity-unavailable notice must never appear:\n%s", combined)
	}
}

// TestAUR538MatchExceptionActivePreferredOverExpiredSameFinding covers
// AC-006: a config can list an expired exception for a finding followed
// later by a renewed one for the exact same (repo, rule, path) -- keeping
// the expired entry for audit history while a fresh one takes over. The
// active, renewed entry must apply regardless of list order.
func TestAUR538MatchExceptionActivePreferredOverExpiredSameFinding(t *testing.T) {
	now := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	expired := config.ExceptionConfig{Repo: "org/repo", Rule: "security#no-hardcoded-secrets", Path: "app.go", Owner: "antigo-dono", Reason: "legado", Expires: "2020-01-01"}
	renewed := config.ExceptionConfig{Repo: "org/repo", Rule: "security#no-hardcoded-secrets", Path: "app.go", Owner: "novo-dono", Reason: "renovado", Expires: "2099-12-31"}

	exc, status := igate.MatchException([]config.ExceptionConfig{expired, renewed}, "org/repo", "security#no-hardcoded-secrets", "app.go", now)
	if status != igate.ExceptionActive {
		t.Fatalf("status = %v, want igate.ExceptionActive: a renewed, still-active exception for the same finding must win over an earlier, expired entry listed before it", status)
	}
	if exc.Owner != "novo-dono" {
		t.Fatalf("matched exception owner = %q, want %q (the renewed entry)", exc.Owner, "novo-dono")
	}
}

// TestAUR538CapStatusDescriptionRuneSafe covers AC-007's own cut: the cap
// counts runes, not bytes (a byte cut could split a multi-byte rune and
// corrupt the string), always keeps the leading result word intact, and
// ends a truncated string with a single ellipsis rune.
func TestAUR538CapStatusDescriptionRuneSafe(t *testing.T) {
	detail := strings.Repeat("ã", 200)
	got := capStatusDescription(gateStatusWordFailure, detail, statusDescriptionLimit)
	if n := utf8.RuneCountInString(got); n != statusDescriptionLimit {
		t.Fatalf("rune count = %d, want exactly %d", n, statusDescriptionLimit)
	}
	if !utf8.ValidString(got) {
		t.Fatalf("capStatusDescription produced invalid UTF-8: %q", got)
	}
	if !strings.HasPrefix(got, gateStatusWordFailure+": ") {
		t.Fatalf("result word must lead the description: %q", got)
	}
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("a truncated description must end with an ellipsis: %q", got)
	}
}

// TestAUR538PublishPolicyGateStatusDescriptionCapped covers AC-007 end to
// end through publishPolicyGateStatus (policygate.go): a long rule title
// and owner/reason text, combined with GitHub's 140-character status
// description limit, must still publish with the result word first and
// the real breach named -- never silently truncated down to just the
// inconclusive alert, and never longer than the limit (which GitHub's own
// API would otherwise reject outright).
func TestAUR538PublishPolicyGateStatusDescriptionCapped(t *testing.T) {
	var published githubclient.CommitStatus
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/owner/repo":
			_, _ = w.Write([]byte(`{"permissions":{"push":true}}`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/statuses/head"):
			_ = json.NewDecoder(r.Body).Decode(&published)
			w.WriteHeader(http.StatusCreated)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()
	client := githubclient.NewClientWithBaseURL("test-token", server.URL)

	longOwner := strings.Repeat("Equipe de Conformidade e Política Interna Bastante Extensa ", 2)
	breachLine := fmt.Sprintf("security#no-hardcoded-secrets: %s (severidade error, limiar error)", longOwner)
	d := gateDecision{Active: true, Fail: true, Breach: true, Inconclusive: true, Lines: []string{"review inconclusive (partial_coverage)", breachLine}}

	var out, errOut strings.Builder
	code := publishPolicyGateStatus(context.Background(), client, &out, &errOut, "owner", "repo", "head", d, 48)
	if code != exitFindings {
		t.Fatalf("code=%d, want exitFindings; stderr=%s", code, errOut.String())
	}
	if n := utf8.RuneCountInString(published.Description); n > statusDescriptionLimit {
		t.Fatalf("published description is %d runes, want <= %d: %q", n, statusDescriptionLimit, published.Description)
	}
	if !strings.HasPrefix(published.Description, gateStatusWordFailure+":") {
		t.Fatalf("description must lead with the result word %q: %q", gateStatusWordFailure, published.Description)
	}
	if !strings.Contains(published.Description, "security#no-hardcoded-secrets") {
		t.Fatalf("the breach's own rule id must survive the cap (ordered ahead of the inconclusive reason): %q", published.Description)
	}
}

// TestAUR538OrderedGateReasonsBreachBeforeExceptionBeforeInconclusive
// covers N3: within the capped status description, a real breach line
// must survive even when gateDecision.Lines lists a long accepted-
// exception line and the inconclusive-reason line ahead of it --
// orderedGateReasons must put the breach first, the exception line
// second, and the fixed inconclusive-reason line last, regardless of
// Lines' own order.
func TestAUR538OrderedGateReasonsBreachBeforeExceptionBeforeInconclusive(t *testing.T) {
	exceptionLine := "other#rule em outro-arquivo.go: aceito por exceção (dono: " +
		strings.Repeat("dono-bastante-longo ", 6) + ", motivo: " + strings.Repeat("motivo-bastante-longo ", 6) + ", validade: 2099-12-31)"
	breachLine := "security#no-hardcoded-secrets: No Hardcoded Secrets (severidade error, limiar error)"
	inconclusiveLine := "review inconclusive (partial_coverage)"

	got := orderedGateReasons([]string{exceptionLine, breachLine, inconclusiveLine})
	want := breachLine + "; " + exceptionLine + "; " + inconclusiveLine
	if got != want {
		t.Fatalf("orderedGateReasons ordering =\n%q\nwant (breach, then exception, then inconclusive):\n%q", got, want)
	}

	capped := capStatusDescription(gateStatusWordFailure, got, statusDescriptionLimit)
	if !strings.Contains(capped, "security#no-hardcoded-secrets") {
		t.Fatalf("the breach line must survive the 140-char cap even with a long exception line ahead of it in Lines: %q", capped)
	}
}

// TestAUR538PublishPolicyGateStatusWordAndStateTable covers B2: every one
// of publishPolicyGateStatus's five outcome branches must publish BOTH
// the correct commit-status State AND the correct leading result word --
// a table test, so swapping which word two branches use (for instance,
// relabeling the blocking-inconclusive branch "aprovado" instead of
// "inconclusivo") cannot silently pass just because some other branch's
// assertion happens to still hold.
func TestAUR538PublishPolicyGateStatusWordAndStateTable(t *testing.T) {
	breachLine := "security#no-hardcoded-secrets: No Hardcoded Secrets (severidade error, limiar error)"
	inconclusiveLine := "review inconclusive (partial_coverage)"

	cases := []struct {
		name      string
		decision  gateDecision
		wantState string
		wantWord  string
		wantCode  int
	}{
		{
			name:      "breach_and_inconclusive",
			decision:  gateDecision{Active: true, Fail: true, Breach: true, Inconclusive: true, Lines: []string{inconclusiveLine, breachLine}},
			wantState: "failure", wantWord: gateStatusWordFailure, wantCode: exitFindings,
		},
		{
			name:      "breach_only",
			decision:  gateDecision{Active: true, Fail: true, Breach: true, Lines: []string{breachLine}},
			wantState: "failure", wantWord: gateStatusWordFailure, wantCode: exitFindings,
		},
		{
			name:      "block_inconclusive_no_breach",
			decision:  gateDecision{Active: true, Fail: true, Breach: false, Inconclusive: true, Lines: []string{inconclusiveLine}},
			wantState: "failure", wantWord: gateStatusWordInconclusive, wantCode: exitQualityNotReviewed,
		},
		{
			name:      "warn_inconclusive_no_breach",
			decision:  gateDecision{Active: true, Fail: false, Breach: false, Inconclusive: true, Lines: []string{inconclusiveLine}},
			wantState: "success", wantWord: gateStatusWordInconclusive, wantCode: 0,
		},
		{
			name:      "clean",
			decision:  gateDecision{Active: true, Fail: false, Breach: false, Inconclusive: false},
			wantState: "success", wantWord: gateStatusWordApproved, wantCode: 0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var published githubclient.CommitStatus
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == http.MethodGet && r.URL.Path == "/repos/owner/repo":
					_, _ = w.Write([]byte(`{"permissions":{"push":true}}`))
				case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/statuses/head"):
					_ = json.NewDecoder(r.Body).Decode(&published)
					w.WriteHeader(http.StatusCreated)
				default:
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
				}
			}))
			defer server.Close()
			client := githubclient.NewClientWithBaseURL("test-token", server.URL)

			var out, errOut strings.Builder
			code := publishPolicyGateStatus(context.Background(), client, &out, &errOut, "owner", "repo", "head", tc.decision, 48)
			if code != tc.wantCode {
				t.Fatalf("code=%d, want %d; stderr=%s", code, tc.wantCode, errOut.String())
			}
			if published.State != tc.wantState {
				t.Fatalf("published state = %q, want %q", published.State, tc.wantState)
			}
			if !strings.HasPrefix(published.Description, tc.wantWord) {
				t.Fatalf("published description = %q, want it to lead with result word %q", published.Description, tc.wantWord)
			}
		})
	}
}

// TestAUR538OrderedGateReasonsExactExceptionMarkersNotBareWord covers the
// non-blocking classification fix: a breach line whose own rule.Title
// happens to contain the EXACT word "exceção" (a policy/repo author
// could title a rule anything) must stay in the breach tier, never be
// misclassified into the exception tier just because the bare word
// appears somewhere in the line. Only acceptedExceptionLine/
// expiredExceptionLine's own fixed markers (aur520.go) select that tier.
//
// The title below is "Tratamento de exceção" (singular), which DOES
// contain the exact substring "exceção" -- unlike an earlier draft of
// this test that used "exceções" (plural): "exceções" ends in "ções"
// (ç, õ, e, s), never "ção" (ç, ã, o), so it does NOT contain "exceção"
// as a substring and would have passed under the OLD, reverted
// bare-word check too, proving nothing.
//
// A GENUINE exception line (built from acceptedExceptionLine's own
// format, which also contains "exceção") is included and placed BEFORE
// the breach line in the input, specifically so only a classification
// difference -- never input order -- can explain the output order:
// under the correct, marker-based classification the breach line still
// sorts first (breach tier is joined before the exception tier); under
// the reverted bare-word check, both lines fall into the same
// "contains exceção" bucket and keep the INPUT's order instead, putting
// the genuine exception line first. This was confirmed, in a scratch
// copy with orderedGateReasons' classification reverted to
// `strings.Contains(line, "exceção")`, to flip the order and fail this
// test's assertion -- see the commit message for that verification.
func TestAUR538OrderedGateReasonsExactExceptionMarkersNotBareWord(t *testing.T) {
	genuineExceptionLine := "other#rule em outro.go: aceito por exceção (dono: time-x, motivo: y, validade: 2099-12-31)"
	breachLineMentioningException := "quality#excecao: Tratamento de exceção (severidade error, limiar error)"
	inconclusiveLine := "review inconclusive (partial_coverage)"

	got := orderedGateReasons([]string{genuineExceptionLine, breachLineMentioningException, inconclusiveLine})
	want := breachLineMentioningException + "; " + genuineExceptionLine + "; " + inconclusiveLine
	if got != want {
		t.Fatalf("orderedGateReasons =\n%q\nwant the breach line (which merely mentions \"exceção\") ordered BEFORE the genuine exception line, not grouped with it:\n%q", got, want)
	}
}
