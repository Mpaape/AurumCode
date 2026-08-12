// Package redactionsuite executes the sealed AUR-009 vectors against the
// single redaction filter across the seven authorized sinks. It emits
// observations only; it never writes evidence, issues a verdict or asserts
// approval.
package redactionsuite

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	redaction "github.com/Mpaape/AurumCode/internal/security/redaction"
)

const (
	// MaxVectors, MaxTotalBytes and MaxDeadlineSeconds are the hard harness
	// ceilings; a spec beyond any of them is refused with a typed error.
	MaxVectors         = 64
	MaxTotalBytes      = 4194304
	MaxDeadlineSeconds = 30
	maxSpecFileBytes   = 1048576
	canaryPlaceholder  = "${CANARY}"
)

// SpecError is the typed refusal of an out-of-contract cases file. Code is a
// stable machine-readable reason; it never carries vector content.
type SpecError struct {
	Code string
	Line int
}

func (e *SpecError) Error() string {
	return fmt.Sprintf("redactionsuite: %s (line %d)", e.Code, e.Line)
}

// Case is one sealed vector.
type Case struct {
	Name        string
	Kind        string // nominal | invalid | boundary
	Channel     string // url | header | body | diff | error | tool_result | sink | limit
	Expect      string // redacted | identity | typed-error
	ErrorCode   string // sink-unauthorized | input-too-large (typed-error only)
	Input       string
	Expected    string
	RepeatUnit  string
	RepeatCount int
}

// Limits is the sealed limits block of the spec.
type Limits struct {
	MaxVectors      int
	MaxTotalBytes   int
	DeadlineSeconds int
}

// Spec is a parsed, bounds-checked cases file.
type Spec struct {
	Limits Limits
	Cases  []Case
}

// CaseResult is the bounded observation of one executed vector. Digests and
// counters only: raw sink content never leaves the harness.
type CaseResult struct {
	Name           string
	Kind           string
	Channel        string
	Result         string // ok | typed-error:<code> | missing-typed-error
	Effects        int
	ArtifactSHA256 string
	Mismatch       bool
	Leak           bool
}

// SweepResult is the ordered outcome of one full vector sweep.
type SweepResult struct {
	Cases  []CaseResult
	Digest string
}

func specErr(code string, line int) error {
	return &SpecError{Code: code, Line: line}
}

func unquote(raw string, line int) (string, error) {
	if len(raw) < 2 || raw[0] != '"' || raw[len(raw)-1] != '"' {
		return "", specErr("value-not-quoted", line)
	}
	body := raw[1 : len(raw)-1]
	if strings.ContainsAny(body, "\"\\") {
		return "", specErr("value-escape-forbidden", line)
	}
	return body, nil
}

func (c *Case) materialInput() string {
	if c.RepeatCount > 0 {
		return strings.Repeat(c.RepeatUnit, c.RepeatCount)
	}
	return c.Input
}

