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
// Errors is Semgrep's own top-level "errors" array: a FATAL failure (a
// rule-pack download failure, a parser crash, an invalid rule) can still
// print a syntactically valid report with "results": [] alongside a
// non-empty "errors" array, at exit 0 OR a non-zero exit -- see
// ErrSemgrepReportedErrors.
type semgrepReport struct {
	Results *[]semgrepResult `json:"results"`
	Errors  []semgrepError   `json:"errors"`
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

// semgrepError is one entry of Semgrep's own top-level "errors" array.
type semgrepError struct {
	Level   string `json:"level"`
	Type    string `json:"type"`
	Message string `json:"message"`
}

// ErrSemgrepNoResults is returned when Semgrep's own JSON decodes but
// carries no "results" key at all -- distinct from a genuinely empty scan
// (an explicit `"results": []`), which is a real, trustworthy "nothing
// found". A missing key is what a truncated, wrong-shaped or
// not-actually-Semgrep JSON document looks like, and must never be read as
// a clean pass (AUR-548/MUT-001: treating a failed execution as zero
// findings).
var ErrSemgrepNoResults = errors.New("semgrep: JSON output has no \"results\" key")

// ErrSemgrepReportedErrors is returned when Semgrep's own report decodes
// successfully (a "results" key is present, possibly empty) but its
// top-level "errors" array is non-empty. A rule-pack download failure, a
// parser crash on one file, or any other fatal-to-Semgrep-itself problem
// can print exactly this shape -- "results": [] with "errors": [...] --
// at ANY exit code, including 0. Reading that as "a clean scan found
// nothing" would let exactly the failure this error guards against pass
// as green (fail-closed per the review that found this gap).
var ErrSemgrepReportedErrors = errors.New("semgrep: report contains one or more errors")

// Semgrep runs `semgrep scan --json --config <pack>...` in dir through the
// injected commandRunner (the same seam Vet already uses) and parses the
// report into Findings. Unlike Analyze/Vet, Semgrep scans the WHOLE
// reviewed tree, not a diff's added lines: Side is always SideRight (there
// is no removed-line concept for a whole-tree SAST pass), and a path's
// leading "./" is trimmed so it aligns with diff/repository paths.
//
// policyOrigin adds two flags that keep the pull request's own AUTHOR
// from silencing a policy-mandated finding -- this run's quality_gates.sast
// section came from a central policy, so the scanned tree is the
// author's own content, not a self-configured, same-trust-boundary scan:
//   - `--disable-nosem`, so a `# nosemgrep` comment cannot suppress a
//     finding;
//   - `--x-ignore-semgrepignore-files` (an internal, undocumented-API
//     Semgrep flag, verified present in the exact pinned version this
//     project ships, 1.172.0: `semgrep scan --help`), so a
//     repository-committed `.semgrepignore` at ANY depth in the tree
//     cannot hide a file from the scan either. This flag was chosen over
//     scanning a filesystem copy with `.semgrepignore` stripped: a copy
//     that must itself defend against symlink tricks (a dangling
//     symlink, a symlink escaping the tree) is a second, harder-to-audit
//     attack surface Semgrep's own, already-audited target-selection
//     code does not have.
//
// Should this internal flag ever be removed in a future Semgrep version,
// the invocation simply errors (an unrecognized flag), which this
// function already turns into a non-nil error -- never a silent,
// unprotected scan.
//
// Any failure to produce a trustworthy result is returned as a non-nil
// error so the caller can route it to the policy gate's own inconclusive
// handling; this function never substitutes an empty or partial finding
// list for a failed scan. Three shapes of failure are distinguished:
//   - the runner itself erroring with nothing parseable left in stdout
//     (ErrNotFound when the binary is missing, wrapped otherwise);
//   - output that is not valid JSON, or valid JSON with no "results" key
//     (ErrSemgrepNoResults);
//   - a decodable report whose own "errors" array is non-empty
//     (ErrSemgrepReportedErrors) -- checked BEFORE the runner's own exit
//     code, so a fatal Semgrep-side failure is never masked by a report
//     that still parses.
//
// This command is never invoked with Semgrep's own `--error` flag, so
// (unlike some other CI integrations) exit code 1 here is NOT "findings
// were reported" -- it is an execution problem like any other non-zero
// exit, checked last, after both report-shape checks above have already
// had a chance to name a more specific reason.
func (r *Runner) Semgrep(ctx context.Context, dir string, packs []string, policyOrigin bool, run commandRunner) ([]Finding, error) {
	if run == nil {
		return nil, errors.New("analysis: nil command runner")
	}
	args := []string{"scan", "--json", "--quiet", "--metrics=off", "--disable-version-check"}
	if policyOrigin {
		args = append(args, "--disable-nosem", "--x-ignore-semgrepignore-files")
	}
	for _, pack := range packs {
		pack = strings.TrimSpace(pack)
		if pack == "" {
			continue
		}
		args = append(args, "--config", pack)
	}
	args = append(args, ".")

	stdout, _, runErr := run(ctx, dir, args...)
	report, parseErr := decodeSemgrepReport(stdout)
	if parseErr != nil {
		if runErr != nil {
			return nil, fmt.Errorf("semgrep: execution failed: %w", runErr)
		}
		return nil, fmt.Errorf("semgrep: %w", parseErr)
	}
	if len(report.Errors) > 0 {
		return nil, fmt.Errorf("semgrep: %w: %s", ErrSemgrepReportedErrors, firstSemgrepErrorMessage(report.Errors))
	}
	if runErr != nil { // AUR-548-MUT-001-ANCHOR
		return nil, fmt.Errorf("semgrep: execution failed: %w", runErr)
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

// decodeSemgrepReport decodes raw as a semgrepReport. See ErrSemgrepNoResults
// for why a decodable document with no "results" key is refused rather than
// read as zero findings.
func decodeSemgrepReport(raw string) (semgrepReport, error) {
	var report semgrepReport
	if err := json.Unmarshal([]byte(raw), &report); err != nil {
		return semgrepReport{}, fmt.Errorf("invalid JSON output: %w", err)
	}
	if report.Results == nil {
		return semgrepReport{}, ErrSemgrepNoResults
	}
	return report, nil
}

// firstSemgrepErrorMessage returns the first reported error's own message,
// trimmed, or "" when there is none. It is Semgrep's own fixed diagnostic
// text, not reviewed source, so it is safe to surface in a wrapped error.
func firstSemgrepErrorMessage(errs []semgrepError) string {
	if len(errs) == 0 {
		return ""
	}
	return strings.TrimSpace(errs[0].Message)
}

// normalizeSemgrepSeverity maps Semgrep's own severity vocabulary onto
// this project's lowercase error/warning/info rank (severityRankOf,
// cmd/aurumcode/policygate.go). Semgrep 1.x's `extra.severity` is
// documented as CRITICAL/HIGH/MEDIUM/LOW/INFO, plus INVENTORY/EXPERIMENT
// for rules not meant to gate anything on their own; the historical
// ERROR/WARNING spelling (this adapter's own fixtures, and some older
// rule packs) is accepted too. CRITICAL/HIGH rank as error, MEDIUM as
// warning, LOW/INFO as info; anything unrecognized (INVENTORY, EXPERIMENT,
// a future addition) defaults to error -- an unknown severity from an
// external tool must never silently fail OPEN to a lower rank.
func normalizeSemgrepSeverity(s string) string {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "CRITICAL", "HIGH", "ERROR":
		return "error"
	case "MEDIUM", "WARNING":
		return "warning"
	case "LOW", "INFO":
		return "info"
	default:
		return "error"
	}
}
