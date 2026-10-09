package analysis

import (
	"path"
	"strings"
)

// A Python triple-quoted string (the docstring idiom) spans lines, and a
// line inside it is text even though, read alone, it looks like code. The
// line-local literal guard of RuleCommandInjection cannot see that, so a
// docstring that documents `subprocess.call("rm " + name)` on its own line
// would still read as the call. The state below is threaded per Python file
// and consulted only by that guard; the secret rule and stripComments keep
// their own, unchanged state.

// tracksDocstrings reports whether path is a Python source, the one language
// whose triple quotes this file tracks. Elsewhere `"""` is not a string
// delimiter, and treating it as one could hide a real call.
func tracksDocstrings(p string) bool {
	switch strings.ToLower(path.Ext(p)) {
	case ".py", ".pyi":
		return true
	}
	return false
}

// scanDocstring scans one raw Python line that is entered with open (the
// triple delimiter still open from an earlier line, or ""). end is the byte
// offset where code resumes after that entering string closes: 0 when the
// line does not start inside one, len(body) when the string does not close
// on this line. next is the delimiter left open when the line ends. Quoted
// single-line strings and a `#` comment are skipped, so a `"""` inside them
// never toggles the state.
func scanDocstring(body, open string) (end int, next string) {
	i := 0
	if open != "" {
		idx := closingTriple(body, 0, open)
		if idx < 0 {
			return len(body), open
		}
		end = idx + len(open)
		i = end
	}
	for i < len(body) {
		c := body[i]
		switch {
		case c == '#':
			return end, ""
		case c == '\\':
			i += 2
		case c == '"' || c == '\'':
			delim := strings.Repeat(string(c), 3)
			if !strings.HasPrefix(body[i:], delim) {
				i = skipQuoted(body, i)
				continue
			}
			idx := closingTriple(body, i+len(delim), delim)
			if idx < 0 {
				return end, delim
			}
			i = idx + len(delim)
		default:
			i++
		}
	}
	return end, ""
}

// closingTriple returns the index of the first unescaped delim in body at or
// after from, or -1.
func closingTriple(body string, from int, delim string) int {
	for i := from; i+len(delim) <= len(body); i++ {
		if body[i] == '\\' {
			i++
			continue
		}
		if body[i:i+len(delim)] == delim {
			return i
		}
	}
	return -1
}

// skipQuoted returns the index just past the single-line string that opens
// at body[i], or len(body) when it does not close.
func skipQuoted(body string, i int) int {
	quote := body[i]
	for j := i + 1; j < len(body); j++ {
		switch body[j] {
		case '\\':
			j++
		case quote:
			return j + 1
		}
	}
	return len(body)
}

// dropDocstringMentions removes, from the findings of one line that starts
// inside a docstring, each finding of a literal-guarded rule that does not
// also match the code after the docstring closes (body[end:]). Findings of
// every other rule, the secret rule included, are kept unchanged.
func (r *Runner) dropDocstringMentions(found []Finding, body string, end int) []Finding {
	tail := ""
	if end < len(body) {
		tail, _, _ = stripComments(body[end:], false, false)
	}
	kept := found[:0]
	for _, f := range found {
		if rl, ok := r.ruleByID(f.RuleID); ok && rl.outsideLiteral && !rl.matches(tail, false) {
			continue
		}
		kept = append(kept, f)
	}
	return kept
}

// ruleByID returns the catalog rule with the given id.
func (r *Runner) ruleByID(id string) (rule, bool) {
	for _, rl := range r.rules {
		if rl.id == id {
			return rl, true
		}
	}
	return rule{}, false
}
