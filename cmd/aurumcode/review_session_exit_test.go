package main

import (
	"context"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/review/cache"
	"github.com/Mpaape/AurumCode/internal/review/session"
	"github.com/Mpaape/AurumCode/internal/scanner"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// sessionCase is one row of AC-002's behavioral table: the same diff,
// configuration, model outcome, findings and scanner fed to a source.
type sessionCase struct {
	name      string
	cfg       string
	model     modelOutcome
	issues    []types.ReviewIssue
	security  []types.ReviewIssue
	failOn    string
	artifacts bool
}

// missingSemgrep is the injected scanner runner of a host without Semgrep.
func missingSemgrep(context.Context, string, string, ...string) (string, string, error) {
	return "", "", exec.ErrNotFound
}

// runSessionCase runs the session's real evidence, gate and exit steps for
// source over c and returns the exit code and the published gate lines.
func runSessionCase(t *testing.T, source session.Source, c sessionCase) (int, []string) {
	t.Helper()
	cfg, err := config.Parse([]byte(c.cfg), "case.yml")
	if err != nil {
		t.Fatalf("%s: config: %v", c.name, err)
	}
	var stdout, stderr strings.Builder
	fixed := func() time.Time { return time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC) }
	s := newReviewState(source, reviewIO{stdout: &stdout, stderr: &stderr, filter: redaction.NewFilter(),
		deps: reviewDeps{clock: fixed, scanners: scanner.Executor{Command: missingSemgrep}, env: &reviewEnv{}}})
	s.cfg, s.model = cfg, c.model
	s.diff = &types.Diff{Files: []types.DiffFile{{Path: "app.go"}}}
	s.result = &types.ReviewResult{Issues: append([]types.ReviewIssue(nil), c.issues...)}
	s.securityFindings = []types.ReviewIssue{{File: "app.go", Line: 9, Severity: "info", Message: "note", RuleID: "security/info-note"}}
	s.securityFindings = append(s.securityFindings, c.security...)
	s.repoIdentity, s.repoIdentityKnown = "owner/repo", true
	if c.failOn != "" {
		if s.threshold, s.thresholdName, err = parseFailOnLevel(c.failOn); err != nil {
			t.Fatal(err)
		}
	}
	s.joinSecurityFindings()
	s.snapshotAndApplyRules()
	s.runScanners(t.TempDir(), "")
	defer s.flush()
	if code, done := s.runGate(); done {
		return code, nil
	}
	code := s.decideExit(publishOutcome{artifactsMissing: c.artifacts})
	var lines []string
	for _, line := range strings.Split(stderr.String(), "\n") {
		if strings.Contains(line, "policy gate:") {
			lines = append(lines, line)
		}
	}
	return code, lines
}

const (
	blockGate = "gate:\n  fail_on: [high]\n  inconclusive: block\n"
	warnGate  = "gate:\n  fail_on: [high]\n  inconclusive: warn\n"
	sastGate  = "gate:\n  fail_on: [high]\n  inconclusive: block\nquality_gates:\n  sast:\n    enabled: true\n"
)

var (
	// securityBreach is a security-pass finding at the gate's fail_on: it
	// counts in every inconclusive mode (AUR-569).
	securityBreach = types.ReviewIssue{File: "app.go", Line: 3, Severity: "error", Message: "hardcoded secret", RuleID: "security/hardcoded-secret"}
	warningFinding = types.ReviewIssue{File: "app.go", Line: 4, Severity: "warning", Message: "smell", RuleID: "quality/smell"}
)

// TestAUR576SameInputsSameExitThroughTheSession is AC-002 through the real
// session steps (evidence, gate pipeline, exit policy): the same inputs give
// the same exit code and the same policy-gate lines on --base and --pr.
// The declared per-source divergences are explicit rows.
func TestAUR576SameInputsSameExitThroughTheSession(t *testing.T) {
	t.Setenv(cache.EnvDir, "")
	shared := []struct {
		c    sessionCase
		want int
	}{
		// Rows 1-3 and 5 must publish policy-gate lines (checked below).
		{sessionCase{name: "provider failure, block", cfg: blockGate, model: modelProviderFailed}, exitQualityNotReviewed},
		{sessionCase{name: "parse failure, block", cfg: blockGate, model: modelParseFailed}, exitQualityNotReviewed},
		{sessionCase{name: "scanner missing", cfg: sastGate}, exitQualityNotReviewed},
		{sessionCase{name: "artifact not written over fail-on", cfg: warnGate, issues: []types.ReviewIssue{warningFinding}, failOn: "low", artifacts: true}, exitArtifactNotWritten},
		{sessionCase{name: "finding above the threshold", cfg: blockGate, security: []types.ReviewIssue{securityBreach}}, exitFindings},
		{sessionCase{name: "finding above --fail-on", cfg: warnGate, issues: []types.ReviewIssue{warningFinding}, failOn: "low"}, exitFindings},
		{sessionCase{name: "clean", cfg: blockGate}, 0},
	}
	for _, row := range shared {
		baseCode, baseLines := runSessionCase(t, session.LocalDiff, row.c)
		prCode, prLines := runSessionCase(t, session.PullRequest, row.c)
		if baseCode != row.want || prCode != row.want {
			t.Errorf("%s: exit --base=%d --pr=%d, want %d on both", row.c.name, baseCode, prCode, row.want)
		}
		if strings.Contains("provider failure, block|parse failure, block|scanner missing|finding above the threshold", row.c.name) && len(baseLines) == 0 {
			t.Errorf("%s: the gate published no line; the comparison would be vacuous", row.c.name)
		}
		if !reflect.DeepEqual(baseLines, prLines) {
			t.Errorf("%s: gate lines differ:\n --base=%q\n --pr  =%q", row.c.name, baseLines, prLines)
		}
	}
	// Declared divergence (session.Source data): a model failure always
	// closes a local run; on a pull request the gate decides.
	divergent := []struct {
		c          sessionCase
		base, prEx int
	}{
		{sessionCase{name: "provider failure, warn", cfg: warnGate, model: modelProviderFailed}, exitQualityNotReviewed, 0},
		{sessionCase{name: "provider failure, breach", cfg: warnGate, model: modelProviderFailed, security: []types.ReviewIssue{securityBreach}}, exitQualityNotReviewed, exitFindings},
	}
	for _, row := range divergent {
		baseCode, _ := runSessionCase(t, session.LocalDiff, row.c)
		prCode, _ := runSessionCase(t, session.PullRequest, row.c)
		if baseCode != row.base || prCode != row.prEx {
			t.Errorf("%s: exit --base=%d --pr=%d, want declared %d/%d", row.c.name, baseCode, prCode, row.base, row.prEx)
		}
	}
}