// Load parses and bounds-checks the cases file, expanding the canary
// placeholder in vector text. Every refusal is a typed *SpecError.
func Load(path, canary string) (*Spec, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, specErr("spec-unreadable", 0)
	}
	if info.Size() > maxSpecFileBytes {
		return nil, specErr("spec-file-too-large", 0)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, specErr("spec-unreadable", 0)
	}
	expand := func(s string) string {
		return strings.ReplaceAll(s, canaryPlaceholder, canary)
	}

	spec := &Spec{}
	var current *Case
	section := ""
	seenNames := map[string]bool{}
	lines := strings.Split(string(raw), "\n")
	for i, line := range lines {
		n := i + 1
		if strings.TrimSpace(line) == "" {
			continue
		}
		switch {
		case line == "cases:":
			section = "cases"
			continue
		case line == "limits:":
			section = "limits"
			continue
		case !strings.HasPrefix(line, " "):
			key, value, ok := strings.Cut(line, ": ")
			if !ok {
				return nil, specErr("top-level-malformed", n)
			}
			switch key {
			case "schema":
				if value != "aurum.redaction-cases" {
					return nil, specErr("schema-unknown", n)
				}
			case "version":
				if value != "1" {
					return nil, specErr("version-unknown", n)
				}
			default:
				return nil, specErr("top-level-key-unknown", n)
			}
			section = ""
			continue
		}
		trimmed := strings.TrimLeft(line, " ")
		key, value, ok := strings.Cut(trimmed, ":")
		if !ok {
			return nil, specErr("entry-malformed", n)
		}
		value = strings.TrimPrefix(value, " ")
		switch section {
		case "limits":
			num, err := strconv.Atoi(value)
			if err != nil || num < 1 {
				return nil, specErr("limit-not-positive", n)
			}
			switch key {
			case "max_vectors":
				spec.Limits.MaxVectors = num
			case "max_total_bytes":
				spec.Limits.MaxTotalBytes = num
			case "deadline_seconds":
				spec.Limits.DeadlineSeconds = num
			default:
				return nil, specErr("limit-key-unknown", n)
			}
		case "cases":
			if strings.HasPrefix(trimmed, "- ") {
				if key != "- name" {
					return nil, specErr("case-start-malformed", n)
				}
				name, err := unquote(value, n)
				if err != nil {
					return nil, err
				}
				if name == "" || seenNames[name] {
					return nil, specErr("case-name-invalid", n)
				}
				seenNames[name] = true
				spec.Cases = append(spec.Cases, Case{Name: name})
				current = &spec.Cases[len(spec.Cases)-1]
				continue
			}
			if current == nil {
				return nil, specErr("case-field-orphan", n)
			}
			switch key {
			case "kind":
				current.Kind = value
			case "channel":
				current.Channel = value
			case "expect":
				current.Expect = value
			case "error":
				current.ErrorCode = value
			case "input":
				text, err := unquote(value, n)
				if err != nil {
					return nil, err
				}
				current.Input = expand(text)
			case "expected":
				text, err := unquote(value, n)
				if err != nil {
					return nil, err
				}
				if strings.Contains(text, canaryPlaceholder) {
					return nil, specErr("expected-carries-secret", n)
				}
				current.Expected = text
			case "repeat_unit":
				text, err := unquote(value, n)
				if err != nil {
					return nil, err
				}
				if len(text) != 1 {
					return nil, specErr("repeat-unit-invalid", n)
				}
				current.RepeatUnit = text
			case "repeat_count":
				num, err := strconv.Atoi(value)
				if err != nil || num < 1 {
					return nil, specErr("repeat-count-invalid", n)
				}
				current.RepeatCount = num
			default:
				return nil, specErr("case-key-unknown", n)
			}
		default:
			return nil, specErr("section-unknown", n)
		}
	}

	if spec.Limits.MaxVectors != MaxVectors ||
		spec.Limits.MaxTotalBytes != MaxTotalBytes ||
		spec.Limits.DeadlineSeconds > MaxDeadlineSeconds ||
		spec.Limits.DeadlineSeconds < 1 {
		return nil, specErr("limits-out-of-contract", 0)
	}
	if len(spec.Cases) == 0 {
		return nil, specErr("cases-empty", 0)
	}
	if len(spec.Cases) > MaxVectors {
		return nil, specErr("cases-too-many", 0)
	}
	kinds := map[string]bool{}
	total := 0
	validKinds := map[string]bool{"nominal": true, "invalid": true, "boundary": true}
	validChannels := map[string]bool{
		"url": true, "header": true, "body": true, "diff": true,
		"error": true, "tool_result": true, "sink": true, "limit": true,
	}
	for idx := range spec.Cases {
		c := &spec.Cases[idx]
		if !validKinds[c.Kind] {
			return nil, specErr("case-kind-invalid", 0)
		}
		if !validChannels[c.Channel] {
			return nil, specErr("case-channel-invalid", 0)
		}
		switch c.Expect {
		case "redacted":
			if c.Expected == "" {
				return nil, specErr("expected-missing", 0)
			}
		case "identity":
		case "typed-error":
			if c.ErrorCode != "sink-unauthorized" && c.ErrorCode != "input-too-large" {
				return nil, specErr("error-code-invalid", 0)
			}
		default:
			return nil, specErr("case-expect-invalid", 0)
		}
		if c.RepeatCount > 0 && c.RepeatUnit == "" {
			return nil, specErr("repeat-unit-missing", 0)
		}
		kinds[c.Kind] = true
		total += len(c.Input) + c.RepeatCount
		if total > MaxTotalBytes {
			return nil, specErr("cases-too-large", 0)
		}
	}
	if !kinds["nominal"] || !kinds["invalid"] || !kinds["boundary"] {
		return nil, specErr("case-kinds-incomplete", 0)
	}
	return spec, nil
}

