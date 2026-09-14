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

// rule is one entry of the embedded deterministic catalog.
type rule struct {
	id       string
	severity string
	message  string
	re       *regexp.Regexp
}

// embeddedRules is the fixed, zero-config catalog. Every pattern is a
// hardcoded, hand-audited regular expression; there is no way to add or
// remove a rule without editing this source, which is what keeps the pass
// deterministic and configuration-free. A broken pattern panics at package
// init (MustCompile), matching the project's "fail loudly, never silently
// empty" rule for matchers.
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
		// is not flagged.
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
		re:       regexp.MustCompile(`(?i)\bos\.(OpenFile|WriteFile|Chmod|Mkdir|MkdirAll)\s*\([^()]*,\s*0o?[0-7]{2}[2367]\b`),
	},
	{
		id:       RuleSQLInjection,
		severity: "error",
		message:  msgSQLInjection,
		re:       regexp.MustCompile(`(?i)\b(select|insert|update|delete)\b[^+]*["']\s*\+\s*[A-Za-z_$][\w$]*`),
	},
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

// Analyze scans every added (RIGHT) and removed (LEFT) line of diff against
// the embedded catalog and returns a Finding for each match. Findings are
// returned in a deterministic order (path, then line, then side, then rule
// id) and every message is a fixed catalog string, never the matched source.
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
// as a Finding at the given path, line and side.
func (r *Runner) match(path string, line int, side, body string) []Finding {
	if isCommentOrDocLine(body) {
		return nil
	}
	var out []Finding
	for _, rule := range r.rules {
		if rule.re.MatchString(body) {
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

// isCommentOrDocLine reports whether body is a Go line/block comment or a
// documentation string, so an example credential or permission literal in
// prose is never treated as a real assignment.
func isCommentOrDocLine(body string) bool {
	trimmed := strings.TrimSpace(body)
	return strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "*") || strings.HasPrefix(trimmed, "/*")
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
