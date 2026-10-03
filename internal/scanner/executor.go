package scanner

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"time"
)

// Timeout bounds one scan: generous for a real registry scan in CI, short
// enough that a hung executable never stalls a review.
const Timeout = 120 * time.Second

// ErrInvalidOutput marks a scan whose output is not a trustworthy report
// (unparseable, wrong shape, or carrying the engine's own errors).
var ErrInvalidOutput = errors.New("scanner: invalid output")

// The suffixes of the inconclusive reason a failed scan yields; the prefix
// is the engine's category (or its name), so Semgrep keeps its
// sast_unavailable/sast_execution_error/sast_invalid_output tokens.
const (
	reasonUnavailable    = "_unavailable"
	reasonExecutionError = "_execution_error"
	reasonInvalidOutput  = "_invalid_output"
	reasonIncomplete     = "_incomplete"
)

// Outcome is one engine's scan as the gate reads it: the findings, or the
// inconclusive reason that replaces them. Findings is nil whenever Reason
// is set, so "failed" can never be read as "clean".
type Outcome struct {
	Engine   Engine
	Findings []Finding
	Reason   string
}

// Executor runs registered engines. A nil Command runs the real binaries.
type Executor struct {
	Command Command
}

// Scan runs engine e over req.Root. An error, a missing binary or an
// incomplete report is an inconclusive Outcome, never an empty finding
// list.
func (x Executor) Scan(ctx context.Context, e Engine, req Request) Outcome {
	if req.Command == nil {
		req.Command = x.command()
	}
	scanCtx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	report, err := e.Scanner.Run(scanCtx, req)
	if reason := FailureReason(e, report, err); reason != "" {
		return Outcome{Engine: e, Reason: reason}
	}
	return Outcome{Engine: e, Findings: report.Findings}
}

// FailureReason names why a scan is inconclusive, "" when it is
// trustworthy (including a clean scan with zero findings).
func FailureReason(e Engine, report Report, err error) string {
	prefix := Normalize(e.Category)
	if prefix == "" {
		prefix = Normalize(e.Name())
	}
	switch {
	case err == nil && report.Complete:
		return ""
	case err == nil:
		return prefix + reasonIncomplete
	case errors.Is(err, exec.ErrNotFound), errors.Is(err, ErrNotRegistered):
		return prefix + reasonUnavailable
	case errors.Is(err, ErrInvalidOutput):
		return prefix + reasonInvalidOutput
	default:
		return prefix + reasonExecutionError
	}
}

func (x Executor) command() Command {
	if x.Command != nil {
		return x.Command
	}
	return ExecCommand
}

// ExecCommand runs binary from PATH in dir: the production Command.
func ExecCommand(ctx context.Context, dir, binary string, args ...string) (stdout, stderr string, err error) {
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir = dir
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err = cmd.Run()
	return outBuf.String(), errBuf.String(), err
}
