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

// Rule fragments shared by the line-oriented and the serialized (JSON, YAML)
// spellings of the same secret. A structured payload is the dominant
// serialization in this system, so every value rule has to match both an
// anchored `Key: value` line and an embedded `"key":"value"` pair.
const (
	authHeaderNames = `(?:authorization|proxy-authorization|x-api-key|api-key|x-auth-token|x-amz-security-token|cookie|set-cookie)`
	secretKeyNames  = `(?:password|passwd|secret|token|api[_-]?key|access[_-]?key|client[_-]?secret|private[_-]?key|credential)`
	// assign is the separator between a key and its value in every spelling
	// this filter accepts.
	assign = `[ \t]*[:=][ \t]*`
	// bareValue stops at the first delimiter that cannot belong to an
	// unquoted value. `"` and `'` are excluded so a quoted value is handled
	// by the quoted rules, and `}` so a JSON object cannot smuggle the tail
	// of a secret out through the closing brace.
	bareValue = `[^\s"',;&}]+`
)

var (
	reQuery = regexp.MustCompile(`((?:https?|wss?|ftp)://[^\s"'<>]*?)\?[^\s"'<>]*`)
	// A credential carried in the userinfo component of a URL is a secret in
	// the URL channel exactly like a query parameter is.
	reUserinfo = regexp.MustCompile(`(?i)((?:https?|wss?|ftp|ssh|git)://)[^\s/@"'<>]+@`)
	reHeader   = regexp.MustCompile(`(?im)^([ \t]*` + authHeaderNames + `[ \t]*:).*$`)
	// The quoted header/key rules run before the bare ones: once a value is
	// replaced it starts with a quote, which the bare rules exclude, so the
	// chain cannot redact the same value twice into a different rendering.
	reHeaderJSONDouble = regexp.MustCompile(`(?i)(["']` + authHeaderNames + `["']` + assign + `)"[^"]*"`)
	reHeaderJSONSingle = regexp.MustCompile(`(?i)(["']` + authHeaderNames + `["']` + assign + `)'[^']*'`)
	reHeaderJSONBare   = regexp.MustCompile(`(?i)(["']` + authHeaderNames + `["']` + assign + `)` + bareValue)
	reKVDouble         = regexp.MustCompile(`(?i)(["']?` + secretKeyNames + `["']?` + assign + `)"[^"]*"`)
	reKVSingle         = regexp.MustCompile(`(?i)(["']?` + secretKeyNames + `["']?` + assign + `)'[^']*'`)
	reKVBare           = regexp.MustCompile(`(?i)(["']?` + secretKeyNames + `["']?` + assign + `)` + bareValue)
	// A private key is the material between the banners, not the banner. The
	// whole block is replaced when the text is redacted at once; the Writer
	// keeps the equivalent state across lines.
	reCredBlock = regexp.MustCompile(`(?s)-----BEGIN[ \t][A-Z ]*PRIVATE KEY-----.*?-----END[ \t][A-Z ]*PRIVATE KEY-----`)
	reCredBegin = regexp.MustCompile(`-----BEGIN[ \t][A-Z ]*PRIVATE KEY-----`)
	reCredEnd   = regexp.MustCompile(`-----END[ \t][A-Z ]*PRIVATE KEY-----`)
	reCred      = regexp.MustCompile(`(?:-----BEGIN[ \t][A-Z ]*PRIVATE KEY-----|-----END[ \t][A-Z ]*PRIVATE KEY-----|AKIA[0-9A-Z]{16}|sk-[A-Za-z0-9_-]{20,}|gh[pousr]_[A-Za-z0-9]{20,})`)
)

// redactQueryStrings replaces every URL query string with the stable marker.
// MUT-001 disables exactly this rule.
func redactQueryStrings(s string) string {
	return reQuery.ReplaceAllString(s, "${1}?"+Marker)
}

// redactURLCredentials replaces the userinfo component of a URL.
func redactURLCredentials(s string) string {
	return reUserinfo.ReplaceAllString(s, "${1}"+Marker+"@")
}

