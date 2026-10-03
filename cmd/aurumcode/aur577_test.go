package main

// Scanners are registered engines: an engine registered only in this test
// reaches the gate line, the audit and the SARIF under its own origin with
// no change in internal/gate or cmd; a missing binary or an incomplete
// report is inconclusive; a central policy's required scanner survives a
// repository's disable; the audit and the SARIF come from the same gate
// findings.

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/render"
	"github.com/Mpaape/AurumCode/internal/review/session"
	"github.com/Mpaape/AurumCode/internal/scanner"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// fakeEngine is an engine compiled only into this test binary.
type fakeEngine struct {
	name     string
	report   scanner.Report
	err      error
	binary   string
	invoked  *int
	findings []scanner.Finding
}

func (f fakeEngine) Name() string { return f.name }
func (f fakeEngine) Run(ctx context.Context, req scanner.Request) (scanner.Report, error) {
	if f.invoked != nil {
		*f.invoked++
	}
	if f.binary != "" {
		if _, _, err := req.Command(ctx, req.Root, f.binary); err != nil {
			return scanner.Report{}, err
		}
	}
	return f.report, f.err
}

// registerFake registers e for the duration of the test.
func registerFake(t *testing.T, e scanner.Engine) {
	t.Helper()
	scanner.Register(e)
	t.Cleanup(func() { scanner.Unregister(e.Name()) })
}

// AC-001: a fake engine declared in quality_gates.scanners reaches the gate
// line, the audit and the SARIF with origin equal to its name.
func TestAUR577FakeEngineReachesEverySinkByOrigin(t *testing.T) {
	registerFake(t, scanner.Engine{Scanner: fakeEngine{name: "fakescan", report: scanner.Report{Complete: true, Findings: []scanner.Finding{
		{Path: "app.go", Line: 4, Side: "RIGHT", RuleID: "fakescan:leak", Severity: "error", Message: "fake leak"},
	}}}})
	aur579Repo(t)
	writeRepoConfig(t, "quality_gates:\n  scanners:\n    - engine: fakescan\n")
	dir := t.TempDir()
	audit, sarif := filepath.Join(dir, "audit.json"), filepath.Join(dir, "out.sarif")
	var out, errOut strings.Builder
	rio := reviewIO{stdout: &out, stderr: &errOut, filter: redaction.NewFilter(), deps: reviewDeps{env: &reviewEnv{}}}
	code := runReviewWith(rio, []string{"--base", "HEAD~1", "--auditoria", audit, "--sarif", sarif})
	if code != exitFindings {
		t.Fatalf("exit=%d, want the fake engine's finding to fail the gate; stdout=%s stderr=%s", code, out.String(), errOut.String())
	}
	if want := "policy gate: fakescan:leak - fake leak (rule fakescan:leak) (severidade error, limiar error, origem fakescan, secao repo)"; !strings.Contains(errOut.String(), want) {
		t.Errorf("gate line lacks %q:\n%s", want, errOut.String())
	}
	var rec struct {
		Blocking []struct {
			RuleID string `json:"rule_id"`
			Origin string `json:"origin"`
		} `json:"blocking_findings"`
	}
	readJSON(t, audit, &rec)
	if len(rec.Blocking) != 1 || rec.Blocking[0].RuleID != "fakescan:leak" || rec.Blocking[0].Origin != "fakescan" {
		t.Errorf("audit blocking_findings = %+v, want fakescan:leak with origin fakescan", rec.Blocking)
	}
	if got := sarifOrigins(t, sarif)["fakescan:leak"]; got != "fakescan" {
		t.Errorf("SARIF origin of fakescan:leak = %q, want fakescan", got)
	}
}

