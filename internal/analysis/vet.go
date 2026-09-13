package analysis

import (
	"context"
	"errors"
	"regexp"
	"strconv"
	"strings"
)

// commandRunner executes a command in dir and returns its output channels
// separately. It is an injected seam so Vet never exec's anything itself:
// the real implementation (supplied by the integrator) shells out to a
// sandboxed "go" binary, while tests substitute a fake that returns canned
// output. ctx is the command's context (deadline and cancellation).
type commandRunner func(ctx context.Context, dir string, args ...string) (stdout, stderr string, err error)

// Vet runs "go vet ./..." in dir through the injected commandRunner and
// parses the standard go tool diagnostic format into Findings.
//
// Vet never executes a command itself, never uses the network, and treats
// the runner's output as untrusted text: only lines matching the
// file.go:line:col: message shape (with or without a column) become
// findings; package headers, summaries and stray lines are ignored. When the
// runner returns an error but parseable diagnostics were still produced (go
// vet exits non-zero whenever it reports findings) the findings are returned
// with a nil error; a runner error with no diagnostics is returned verbatim
// so the caller can distinguish "vet failed to run" from "vet found issues".
func (r *Runner) Vet(ctx context.Context, dir string, run commandRunner) ([]Finding, error) {
	if run == nil {
		return nil, errors.New("analysis: nil command runner")
	}

	stdout, stderr, runErr := run(ctx, dir, "vet", "./...")
	findings := parseVetOutput(stdout)
	findings = append(findings, parseVetOutput(stderr)...)
	sortFindings(findings)

	if runErr != nil && len(findings) == 0 {
		return nil, runErr
	}
	return findings, nil
}

// go vet prints one diagnostic per line as path/file.go:line:col: message
// (column is always present in vet output, but the no-column form is
// accepted for robustness against older tooling). Header lines begin with
// '#' and are skipped; a path's leading "./" is trimmed so findings align
// with diff paths.
var (
	vetDiagRe    = regexp.MustCompile(`^([^:\s]+\.go):(\d+):(\d+):\s*(.*)$`)
	vetDiagNoCol = regexp.MustCompile(`^([^:\s]+\.go):(\d+):\s*(.*)$`)
)

// parseVetOutput converts the raw go vet output into Findings, ignoring
// package headers ("# pkg"), blank lines, and any line that does not carry a
// file.go:line[:col]: message diagnostic.
func parseVetOutput(out string) []Finding {
	var findings []Finding
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if f, ok := parseVetLine(line); ok {
			findings = append(findings, f)
		}
	}
	return findings
}

// parseVetLine tries the with-column shape first, then the no-column shape,
// so a diagnostic that already matched the former is never re-read by the
// latter as line:col text in the message.
func parseVetLine(line string) (Finding, bool) {
	if m := vetDiagRe.FindStringSubmatch(line); m != nil {
		n, err := strconv.Atoi(m[2])
		if err != nil {
			return Finding{}, false
		}
		return Finding{
			Path:     strings.TrimPrefix(m[1], "./"),
			Line:     n,
			RuleID:   RuleGoVet,
			Severity: "warning",
			Message:  m[4],
		}, true
	}
	if m := vetDiagNoCol.FindStringSubmatch(line); m != nil {
		n, err := strconv.Atoi(m[2])
		if err != nil {
			return Finding{}, false
		}
		return Finding{
			Path:     strings.TrimPrefix(m[1], "./"),
			Line:     n,
			RuleID:   RuleGoVet,
			Severity: "warning",
			Message:  m[3],
		}, true
	}
	return Finding{}, false
}
