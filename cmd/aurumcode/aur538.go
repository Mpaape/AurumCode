// AUR-538 closes the non-blocking proof gaps the AUR-519/520 reviews left
// open: AC-007's own commit-status description cap lives here, next to
// (but not inside) policygate.go's publishPolicyGateStatus, which AUR-521
// is editing concurrently for its own audit/SARIF work -- this file only
// adds the two small helpers that function's Description branch calls.
package main

import (
	"strings"
	"unicode/utf8"
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
// touches): every line that is NOT the fixed "review inconclusive (...)"
// prefix -- an accepted/expired-exception line or a real severity-breach
// line -- is joined first, then every inconclusive-reason line. A real
// finding is the more actionable fact, so it gets the best chance of
// surviving statusDescriptionLimit's cut; the inconclusive reason, which
// is already named by the status's own State/word, is the one most
// affordable to lose first.
func orderedGateReasons(lines []string) string {
	primary := make([]string, 0, len(lines))
	secondary := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.HasPrefix(line, "review inconclusive (") {
			secondary = append(secondary, line)
			continue
		}
		primary = append(primary, line)
	}
	return strings.Join(append(primary, secondary...), "; ")
}