// AC-001: no switch, case or comparison on an engine name in internal/gate
// or cmd/aurumcode, and neither imports an engine package.
func TestAUR577NoEngineSwitchInGateOrCmd(t *testing.T) {
	engines := map[string]bool{}
	for _, name := range append(scanner.Names(), scanner.Categories()...) {
		engines[name] = true
	}
	if !engines["semgrep"] {
		t.Fatal("semgrep must be registered in the binary")
	}
	for _, dir := range []string{".", filepath.Join("..", "..", "internal", "gate")} {
		files, err := filepath.Glob(filepath.Join(dir, "*.go"))
		if err != nil || len(files) == 0 {
			t.Fatalf("no Go files in %s: %v", dir, err)
		}
		for _, path := range files {
			if strings.HasSuffix(path, "_test.go") {
				continue
			}
			checkNoEngineBranch(t, path, engines)
		}
	}
}

func checkNoEngineBranch(t *testing.T, path string, engines map[string]bool) {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, imp := range file.Imports {
		if p, _ := strconv.Unquote(imp.Path.Value); strings.HasPrefix(p, "github.com/Mpaape/AurumCode/internal/scanner/") {
			t.Errorf("%s imports the engine package %s", path, p)
		}
	}
	isEngine := func(e ast.Expr) bool {
		lit, ok := e.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return false
		}
		v, _ := strconv.Unquote(lit.Value)
		return engines[strings.ToLower(v)]
	}
	ast.Inspect(file, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.CaseClause:
			for _, e := range x.List {
				if isEngine(e) {
					t.Errorf("%s: a case on the engine name %s", path, e.(*ast.BasicLit).Value)
				}
			}
		case *ast.BinaryExpr:
			if (x.Op == token.EQL || x.Op == token.NEQ) && (isEngine(x.X) || isEngine(x.Y)) {
				t.Errorf("%s: a comparison with an engine name", path)
			}
		case *ast.CallExpr:
			for _, a := range x.Args {
				if isEngine(a) {
					t.Errorf("%s: an engine name passed as a literal", path)
				}
			}
		}
		return true
	})
}

// AC-002: an unknown engine is refused by Parse, naming the key; the
// legacy quality_gates.sast is the semgrep entry.
func TestAUR577UnknownEngineRefusedAndSastIsAlias(t *testing.T) {
	_, err := config.Parse([]byte("quality_gates:\n  scanners:\n    - engine: inexistente\n"), "repo.yml")
	if err == nil || !strings.Contains(err.Error(), `quality_gates.scanners[inexistente].engine: unknown engine "inexistente"`) {
		t.Fatalf("Parse error = %v, want the unknown engine named", err)
	}
	cfg, err := config.Parse([]byte("quality_gates:\n  sast:\n    enabled: true\n    rule_packs: [p/ci]\n"), "repo.yml")
	if err != nil {
		t.Fatal(err)
	}
	got := cfg.QualityGates.EnabledScanners()
	if len(got) != 1 || got[0].Name() != "semgrep" || got[0].Label() != "quality_gates.sast" {
		t.Fatalf("quality_gates.sast must be the semgrep entry, got %+v", got)
	}
	if _, err := config.Parse([]byte("quality_gates:\n  sast:\n    enabled: true\n  scanners:\n    - engine: semgrep\n"), "repo.yml"); err == nil || !strings.Contains(err.Error(), "already declared by quality_gates.sast") {
		t.Fatalf("semgrep declared twice must be refused, got %v", err)
	}
	if _, err := config.Parse([]byte("gate:\n  fail_on: [error]\n  sources: [sast, semgrep, skills]\n  triage:\n    semgrep: model\n"), "repo.yml"); err != nil {
		t.Fatalf("an engine name and its category are gate sources: %v", err)
	}
}

// AC-002: the command refuses an unknown engine before the model.
func TestAUR577UnknownEngineStopsTheReview(t *testing.T) {
	aur579Repo(t)
	writeRepoConfig(t, "quality_gates:\n  scanners:\n    - engine: inexistente\n")
	var out, errOut strings.Builder
	rio := reviewIO{stdout: &out, stderr: &errOut, filter: redaction.NewFilter(), deps: reviewDeps{env: &reviewEnv{}}}
	if code := runReviewWith(rio, []string{"--base", "HEAD~1"}); code != 1 || !strings.Contains(errOut.String(), `unknown engine "inexistente"`) {
		t.Fatalf("exit=%d stderr=%s, want exit 1 (every load error, as AUR-575 measured) naming the engine", code, errOut.String())
	}
	if out.Len() != 0 {
		t.Fatalf("no report may be printed:\n%s", out.String())
	}
}

