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
	// this filter accepts: `:`, `=` and Go's `:=`, taken whole so its `=`
	// is never left behind as the start of a value.
	assign = `[ \t]*(?::=|[:=])[ \t]*`
	// bareValue stops at the first delimiter that cannot belong to an
	// unquoted value. `"` and `'` are excluded so a quoted value is handled
	// by the quoted rules, and `}` so a JSON object cannot smuggle the tail
	// of a secret out through the closing brace. Its first byte is never
	// `=` or `:`, so the second byte of a comparison (`==`, `===`) or of a
	// scope operator is an operator, not a value.
	bareValue = `[^\s"',;&}=:][^\s"',;&}]*`
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
	// reKVKey matches a secret-bearing key and its separator only. The bare
	// value after it is measured with the tables of lexTable, never by the
	// search itself, so a search that resumes inside a value kept as code
	// does not read the rest of the line again.
	reKVKey = regexp.MustCompile(`(?i)(["']?` + secretKeyNames + `["']?` + assign + `)`)
	// A private key is the material between the banners, not the banner. The
	// whole block is replaced when the text is redacted at once; the Writer
	// keeps the equivalent state across lines.
	reCredBlock = regexp.MustCompile(`(?s)-----BEGIN[ \t][A-Z ]*PRIVATE KEY-----.*?-----END[ \t][A-Z ]*PRIVATE KEY-----`)
	reCredBegin = regexp.MustCompile(`-----BEGIN[ \t][A-Z ]*PRIVATE KEY-----`)
	reCredEnd   = regexp.MustCompile(`-----END[ \t][A-Z ]*PRIVATE KEY-----`)
	// The SendGrid, Slack and Discord shapes are AUR-609's: a well-known token
	// is masked wherever it appears, inside a call a bare value keeps as code
	// too. A Discord token's first
	// segment is the base64 of a numeric id, whose 4-character groups use a
	// fixed alphabet ([MNO][DTjz][AEIMQUYcgk][w-z0-5]); requiring at least four
	// such groups, a 6-character middle and a 27+ character tail keeps a
	// chain of long identifiers from matching.
	reCred = regexp.MustCompile(`(?:-----BEGIN[ \t][A-Z ]*PRIVATE KEY-----|-----END[ \t][A-Z ]*PRIVATE KEY-----|AKIA[0-9A-Z]{16}|sk-[A-Za-z0-9_-]{20,}|gh[pousr]_[A-Za-z0-9]{20,}|SG\.[\w-]{16,}\.[\w-]{16,}|xox[abpors]-[\w.-]{10,}|(?:[MNO][DTjz][AEIMQUYcgk][w-z0-5]){4,}[\w-]{0,3}\.[\w-]{6}\.[\w-]{27,})`)
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
// parseable by whatever produced it. A quoted value is a literal and is
// always replaced; a bare value unless it is code (see redactBareKeyValues).
func redactKeyValues(s string) string {
	s = reKVDouble.ReplaceAllString(s, `${1}"`+Marker+`"`)
	s = reKVSingle.ReplaceAllString(s, "${1}'"+Marker+"'")
	return redactBareKeyValues(s)
}

// redactBareKeyValues replaces the unquoted value of a secret-bearing key
// unless the value is code. In source code the right-hand side of
// `API_KEY = os.environ.get("API_KEY")` or `token := os.Getenv("TOKEN")` is
// the code under review, not a secret: masking it would hand the model a
// broken line and turn every finding about it into one that cites the marker.
// Anything else, a weak password included, is masked: the filter fails
// closed. The work is linear in len(s): the lexical tables are built once,
// each value question is answered from them in constant time, and every key
// search starts where the previous one stopped.
func redactBareKeyValues(s string) string {
	var t *lexTable
	var b strings.Builder
	last, pos := 0, 0
	for {
		start, ok := nextBareKey(s, pos)
		if !ok {
			break
		}
		if t == nil {
			t = newLexTable(s)
		}
		end := int(t.bareEnd[start])
		if t.codeValue(s, start, end) {
			// The value is kept as code, so the arguments of the call it
			// makes are scanned too: `token=build(secret=x)` must not hide
			// the inner secret behind the outer key.
			pos = start + 1
			continue
		}
		end = t.trimEnclosingClosers(s, start, end)
		b.WriteString(s[last:start])
		b.WriteString(Marker)
		last, pos = end, end
	}
	if last == 0 && b.Len() == 0 {
		return s
	}
	b.WriteString(s[last:])
	return b.String()
}

