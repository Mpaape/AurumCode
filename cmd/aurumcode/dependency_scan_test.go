package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/dependencies"
	"github.com/Mpaape/AurumCode/internal/llm"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
	"gopkg.in/yaml.v3"
)

// scanOSV is an OSV fake: minimist below 1.2.6 carries one advisory whose
// severity the test may re-grade; down makes it unreachable.
type scanOSV struct {
	mu       sync.Mutex
	severity string
	down     bool
}

func (s *scanOSV) serve(w http.ResponseWriter, r *http.Request) {
	var q struct {
		Version string `json:"version"`
		Package struct {
			Name string `json:"name"`
		} `json:"package"`
	}
	_ = json.NewDecoder(r.Body).Decode(&q)
	s.mu.Lock()
	down, sev := s.down, s.severity
	s.mu.Unlock()
	if down {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}
	if q.Package.Name != "minimist" || q.Version == "1.2.6" {
		_, _ = w.Write([]byte(`{}`))
		return
	}
	_, _ = w.Write([]byte(`{"vulns":[{"id":"GHSA-xvch-5gv4-984h","aliases":["CVE-2021-44906"],"summary":"Prototype pollution in minimist","database_specific":{"severity":"` + sev + `"},"affected":[{"package":{"name":"minimist","ecosystem":"npm"},"ranges":[{"events":[{"introduced":"0"},{"fixed":"1.2.6"}]}]}]}]}`))
}

// scanModel names no manifest of its own: the lockfile is the scanner's.
type scanModel struct{}

func (scanModel) CompleteMessages(context.Context, []llm.Message, llm.Options) (llm.Response, error) {
	return llm.Response{Text: `{"manifests":[]}`}, nil
}

// scanExtractor is osv-scanner's extraction of the lockfile at a version.
type scanExtractor struct{ version string }

func (x *scanExtractor) Extract(context.Context, string, []string) (map[string][]dependencies.Package, error) {
	return map[string][]dependencies.Package{"app/package-lock.json": {{Ecosystem: "npm", Name: "minimist", Version: x.version}}}, nil
}

type scanRun struct {
	t       *testing.T
	root    string
	osv     *scanOSV
	scanner *scanExtractor
	server  *httptest.Server
}

func newScanRun(t *testing.T) *scanRun {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".aurumcode"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".aurumcode", "config.yml"), []byte("dependencies:\n  fail_on: [high]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := &scanRun{t: t, root: root, osv: &scanOSV{severity: "CRITICAL"}, scanner: &scanExtractor{version: "1.2.5"}}
	r.server = httptest.NewServer(http.HandlerFunc(r.osv.serve))
	t.Cleanup(r.server.Close)
	return r
}

// run executes `aurumcode dependencies` and returns the exit code, the
// SARIF document (nil when none was written) and stderr.
func (r *scanRun) run() (int, map[string]any, string) {
	out := filepath.Join(r.t.TempDir(), "dependencies.sarif")
	deps := dependencyScanDeps{
		model:     func() (dependencies.Completer, error) { return scanModel{}, nil },
		listFiles: func(string) ([]string, error) { return []string{"app/package-lock.json", "app/index.js"}, nil },
		sources: dependencySources{source: func(*config.DependenciesConfig, func() time.Time) dependencies.Source {
			return dependencies.OSV{BaseURL: r.server.URL, Client: r.server.Client()}
		}},
		scanner: func(*config.DependenciesConfig) dependencies.Extractor { return r.scanner },
	}
	var stdout, stderr bytes.Buffer
	code := runDependencyScanWith([]string{"--repo", r.root, "--sarif", out}, &stdout, &stderr, redaction.NewFilter(), deps)
	data, err := os.ReadFile(out)
	if err != nil {
		return code, nil, stderr.String()
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		r.t.Fatalf("SARIF is not JSON: %v", err)
	}
	return code, doc, stderr.String()
}

func sarifRun0(t *testing.T, doc map[string]any) map[string]any {
	t.Helper()
	runs, _ := doc["runs"].([]any)
	if len(runs) != 1 {
		t.Fatalf("runs = %v", doc["runs"])
	}
	return runs[0].(map[string]any)
}

func sarifResults(t *testing.T, doc map[string]any) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, r := range sarifRun0(t, doc)["results"].([]any) {
		out = append(out, r.(map[string]any))
	}
	return out
}