// SyntheticSpec renders an otherwise valid spec document carrying n trivial
// vectors, used to prove the harness refuses out-of-bound specs.
func SyntheticSpec(n int) []byte {
	var b bytes.Buffer
	b.WriteString("schema: aurum.redaction-cases\nversion: 1\nlimits:\n")
	fmt.Fprintf(&b, "  max_vectors: %d\n  max_total_bytes: %d\n  deadline_seconds: %d\ncases:\n",
		MaxVectors, MaxTotalBytes, MaxDeadlineSeconds)
	kinds := []string{"nominal", "invalid", "boundary"}
	for i := 0; i < n; i++ {
		kind := kinds[i%len(kinds)]
		if kind == "nominal" {
			fmt.Fprintf(&b, "  - name: \"synthetic-%d\"\n    kind: nominal\n    channel: body\n    expect: redacted\n    input: \"note ${CANARY} end\"\n    expected: \"note %s end\"\n", i, redaction.Marker)
			continue
		}
		fmt.Fprintf(&b, "  - name: \"synthetic-%d\"\n    kind: %s\n    channel: sink\n    expect: typed-error\n    error: sink-unauthorized\n    input: \"telemetry\"\n", i, kind)
	}
	return b.Bytes()
}

// SyntheticOversizeSpec renders a spec whose expanded vector bytes exceed
// MaxTotalBytes while keeping the document itself small.
func SyntheticOversizeSpec() []byte {
	head := SyntheticSpec(3)
	var b bytes.Buffer
	b.Write(head)
	fmt.Fprintf(&b, "  - name: \"synthetic-oversize\"\n    kind: boundary\n    channel: limit\n    expect: identity\n    repeat_unit: \"A\"\n    repeat_count: %d\n", MaxTotalBytes+1)
	return b.Bytes()
}

func digestBytes(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		io.WriteString(h, p)
		h.Write([]byte{0x1f})
	}
	return hex.EncodeToString(h.Sum(nil))
}

func runFilteredChannel(filter *redaction.Filter, input, expected, canary string) CaseResult {
	res := CaseResult{Result: "ok"}
	var sinkDigests []string
	for _, sink := range redaction.Sinks() {
		var buf bytes.Buffer
		w, err := filter.NewWriter(sink, &buf)
		if err != nil {
			res.Result = "sink-construction-failed"
			res.Mismatch = true
			continue
		}
		if _, err := w.Write([]byte(input)); err != nil {
			res.Result = "unexpected-write-error"
			res.Mismatch = true
			continue
		}
		if err := w.Flush(); err != nil {
			res.Result = "unexpected-flush-error"
			res.Mismatch = true
			continue
		}
		out := buf.String()
		res.Effects += w.Effects()
		if out != expected {
			res.Mismatch = true
		}
		if canary != "" && strings.Contains(out, canary) {
			res.Leak = true
		}
		sinkDigests = append(sinkDigests, digestBytes(string(sink), out))
	}
	res.ArtifactSHA256 = digestBytes(sinkDigests...)
	return res
}

func runErrorChannel(filter *redaction.Filter, input, expected, canary string) CaseResult {
	res := CaseResult{Result: "ok"}
	wrapped := filter.WrapError(errors.New(input))
	var typed *redaction.RedactedError
	if !errors.As(wrapped, &typed) {
		res.Result = "missing-typed-error"
		res.Mismatch = true
		return res
	}
	// An error value propagates outside filtered writers, so its rendering is
	// inspected raw: this is exactly the escape MUT-002 must make visible.
	artifact := wrapped.Error()
	res.Effects = 1
	if artifact != expected {
		res.Mismatch = true
	}
	if canary != "" && strings.Contains(artifact, canary) {
		res.Leak = true
	}
	res.ArtifactSHA256 = digestBytes("error", artifact)
	return res
}

