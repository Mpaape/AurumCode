package analysis

import (
	"regexp"
	"sort"
	"strings"

	"github.com/Mpaape/AurumCode/pkg/types"
)

// Finding is a single deterministic static-analysis result. It carries the
// publication fields an integrator needs to merge it with model findings
// without depending on internal/review: Path and Line locate the change,
// Side carries the same RIGHT=added / LEFT=removed convention as
// types.ReviewIssue.Side, and RuleID / Severity / Message carry the catalog
// identity and a fixed, trusted message. Message never contains the matched
// source text, so a Finding is always safe to surface verbatim.
type Finding struct {
	Path     string
	Line     int
	Side     string
	RuleID   string
	Severity string
	Message  string
}

// Side conventions, shared with types.ReviewIssue.
const (
	SideRight = "RIGHT" // an addition, located in the new file
	SideLeft  = "LEFT"  // a deletion, located in the old file
)

// Rule identifiers for the embedded deterministic catalog. Exported so an
// integrator can dedupe deterministic findings against model findings by
// RuleID and merge them deterministically.
const (
	RuleHardcodedSecret  = "analysis/hardcoded-secret"
	RuleCommandInjection = "analysis/command-injection"
	RuleFilePermissions  = "analysis/overly-permissive-file-perms"
	RuleSQLInjection     = "analysis/sql-injection"
	RuleGoVet            = "go-vet"
)

// Fixed, trusted messages for the embedded catalog. They are constants so a
// finding can never leak reviewed source text into its message.
const (
	msgHardcodedSecret  = "Hardcoded secret or credential assigned inline"
	msgCommandInjection = "Command built by string concatenation"
	msgFilePermissions  = "File permissions set explicitly; prefer restrictive modes"
	msgSQLInjection     = "SQL query built by string concatenation"
)

// rule is one entry of the embedded deterministic catalog. Most rules are a
// hand-audited regular expression; RuleFilePermissions instead uses checkFn,
// a small argument-aware parser, because "is this call's last argument a
// world-writable mode" cannot be answered by a regex without also matching
// unrelated digits in a filename or an earlier argument.
type rule struct {
	id       string
	severity string
	message  string
	re       *regexp.Regexp
	checkFn  func(string) bool
}

// embeddedRules is the fixed, zero-config catalog. Every pattern or checkFn
// is hardcoded and hand-audited; there is no way to add or remove a rule
// without editing this source, which is what keeps the pass deterministic
// and configuration-free. A broken regex panics at package init
// (MustCompile), matching the project's "fail loudly, never silently empty"
// rule for matchers.
var embeddedRules = []rule{
	{
		id:       RuleHardcodedSecret,
		severity: "error",
		message:  msgHardcodedSecret,
		// AUR-489/AC-002: the keyword must sit at the END of the
		// identifier (an optional plural "s" allowed, e.g. "apiKeys"),
		// preceded by an optional camelCase/snake_case prefix of letters
		// and digits only ("dbPassword", "admin_token", "userSecret").
		// Requiring \b immediately before AND after the keyword (as the
		// prior pattern did) matched a bare "password" but missed every
		// prefixed identifier, since there is no word boundary between
		// two letters/digits ("dbPassword" has none between "b" and
		// "P"). The prefix charclass deliberately excludes "_", so a
		// keyword stuck to the END of a longer word by an underscore
		// ("access_token_expiry") still cannot match: \b right after the
		// keyword requires a non-word character there, and "_" is a word
		// character, same as a letter. The value must be 8+ characters,
		// so a placeholder like `token = "x"` or an empty `secret = ""`
		// is not flagged. match() additionally rejects a hit whose
		// keyword starts inside an already-open string or raw-string
		// literal (AUR-496/AC-002), so a documentation example quoted or
		// backtick-quoted around the whole assignment is not mistaken
		// for a real one.
		re: regexp.MustCompile(`(?i)\b(?:[a-z][a-z0-9]*)?(?:api[_-]?key|secret|password|passwd|token|credential|access[_-]?key)s?\b\s*[:=]=?\s*["'][^"'\r\n]{8,}["']`),
	},
	{
		id:       RuleCommandInjection,
		severity: "error",
		message:  msgCommandInjection,
		re:       regexp.MustCompile(`(?i)\b(system|popen|exec[lv]p?e?|exec(Sync)?|subprocess\.(run|call|Popen))\s*\(.*["']\s*\+`),
	},
	{
		id:       RuleFilePermissions,
		severity: "warning",
		message:  msgFilePermissions,
		checkFn:  matchesWorldWritablePermission,
	},
	{
		id:       RuleSQLInjection,
		severity: "error",
		message:  msgSQLInjection,
		re:       regexp.MustCompile(`(?i)\b(select|insert|update|delete)\b[^+]*["']\s*\+\s*[A-Za-z_$][\w$]*`),
	},
}