// nextBareKey returns where the bare value of the first secret-bearing key
// at or after pos starts. A key whose separator is followed by no value
// byte (a comparison's second `=`, a quote, a delimiter) is skipped.
func nextBareKey(s string, pos int) (int, bool) {
	for pos < len(s) {
		m := reKVKey.FindStringSubmatchIndex(s[pos:])
		if m == nil {
			return 0, false
		}
		// m[3] ends the key-and-separator group; the value starts there.
		if start := pos + m[3]; start < len(s) && bareValueStart(s[start]) {
			return start, true
		}
		pos += m[0] + 1
	}
	return 0, false
}

// bareValueStart reports whether c can open a bare value (bareValue): not a
// delimiter, and never `=` or `:`, so the second byte of a comparison
// (`==`, `===`) or of a scope operator is an operator, not a value.
func bareValueStart(c byte) bool {
	return !isBareDelimiter(c) && c != '=' && c != ':'
}

// isBareDelimiter reports whether c ends a bare value: whitespace (as `\s`
// in bareValue), a quote, `,`, `;`, `&` or `}`.
func isBareDelimiter(c byte) bool {
	switch c {
	case ' ', '\t', '\n', '\f', '\r', '"', '\'', ',', ';', '&', '}':
		return true
	}
	return false
}

// The bare-value classification below lives in this file, not beside it:
// the AUR-009 acceptance stages the filter as this single file, so the
// filter must compile on its own.

// lexTable holds, for one text, the lexical facts the bare-value rule asks
// about, each computed in a single pass so that every question costs O(1):
// where a bare value ends, which bracket closes an opening one on the same
// line (quotes respected), which bracket a closing one closes, and where a
// run of `)`/`]` ends.
type lexTable struct {
	bareEnd []int32
	match   []int32
	opener  []int32
	closers []int32
}

// newLexTable builds the tables for s. Brackets pair only within one line
// and outside quotes: a line break resets both, so a call left open at the
// end of a line never closes on a later one.
func newLexTable(s string) *lexTable {
	n := len(s)
	t := &lexTable{
		bareEnd: make([]int32, n+1),
		match:   make([]int32, n+1),
		opener:  make([]int32, n+1),
		closers: make([]int32, n+1),
	}
	t.bareEnd[n], t.closers[n], t.match[n], t.opener[n] = int32(n), int32(n), -1, -1
	for i := n - 1; i >= 0; i-- {
		t.match[i], t.opener[i] = -1, -1
		t.bareEnd[i] = t.bareEnd[i+1]
		if isBareDelimiter(s[i]) {
			t.bareEnd[i] = int32(i)
		}
		t.closers[i] = int32(i)
		if s[i] == ')' || s[i] == ']' {
			t.closers[i] = t.closers[i+1]
		}
	}
	var stack []int32
	var quote byte
	for i := 0; i < n; i++ {
		c := s[i]
		if c == '\r' || c == '\n' {
			stack, quote = stack[:0], 0
			continue
		}
		switch {
		case quote != 0:
			if c == '\\' && i+1 < n && s[i+1] != '\n' && s[i+1] != '\r' {
				i++
			} else if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'' || c == '`':
			quote = c
		case c == '(' || c == '[' || c == '{':
			stack = append(stack, int32(i))
		case c == ')' || c == ']' || c == '}':
			if k := len(stack); k > 0 && closes(s[stack[k-1]], c) {
				t.match[stack[k-1]], t.opener[i] = int32(i), stack[k-1]
				stack = stack[:k-1]
			}
		}
	}
	return t
}

// closes reports whether closer c closes the opening bracket o.
func closes(o, c byte) bool {
	return o == '(' && c == ')' || o == '[' && c == ']' || o == '{' && c == '}'
}

// codeValue reports whether the bare value s[start:valueEnd] is code that
// stays visible: a whole call chain or a whole variable, workflow or
// command reference that ends at an expression boundary and covers the
// value (or leaves only the closers of an enclosing call after it). A
// member chain or an identifier is not code here: masked whole, it leaves a
// valid line (`apiKey = [REDACTED]`).
func (t *lexTable) codeValue(s string, start, valueEnd int) bool {
	end, ok := t.codeEnd(s, start)
	return ok && expressionStop(s, end) && (end >= valueEnd || int(t.closers[end]) >= valueEnd)
}