// AC-003: a policy's required scanner runs although the repository turned
// it off, and the override becomes a named policy warning; a policy entry
// that is not required yields to the repository.
func TestAUR577PolicyRequiredScannerSurvivesRepositoryDisable(t *testing.T) {
	invoked := 0
	registerFake(t, scanner.Engine{Scanner: fakeEngine{name: "policyscan", invoked: &invoked, report: scanner.Report{Complete: true}}})
	for _, repoYAML := range []string{
		"quality_gates:\n  scanners:\n    - engine: policyscan\n      enabled: false\n",
		"quality_gates:\n  scanners:\n    - engine: policyscan\n      enabled: false\n      required: false\n      fail_on: info\n",
	} {
		invoked = 0
		central := mustParse(t, "quality_gates:\n  scanners:\n    - engine: policyscan\n      required: true\n")
		repo := mustParse(t, repoYAML)
		effective, warnings := config.ApplyCentralPolicy(repo, central)
		var named bool
		for _, w := range warnings {
			named = named || strings.Contains(w.Reason, "quality_gates.scanners[policyscan] do config do repositório foi ignorado")
		}
		if !named {
			t.Errorf("the repository's override must be a policy warning, got %+v", warnings)
		}
		s := scanSession(t, effective, central)
		if invoked != 1 || len(s.scans) != 1 || s.scans[0].Section != gateOriginPolicy {
			t.Fatalf("the required engine must run once under the policy section; invoked=%d scans=%+v", invoked, s.scans)
		}
	}
	central := mustParse(t, "quality_gates:\n  scanners:\n    - engine: policyscan\n")
	effective, warnings := config.ApplyCentralPolicy(mustParse(t, "quality_gates:\n  scanners:\n    - engine: policyscan\n      enabled: false\n"), central)
	if len(effective.QualityGates.EnabledScanners()) != 0 {
		t.Fatalf("a policy entry that is not required yields to the repository: %+v", effective.QualityGates.EnabledScanners())
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0].Reason, "quality_gates.scanners[policyscan] da política central não é obrigatória") {
		t.Fatalf("the yielded policy entry must be named: %+v", warnings)
	}
}

// AC-004: a registered engine whose binary is absent, and one whose report
// is incomplete, are inconclusive and block by default; never zero findings.
func TestAUR577MissingBinaryAndIncompleteAreInconclusive(t *testing.T) {
	registerFake(t, scanner.Engine{Scanner: fakeEngine{name: "ghostscan", binary: "ghostscan-absent-binary", report: scanner.Report{Complete: true}}})
	registerFake(t, scanner.Engine{Scanner: fakeEngine{name: "halfscan", report: scanner.Report{Complete: false, Findings: []scanner.Finding{{Path: "a.go", Line: 1, RuleID: "half:x", Severity: "info"}}}}})
	for engine, reason := range map[string]string{"ghostscan": "ghostscan_unavailable", "halfscan": "halfscan_incomplete"} {
		cfg := mustParse(t, "quality_gates:\n  scanners:\n    - engine: "+engine+"\n")
		s := scanSession(t, cfg, nil)
		if len(s.scans) != 1 || s.scans[0].Reason != reason || s.scans[0].Issues != nil {
			t.Fatalf("%s: scans=%+v, want reason %s and no findings", engine, s.scans, reason)
		}
		if got := s.inconclusiveReason(); got != reason {
			t.Errorf("%s: inconclusive reason = %q, want %q", engine, got, reason)
		}
		if code, done := s.runGate(); done || !s.gateRes.Inconclusive || !s.gateRes.Fail {
			t.Errorf("%s: gate (code %d) = %+v, want inconclusive and failed (block by default)", engine, code, s.gateRes)
		}
	}
}

