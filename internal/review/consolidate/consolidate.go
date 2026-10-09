// Package consolidate prepares the findings a review publishes: the same
// problem at the same place reported by two passes is published once, with
// every pass's evidence, and the repository's explicit presentation
// preferences condense non-blocking findings into one explained line. It
// only shapes what is published; the gate and the statuses keep reading the
// review's own findings.
package consolidate

import (
	"strconv"
	"strings"

	"github.com/Mpaape/AurumCode/pkg/types"
)

// Options are the explicit preferences and the run's blocking rule.
type Options struct {
	// Collapse names the severities whose non-blocking findings are not
	// published one by one (review.presentation.collapse).
	Collapse map[string]bool
	// Blocking reports a finding the run blocks on; it is never collapsed.
	Blocking func(types.ReviewIssue) bool
}

// Result is what the review publishes.
type Result struct {
	// Issues are the published findings, in input order of their first
	// occurrence.
	Issues []types.ReviewIssue
	// AlsoFrom is parallel to Issues: the sources of the occurrences merged
	// into each one ("" names the model).
	AlsoFrom [][]string
	// Merged counts the occurrences merged into another.
	Merged int
	// Collapsed are the findings condensed by preference, in order.
	Collapsed []types.ReviewIssue
}

// Apply merges occurrences of the same problem and applies the collapse
// preference. Two occurrences are the same problem when file, line, side
// and rule are equal; a different rule on the same line is another
// problem. The kept occurrence is the first deterministic one (an engine
// origin) or else the first; it takes the highest severity of the group,
// every distinct evidence, and fills an empty impact, suggestion or
// verification from the others. Nothing is dropped by count.
func Apply(issues []types.ReviewIssue, opts Options) Result {
	var res Result
	at := map[string]int{}
	engines := enginesByLocation(issues)
	for _, issue := range issues {
		key := identity(issue)
		if echoed, ok := echoesEngine(issue, engines[location(issue)]); ok {
			// The model's own finding at the place an engine of the same
			// category already reported: one problem, published once
			// under the engine's rule, with the model's text as evidence.
			key = identity(echoed)
		}
		i, seen := at[key]
		if !seen {
			at[key] = len(res.Issues)
			res.Issues = append(res.Issues, issue)
			res.AlsoFrom = append(res.AlsoFrom, nil)
			continue
		}
		res.Merged++
		kept, other := res.Issues[i], issue
		if prefersOther(kept, other, opts.Blocking) {
			kept, other = other, kept
		}
		res.Issues[i] = merge(kept, other)
		res.AlsoFrom[i] = append(res.AlsoFrom[i], other.Origin)
	}
	return collapse(res, opts)
}

func collapse(res Result, opts Options) Result {
	if len(opts.Collapse) == 0 {
		return res
	}
	out := Result{Merged: res.Merged}
	for i, issue := range res.Issues {
		blocking := opts.Blocking == nil || opts.Blocking(issue)
		if !blocking && opts.Collapse[strings.ToLower(strings.TrimSpace(issue.Severity))] {
			out.Collapsed = append(out.Collapsed, issue)
			continue
		}
		out.Issues = append(out.Issues, issue)
		out.AlsoFrom = append(out.AlsoFrom, res.AlsoFrom[i])
	}
	return out
}

// location is the file, line and side of a finding, whatever its rule.
func location(issue types.ReviewIssue) string {
	side := strings.ToUpper(strings.TrimSpace(issue.Side))
	if side == "" {
		side = "RIGHT"
	}
	return strings.Join([]string{issue.File, strconv.Itoa(issue.Line), side}, "\x00")
}

// enginesByLocation indexes the deterministic engines' findings by place.
func enginesByLocation(issues []types.ReviewIssue) map[string][]types.ReviewIssue {
	out := map[string][]types.ReviewIssue{}
	for _, issue := range issues {
		if issue.Origin != "" {
			out[location(issue)] = append(out[location(issue)], issue)
		}
	}
	return out
}

// prefersOther decides which occurrence of one problem is kept: the one
// that blocks the run when only one does (its rule is the gate's key, and
// the parecer must list what blocks under that rule), otherwise the
// deterministic one over the model's.
func prefersOther(kept, other types.ReviewIssue, blocking func(types.ReviewIssue) bool) bool {
	if blocking != nil && blocking(kept) != blocking(other) {
		return blocking(other)
	}
	return kept.Origin == "" && other.Origin != ""
}

// echoesEngine reports the engine finding a model finding repeats: same
// place, the model rule's category (the segment before "/", such as
// "security") named in the engine's rule id, and a word of the model
// rule's name (such as "crypto" of "weak-crypto") named there too. A model
// finding of another category, or of another problem of the same category
// ("path-traversal" beside a weak-crypto scan), stays its own finding.
func echoesEngine(issue types.ReviewIssue, engines []types.ReviewIssue) (types.ReviewIssue, bool) {
	if issue.Origin != "" || issue.Assessment != nil || len(engines) == 0 {
		return types.ReviewIssue{}, false
	}
	category, name, ok := strings.Cut(strings.ToLower(strings.TrimSpace(issue.RuleID)), "/")
	if !ok || category == "" {
		return types.ReviewIssue{}, false
	}
	words := ruleWords(name)
	if len(words) == 0 {
		return types.ReviewIssue{}, false
	}
	for _, engine := range engines {
		id := strings.ToLower(engine.RuleID)
		if !strings.Contains(id, category) {
			continue
		}
		for _, w := range words {
			if strings.Contains(id, w) {
				return engine, true
			}
		}
	}
	return types.ReviewIssue{}, false
}

// ruleWords are the words of a rule name with at least three letters.
func ruleWords(name string) []string {
	var out []string
	for _, w := range strings.FieldsFunc(name, func(r rune) bool { return r == '-' || r == '_' || r == '.' || r == ':' }) {
		if len(w) >= 3 {
			out = append(out, w)
		}
	}
	return out
}

func identity(issue types.ReviewIssue) string {
	side := strings.ToUpper(strings.TrimSpace(issue.Side))
	if side == "" {
		side = "RIGHT"
	}
	return strings.Join([]string{issue.File, strconv.Itoa(issue.Line), side, issue.RuleID}, "\x00")
}

// severityRank orders the severities; an unknown one ranks lowest.
var severityRank = map[string]int{"info": 1, "warning": 2, "error": 3, "critical": 4}

func merge(kept, other types.ReviewIssue) types.ReviewIssue {
	if severityRank[strings.ToLower(other.Severity)] > severityRank[strings.ToLower(kept.Severity)] {
		kept.Severity = other.Severity
	}
	kept.Evidence = joinDistinct(kept.Evidence, other.Evidence)
	kept.Impact = firstNonEmpty(kept.Impact, other.Impact)
	kept.Suggestion = firstNonEmpty(kept.Suggestion, other.Suggestion)
	kept.Verification = firstNonEmpty(kept.Verification, other.Verification)
	if kept.Assessment == nil {
		kept.Assessment = other.Assessment
	}
	return kept
}

func joinDistinct(kept, extra string) string {
	extra = strings.TrimSpace(extra)
	switch {
	case extra == "":
		return kept
	case strings.TrimSpace(kept) == "":
		return extra
	case strings.Contains(kept, extra):
		return kept
	default:
		return kept + "\n" + extra
	}
}

func firstNonEmpty(kept, other string) string {
	if strings.TrimSpace(kept) != "" {
		return kept
	}
	return other
}
