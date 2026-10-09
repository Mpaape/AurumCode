// Package scanner is the contract every deterministic scanner engine
// implements and the closed registry of the engines compiled into the
// binary. An engine registers itself from its package's init; the binary
// imports internal/scanner/engines, the one list of compiled engines. No
// engine is ever loaded at run time: an `engine` the registry does not hold
// is a configuration error.
//
// A scanner's findings are deterministic evidence: the model may weigh one,
// never remove or downgrade it. A scanner that cannot produce a trustworthy
// report returns an error or a Report with Complete false, and the caller
// reads either as inconclusive, never as "zero findings".
package scanner

import (
	"context"
	"fmt"
	"strings"

	"github.com/Mpaape/AurumCode/pkg/types"
)

// Trust is who decided this scan: the repository itself, or a central
// policy over content the pull request's author controls. Under a policy
// an engine must ignore the author's in-tree suppressions.
type Trust int

const (
	// TrustRepository: the repository configured its own scan.
	TrustRepository Trust = iota
	// TrustPolicy: a central policy mandated the scan.
	TrustPolicy
)

// Options is an engine's own `options` map, as written in the YAML.
type Options map[string]any

// Request is one scan: the tree to scan, who mandated it, the engine's
// options, the reviewed commit range and the command runner (nil runs the
// real binary).
type Request struct {
	Root    string
	Trust   Trust
	Options Options
	Range   Range
	Command Command
	// Ignored reports a path the review leaves out (the repository's
	// `ignore` globs). A change-scoped engine drops those paths from the
	// lines the range added, so neither a finding nor a parse error there
	// judges the change. Nil ignores nothing.
	Ignored func(path string) bool
}

// Range is the reviewed commit range: the commits reachable from Head and
// not from Base. A history engine scans exactly these commits; a tree
// engine ignores it. Both ends are full commit ids resolved by the caller;
// an engine that needs the range treats an empty end as a failed scan,
// never as "scan the final tree instead".
type Range struct {
	Base string
	Head string
}

// Empty reports a range with either end missing.
func (r Range) Empty() bool {
	return strings.TrimSpace(r.Base) == "" || strings.TrimSpace(r.Head) == ""
}

// Command runs binary with args in dir and returns its streams.
type Command func(ctx context.Context, dir, binary string, args ...string) (stdout, stderr string, err error)

// Finding is one engine-reported finding.
type Finding struct {
	Path     string
	Line     int
	Side     string
	RuleID   string
	Severity string
	Message  string
}

// ToIssue is the one conversion of a deterministic finding into a review
// issue: the message cites the rule, and the origin is the engine's typed
// origin (never written by the model).
func (f Finding) ToIssue(origin string) types.ReviewIssue {
	return types.ReviewIssue{
		File:     f.Path,
		Line:     f.Line,
		Side:     f.Side,
		Severity: f.Severity,
		RuleID:   f.RuleID,
		Message:  fmt.Sprintf("%s (rule %s)", f.Message, f.RuleID),
		Origin:   origin,
	}
}

// Report is what one scan produced. Complete is false when the engine ran
// but could not vouch for the whole tree; such a report is inconclusive.
type Report struct {
	Findings []Finding
	Complete bool
	Version  string
}

// Scanner is one engine.
type Scanner interface {
	Name() string
	Run(ctx context.Context, req Request) (Report, error)
}

// Engine is one registry entry. Category is the gate.sources name the
// engine also answers to (every "sast" engine counts under gate.sources:
// [sast]); Origin is the typed origin of its findings in the gate line, the
// audit and the SARIF (empty: the engine's name). Validate checks the
// engine's options when the configuration is parsed. Environment is what
// the engine's child process receives beyond BaseEnvironment.
type Engine struct {
	Scanner     Scanner
	Category    string
	Origin      string
	Validate    func(Options) error
	Environment Environment
}

// Name is the engine's registered name.
func (e Engine) Name() string { return e.Scanner.Name() }

// TypedOrigin is the origin the engine's findings carry.
func (e Engine) TypedOrigin() string {
	if e.Origin != "" {
		return e.Origin
	}
	return e.Name()
}

// ValidateOptions checks opts with the engine's own validator, if any.
func (e Engine) ValidateOptions(opts Options) error {
	if e.Validate == nil {
		return nil
	}
	return e.Validate(opts)
}

// Normalize is the one spelling of an engine or source name.
func Normalize(name string) string { return strings.ToLower(strings.TrimSpace(name)) }
