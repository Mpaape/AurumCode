package analysis

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Mpaape/AurumCode/internal/scanner"
)

// commandRunner executes a command in dir and returns its output channels
// separately. It is an injected seam so Vet never exec's anything itself:
// the scanner engine supplies a runner with an explicit, secret-free
// environment, while tests substitute a fake that returns canned output.
// ctx is the command's context (deadline and cancellation).
type commandRunner func(ctx context.Context, dir string, args ...string) (stdout, stderr string, err error)

// VetArgs is the go command line Vet runs: `go vet -json ./...`. In JSON
// mode go vet reports every analyzer diagnostic on stdout and exits 0; a
// non-zero exit means a package could not be loaded or type-checked, so the
// report does not cover the whole module (measured on go1.27: a module with
// one broken package and one printf diagnostic exits 1).
var VetArgs = []string{"vet", "-json", "./..."}

// RuleGoVetSeparator joins RuleGoVet and the go vet analyzer that reported
// a finding ("go-vet/printf"): the rule names the analyzer that a reader
// can rerun, so the finding's origin is verifiable.
const RuleGoVetSeparator = "/"

// vetSeverity: go vet diagnostics are suspicious constructs, not proven
// defects; a scanner entry's fail_on decides whether they block.
const vetSeverity = "warning"

// ErrVetFailed: go vet exited non-zero, so at least one package was not
// vetted; its partial report is never read as a complete scan.
var ErrVetFailed = errors.New("analysis: go vet did not vet every package")

// vetDiagnostic is one entry of go vet's JSON report.
type vetDiagnostic struct {
	Posn    string `json:"posn"`
	Message string `json:"message"`
}

// Vet runs `go vet -json ./...` in dir through the injected commandRunner
// and converts its JSON report into Findings with paths relative to dir.
//
// Vet never executes a command itself and treats the output as untrusted
// data: it is decoded as JSON, never interpreted. Any runner error (a
// missing go binary, a package that does not load, a timeout) is returned
// with no findings, so "vet failed" can never read as "vet found nothing"
// or as a partial list of findings.
func (r *Runner) Vet(ctx context.Context, dir string, run commandRunner) ([]Finding, error) {
	if run == nil {
		return nil, errors.New("analysis: nil command runner")
	}
	stdout, stderr, runErr := run(ctx, dir, VetArgs...)
	if runErr != nil {
		return nil, fmt.Errorf("%w: %w: %s", ErrVetFailed, runErr, firstLine(stderr))
	}
	findings, err := parseVetJSON(dir, stdout)
	if err != nil {
		return nil, err
	}
	sortFindings(findings)
	return findings, nil
}

// parseVetJSON decodes the stream of JSON objects go vet prints, one per
// package: {"import/path": {"analyzer": [{"posn": ..., "message": ...}]}}.
func parseVetJSON(dir, out string) ([]Finding, error) {
	var findings []Finding
	dec := json.NewDecoder(strings.NewReader(out))
	for {
		var pkgs map[string]map[string][]vetDiagnostic
		err := dec.Decode(&pkgs)
		if errors.Is(err, io.EOF) {
			return findings, nil
		}
		if err != nil {
			return nil, fmt.Errorf("analysis: go vet report: %v: %w", err, scanner.ErrInvalidOutput)
		}
		for _, analyzers := range pkgs {
			for analyzer, diags := range analyzers {
				for _, d := range diags {
					f, err := vetFinding(dir, analyzer, d)
					if err != nil {
						return nil, err
					}
					findings = append(findings, f)
				}
			}
		}
	}
}

// vetFinding converts one diagnostic; a position outside dir or without a
// line is an invalid report, never a finding on some other file.
func vetFinding(dir, analyzer string, d vetDiagnostic) (Finding, error) {
	path, line, ok := splitPosition(d.Posn)
	if ok && filepath.IsAbs(path) {
		rel, err := filepath.Rel(dir, path)
		path, ok = rel, err == nil
	}
	path = filepath.ToSlash(filepath.Clean(path))
	if !ok || path == ".." || strings.HasPrefix(path, "../") || strings.TrimSpace(analyzer) == "" {
		return Finding{}, fmt.Errorf("analysis: go vet position %q: %w", d.Posn, scanner.ErrInvalidOutput)
	}
	return Finding{
		Path:     path,
		Line:     line,
		Side:     SideRight,
		RuleID:   RuleGoVet + RuleGoVetSeparator + analyzer,
		Severity: vetSeverity,
		Message:  d.Message,
	}, nil
}

// splitPosition reads file:line:col (go vet's form) or file:line. The
// file part may itself hold a colon, so the numbers are read from the end.
func splitPosition(posn string) (string, int, bool) {
	rest := posn
	if head, tail, ok := cutLast(rest); ok && isNumber(tail) {
		if file, line, ok := cutLast(head); ok && isNumber(line) {
			rest = file + ":" + line
		}
	}
	file, raw, ok := cutLast(rest)
	line, err := strconv.Atoi(raw)
	if !ok || err != nil || line <= 0 || file == "" {
		return "", 0, false
	}
	return file, line, true
}

func cutLast(s string) (string, string, bool) {
	i := strings.LastIndex(s, ":")
	if i < 0 {
		return "", "", false
	}
	return s[:i], s[i+1:], true
}

func isNumber(s string) bool {
	_, err := strconv.Atoi(s)
	return err == nil
}

// firstLine is the first line of a tool's stderr, bounded, for an error.
func firstLine(s string) string {
	line, _, _ := bytes.Cut([]byte(strings.TrimSpace(s)), []byte("\n"))
	const max = 160
	if len(line) > max {
		line = line[:max]
	}
	return string(line)
}
