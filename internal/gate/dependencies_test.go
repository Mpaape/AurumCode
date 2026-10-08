package gate

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/dependencies"
	"github.com/Mpaape/AurumCode/pkg/types"
)

func depRun(cfg *config.DependenciesConfig) *Run {
	return &Run{
		Cfg:          &config.Config{Dependencies: cfg},
		Review:       &types.ReviewResult{},
		Stderr:       io.Discard,
		Stdout:       io.Discard,
		RepoIdentity: "acme/app",
		Language:     "pt-BR",
		Now:          func() time.Time { return time.Date(2026, 10, 7, 0, 0, 0, 0, time.UTC) },
	}
}

func applyDeps(t *testing.T, run *Run, report dependencies.Report) Result {
	t.Helper()
	res := &Result{}
	if err := NewPipeline(DependenciesContributor{Report: &report}).Run(context.Background(), run, res); err != nil {
		t.Fatal(err)
	}
	return *res
}

func depFinding(id, severity string, status dependencies.Status) dependencies.Finding {
	return dependencies.Finding{
		Change: dependencies.Change{Manifest: "app/package-lock.json", Ecosystem: "npm", Name: "minimist", Base: "1.2.6", Head: "1.2.5"},
		Vuln:   dependencies.Vulnerability{ID: id, Severity: severity, Fixed: []string{"1.2.6"}, Link: "https://example.invalid/" + id},
		Status: status,
	}
}

// AUR-495 AC-006: an inconclusive check follows gate.inconclusive and
// states its reason; with the section declared the default is block.
func TestAUR495AC006InconclusiveFollowsPolicy(t *testing.T) {
	res := applyDeps(t, depRun(&config.DependenciesConfig{}), dependencies.Report{Reason: dependencies.ReasonSourceFailed, Detail: "HTTP 503"})
	if !res.Inconclusive || !res.Fail || !strings.Contains(res.Reason, dependencies.ReasonSourceFailed) {
		t.Fatalf("inconclusive check = %+v", res)
	}
	warn := depRun(&config.DependenciesConfig{})
	warn.Cfg.Gate.Inconclusive = config.InconclusiveWarn
	if res := applyDeps(t, warn, dependencies.Report{Reason: dependencies.ReasonSourceFailed}); res.Fail || !res.Inconclusive {
		t.Fatalf("inconclusive under warn = %+v", res)
	}
}

// AUR-495 AC-001: a finding line carries the identifiers, package, version,
// severity, fixed version and link, and where it stands.
func TestAUR495AC001FindingLine(t *testing.T) {
	f := depFinding("GHSA-xvch-5gv4-984h", "critical", dependencies.StatusIntroduced)
	f.Vuln.Aliases = []string{"CVE-2021-44906"}
	line := DependencyFindingLine("pt-BR", f, depDecisionInfo)
	for _, want := range []string{"introduzida pelo PR", "GHSA-xvch-5gv4-984h/CVE-2021-44906", "minimist 1.2.5", "severidade critical", "corrigida em 1.2.6", "https://example.invalid/"} {
		if !strings.Contains(line, want) {
			t.Errorf("line %q lacks %q", line, want)
		}
	}
}

// The gate lines follow the review language.
func TestDependencyLinesFollowLanguage(t *testing.T) {
	f := depFinding("GHSA-1", "critical", dependencies.StatusIntroduced)
	if line := DependencyFindingLine("en", f, depDecisionBlock); !strings.Contains(line, "introduced by the PR") || !strings.Contains(line, "[fails]") {
		t.Fatalf("en line = %q", line)
	}
	if line := DependencyFindingLine("pt-BR", f, depDecisionBlock); !strings.Contains(line, "DEPENDÊNCIAS introduzida pelo PR") || !strings.Contains(line, "[reprova]") {
		t.Fatalf("pt-BR line = %q", line)
	}
}
