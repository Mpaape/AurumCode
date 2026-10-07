// multi.go implements MULTI-AGENT review: two or more profiles run in the
// same review, and their findings are merged deterministically into one
// report where every finding names the profile that produced it.
//
// A finding is a value, never an instruction. MergeFindings is a pure
// function: given the same findings it returns the same bytes, in a stable
// order, with no duplicate, regardless of the order the passes finished in.
// Attribution is the FIRST profile (in declaration order) that produced a
// finding, so a finding two profiles agree on is reported once and does not
// depend on scheduling.
//
// Profile boundaries are untouched by running more than one: each resolved
// profile carries the same fixed Effective preset a single-profile run
// carries, and ResolveAll refuses an unknown name before any model call.
package reviewprofile

import (
	"fmt"
	"sort"
	"strings"
)

// Finding is one review finding attributed to the profile that produced it.
// It is deliberately independent of pkg/types so the merge contract can be
// proven in isolation; the caller maps its own findings onto it.
type Finding struct {
	// Profile is the name of the profile that produced the finding.
	Profile string
	// RuleID, File, Line and Message identify the finding's content.
	RuleID  string
	File    string
	Line    int
	Message string
	// Severity is carried verbatim; a profile never rewrites it.
	Severity string
	// Side is the diff side of Line (LEFT for a removed line); empty is
	// RIGHT. It is part of the identity: the same line number on the two
	// sides is two different lines.
	Side string
	// Impact, Evidence, Suggestion and Verification are the finding's
	// actionable detail. They never decide identity; the merge keeps them
	// (see mergeDuplicate) so a combined run publishes what a single
	// profile would have.
	Impact       string
	Evidence     string
	Suggestion   string
	Verification string
	// Ref is the caller's opaque handle to its own record of the finding;
	// the merge carries the kept occurrence's Ref verbatim.
	Ref int
	// AlsoFrom names, in declaration order, every later profile that
	// reported the same finding; Profile stays the first one.
	AlsoFrom []string
}

// identity is the content key a duplicate shares. It deliberately excludes
// Profile: when two profiles report the same defect it is one finding.
func (f Finding) identity() string {
	return strings.Join([]string{
		f.RuleID,
		f.File,
		fmt.Sprintf("%d", f.Line),
		f.Message,
		sideOf(f.Side),
	}, "\x00")
}

// orderKey is the deterministic output order: file, then line, then rule,
// then message. Profile is only a tiebreaker for otherwise identical keys,
// which the dedup above already collapses.
func (f Finding) orderKey() string {
	return strings.Join([]string{
		strings.ToLower(f.File),
		fmt.Sprintf("%09d", f.Line),
		strings.ToLower(f.RuleID),
		f.Message,
		sideOf(f.Side),
	}, "\x00")
}

// MergeFindings merges findings from every profile into one deterministic
// list: duplicates collapse to the first occurrence (the earliest profile in
// declaration order), every finding keeps its source Profile, a duplicate's
// profile is recorded in AlsoFrom and its detail fills what the kept
// occurrence lacked (mergeDuplicate), and the result is sorted by a stable
// content key. The input slice is not modified.
func MergeFindings(in []Finding) []Finding {
	if len(in) == 0 {
		return nil
	}
	seen := map[string]int{}
	out := make([]Finding, 0, len(in))
	for _, f := range in {
		key := f.identity()
		if at, dup := seen[key]; dup {
			out[at] = mergeDuplicate(out[at], f)
			continue
		}
		seen[key] = len(out)
		f.AlsoFrom = append([]string(nil), f.AlsoFrom...)
		out = append(out, f)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].orderKey() < out[j].orderKey()
	})
	return out
}

// MultiResult is a resolved multi-profile selection. Applied is false for the
// zero-config case, where the caller keeps today's behavior. Declared names
// every profile that entered, in declaration order.
type MultiResult struct {
	Applied  bool
	Profiles []Profile
	Names    []string
	Declared string
}

// BoundariesFixed reports whether every resolved profile leaves the five
// boundaries untouched: security pass on, redaction on, and severity,
// --fail-on and the cost cap unset. A profile system that ever returned
// false here has failed AC-003.
func (m *MultiResult) BoundariesFixed() bool {
	if m == nil {
		return true
	}
	for _, p := range m.Profiles {
		e := p.Effective
		if !e.SecurityPassEnabled || !e.RedactionEnabled {
			return false
		}
		if e.SeverityFloor != "" || e.FailOnThreshold != "" || e.CostCap != -1 {
			return false
		}
	}
	return true
}

// ResolveAll turns a selection of one or more names into a MultiResult. An
// empty selection is zero-config (Applied=false). A name that resolves to
// neither a built-in nor a team profile is a named UnknownProfileError
// returned BEFORE any model call. An empty or duplicate entry is refused
// naming the problem. The declared output names every profile that entered.
func ResolveAll(sel Selection, team *Team) (*MultiResult, error) {
	names := sel.SelectedNames()
	if len(names) == 0 {
		return &MultiResult{Declared: "profiles: none (zero-config)"}, nil
	}
	seen := map[string]bool{}
	profiles := make([]Profile, 0, len(names))
	entered := make([]string, 0, len(names))
	for _, raw := range names {
		name := strings.TrimSpace(raw)
		if name == "" {
			return nil, fmt.Errorf("%w: an empty profile name was selected", ErrEmptyProfile)
		}
		key := strings.ToLower(name)
		if seen[key] {
			return nil, fmt.Errorf("%w: profile %q selected more than once", ErrDuplicateProfile, name)
		}
		seen[key] = true
		p, ok := Builtin(name)
		if !ok {
			if tp, teamOK := team.Lookup(name); teamOK {
				p, ok = tp, true
			}
		}
		if !ok {
			return nil, &UnknownProfileError{Name: name}
		}
		profiles = append(profiles, p)
		entered = append(entered, p.Name)
	}
	return &MultiResult{
		Applied:  true,
		Profiles: profiles,
		Names:    entered,
		Declared: fmt.Sprintf("profiles: %s", strings.Join(entered, ", ")),
	}, nil
}
