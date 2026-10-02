package analysis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// RuleSemgrepPrefix prefixes every Semgrep-sourced Finding's RuleID with
// its own check_id (RuleSemgrepPrefix+check_id), so a reviewer and the
// policy gate can always tell a Semgrep finding apart from the embedded
// catalog, go vet, or a model-cited skill-section rule.
const RuleSemgrepPrefix = "semgrep:"

// semgrepReport is the subset of `semgrep scan --json`'s own report this
// adapter reads. A JSON document that decodes but carries no "results" key
// at all (Results stays nil, as opposed to an explicit empty array) is
// treated as a failed scan by Semgrep below -- see ErrSemgrepNoResults.
type semgrepReport struct {
	Results *[]semgrepResult `json:"results"`
}

type semgrepResult struct {
	CheckID string `json:"check_id"`
	Path    string `json:"path"`
	Start   struct {
		Line int `json:"line"`
	} `json:"start"`
	Extra struct {
		Severity string `json:"severity"`
		Message  string `json:"message"`
	} `json:"extra"`
}

// ErrSemgrepNoResults is returned when Semgrep's own JSON decodes but
// carries no "results" key at all -- distinct from a genuinely empty scan
// (an explicit `"results": []`), which is a real, trustworthy "nothing
// found". A missing key is what a truncated, wrong-shaped or
// not-actually-Semgrep JSON document looks like, and must never be read as
// a clean pass (AUR-548/MUT-001: treating a failed execution as zero
// findings).
var ErrSemgrepNoResults = errors.New("semgrep: JSON output has no \"results\" key")

// Semgrep runs `semgrep scan --json --config <pack>...` in dir through the
// injected commandRunner (the same seam Vet already uses) and parses the
// report into Findings. Unlike Analyze/Vet, Semgrep scans the WHOLE
// reviewed tree, not a diff's added lines: Side is always SideRight (there
// is no removed-line concept for a whole-tree SAST pass), and a path's
// leading "./" is trimmed so it aligns with diff/repository paths.
//
// Any failure to produce a trustworthy result -- the runner itself
// erroring with no parseable JSON, output that is not valid JSON, or valid
// JSON with no "results" key -- is returned as a non-nil error so the
// caller can route it to the policy gate's own inconclusive handling. This
// function never substitutes an empty finding list for a failed scan.
func (r *Runner) Semgrep(ctx context.Context, dir string, packs []string, run commandRunner) ([]Finding, error) {
	if run == nil {
		return nil, errors.New("analysis: nil command runner")
	}
	args := []string{"scan", "--json", "--quiet", "--metrics=off", "--disable-version-check"}
	for _, pack := range packs {
		pack = strings.TrimSpace(pack)
		if pack == "" {
			continue
		}
		args = append(args, "--config", pack)
	}
	args = append(args, ".")

	stdout, _, runErr := run(ctx, dir, args...)
	// Semgrep's own exit code is 1 whenever it reports findings -- that is
	// a successful scan, not a failure. An error with stdout that still
	// parses as a valid report (the exit==1 "findings reported" case) must
	// not be treated as an execution failure; only a runner error with
	// nothing parseable afterward is.
	findings, parseErr := parseSemgrepJSON(stdout)
	if parseErr != nil {
		if runErr != nil {
			return nil, fmt.Errorf("semgrep: execution failed: %w", runErr)
		}
		return nil, fmt.Errorf("semgrep: %w", parseErr)
	}
	return findings, nil
}

// parseSemgrepJSON decodes raw as a semgrepReport and converts every entry
// into a Finding. See ErrSemgrepNoResults for why a decodable document with
// no "results" key is refused rather than read as zero findings.
func parseSemgrepJSON(raw string) ([]Finding, error) {
	var report semgrepReport
	if err := json.Unmarshal([]byte(raw), &report); err != nil {
		return nil, fmt.Errorf("invalid JSON output: %w", err)
	}
	if report.Results == nil {
		return nil, ErrSemgrepNoResults
	}
	findings := make([]Finding, 0, len(*report.Results))
	for _, res := range *report.Results {
		findings = append(findings, Finding{
			Path:     strings.TrimPrefix(res.Path, "./"),
			Line:     res.Start.Line,
			Side:     SideRight,
			RuleID:   RuleSemgrepPrefix + res.CheckID,
			Severity: normalizeSemgrepSeverity(res.Extra.Severity),
			// Message is Semgrep's own fixed rule message, never the
			// matched source line (semgrep's "lines" field, which would
			// carry reviewed source text, is never read by this adapter).
			Message: strings.TrimSpace(res.Extra.Message),
		})
	}
	sortFindings(findings)
	return findings, nil
}

// normalizeSemgrepSeverity maps Semgrep's own ERROR/WARNING/INFO severity
// onto this project's lowercase error/warning/info vocabulary
// (severityRankOf, cmd/aurumcode/policygate.go). Anything unrecognized
// defaults to "error": an unknown severity from an external tool must
// never silently fail OPEN to a lower rank.
func normalizeSemgrepSeverity(s string) string {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "ERROR":
		return "error"
	case "WARNING":
		return "warning"
	case "INFO":
		return "info"
	default:
		return "error"
	}
}