// filePermCallRe locates the start of a call to one of the file-permission
// API family; matchesWorldWritablePermission then parses that call's actual
// argument list instead of scanning the whole line for a digit.
var filePermCallRe = regexp.MustCompile(`\bos\.(?:OpenFile|WriteFile|Chmod|Mkdir|MkdirAll)\s*\(`)

// modeLiteralRe matches a full octal mode literal (0NNN or 0oNNN) and
// nothing else, so it is only applied to an already-isolated argument, never
// to a substring of a longer token.
var modeLiteralRe = regexp.MustCompile(`^0o?[0-7]{3}$`)

// matchesWorldWritablePermission reports whether body contains a call to the
// embedded file-permission API family whose LAST argument (always the mode
// in this API family) grants write access to "other" (0600/0644/0700 stay
// clear; 0666/0777/0o777 flag). Locating the call's own argument list with
// splitTopLevelArgs, instead of scanning the whole line for a digit, is what
// keeps a digit inside a filename string, inside a nested call, or in an
// earlier argument from ever being mistaken for the mode.
func matchesWorldWritablePermission(body string) bool {
	for _, loc := range filePermCallRe.FindAllStringIndex(body, -1) {
		args, ok := splitTopLevelArgs(body, loc[1])
		if !ok || len(args) == 0 {
			continue
		}
		last := strings.TrimSpace(args[len(args)-1])
		if isWorldWritableMode(last) {
			return true
		}
	}
	return false
}

// isWorldWritableMode reports whether arg is exactly an octal mode literal
// whose "other" digit (the last one) has the write bit set.
func isWorldWritableMode(arg string) bool {
	if !modeLiteralRe.MatchString(arg) {
		return false
	}
	switch arg[len(arg)-1] {
	case '2', '3', '6', '7':
		return true
	default:
		return false
	}
}

// splitTopLevelArgs splits a call's argument list into its top-level
// arguments, given the index of the first character after the call's
// opening "(". It tracks nested (), [], {} and quoted/raw string literals
// (respecting backslash escapes in quoted, not raw, strings) so a comma or
// digit inside a nested call or a string value is never treated as a
// top-level argument boundary. ok is false if the call's closing ")" is
// never found (malformed or truncated input).
func splitTopLevelArgs(s string, start int) (args []string, ok bool) {
	depth := 1
	argStart := start
	i := start
	for i < len(s) {
		c := s[i]
		switch c {
		case '"', '\'':
			quote := c
			i++
			for i < len(s) && s[i] != quote {
				if s[i] == '\\' && i+1 < len(s) {
					i++
				}
				i++
			}
		case '`':
			i++
			for i < len(s) && s[i] != '`' {
				i++
			}
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
			if depth == 0 {
				args = append(args, s[argStart:i])
				return args, true
			}
		case ',':
			if depth == 1 {
				args = append(args, s[argStart:i])
				argStart = i + 1
			}
		}
		i++
	}
	return nil, false
}

// isInsideStringLiteral reports whether pos in body falls inside an already
// open quoted or raw string literal that starts before pos, by scanning
// body[:pos] and tracking quote/backtick state (respecting backslash
// escapes in quoted, not raw, strings). It is used to reject a
// hardcoded-secret match whose keyword itself is embedded inside a
// documentation string or backtick literal, e.g. a line that is itself a
// string constant showing an example assignment, rather than a real one.
func isInsideStringLiteral(body string, pos int) bool {
	inBacktick := false
	var inQuote byte
	for i := 0; i < pos && i < len(body); i++ {
		c := body[i]
		switch {
		case inBacktick:
			if c == '`' {
				inBacktick = false
			}
		case inQuote != 0:
			if c == '\\' {
				i++
				continue
			}
			if c == inQuote {
				inQuote = 0
			}
		case c == '`':
			inBacktick = true
		case c == '"' || c == '\'':
			inQuote = c
		}
	}
	return inBacktick || inQuote != 0
}