func runSinkChannel(filter *redaction.Filter, name, canary string) CaseResult {
	res := CaseResult{}
	_, err := filter.NewWriter(redaction.Sink(name), io.Discard)
	var typed *redaction.SinkError
	if err == nil || !errors.As(err, &typed) {
		res.Result = "missing-typed-error"
		res.Mismatch = true
		res.ArtifactSHA256 = digestBytes("sink", "no-error")
		return res
	}
	res.Result = "typed-error:sink-unauthorized"
	if canary != "" && strings.Contains(typed.Error(), canary) {
		res.Leak = true
	}
	res.ArtifactSHA256 = digestBytes("sink", typed.Error())
	return res
}

func runLimitChannel(filter *redaction.Filter, c *Case, canary string) CaseResult {
	res := CaseResult{Result: "ok"}
	input := c.materialInput()
	var sinkDigests []string
	for _, sink := range redaction.Sinks() {
		var buf bytes.Buffer
		w, err := filter.NewWriter(sink, &buf)
		if err != nil {
			res.Result = "sink-construction-failed"
			res.Mismatch = true
			continue
		}
		_, writeErr := w.Write([]byte(input))
		if writeErr == nil {
			writeErr = w.Flush()
		}
		var limitErr *redaction.LimitError
		if c.Expect == "typed-error" {
			if !errors.As(writeErr, &limitErr) {
				res.Result = "missing-typed-error"
				res.Mismatch = true
			} else if buf.Len() != 0 || w.Effects() != 0 {
				// A forbidden effect happened before the typed refusal.
				res.Result = "effect-before-typed-error"
				res.Mismatch = true
			} else if res.Result == "ok" {
				res.Result = "typed-error:input-too-large"
			}
			sinkDigests = append(sinkDigests, digestBytes(string(sink), fmt.Sprintf("len=%d", buf.Len())))
			continue
		}
		if writeErr != nil {
			res.Result = "unexpected-write-error"
			res.Mismatch = true
			continue
		}
		res.Effects += w.Effects()
		if buf.String() != input {
			res.Mismatch = true
		}
		if canary != "" && strings.Contains(buf.String(), canary) {
			res.Leak = true
		}
		sinkDigests = append(sinkDigests, digestBytes(string(sink), fmt.Sprintf("len=%d", buf.Len())))
	}
	res.ArtifactSHA256 = digestBytes(sinkDigests...)
	return res
}

// Run executes every vector, in file order, against a filter that registers
// the canary. It returns bounded observations and a deterministic sweep
// digest; the same spec and canary always reproduce the same digest.
func Run(spec *Spec, canary string) (*SweepResult, error) {
	if canary == "" {
		return nil, errors.New("redactionsuite: an empty canary cannot prove absence")
	}
	filter := redaction.NewFilter(canary)
	sweep := &SweepResult{}
	var caseDigests []string
	for idx := range spec.Cases {
		c := &spec.Cases[idx]
		var res CaseResult
		switch c.Channel {
		case "sink":
			res = runSinkChannel(filter, c.Input, canary)
		case "error":
			res = runErrorChannel(filter, c.Input, c.Expected, canary)
		case "limit":
			res = runLimitChannel(filter, c, canary)
		default:
			expected := c.Expected
			if c.Expect == "identity" {
				expected = c.materialInput()
			}
			res = runFilteredChannel(filter, c.materialInput(), expected, canary)
		}
		if c.Expect == "typed-error" {
			want := "typed-error:" + c.ErrorCode
			if res.Result != want {
				res.Mismatch = true
			}
		}
		res.Name = c.Name
		res.Kind = c.Kind
		res.Channel = c.Channel
		sweep.Cases = append(sweep.Cases, res)
		caseDigests = append(caseDigests, digestBytes(res.Name, res.Result, strconv.Itoa(res.Effects), res.ArtifactSHA256))
	}
	sweep.Digest = digestBytes(caseDigests...)
	return sweep, nil
}
