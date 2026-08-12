// Package redaction implements the single AUR-009 redaction filter.
//
// Every process sink -- stdout, stderr, log, finding, diff, cache and
// evidence -- must write through this one filter. The filter replaces URL
// query strings, authentication headers, credential-shaped tokens,
// secret-bearing key/value assignments and registered secret values (such as
// the AURUM_SECRET_CANARY environment value) with a stable marker before any
// byte reaches a sink. Secret custody stays outside this package: it only
// prevents leakage, it never stores, manages or rotates a credential.
package redaction

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
)

const (
	// Marker is the stable replacement written in place of redacted content.
	Marker = "[REDACTED]"
	// MaxInputBytes bounds a single logical write (one line or one flush).
	// The first value beyond this boundary fails with a typed *LimitError
	// before any byte reaches the underlying sink.
	MaxInputBytes = 65536
	// CanaryEnv names the environment variable whose value, when present,
	// is registered as a secret by FromEnv.
	CanaryEnv = "AURUM_SECRET_CANARY"
)

// Sink names one authorized output channel. Only the seven canonical sinks
// are constructible through NewWriter; any other name is refused with a
// typed error before a writer exists.
type Sink string

const (
	SinkStdout   Sink = "stdout"
	SinkStderr   Sink = "stderr"
	SinkLog      Sink = "log"
	SinkFinding  Sink = "finding"
	SinkDiff     Sink = "diff"
	SinkCache    Sink = "cache"
	SinkEvidence Sink = "evidence"
)

var canonicalSinks = []Sink{
	SinkStdout, SinkStderr, SinkLog, SinkFinding, SinkDiff, SinkCache, SinkEvidence,
}

// Sinks returns the seven authorized sinks in canonical order.
func Sinks() []Sink {
	out := make([]Sink, len(canonicalSinks))
	copy(out, canonicalSinks)
	return out
}

// ValidSink reports whether s is one of the seven authorized sinks.
func ValidSink(s Sink) bool {
	for _, known := range canonicalSinks {
		if s == known {
			return true
		}
	}
	return false
}

// SinkError is the typed refusal of an unauthorized sink name. Name is
// already redacted by the filter that produced the error.
type SinkError struct {
	Name string
}

func (e *SinkError) Error() string {
	return "redaction: sink is not authorized: " + e.Name
}

// LimitError is the typed refusal of an input beyond MaxInputBytes. It is
// returned before any byte is written to the underlying sink.
type LimitError struct {
	Limit int
}

func (e *LimitError) Error() string {
	return fmt.Sprintf("redaction: input exceeds %d bytes before any sink write", e.Limit)
}

// RedactedError carries a fully redacted message and deliberately does not
// expose its raw cause: an error value escapes writer boundaries, so the
// redaction must already have happened when the error is constructed.
type RedactedError struct {
	msg string
}

func (e *RedactedError) Error() string {
	return e.msg
}

var (
	reQuery  = regexp.MustCompile(`((?:https?|wss?|ftp)://[^\s"'<>]*?)\?[^\s"'<>]*`)
	reHeader = regexp.MustCompile(`(?im)^([ \t]*(?:authorization|proxy-authorization|x-api-key|api-key|x-auth-token|x-amz-security-token|cookie|set-cookie)[ \t]*:).*$`)
	reKV     = regexp.MustCompile(`(?i)((?:password|passwd|secret|token|api[_-]?key|access[_-]?key|client[_-]?secret|private[_-]?key|credential)[ \t]*[:=][ \t]*)[^\s"'&,;]+`)
	reCred   = regexp.MustCompile(`(?:-----BEGIN[ \t][A-Z ]*PRIVATE KEY-----|AKIA[0-9A-Z]{16}|sk-[A-Za-z0-9_-]{20,}|gh[pousr]_[A-Za-z0-9]{20,})`)
)

// redactQueryStrings replaces every URL query string with the stable marker.
// MUT-001 disables exactly this rule.
func redactQueryStrings(s string) string {
	return reQuery.ReplaceAllString(s, "${1}?"+Marker)
}

// redactAuthHeaders replaces the value of authentication and session headers.
func redactAuthHeaders(s string) string {
	return reHeader.ReplaceAllString(s, "${1} "+Marker)
}

// redactKeyValues replaces the value of secret-bearing key/value assignments.
func redactKeyValues(s string) string {
	return reKV.ReplaceAllString(s, "${1}"+Marker)
}

