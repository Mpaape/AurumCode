package gitleaks_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/gate"
	"github.com/Mpaape/AurumCode/internal/scanner"
	"github.com/Mpaape/AurumCode/internal/scanner/gitleaks"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// fakeToken is credential-shaped only at run time: no tracked file may
// carry a literal with a vendor token's shape.
var fakeToken = "ghp" + "_" + strings.Repeat("a1B2c3D4", 4) + "e5F6"

const (
	baseSHA      = "1111111111111111111111111111111111111111"
	headSHA      = "2222222222222222222222222222222222222222"
	middleCommit = "3333333333333333333333333333333333333333"
)

// fakeLeak is one leak the fake repository holds: in the history of the
// range only, or also in the final tree, and whether an inline
// `gitleaks:allow` marks it.
type fakeLeak struct {
	file   string
	line   int
	inTree bool
	allow  bool
}

// fakeRepo emulates git and the pinned gitleaks over a repository whose
// leaks are fakeLeaks: `gitleaks git --log-opts=<range>` sees every leak of
// the range, `gitleaks dir` only those still in the tree, and an allow
// comment hides a leak unless --ignore-gitleaks-allow is given. The report
// carries the secret in Secret and Match, as gitleaks' own report does.
type fakeRepo struct {
	version   string
	missing   map[string]bool
	shallow   bool
	leaks     []fakeLeak
	stderr    string
	report    string
	scanCalls [][]string
}

func newFakeRepo(leaks ...fakeLeak) *fakeRepo {
	return &fakeRepo{version: gitleaks.PinnedVersion, missing: map[string]bool{}, leaks: leaks}
}

func (f *fakeRepo) run(_ context.Context, _ string, binary string, args ...string) (string, string, error) {
	if f.missing[binary] {
		return "", "", fmt.Errorf("exec: %q: %w", binary, exec.ErrNotFound)
	}
	if binary == "git" {
		return f.git(args)
	}
	if len(args) == 1 && args[0] == "version" {
		return f.version + "\n", "", nil
	}
	f.scanCalls = append(f.scanCalls, args)
	report := argAfter(args, "--report-path")
	body := f.report
	if body == "" {
		body = f.reportFor(args)
	}
	if report != "" {
		if err := os.WriteFile(report, []byte(body), 0o600); err != nil {
			return "", "", err
		}
	}
	return "", f.stderr, nil
}

func (f *fakeRepo) git(args []string) (string, string, error) {
	switch {
	case len(args) == 2 && args[0] == "rev-parse":
		return fmt.Sprintf("%v\n", f.shallow), "", nil
	case len(args) == 3 && args[0] == "cat-file":
		if f.missing[strings.TrimSuffix(args[2], "^{commit}")] {
			return "", "fatal: not a valid object", errors.New("exit status 1")
		}
		return "", "", nil
	}
	return "", "", fmt.Errorf("unexpected git %v", args)
}

func (f *fakeRepo) reportFor(args []string) string {
	history := len(args) > 0 && args[0] == "git" && argWithPrefix(args, "--log-opts=") == "--log-opts="+baseSHA+".."+headSHA
	ignoreAllow := hasArg(args, "--ignore-gitleaks-allow")
	var out []map[string]any
	for _, l := range f.leaks {
		if (!history && !l.inTree) || (l.allow && !ignoreAllow) {
			continue
		}
		out = append(out, map[string]any{
			"RuleID": "github-pat", "Description": "Uncovered a GitHub Personal Access Token",
			"File": l.file, "StartLine": l.line, "Commit": middleCommit,
			"Secret": fakeToken, "Match": "token = " + fakeToken, "Line": "token = " + fakeToken,
			"Message": "add config " + fakeToken, "Author": "dev", "Email": "dev@example.com",
		})
	}
	if out == nil {
		return "[]"
	}
	raw, _ := json.Marshal(out)
	return string(raw)
}

