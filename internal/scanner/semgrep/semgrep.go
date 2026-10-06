// Package semgrep is the Semgrep engine of the scanner registry: a
// multi-language SAST pass over the whole reviewed tree, whose findings and
// recovered parse errors are kept only where the reviewed range added lines
// (the change scope, scanner.AddedLines). Its category is "sast" and, so the gate line, the audit and the
// SARIF of existing policies keep their bytes, its typed origin is "sast"
// too.
package semgrep

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/Mpaape/AurumCode/internal/scanner"
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

// semgrepError is one entry of Semgrep's own top-level "errors" array. Type
// is kept raw: Semgrep writes it as a string ("Syntax error") or as an array
// (["PartialParsing", [...]]), and decoding it into a string made every
// report with a partial parse unreadable (measured on this repository: 8 of
// its 13 errors). Path and Spans name the file and lines a parse error
// concerns, when Semgrep knows them.
type semgrepError struct {
	Level   string          `json:"level"`
	Type    json.RawMessage `json:"type"`
	Message string          `json:"message"`
	Path    string          `json:"path"`
	Spans   []semgrepSpan   `json:"spans"`
}

// semgrepSpan is the line range of one error location.
type semgrepSpan struct {
	Start struct {
		Line int `json:"line"`
	} `json:"start"`
	End struct {
		Line int `json:"line"`
	} `json:"end"`
}

// semgrepLevelWarn is Semgrep's level for an error it recovered from: the
// scan went on, without the part it could not parse.
const semgrepLevelWarn = "warn"

// blocking reports whether e makes the report untrustworthy for the change
// in added. A fatal error (any level but warn), or one Semgrep cannot place
// in a file, always does. A recovered parse error (warn) only does when it
// touches what the change added: a whole-file failure on a touched file, or
// a span over an added line. A parse error in a file the change did not
// touch hides nothing the review judges.
func (e semgrepError) blocking(added scanner.LineSet) bool {
	path := strings.TrimPrefix(e.Path, "./")
	if !strings.EqualFold(strings.TrimSpace(e.Level), semgrepLevelWarn) || path == "" {
		return true
	}
	if !added.Touched(path) {
		return false
	}
	if len(e.Spans) == 0 {
		return true
	}
	for _, span := range e.Spans {
		for line := span.Start.Line; line <= span.End.Line; line++ {
			if added[path][line] {
				return true
			}
		}
	}
	return false
}

// blockingErrors keeps the errors that make the report untrustworthy for the
// change in added.
func blockingErrors(errs []semgrepError, added scanner.LineSet) []semgrepError {
	var out []semgrepError
	for _, e := range errs {
		if e.blocking(added) {
			out = append(out, e)
		}
	}
	return out
}

// ErrSemgrepNoResults is returned when Semgrep's own JSON decodes but
// carries no "results" key at all -- distinct from a genuinely empty scan
// (an explicit `"results": []`), which is a real, trustworthy "nothing
// found". A missing key is what a truncated, wrong-shaped or
// not-actually-Semgrep JSON document looks like, and must never be read as
// a clean pass (AUR-548/MUT-001: treating a failed execution as zero
// findings).
var ErrSemgrepNoResults = fmt.Errorf("semgrep: JSON output has no \"results\" key: %w", scanner.ErrInvalidOutput)

// ErrSemgrepReportedErrors is returned when Semgrep's own report decodes
// successfully (a "results" key is present, possibly empty) but its
// top-level "errors" array is non-empty. A rule-pack download failure, a
// parser crash on one file, or any other fatal-to-Semgrep-itself problem
// can print exactly this shape -- "results": [] with "errors": [...] --
// at ANY exit code, including 0. Reading that as "a clean scan found
// nothing" would let exactly the failure this error guards against pass
// as green (fail-closed per the review that found this gap).
var ErrSemgrepReportedErrors = fmt.Errorf("semgrep: report contains one or more errors: %w", scanner.ErrInvalidOutput)

// scan runs `semgrep scan --json --config <pack>...` in dir through the
// injected scanner.Command and parses the report into findings. It scans the WHOLE
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
//   - a decodable report whose own "errors" array holds an error that
//     concerns the change (ErrSemgrepReportedErrors, see
//     semgrepError.blocking) -- checked BEFORE the runner's own exit
//     code, so a fatal Semgrep-side failure is never masked by a report
//     that still parses.
//
// This command is never invoked with Semgrep's own `--error` flag, so
// (unlike some other CI integrations) exit code 1 here is NOT "findings
// were reported" -- it is an execution problem like any other non-zero
// exit, checked last, after both report-shape checks above have already
// had a chance to name a more specific reason.
func scan(ctx context.Context, dir string, packs []string, policyOrigin bool, run scanner.Command, added scanner.LineSet) ([]scanner.Finding, error) {
	if run == nil {
		return nil, errors.New("semgrep: nil command runner")
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

	stdout, _, runErr := run(ctx, dir, binary, args...)
	report, parseErr := decodeSemgrepReport(stdout)
	if parseErr != nil {
		if runErr != nil {
			return nil, fmt.Errorf("semgrep: execution failed: %w", runErr)
		}
		return nil, fmt.Errorf("semgrep: %w", parseErr)
	}
	if errs := blockingErrors(report.Errors, added); len(errs) > 0 {
		return nil, fmt.Errorf("semgrep: %w: %s", ErrSemgrepReportedErrors, firstSemgrepErrorMessage(errs))
	}
	if runErr != nil { // AUR-548-MUT-001-ANCHOR
		return nil, fmt.Errorf("semgrep: execution failed: %w", runErr)
	}

	findings := make([]scanner.Finding, 0, len(*report.Results))
	for _, res := range *report.Results {
		findings = append(findings, scanner.Finding{
			Path:     strings.TrimPrefix(res.Path, "./"),
			Line:     res.Start.Line,
			Side:     sideRight,
			RuleID:   RuleSemgrepPrefix + res.CheckID,
			Severity: normalizeSemgrepSeverity(res.Extra.Severity),
			// Message is Semgrep's own fixed rule message, never the
			// matched source line (semgrep's "lines" field, which would
			// carry reviewed source text, is never read by this adapter).
			Message: strings.TrimSpace(res.Extra.Message),
		})
	}
	sort.SliceStable(findings, func(i, j int) bool {
		a, b := findings[i], findings[j]
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		return a.RuleID < b.RuleID
	})
	return findings, nil
}

// decodeSemgrepReport decodes raw as a semgrepReport. See ErrSemgrepNoResults
// for why a decodable document with no "results" key is refused rather than
// read as zero findings.
func decodeSemgrepReport(raw string) (semgrepReport, error) {
	var report semgrepReport
	if err := json.Unmarshal([]byte(raw), &report); err != nil {
		return semgrepReport{}, fmt.Errorf("invalid JSON output: %w: %v", scanner.ErrInvalidOutput, err)
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