func fingerprintOf(t *testing.T, result map[string]any) string {
	t.Helper()
	fp, _ := result["partialFingerprints"].(map[string]any)
	v, _ := fp["aurumcode/findingId/v1"].(string)
	if v == "" {
		t.Fatalf("result without fingerprint: %v", result)
	}
	return v
}

// AUR-530 AC-001: the scheduled run over the default branch writes a SARIF
// with the new vulnerability, in a category distinct from the review's.
func TestAUR530AC001ScheduledSARIF(t *testing.T) {
	code, doc, stderr := newScanRun(t).run()
	if code != 0 || doc == nil {
		t.Fatalf("exit=%d stderr=%s", code, stderr)
	}
	details, _ := sarifRun0(t, doc)["automationDetails"].(map[string]any)
	if details["id"] != dependencyScanCategory+"/" {
		t.Fatalf("category = %v", details)
	}
	results := sarifResults(t, doc)
	if len(results) != 1 || results[0]["ruleId"] != "cve/GHSA-xvch-5gv4-984h" || !strings.Contains(results[0]["message"].(map[string]any)["text"].(string), "CVE-2021-44906") {
		t.Fatalf("results = %v", results)
	}
}

// AUR-530 AC-002 / MUT-001: the identity of the alert is stable across runs
// (another vulnerable version, a re-graded severity), and once the
// dependency is fixed the next run emits no result for it.
func TestAUR530AC002StableFingerprintAndClosure(t *testing.T) {
	r := newScanRun(t)
	_, first, _ := r.run()
	r.scanner.version = "1.2.0"
	r.osv.mu.Lock()
	r.osv.severity = "HIGH"
	r.osv.mu.Unlock()
	_, second, _ := r.run()
	a, b := sarifResults(t, first), sarifResults(t, second)
	if len(a) != 1 || len(b) != 1 || fingerprintOf(t, a[0]) != fingerprintOf(t, b[0]) {
		t.Fatalf("fingerprint unstable across runs: %v / %v", a, b)
	}
	r.scanner.version = "1.2.6"
	code, fixed, stderr := r.run()
	if code != 0 || fixed == nil || len(sarifResults(t, fixed)) != 0 {
		t.Fatalf("fixed run must upload a conclusive document without the result: exit=%d stderr=%s doc=%v", code, stderr, fixed)
	}
}

// AUR-530 AC-003: an unreachable base marks the run inconclusive and writes
// no SARIF, so the open alerts are not closed.
func TestAUR530AC003UnreachableKeepsAlerts(t *testing.T) {
	r := newScanRun(t)
	r.osv.down = true
	code, doc, stderr := r.run()
	if code != 1 || doc != nil || !strings.Contains(stderr, dependencies.ReasonSourceFailed) {
		t.Fatalf("unreachable base: exit=%d doc=%v stderr=%s", code, doc, stderr)
	}
}

// AUR-530 AC-003: the workflow runs the scan only on the caller's schedule
// and uploads the SARIF only after a conclusive scan, never on always().
func TestAUR530AC003WorkflowUploadsOnlyConclusive(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "review.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var wf struct {
		Jobs map[string]struct {
			If    string `yaml:"if"`
			Steps []struct {
				Name string `yaml:"name"`
				If   string `yaml:"if"`
				Run  string `yaml:"run"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(raw, &wf); err != nil {
		t.Fatal(err)
	}
	scan, ok := wf.Jobs["dependencies"]
	if !ok || !strings.Contains(scan.If, "'schedule'") || !strings.Contains(wf.Jobs["review"].If, "!= 'schedule'") {
		t.Fatalf("schedule routing: dependencies.if=%q review.if=%q", scan.If, wf.Jobs["review"].If)
	}
	var ran, uploaded bool
	for _, s := range scan.Steps {
		if strings.Contains(s.Run, "dependencies --repo") && strings.Contains(s.Run, "--sarif") {
			ran = true
		}
		if s.Name == "Upload dependency SARIF" {
			uploaded = true
			if s.If != "success()" {
				t.Fatalf("upload runs on %q; an inconclusive scan must never upload", s.If)
			}
		}
	}
	if !ran || !uploaded {
		t.Fatalf("scan step=%v upload step=%v", ran, uploaded)
	}
}
