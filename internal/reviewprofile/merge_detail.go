// The detail of a finding two or more profiles agree on: the merge keeps
// the earliest profile's occurrence, records who else reported it and
// never lets a later occurrence's evidence disappear.
package reviewprofile

import "strings"

// sideRight is the diff side an empty Side means.
const sideRight = "RIGHT"

// sideOf normalizes a diff side for identity and order: empty is RIGHT, so
// a finding that omits the side and one that names RIGHT are one line.
func sideOf(side string) string {
	side = strings.ToUpper(strings.TrimSpace(side))
	if side == "" {
		return sideRight
	}
	return side
}

// mergeDuplicate folds a later occurrence (dup) of the same finding into
// the kept one. The kept occurrence's Profile, Severity, Ref and non-empty
// detail never change; dup's profile joins AlsoFrom (once, in arrival
// order), an empty detail field is filled from dup, and a different
// non-empty Evidence is appended so no profile's evidence is lost.
func mergeDuplicate(kept, dup Finding) Finding {
	if dup.Profile != "" && dup.Profile != kept.Profile && !containsName(kept.AlsoFrom, dup.Profile) {
		kept.AlsoFrom = append(kept.AlsoFrom, dup.Profile)
	}
	kept.Impact = firstNonEmpty(kept.Impact, dup.Impact)
	kept.Suggestion = firstNonEmpty(kept.Suggestion, dup.Suggestion)
	kept.Verification = firstNonEmpty(kept.Verification, dup.Verification)
	kept.Evidence = joinEvidence(kept.Evidence, dup.Evidence)
	return kept
}

// evidenceSeparator joins distinct evidence of the same finding.
const evidenceSeparator = "\n"

// joinEvidence keeps kept's evidence and appends extra when it is
// non-empty and not already present.
func joinEvidence(kept, extra string) string {
	extra = strings.TrimSpace(extra)
	switch {
	case extra == "":
		return kept
	case strings.TrimSpace(kept) == "":
		return extra
	case strings.Contains(kept, extra):
		return kept
	default:
		return kept + evidenceSeparator + extra
	}
}

func firstNonEmpty(kept, other string) string {
	if strings.TrimSpace(kept) != "" {
		return kept
	}
	return other
}

func containsName(names []string, name string) bool {
	for _, n := range names {
		if n == name {
			return true
		}
	}
	return false
}
