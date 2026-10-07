package gate

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/dependencies"
	"github.com/Mpaape/AurumCode/internal/llm"
	"github.com/Mpaape/AurumCode/pkg/types"
)

func failOn(levels ...string) *config.DependenciesConfig {
	return &config.DependenciesConfig{FailOn: levels}
}

func hasLine(res Result, want string) bool {
	for _, l := range res.Lines {
		if strings.Contains(l, want) {
			return true
		}
	}
	return false
}

// AUR-527 AC-001: an introduced advisory in fail_on fails; below it alerts.
func TestAUR527AC001IntroducedFollowsFailOn(t *testing.T) {
	res := applyDeps(t, depRun(failOn("critical", "high")), dependencies.Report{Findings: []dependencies.Finding{depFinding("GHSA-1", "critical", dependencies.StatusIntroduced)}})
	if !res.Fail || !res.Breach || len(res.BlockingFindings) != 1 || res.BlockingFindings[0].RuleID != "cve/GHSA-1" {
		t.Fatalf("critical introduced = %+v", res)
	}
	res = applyDeps(t, depRun(failOn("critical", "high")), dependencies.Report{Findings: []dependencies.Finding{depFinding("GHSA-2", "low", dependencies.StatusIntroduced)}})
	if res.Fail || !hasLine(res, "[alerta]") {
		t.Fatalf("low introduced = %+v", res)
	}
	res = applyDeps(t, depRun(failOn("high")), dependencies.Report{Findings: []dependencies.Finding{depFinding("GHSA-3", dependencies.SeverityUnknown, dependencies.StatusIntroduced)}})
	if !res.Fail {
		t.Fatalf("unknown severity must count at the top: %+v", res)
	}
}

// AUR-527 AC-002: a pre-existing advisory follows `preexisting`.
func TestAUR527AC002Preexisting(t *testing.T) {
	report := dependencies.Report{Findings: []dependencies.Finding{depFinding("GHSA-1", "critical", dependencies.StatusPreexisting)}}
	warn := failOn("high")
	if res := applyDeps(t, depRun(warn), report); res.Fail || !hasLine(res, "[alerta]") {
		t.Fatalf("preexisting warn = %+v", res)
	}
	block := failOn("high")
	block.Preexisting = config.PreexistingBlock
	if res := applyDeps(t, depRun(block), report); !res.Fail || !hasLine(res, "[reprova]") {
		t.Fatalf("preexisting block = %+v", res)
	}
}

// AUR-527 AC-003: a valid exception for the CVE and the repository passes
// and is listed as accepted; an expired one no longer applies.
func TestAUR527AC003CVEException(t *testing.T) {
	f := depFinding("GHSA-xvch-5gv4-984h", "critical", dependencies.StatusIntroduced)
	f.Vuln.Aliases = []string{"CVE-2021-44906"}
	report := dependencies.Report{Findings: []dependencies.Finding{f}}
	exc := config.ExceptionConfig{Repo: "acme/app", Rule: "cve/CVE-2021-44906", Path: "app/package-lock.json", Owner: "seguranca", Reason: "sem uso do parser", Expires: "2026-12-31"}

	run := depRun(failOn("high"))
	run.Cfg.Exceptions = []config.ExceptionConfig{exc}
	res := applyDeps(t, run, report)
	if res.Fail || len(res.AppliedExceptions) != 1 || !hasLine(res, AcceptedExceptionMarker) {
		t.Fatalf("valid exception = %+v", res)
	}

	exc.Expires = "2026-10-01"
	run = depRun(failOn("high"))
	run.Cfg.Exceptions = []config.ExceptionConfig{exc}
	res = applyDeps(t, run, report)
	if !res.Fail || len(res.AppliedExceptions) != 0 || !hasLine(res, ExpiredExceptionMarker) {
		t.Fatalf("expired exception = %+v", res)
	}

	exc.Expires, exc.Repo = "2026-12-31", "acme/other"
	run = depRun(failOn("high"))
	run.Cfg.Exceptions = []config.ExceptionConfig{exc}
	if res = applyDeps(t, run, report); !res.Fail {
		t.Fatalf("exception of another repository applied: %+v", res)
	}
}

