package changelog

import (
	"regexp"
	"strings"
)

// Commit is one reviewed commit message. Only Subject and Body are inspected;
// Hash is optional and, when set, is rendered as a bounded reference. Every
// field is untrusted input.
type Commit struct {
	Subject string
	Body    string
	Hash    string
}

// Kind is a Conventional Commit type recognized by this engine. A subject
// whose type is absent from the recognized set is residual (KindUnknown) and
// never counts as a breaking change on its own.
type Kind string

const (
	KindFeat     Kind = "feat"
	KindFix      Kind = "fix"
	KindPerf     Kind = "perf"
	KindRefactor Kind = "refactor"
	KindDocs     Kind = "docs"
	KindChore    Kind = "chore"
	KindTest     Kind = "test"
	KindBuild    Kind = "build"
	KindCI       Kind = "ci"
	KindStyle    Kind = "style"
	KindUnknown  Kind = "unknown"
)

// recognizedKinds is the fixed Conventional Commit type catalog. A type outside
// this catalog is residual, not an extension point.
var recognizedKinds = map[Kind]struct{}{
	KindFeat:     {},
	KindFix:      {},
	KindPerf:     {},
	KindRefactor: {},
	KindDocs:     {},
	KindChore:    {},
	KindTest:     {},
	KindBuild:    {},
	KindCI:       {},
	KindStyle:    {},
}

// subjectRE matches "type(scope)!: description". The scope is optional and may
// not contain parentheses; the bang is optional. The description may be empty.
var subjectRE = regexp.MustCompile(`^([a-z]+)(?:\(([^()]*)\))?(!)?: (.*)$`)

const (
	breakingSpace = "BREAKING CHANGE:"
	breakingDash  = "BREAKING-CHANGE:"
)

// Classification is the result of parsing one commit against the Conventional
// Commits contract. Subject and Hash are the raw, untrusted values; callers
// that render them MUST escape them first.
type Classification struct {
	Kind         Kind
	Scope        string
	Description  string
	Breaking     bool
	Conventional bool
	Subject      string
	Hash         string
}

// Classify parses a single commit. A subject that does not match the
// Conventional Commits grammar (or whose type is not recognized) is residual:
// Conventional is false, Kind is KindUnknown and the subject alone never marks
// the commit breaking. An explicit BREAKING CHANGE: footer in the body is
// always honored, because it is a deliberate machine-readable declaration.
func Classify(c Commit) Classification {
	cl := Classification{Subject: c.Subject, Hash: c.Hash, Kind: KindUnknown}

	bang := false
	if m := subjectRE.FindStringSubmatch(c.Subject); m != nil {
		kind := Kind(m[1])
		if _, ok := recognizedKinds[kind]; ok {
			cl.Conventional = true
			cl.Kind = kind
			cl.Scope = m[2]
			cl.Description = m[4]
			bang = m[3] == "!"
		}
	}

	cl.Breaking = bang || hasBreakingFooter(c.Body)
	return cl
}

// ClassifyCommits parses every commit: none is ignored, so the version bump
// and the entry always see the whole range.
func ClassifyCommits(commits []Commit) []Classification {
	out := make([]Classification, 0, len(commits))
	for _, c := range commits {
		out = append(out, Classify(c))
	}
	return out
}

// hasBreakingFooter reports whether body contains a BREAKING CHANGE: or
// BREAKING-CHANGE: footer on a line of its own.
func hasBreakingFooter(body string) bool {
	if body == "" {
		return false
	}
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.HasPrefix(line, breakingSpace) || strings.HasPrefix(line, breakingDash) {
			return true
		}
	}
	return false
}