// redactAuthHeaders replaces the value of authentication and session headers,
// both as an anchored header line and as an embedded serialized pair.
func redactAuthHeaders(s string) string {
	s = reHeader.ReplaceAllString(s, "${1} "+Marker)
	s = reHeaderJSONDouble.ReplaceAllString(s, `${1}"`+Marker+`"`)
	s = reHeaderJSONSingle.ReplaceAllString(s, "${1}'"+Marker+"'")
	return reHeaderJSONBare.ReplaceAllString(s, "${1}"+Marker)
}

// redactKeyValues replaces the value of secret-bearing key/value assignments,
// quoted or bare. The quoting itself is preserved so a redacted payload stays
// parseable by whatever produced it.
func redactKeyValues(s string) string {
	s = reKVDouble.ReplaceAllString(s, `${1}"`+Marker+`"`)
	s = reKVSingle.ReplaceAllString(s, "${1}'"+Marker+"'")
	return reKVBare.ReplaceAllString(s, "${1}"+Marker)
}

// redactCredentialShapes replaces well-known credential token shapes and any
// complete private-key block present in the same text.
func redactCredentialShapes(s string) string {
	s = reCredBlock.ReplaceAllString(s, Marker)
	return reCred.ReplaceAllString(s, Marker)
}

// Filter is the single redaction filter. A zero secrets list still applies
// every structural rule; registered secret values add exact-value defense in
// depth on top of the structural rules.
type Filter struct {
	secrets []string
	// sliced[i] matches secrets[i] even when line breaks were inserted
	// between its bytes, so a value cut by a newline cannot leave the
	// process as two verbatim fragments. It is nil for a secret too long to
	// expand into a bounded pattern; that secret keeps the literal rule.
	sliced []*regexp.Regexp
}

// maxSlicedSecretBytes bounds the secret length for which the line-break
// tolerant pattern is built, so a pathologically long registered value cannot
// expand into an unbounded regular expression.
const maxSlicedSecretBytes = 512

// slicedSecretPattern compiles a pattern matching secret with any number of
// line breaks between its bytes. It also matches the contiguous secret.
func slicedSecretPattern(secret string) *regexp.Regexp {
	if len(secret) > maxSlicedSecretBytes {
		return nil
	}
	parts := make([]string, 0, len(secret))
	for i := 0; i < len(secret); i++ {
		c := secret[i]
		if c < 0x20 || c > 0x7e {
			// A non-ASCII byte cannot be split into a single-byte pattern
			// safely; that secret keeps the literal rule.
			return nil
		}
		parts = append(parts, regexp.QuoteMeta(string(rune(c))))
	}
	return regexp.MustCompile(strings.Join(parts, `[\r\n]*`))
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
	sliced := make([]*regexp.Regexp, len(kept))
	for i, s := range kept {
		sliced[i] = slicedSecretPattern(s)
	}
	return &Filter{secrets: kept, sliced: sliced}
}

// FromEnv builds a filter that registers the AURUM_SECRET_CANARY environment
// value when it is present.
func FromEnv() *Filter {
	return NewFilter(os.Getenv(CanaryEnv))
}

func (f *Filter) redactRegisteredSecrets(s string) string {
	for i, secret := range f.secrets {
		if re := f.sliced[i]; re != nil {
			s = re.ReplaceAllString(s, Marker)
			continue
		}
		s = strings.ReplaceAll(s, secret, Marker)
	}
	return s
}

// holdBack reports how many trailing bytes of buf must stay buffered because
// they could still grow into a registered secret, including a secret being
// sliced by a line break whose remainder has not arrived yet. Without it a
// line-oriented sink would emit the first half of a secret verbatim.
func (f *Filter) holdBack(buf []byte) int {
	hold := 0
	for _, secret := range f.secrets {
		// A sliced secret can carry at most one CRLF between two bytes, so
		// its stretched form is bounded by three times its length.
		window := 3 * len(secret)
		if window > len(buf) {
			window = len(buf)
		}
		for k := window; k > hold; k-- {
			tail := stripLineBreaks(buf[len(buf)-k:])
			if tail == "" || len(tail) >= len(secret) {
				continue
			}
			if strings.HasPrefix(secret, tail) {
				hold = k
				break
			}
		}
	}
	return hold
}