// redactCredentialShapes replaces well-known credential token shapes.
func redactCredentialShapes(s string) string {
	return reCred.ReplaceAllString(s, Marker)
}

// Filter is the single redaction filter. A zero secrets list still applies
// every structural rule; registered secret values add exact-value defense in
// depth on top of the structural rules.
type Filter struct {
	secrets []string
}

// NewFilter builds a filter over the given registered secret values. Empty
// values are dropped; longer secrets are replaced first so an overlapping
// shorter secret can never split a longer one in half.
func NewFilter(secrets ...string) *Filter {
	kept := make([]string, 0, len(secrets))
	for _, s := range secrets {
		if s != "" {
			kept = append(kept, s)
		}
	}
	for i := 1; i < len(kept); i++ {
		for j := i; j > 0 && len(kept[j]) > len(kept[j-1]); j-- {
			kept[j], kept[j-1] = kept[j-1], kept[j]
		}
	}
	return &Filter{secrets: kept}
}

// FromEnv builds a filter that registers the AURUM_SECRET_CANARY environment
// value when it is present.
func FromEnv() *Filter {
	return NewFilter(os.Getenv(CanaryEnv))
}

func (f *Filter) redactRegisteredSecrets(s string) string {
	for _, secret := range f.secrets {
		s = strings.ReplaceAll(s, secret, Marker)
	}
	return s
}

// Redact applies the full rule chain and returns the sanitized text.
func (f *Filter) Redact(s string) string {
	out := redactQueryStrings(s)
	out = redactAuthHeaders(out)
	out = redactKeyValues(out)
	out = redactCredentialShapes(out)
	out = f.redactRegisteredSecrets(out)
	return out
}

// RedactBounded redacts s, refusing inputs beyond MaxInputBytes with a typed
// *LimitError before producing any output.
func (f *Filter) RedactBounded(s string) (string, error) {
	if len(s) > MaxInputBytes {
		return "", &LimitError{Limit: MaxInputBytes}
	}
	return f.Redact(s), nil
}

// WrapError returns a typed *RedactedError whose message is the redacted
// rendering of err. A nil error stays nil.
func (f *Filter) WrapError(err error) error {
	if err == nil {
		return nil
	}
	return &RedactedError{msg: f.Redact(err.Error())}
}

// Writer applies the filter to every line before it reaches the wrapped
// sink. Writes are buffered per line; Flush drains a trailing partial line.
type Writer struct {
	filter  *Filter
	sink    Sink
	dst     io.Writer
	buf     []byte
	effects int
}

// NewWriter wraps dst as the named sink. An unauthorized sink name or a nil
// destination is refused with a typed error before any writer exists.
func (f *Filter) NewWriter(sink Sink, dst io.Writer) (*Writer, error) {
	if !ValidSink(sink) {
		return nil, &SinkError{Name: f.Redact(string(sink))}
	}
	if dst == nil {
		return nil, errors.New("redaction: nil sink destination")
	}
	return &Writer{filter: f, sink: sink, dst: dst}, nil
}

// Sink returns the authorized sink this writer serves.
func (w *Writer) Sink() Sink {
	return w.sink
}

// Effects counts the writes actually performed on the underlying sink.
func (w *Writer) Effects() int {
	return w.effects
}

// Write buffers p, redacts every completed line and forwards it. A line
// beyond MaxInputBytes fails with a typed *LimitError before any byte of it
// reaches the underlying sink.
func (w *Writer) Write(p []byte) (int, error) {
	w.buf = append(w.buf, p...)
	for {
		idx := bytes.IndexByte(w.buf, '\n')
		if idx < 0 {
			break
		}
		line := string(w.buf[:idx])
		w.buf = w.buf[idx+1:]
		if len(line) > MaxInputBytes {
			w.buf = nil
			return 0, &LimitError{Limit: MaxInputBytes}
		}
		if _, err := io.WriteString(w.dst, w.filter.Redact(line)+"\n"); err != nil {
			return 0, err
		}
		w.effects++
	}
	if len(w.buf) > MaxInputBytes {
		w.buf = nil
		return 0, &LimitError{Limit: MaxInputBytes}
	}
	return len(p), nil
}

// Flush redacts and forwards any buffered partial line.
func (w *Writer) Flush() error {
	if len(w.buf) == 0 {
		return nil
	}
	line := string(w.buf)
	w.buf = w.buf[:0]
	if _, err := io.WriteString(w.dst, w.filter.Redact(line)); err != nil {
		return err
	}
	w.effects++
	return nil
}