func argAfter(args []string, flag string) string {
	for i, a := range args {
		if a == flag && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func argWithPrefix(args []string, prefix string) string {
	for _, a := range args {
		if strings.HasPrefix(a, prefix) {
			return a
		}
	}
	return ""
}

func hasArg(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}

func engine(t *testing.T) scanner.Engine {
	t.Helper()
	e, ok := scanner.Lookup(gitleaks.Name)
	if !ok {
		t.Fatal("gitleaks is not registered")
	}
	return e
}

func scan(t *testing.T, f *fakeRepo, trust scanner.Trust, r scanner.Range) scanner.Outcome {
	t.Helper()
	x := scanner.Executor{Command: f.run}
	return x.Scan(context.Background(), engine(t), scanner.Request{Root: t.TempDir(), Trust: trust, Range: r})
}

var prRange = scanner.Range{Base: baseSHA, Head: headSHA}

// gateOf runs one gitleaks outcome through the scanner gate as the review
// does: issues with the engine's typed origin, the default fail_on.
func gateOf(t *testing.T, out scanner.Outcome, section string) gate.Result {
	t.Helper()
	var issues []types.ReviewIssue
	for _, f := range out.Findings {
		issues = append(issues, f.ToIssue(out.Engine.TypedOrigin()))
	}
	var d gate.Result
	s := gate.Scan{Config: config.ScannerConfig{Engine: gitleaks.Name}, Engine: out.Engine, Section: section, Issues: issues, Reason: out.Reason}
	if err := gate.ApplyScannerGate(&d, s, issues); err != nil {
		t.Fatal(err)
	}
	return d
}

// A token added in an intermediate commit and removed before the head is
// in the range's history only: the engine scans the range, the gate line
// and the audit record name origin gitleaks, file:line and rule, and no
// output carries the secret.
func TestGitleaksSecretInPullRequestHistoryReachesGateWithoutValue(t *testing.T) {
	f := newFakeRepo(fakeLeak{file: "config/app.env", line: 3})
	out := scan(t, f, scanner.TrustRepository, prRange)
	if out.Reason != "" {
		t.Fatalf("reason %q", out.Reason)
	}
	if len(f.scanCalls) != 1 {
		t.Fatalf("scan calls %d", len(f.scanCalls))
	}
	args := f.scanCalls[0]
	for _, want := range []string{"git", "--log-opts=" + baseSHA + ".." + headSHA, "json", "--exit-code", "--no-banner", "--redact", "--config"} {
		if !hasArg(args, want) {
			t.Errorf("args %v lack %q", args, want)
		}
	}
	if len(out.Findings) != 1 {
		t.Fatalf("history leak not reported: %+v", out.Findings)
	}
	got := out.Findings[0]
	if got.Path != "config/app.env" || got.Line != 3 || got.RuleID != "gitleaks:github-pat" || got.Severity != "error" {
		t.Fatalf("finding %+v", got)
	}
	if !strings.Contains(got.Message, middleCommit[:12]) {
		t.Errorf("message %q does not name the commit", got.Message)
	}
	d := gateOf(t, out, "policy")
	if !d.Fail || len(d.Lines) != 1 || !strings.Contains(d.Lines[0], "origem gitleaks, secao policy") || !strings.HasPrefix(d.Lines[0], "gitleaks:github-pat") {
		t.Fatalf("gate %+v", d)
	}
	if len(d.BlockingFindings) != 1 || d.BlockingFindings[0].Origin != "gitleaks" || d.BlockingFindings[0].Path != "config/app.env" || d.BlockingFindings[0].Line != 3 {
		t.Fatalf("audit findings %+v", d.BlockingFindings)
	}
	everything, _ := json.Marshal([]any{out, d, got.ToIssue("gitleaks")})
	for _, leaked := range []string{fakeToken, fakeToken[4:], "dev@example.com", "add config"} {
		if strings.Contains(string(everything), leaked) {
			t.Fatalf("output carries %q: %s", leaked, everything)
		}
	}
}

// Under a central policy an inline gitleaks:allow does not suppress the
// finding; in the repository's own scan it does.
func TestGitleaksInlineAllowOnlyHonoredWithoutPolicy(t *testing.T) {
	leak := fakeLeak{file: "app.go", line: 7, inTree: true, allow: true}
	if out := scan(t, newFakeRepo(leak), scanner.TrustPolicy, prRange); out.Reason != "" || len(out.Findings) != 1 {
		t.Fatalf("policy: %+v", out)
	}
	if out := scan(t, newFakeRepo(leak), scanner.TrustRepository, prRange); out.Reason != "" || len(out.Findings) != 0 {
		t.Fatalf("repository: %+v", out)
	}
}

// gitleaks reads the root's .gitleaksignore whatever its flags say: under
// policy the file's presence is itself a blocking finding.
func TestGitleaksIgnoreFileIsAFindingUnderPolicy(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".gitleaksignore"), []byte("x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for trust, want := range map[scanner.Trust]int{scanner.TrustPolicy: 1, scanner.TrustRepository: 0} {
		f := newFakeRepo()
		x := scanner.Executor{Command: f.run}
		out := x.Scan(context.Background(), engine(t), scanner.Request{Root: root, Trust: trust, Range: prRange})
		if out.Reason != "" || len(out.Findings) != want {
			t.Fatalf("trust %d: %+v", trust, out)
		}
		if want == 1 && out.Findings[0].RuleID != gitleaks.RuleIgnoreFilePresent {
			t.Fatalf("finding %+v", out.Findings[0])
		}
		if trust == scanner.TrustPolicy && !hasArg(f.scanCalls[0], "--gitleaks-ignore-path") {
			t.Fatalf("policy args %v", f.scanCalls[0])
		}
	}
}

// Every failure is the single inconclusive rule, never zero findings.
func TestGitleaksFailuresAreInconclusive(t *testing.T) {
	cases := map[string]struct {
		mutate func(*fakeRepo)
		r      scanner.Range
		want   string
	}{
		"missing binary":     {func(f *fakeRepo) { f.missing["gitleaks"] = true }, prRange, "secrets_unavailable"},
		"no range":           {func(*fakeRepo) {}, scanner.Range{}, "secrets_execution_error"},
		"range is a flag":    {func(*fakeRepo) {}, scanner.Range{Base: "--all", Head: headSHA}, "secrets_execution_error"},
		"base not fetched":   {func(f *fakeRepo) { f.missing[baseSHA] = true }, prRange, "secrets_execution_error"},
		"shallow clone":      {func(f *fakeRepo) { f.shallow = true }, prRange, "secrets_execution_error"},
		"other version":      {func(f *fakeRepo) { f.version = "v8.18.0" }, prRange, "secrets_execution_error"},
		"logged error exit0": {func(f *fakeRepo) { f.stderr = "7:23PM ERR [git] fatal: bad revision" }, prRange, "secrets_execution_error"},
		"report not array":   {func(f *fakeRepo) { f.report = `{"results":[]}` }, prRange, "secrets_invalid_output"},
		"report null":        {func(f *fakeRepo) { f.report = `null` }, prRange, "secrets_invalid_output"},
		"entry without file": {func(f *fakeRepo) { f.report = `[{"RuleID":"x","StartLine":1}]` }, prRange, "secrets_invalid_output"},
	}
	for name, c := range cases {
		f := newFakeRepo(fakeLeak{file: "a", line: 1})
		c.mutate(f)
		out := scan(t, f, scanner.TrustPolicy, c.r)
		if out.Reason != c.want || out.Findings != nil {
			t.Errorf("%s: reason %q findings %v, want %q", name, out.Reason, out.Findings, c.want)
		}
		if strings.Contains(c.want, "execution") && name != "logged error exit0" && len(f.scanCalls) != 0 {
			t.Errorf("%s: gitleaks scanned a range it could not verify", name)
		}
		if g := gateOf(t, out, "policy"); !g.Inconclusive {
			t.Errorf("%s: gate not inconclusive: %+v", name, g)
		}
	}
}

// The report's version is the engine identity: binary version and the rule
// base digest, both equal to the lock.
func TestGitleaksIdentityMatchesScannersLock(t *testing.T) {
	out := scan(t, newFakeRepo(), scanner.TrustRepository, prRange)
	if out.Version != gitleaks.Identity(gitleaks.PinnedVersion) || !strings.Contains(out.Version, gitleaks.RulebaseSHA256) {
		t.Fatalf("version %q", out.Version)
	}
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", ".board", "bootstrap", "locks", "scanners.yml"))
	if err != nil {
		t.Fatalf("scanners lock: %v", err)
	}
	lock := string(raw)
	for key, want := range map[string]string{
		"secrets_scanner_name":    gitleaks.Name,
		"secrets_scanner_version": gitleaks.PinnedVersion,
		"secrets_rulebase_id":     gitleaks.RulebaseID,
		"secrets_rulebase_sha256": gitleaks.RulebaseSHA256,
	} {
		if !strings.Contains(lock, "\n"+key+": "+want+"\n") {
			t.Errorf("lock %s is not %s", key, want)
		}
	}
}

func TestGitleaksRegistrationAndOptions(t *testing.T) {
	e := engine(t)
	if e.Category != "secrets" || e.TypedOrigin() != "gitleaks" {
		t.Fatalf("engine %+v", e)
	}
	if err := e.ValidateOptions(scanner.Options{"config": "x"}); err == nil {
		t.Fatal("an option was accepted")
	}
	if err := e.ValidateOptions(nil); err != nil {
		t.Fatal(err)
	}
}