// Runner applies the embedded deterministic analysis to a diff and, when
// asked, drives an external "go vet" through an injected commandRunner. A
// Runner is immutable after construction and therefore safe to reuse across
// calls and goroutines: Analyze and Vet never mutate it.
type Runner struct {
	rules []rule
}

// NewRunner returns a Runner configured with the embedded catalog only and
// no external commands. It needs no configuration, no filesystem access and
// no network; Analyze works entirely in-process.
func NewRunner() *Runner {
	return &Runner{rules: embeddedRules}
}

// Analyze scans every added (RIGHT) line of diff against the embedded
// catalog and returns a Finding for each match. Removed (LEFT) lines are
// never scanned by this deterministic pass (AUR-496/AC-001): removing a
// secret must not itself produce a finding, though a model pass may still
// flag the removed protection using LEFT evidence, which is a different
// capability from this one. Findings are returned in a deterministic order
// (path, then line, then side, then rule id) and every message is a fixed
// catalog string, never the matched source.
func (r *Runner) Analyze(diff *types.Diff) []Finding {
	if diff == nil {
		return nil
	}

	var findings []Finding
	for _, file := range diff.Files {
		for _, hunk := range file.Hunks {
			newLine := hunk.NewStart
			oldLine := hunk.OldStart
			for _, raw := range hunk.Lines {
				if raw == "" {
					continue
				}
				marker, body := splitDiffMarker(raw)
				switch marker {
				case "+":
					findings = append(findings, r.match(file.Path, newLine, SideRight, body)...)
					newLine++
				case "-":
					oldLine++
				case " ":
					newLine++
					oldLine++
				}
			}
		}
	}

	sortFindings(findings)
	return findings
}

// match returns the catalog rules whose pattern matches body, each rendered
// as a Finding at the given path, line and side. A comment or documentation
// line never matches (isCommentOrDocLine), and a RuleHardcodedSecret hit
// whose keyword starts inside an already-open string or raw-string literal
// is rejected (isInsideStringLiteral), so an example assignment quoted in
// prose is never treated as a real one.
func (r *Runner) match(path string, line int, side, body string) []Finding {
	if isCommentOrDocLine(body) {
		return nil
	}
	var out []Finding
	for _, rule := range r.rules {
		matched := false
		switch {
		case rule.checkFn != nil:
			matched = rule.checkFn(body)
		case rule.id == RuleHardcodedSecret:
			if loc := rule.re.FindStringIndex(body); loc != nil && !isInsideStringLiteral(body, loc[0]) {
				matched = true
			}
		case rule.re != nil:
			matched = rule.re.MatchString(body)
		}
		if matched {
			out = append(out, Finding{
				Path:     path,
				Line:     line,
				Side:     side,
				RuleID:   rule.id,
				Severity: rule.severity,
				Message:  rule.message,
			})
		}
	}
	return out
}

// splitDiffMarker splits a unified-diff line into its one-character
// +/-/space marker and the body the matchers should see. A line without a
// marker is all body and therefore never matched, since Analyze only
// dispatches on an explicit marker.
func splitDiffMarker(line string) (marker, body string) {
	if line == "" {
		return "", ""
	}
	switch line[0] {
	case '+', '-', ' ':
		return line[:1], line[1:]
	}
	return "", line
}

// isCommentOrDocLine reports whether body is a Go line or block-comment
// opener, so an example credential or permission literal in prose is never
// treated as a real assignment. It deliberately does NOT treat a line
// starting with "*" as a comment continuation: that would also catch a
// pointer dereference assignment like `*password = "..."`, which is real
// code, not a comment (AUR-496/AC-002).
func isCommentOrDocLine(body string) bool {
	trimmed := strings.TrimSpace(body)
	return strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "/*")
}

// sortFindings orders findings deterministically by path, line, side, then
// rule id, so repeated runs and the merged report are stable.
func sortFindings(fs []Finding) {
	sort.SliceStable(fs, func(i, j int) bool {
		if fs[i].Path != fs[j].Path {
			return fs[i].Path < fs[j].Path
		}
		if fs[i].Line != fs[j].Line {
			return fs[i].Line < fs[j].Line
		}
		if fs[i].Side != fs[j].Side {
			return fs[i].Side < fs[j].Side
		}
		return fs[i].RuleID < fs[j].RuleID
	})
}
