// Commit-status descriptions: GitHub caps a status description, so the
// policy gate's description is shortened here without losing its verdict.
package main

import (
	"strings"
	"unicode/utf8"

	"github.com/Mpaape/AurumCode/internal/gate/reasons"
)

// statusDescriptionLimit is GitHub's own hard limit on a commit status's
// "description" field (the Create a Commit Status API): 140 characters.
// A longer string is rejected by GitHub itself, not merely clipped there,
// so this program truncates first rather than let SetStatus fail on a
// long rule title, owner name or reason string. The review body and
// stderr limitations list (built from gateDecision.Lines directly, in
// main.go/pr.go) carry the full, uncapped account; only this one
// published field is bounded.
const statusDescriptionLimit = 140

// gateStatusWordFailure/Inconclusive/Approved are the three fixed,
// non-model-authored Portuguese words capStatusDescription always leads
// with (AC-007): a status reader can tell the outcome at a glance even
// when the rest of the description had to be cut to fit the cap.
const (
	gateStatusWordFailure      = "falha"
	gateStatusWordInconclusive = "inconclusivo"
	gateStatusWordApproved     = "aprovado"
)

// capStatusDescription returns "resultWord: detail" (or just resultWord
// when detail is blank), truncated to at most limit RUNES -- never bytes,
// since a byte cut could split a multi-byte rune (an accented pt-BR
// letter, for instance) in half and corrupt the string GitHub stores.
// resultWord itself is always short and fixed (see the three constants
// above) and is never the part cut: only detail is ellipsized, with a
// single "…" rune replacing whatever had to be dropped, so a reader can
// always tell the description was shortened rather than reading a
// silently incomplete sentence as if it were the whole story.
func capStatusDescription(resultWord, detail string, limit int) string {
	full := resultWord
	if strings.TrimSpace(detail) != "" {
		full = resultWord + ": " + detail
	}
	if utf8.RuneCountInString(full) <= limit {
		return full
	}
	runes := []rune(full)
	if limit <= 1 {
		return string(runes[:limit])
	}
	return string(runes[:limit-1]) + "…"
}

// orderedGateReasons joins gateDecision.Lines for the capped commit-status
// description only (never for the uncapped review body/stderr list built
// from gateResult.Lines directly elsewhere, whose order this never
// touches), in three tiers so the most actionable fact gets the best
// chance of surviving statusDescriptionLimit's cut:
//
//  1. a real severity-breach line (evaluateGate's threshold loop,
//     policygate.go) -- the one fact that actually closes the gate;
//  2. an accepted/expired-exception line (acceptedExceptionLine/
//     expiredExceptionLine, aur520.go -- matched by their own fixed
//     acceptedExceptionMarker/expiredExceptionMarker substrings, never by
//     the bare word "exceção", so a breach line whose policy/repo-
//     authored rule.Title happens to mention "exceção" is never
//     misclassified into this tier) -- context a reader may want, but
//     never the reason the check failed;
//  3. the fixed "review inconclusive (...)" line -- already named by the
//     status's own State and leading result word
//     (gateStatusWordFailure/Inconclusive), so it is the one most
//     affordable to lose first.
//
// Without this ordering, a run with both a real breach AND one or more
// accepted-exception lines could have the exception lines alone exhaust
// the 140-character budget and push the breach itself out of the
// description entirely -- exactly as affordable to lose as the
// inconclusive reason, but reordering it ahead of a real breach would be
// wrong in the same way.
func orderedGateReasons(lines []string) string {
	return orderedGateReasonsIn("", lines)
}

// orderedGateReasonsIn is orderedGateReasons for lines in language: the
// inconclusive line is recognized by the catalog's own template
// (reasons.IsLine), never by its English words.
func orderedGateReasonsIn(language string, lines []string) string {
	breach := make([]string, 0, len(lines))
	exception := make([]string, 0, len(lines))
	inconclusive := make([]string, 0, len(lines))
	for _, line := range lines {
		switch {
		case reasons.IsLine(language, line):
			inconclusive = append(inconclusive, line)
		case strings.Contains(line, acceptedExceptionMarker) || strings.Contains(line, expiredExceptionMarker):
			exception = append(exception, line)
		default:
			breach = append(breach, line)
		}
	}
	ordered := append(append(breach, exception...), inconclusive...)
	return strings.Join(ordered, "; ")
}