// AC-005: the audit and the SARIF come from the same gate findings: the
// Dependency-Track breach appears in both with its origin.
func TestAUR577AuditAndSARIFShareTheGateFindings(t *testing.T) {
	dir := t.TempDir()
	audit, sarif := filepath.Join(dir, "audit.json"), filepath.Join(dir, "out.sarif")
	res := gateDecision{Active: true, Fail: true, Breach: true}
	res.BlockingFindings = append(res.BlockingFindings,
		auditFinding("ssor_dtrack", "project-1", 0, "dtrack"),
		auditFinding("semgrep:demo", "app.go", 4, "sast"))
	issues := []types.ReviewIssue{{File: "app.go", Line: 4, RuleID: "semgrep:demo", Severity: "error", Message: "demo (rule semgrep:demo)"}}
	in := complianceArtifactInputs{auditoriaPath: audit, sarifPath: sarif, issues: issues, diff: &types.Diff{}}
	var errOut strings.Builder
	if failures := writeComplianceArtifacts(in, &gateRun{}, &res, redaction.NewFilter(), &errOut); len(failures) != 0 {
		t.Fatalf("artifacts not written: %+v", failures)
	}
	var rec struct {
		Blocking []struct {
			RuleID string `json:"rule_id"`
			Origin string `json:"origin"`
		} `json:"blocking_findings"`
	}
	readJSON(t, audit, &rec)
	inAudit := map[string]string{}
	for _, b := range rec.Blocking {
		inAudit[b.RuleID] = b.Origin
	}
	inSARIF := sarifOrigins(t, sarif)
	for rule, origin := range map[string]string{"ssor_dtrack": "dtrack", "semgrep:demo": "sast"} {
		if inAudit[rule] != origin || inSARIF[rule] != origin {
			t.Errorf("%s: audit origin %q, SARIF origin %q, want %q in both", rule, inAudit[rule], inSARIF[rule], origin)
		}
	}
}

// writeRepoConfig writes the fixture repository's .aurumcode/config.yml.
func writeRepoConfig(t *testing.T, body string) {
	t.Helper()
	if err := os.MkdirAll(".aurumcode", 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(".aurumcode", "config.yml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func mustParse(t *testing.T, body string) *config.Config {
	t.Helper()
	cfg, err := config.Parse([]byte(body), "case.yml")
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

// scanSession runs the session's real scanner pass with cfg as the
// effective configuration (central, when not nil, as the policy).
func scanSession(t *testing.T, cfg, central *config.Config) *reviewState {
	t.Helper()
	var stdout, stderr strings.Builder
	s := newReviewState(session.LocalDiff, reviewIO{stdout: &stdout, stderr: &stderr, filter: redaction.NewFilter(), deps: reviewDeps{env: &reviewEnv{}}})
	s.cfg, s.centralCfg = cfg, central
	s.diff = &types.Diff{}
	s.result = &types.ReviewResult{}
	s.runScanners(t.TempDir(), scanner.Range{}, "")
	t.Cleanup(s.flush)
	return &s
}

func auditFinding(rule, path string, line int, origin string) render.AuditFinding {
	return render.AuditFinding{RuleID: rule, Path: path, Line: line, Severity: "error", Origin: origin}
}

// sarifOrigins maps each SARIF result's rule to its origin property.
func sarifOrigins(t *testing.T, path string) map[string]string {
	t.Helper()
	var log struct {
		Runs []struct {
			Results []struct {
				RuleID     string `json:"ruleId"`
				Properties struct {
					Origin string `json:"origin"`
				} `json:"properties"`
			} `json:"results"`
		} `json:"runs"`
	}
	readJSON(t, path, &log)
	out := map[string]string{}
	for _, run := range log.Runs {
		for _, r := range run.Results {
			out[r.RuleID] = r.Properties.Origin
		}
	}
	return out
}