// codeEnd parses, from start, a reference or a call chain and returns where
// it ends.
func (t *lexTable) codeEnd(s string, start int) (int, bool) {
	if s[start] == '$' {
		return t.referenceEnd(s, start)
	}
	return t.callChainEnd(s, start)
}

// referenceEnd parses `${{ … }}`, `$( … )`, `${NAME}` or `$NAME` whole;
// `${NAME:-literal}` is not a reference, it carries a literal.
func (t *lexTable) referenceEnd(s string, start int) (int, bool) {
	i := start + 1
	if i >= len(s) {
		return 0, false
	}
	switch {
	case s[i] == '{' && i+1 < len(s) && s[i+1] == '{':
		m := t.match[i]
		if m < 0 || t.match[i+1] != m-1 {
			return 0, false
		}
		return int(m) + 1, true
	case s[i] == '(':
		if m := t.match[i]; m >= 0 {
			return int(m) + 1, true
		}
		return 0, false
	case s[i] == '{':
		j := identEnd(s, i+1)
		if j == i+1 || j >= len(s) || s[j] != '}' {
			return 0, false
		}
		return j + 1, true
	default:
		j := identEnd(s, i)
		return j, j > i
	}
}

// callChainEnd parses identifiers joined by `.`, `->` or `::`, each
// optionally called, and accepts the chain only when it ends with a call
// (`os.environ.get("X")`, `os.getenv("X").strip()`, `Config::token()`).
func (t *lexTable) callChainEnd(s string, start int) (int, bool) {
	i := identEnd(s, start)
	if i == start {
		return 0, false
	}
	called := false
	for {
		if i < len(s) && s[i] == '(' {
			m := t.match[i]
			if m < 0 {
				return 0, false
			}
			i, called = int(m)+1, true
		}
		sep := separatorLen(s, i)
		if sep == 0 {
			return i, called
		}
		j := identEnd(s, i+sep)
		if j == i+sep {
			return 0, false
		}
		i, called = j, false
	}
}

// identEnd returns the end of the identifier that starts at i, or i.
func identEnd(s string, i int) int {
	if i >= len(s) || !(isLetter(s[i]) || s[i] == '_') {
		return i
	}
	j := i + 1
	for j < len(s) && (isLetter(s[j]) || s[j] == '_' || s[j] >= '0' && s[j] <= '9') {
		j++
	}
	return j
}

func isLetter(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

// separatorLen returns the length of a member or scope separator at i.
func separatorLen(s string, i int) int {
	switch {
	case i < len(s) && s[i] == '.':
		return 1
	case i+1 < len(s) && (s[i:i+2] == "->" || s[i:i+2] == "::"):
		return 2
	}
	return 0
}

// expressionStop reports whether the code that ends at i ends the
// expression: the text ends, or a space, `;`, `,`, `&`, line break or
// closing bracket follows.
func expressionStop(s string, i int) bool {
	if i >= len(s) {
		return true
	}
	return strings.IndexByte(" \t\r\n;,&)]}", s[i]) >= 0
}

// trimEnclosingClosers keeps out of the masked span the trailing `)`/`]`
// that close a bracket opened before the value (`f(password=x)`), so the
// enclosing call stays balanced.
func (t *lexTable) trimEnclosingClosers(s string, start, end int) int {
	for end > start && (s[end-1] == ')' || s[end-1] == ']') && int(t.opener[end-1]) < start {
		end--
	}
	return end
}

// reJWT is the JSON Web Token shape and reDERBase64 base64 DER key material
// written on one line (a PKCS#8 or PKCS#1 body starts with "MII"). Both are
// known shapes masked wherever they appear (redactCredentialShapes), inside
// a call kept as code too.
var (
	reJWT       = regexp.MustCompile(`eyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]+`)
	reDERBase64 = regexp.MustCompile(`MII[A-Za-z0-9+/]{12,}`)
)

// redactCredentialShapes replaces well-known credential token shapes and any
// complete private-key block present in the same text.
func redactCredentialShapes(s string) string {
	s = reCredBlock.ReplaceAllString(s, Marker)
	s = reJWT.ReplaceAllString(s, Marker)
	s = reDERBase64.ReplaceAllString(s, Marker)
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
