package scanner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"
)

// Timeout bounds one scan: generous for a real registry scan in CI, short
// enough that a hung executable never stalls a review.
const Timeout = 120 * time.Second

// WaitDelay bounds how long a killed scan may keep its output pipes open
// (a grandchild such as a vet tool or semgrep-core): past it the pipes are
// closed and the scan fails, so Timeout is a real limit.
const WaitDelay = 5 * time.Second

// MaxOutputBytes bounds each output stream of one engine's process. A
// larger stream is not a trustworthy report: the scan is inconclusive.
const MaxOutputBytes = 32 << 20

// ErrOutputTooLarge marks a stream that exceeded MaxOutputBytes.
var ErrOutputTooLarge = fmt.Errorf("scanner: output exceeds %d bytes: %w", MaxOutputBytes, ErrInvalidOutput)

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
	// Detail is the engine's own account of a failure, one bounded line
	// (Summarize), so an inconclusive reason can be diagnosed; empty when
	// the scan is trustworthy. The caller redacts it before publishing.
	Detail string
	// Version is the report's engine identity (binary version and rule
	// base), for the caller's result digest.
	Version string
}

// Executor runs registered engines. A nil Command runs the real binaries,
// each with the engine's explicit ChildEnvironment, never the reviewing
// process's own.
type Executor struct {
	Command Command
}

// Scan runs engine e over req.Root. An error, a missing binary or an
// incomplete report is an inconclusive Outcome, never an empty finding
// list.
func (x Executor) Scan(ctx context.Context, e Engine, req Request) Outcome {
	if req.Command == nil {
		req.Command = x.command(e)
	}
	scanCtx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	report, err := e.Scanner.Run(scanCtx, req)
	if reason := FailureReason(e, report, err); reason != "" {
		return Outcome{Engine: e, Reason: reason, Detail: Summarize(err)}
	}
	return Outcome{Engine: e, Findings: report.Findings, Version: report.Version}
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

func (x Executor) command(e Engine) Command {
	if x.Command != nil {
		return x.Command
	}
	return IsolatedCommand(ChildEnvironment(os.LookupEnv, e.Environment))
}

// ExecCommand runs binary from PATH in dir with only BaseEnvironment.
func ExecCommand(ctx context.Context, dir, binary string, args ...string) (stdout, stderr string, err error) {
	return IsolatedCommand(ChildEnvironment(os.LookupEnv, Environment{}))(ctx, dir, binary, args...)
}

// IsolatedCommand is the production Command: binary from PATH in dir, with
// exactly env as its environment (nil is read as empty, never as
// "inherit"), each stream bounded by MaxOutputBytes, and its pipes closed
// WaitDelay after a cancelled context.
func IsolatedCommand(env []string) Command {
	if env == nil {
		env = []string{}
	}
	return func(ctx context.Context, dir, binary string, args ...string) (string, string, error) {
		cmd := exec.CommandContext(ctx, binary, args...)
		cmd.Dir = dir
		cmd.Env = env
		cmd.WaitDelay = WaitDelay
		outBuf, errBuf := &boundedBuffer{}, &boundedBuffer{}
		cmd.Stdout, cmd.Stderr = outBuf, errBuf
		err := cmd.Run()
		if outBuf.exceeded || errBuf.exceeded {
			return "", "", ErrOutputTooLarge
		}
		return outBuf.String(), errBuf.String(), err
	}
}

// boundedBuffer keeps at most MaxOutputBytes and remembers it was cut. It
// never fails a write, so the child is not killed by a broken pipe before
// the caller learns the output was too large. The buffer is a field, not
// embedded, so io.Copy cannot bypass Write through bytes.Buffer.ReadFrom.
type boundedBuffer struct {
	buf      bytes.Buffer
	exceeded bool
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if room := MaxOutputBytes - b.buf.Len(); len(p) > room {
		b.exceeded = true
		if room > 0 {
			b.buf.Write(p[:room])
		}
		return len(p), nil
	}
	return b.buf.Write(p)
}

func (b *boundedBuffer) String() string { return b.buf.String() }