// AUR-527 AC-005: without the section nothing is added; declared without
// fail_on the findings are informative only.
func TestAUR527AC005InformativeWithoutFailOn(t *testing.T) {
	res := &Result{}
	if err := NewPipeline(DependenciesContributor{}).Run(context.Background(), depRun(nil), res); err != nil || res.Active || len(res.Lines) != 0 {
		t.Fatalf("undeclared section = %+v %v", res, err)
	}
	got := applyDeps(t, depRun(&config.DependenciesConfig{}), dependencies.Report{Findings: []dependencies.Finding{depFinding("GHSA-1", "critical", dependencies.StatusIntroduced)}})
	if got.Fail || !hasLine(got, "[informativo]") {
		t.Fatalf("no fail_on = %+v", got)
	}
}

// liveOSV is an OSV fake whose one advisory's severity the test changes
// between runs.
type liveOSV struct {
	mu       sync.Mutex
	severity string
}

func (l *liveOSV) serve(w http.ResponseWriter, r *http.Request) {
	var q struct {
		Version string `json:"version"`
	}
	_ = json.NewDecoder(r.Body).Decode(&q)
	if q.Version != "1.2.5" {
		_, _ = w.Write([]byte(`{}`))
		return
	}
	l.mu.Lock()
	sev := l.severity
	l.mu.Unlock()
	_, _ = w.Write([]byte(`{"vulns":[{"id":"GHSA-xvch-5gv4-984h","aliases":["CVE-2021-44906"],"database_specific":{"severity":"` + sev + `"},"affected":[{"package":{"name":"minimist","ecosystem":"npm"},"ranges":[{"events":[{"introduced":"0"},{"fixed":"1.2.6"}]}]}]}]}`))
}

// policyModel names the lockfile and the bump.
type policyModel struct{}

func (policyModel) CompleteMessages(_ context.Context, msgs []llm.Message, _ llm.Options) (llm.Response, error) {
	if strings.Contains(msgs[0].Content, `{"manifests"`) {
		return llm.Response{Text: `{"manifests":["app/package-lock.json"]}`}, nil
	}
	return llm.Response{Text: `{"changes":[{"manifest":"app/package-lock.json","ecosystem":"npm","name":"minimist","base_version":"1.2.6","head_version":"1.2.5"}]}`}, nil
}

// AUR-527 AC-004 / MUT-001: the same advisory re-graded at the source
// changes the next run's result without touching the configuration.
func TestAUR527AC004SeverityFollowsSource(t *testing.T) {
	live := &liveOSV{severity: "CRITICAL"}
	server := httptest.NewServer(http.HandlerFunc(live.serve))
	defer server.Close()
	diff := &types.Diff{Files: []types.DiffFile{{Path: "app/package-lock.json", Hunks: []types.DiffHunk{{Lines: []string{`     "node_modules/minimist": {`, `-      "version": "1.2.6",`, `+      "version": "1.2.5",`}}}}}}
	check := func() dependencies.Report {
		return dependencies.Check(context.Background(), dependencies.Inputs{Diff: diff, Model: policyModel{}, Source: dependencies.OSV{BaseURL: server.URL, Client: server.Client()}})
	}
	cfg := failOn("high")
	if res := applyDeps(t, depRun(cfg), check()); !res.Fail {
		t.Fatalf("critical run = %+v", res)
	}
	live.mu.Lock()
	live.severity = "LOW"
	live.mu.Unlock()
	if res := applyDeps(t, depRun(cfg), check()); res.Fail || !hasLine(res, "severidade low") {
		t.Fatalf("re-graded run = %+v", res)
	}
}