func stripLineBreaks(b []byte) string {
	var sb strings.Builder
	for _, c := range b {
		if c == '\n' || c == '\r' {
			continue
		}
		sb.WriteByte(c)
	}
	return sb.String()
}

// Redact applies the full rule chain and returns the sanitized text.
func (f *Filter) Redact(s string) string {
	out := redactQueryStrings(s)
	out = redactURLCredentials(out)
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
// Two pieces of state cross the line boundary, because a secret does not stop
// at one: inCredBlock suppresses the body of a private-key block, and the
// filter's hold-back keeps a fragment that could still complete a registered
// secret out of the sink until it is resolved.
type Writer struct {
	filter      *Filter
	sink        Sink
	dst         io.Writer
	buf         []byte
	effects     int
	inCredBlock bool
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
// beyond MaxInputBytes fails with a typed *LimitError before any byte of the
// current buffer reaches the underlying sink.
func (w *Writer) Write(p []byte) (int, error) {
	w.buf = append(w.buf, p...)
	// The boundary is a property of the raw input, so it is enforced before
	// any rule can shrink the buffer, and it refuses the whole buffer rather
	// than emitting the lines that preceded the oversized one.
	if err := w.checkLimit(); err != nil {
		return 0, err
	}
	// A registered secret sliced by a line break is joined here, while both
	// halves are still buffered, so no fragment of it can be forwarded.
	if bytes.IndexByte(w.buf, '\n') >= 0 {
		w.buf = []byte(w.filter.redactRegisteredSecrets(string(w.buf)))
	}
	hold := w.filter.holdBack(w.buf)
	for {
		safe := len(w.buf) - hold
		if safe <= 0 {
			break
		}
		idx := bytes.IndexByte(w.buf[:safe], '\n')
		if idx < 0 {
			break
		}
		line := string(w.buf[:idx])
		w.buf = w.buf[idx+1:]
		if err := w.emit(line, true); err != nil {
			return 0, err
		}
	}
	return len(p), nil
}

// checkLimit refuses the first raw line, or the trailing raw fragment, beyond
// MaxInputBytes with a typed *LimitError and drops the buffer.
func (w *Writer) checkLimit() error {
	rest := w.buf
	for {
		idx := bytes.IndexByte(rest, '\n')
		if idx < 0 {
			break
		}
		if idx > MaxInputBytes {
			w.buf = nil
			return &LimitError{Limit: MaxInputBytes}
		}
		rest = rest[idx+1:]
	}
	if len(rest) > MaxInputBytes {
		w.buf = nil
		return &LimitError{Limit: MaxInputBytes}
	}
	return nil
}

// emit redacts one line and forwards it unless it belongs to the body of a
// private-key block, which never reaches a sink at all.
func (w *Writer) emit(line string, newline bool) error {
	if w.inCredBlock {
		// Fail closed: everything up to and including the closing banner is
		// key material as far as this filter is concerned.
		if reCredEnd.MatchString(line) {
			w.inCredBlock = false
		}
		return nil
	}
	if reCredBegin.MatchString(line) && !reCredEnd.MatchString(line) {
		w.inCredBlock = true
	}
	out := w.filter.Redact(line)
	if newline {
		out += "\n"
	}
	if _, err := io.WriteString(w.dst, out); err != nil {
		return err
	}
	w.effects++
	return nil
}

// Flush redacts and forwards everything still buffered, including any line
// that was held back while it could still complete a registered secret.
func (w *Writer) Flush() error {
	if len(w.buf) == 0 {
		return nil
	}
	pending := w.filter.redactRegisteredSecrets(string(w.buf))
	w.buf = w.buf[:0]
	for {
		idx := strings.IndexByte(pending, '\n')
		if idx < 0 {
			break
		}
		line := pending[:idx]
		pending = pending[idx+1:]
		if err := w.emit(line, true); err != nil {
			return err
		}
	}
	if pending == "" {
		return nil
	}
	return w.emit(pending, false)
}
