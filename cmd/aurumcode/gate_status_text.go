// The policy-gate commit status's state and description, per language.
// English keeps the description of earlier releases byte for
// byte; Portuguese drops the pull request number the page already shows,
// names the first finding that failed the gate, gives each inconclusive
// reason as a sentence with its code in brackets, and cuts at a word.
package main

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/Mpaape/AurumCode/internal/gate/reasons"
	"github.com/Mpaape/AurumCode/internal/i18n"
)

// minTitleRunes is the shortest finding title worth keeping in a capped
// description; with less room the whole description is cut instead.
const minTitleRunes = 12

// gateStatusDescription returns the policy-gate status's state and its
// capped description in language.
func gateStatusDescription(language string, g gateDecision, prNumber int) (state, description string) {
	state = "success"
	if g.Breach || g.Fail {
		state = "failure"
	}
	if i18n.LocaleOf(language) == i18n.English {
		return state, legacyGateStatusDescription(g, prNumber)
	}
	return state, localizedGateStatusDescription(language, g)
}

// legacyGateStatusDescription is the description of earlier releases.
// AUR-538 AC-007: reasons here feeds only the CAPPED commit-status
// description (capStatusDescription below), never gateResult.Lines itself
// or the review body/stderr limitations list built from it elsewhere
// (main.go/pr.go) -- those stay the full, untruncated account.
// orderedGateReasons puts a real finding ahead of the inconclusive-reason
// line within that cap, so a long rule title still has the best chance of
// surviving the 140-character cut.
func legacyGateStatusDescription(g gateDecision, prNumber int) string {
	ordered := orderedGateReasons(g.Lines)
	var word, detail string
	switch {
	case g.Breach && g.Inconclusive:
		word = gateStatusWordFailure
		detail = fmt.Sprintf("achado(s) reprovam o gate numa revisão também inconclusiva no pull request #%d: %s", prNumber, ordered)
	case g.Breach:
		word = gateStatusWordFailure
		detail = fmt.Sprintf("achado(s) reprovam o gate no pull request #%d: %s", prNumber, ordered)
	case g.Fail:
		// Fail without Breach: gate.inconclusive: block fired and the
		// threshold loop never ran -- this run was never graded at all.
		word = gateStatusWordInconclusive
		detail = fmt.Sprintf("revisão inconclusiva (bloqueio) no pull request #%d: %s", prNumber, ordered)
	case g.Inconclusive:
		// B5: fail_on declared (or not) with no breach found, but the
		// review itself was inconclusive -- never "aprovado".
		word = gateStatusWordInconclusive
		detail = fmt.Sprintf("revisão inconclusiva (alerta) no pull request #%d: %s", prNumber, ordered)
	default:
		word = gateStatusWordApproved
		detail = fmt.Sprintf("gate de política aprovado no pull request #%d", prNumber)
	}
	return capStatusDescription(word, detail, statusDescriptionLimit)
}

// localizedGateStatusDescription is the description in a catalog language
// other than English.
func localizedGateStatusDescription(language string, g gateDecision) string {
	switch {
	case g.Breach:
		return breachStatusDescription(language, g)
	case g.Fail:
		return inconclusiveStatusDescription(language, "gate.status.block", g)
	case g.Inconclusive:
		return inconclusiveStatusDescription(language, "gate.status.warn", g)
	}
	return gateStatusWordApproved + ": " + i18n.Text(language, "gate.status.approved")
}

// inconclusiveStatusDescription says why the run did not conclude: each
// reason as a sentence with its code (reasons.List); when the sentences do
// not fit the cap, the codes alone in brackets, so support can still read
// every one; the gate's lines when no code was recorded.
func inconclusiveStatusDescription(language, key string, g gateDecision) string {
	head := gateStatusWordInconclusive + ": "
	if strings.TrimSpace(g.Reason) == "" {
		return capAtWord(head+i18n.Format(language, key, strings.Join(g.Lines, "; ")), statusDescriptionLimit)
	}
	full := head + i18n.Format(language, key, reasons.List(language, g.Reason))
	if utf8.RuneCountInString(full) <= statusDescriptionLimit {
		return full
	}
	codes := strings.Split(g.Reason, ",")
	for i, code := range codes {
		codes[i] = "[" + strings.TrimSpace(code) + "]"
	}
	return capAtWord(head+i18n.Format(language, key, strings.Join(codes, i18n.Text(language, "gate.reason_list_separator"))), statusDescriptionLimit)
}

// breachStatusDescription names how many findings failed the gate and the
// first of them: title, severity and place. The title is what gives way to
// the cap, so the place and the pointer to the review survive it.
func breachStatusDescription(language string, g gateDecision) string {
	n := len(g.BlockingFindings)
	if n == 0 {
		n = 1
	}
	b := g.FirstBreach
	if b == nil {
		// A breach without a finding to name (dependencies, Dependency-Track):
		// its lines, the breach ahead of the inconclusive one.
		return capAtWord(gateStatusWordFailure+": "+i18n.Format(language, "gate.status.breach_lines", orderedGateReasonsIn(language, g.Lines)), statusDescriptionLimit)
	}
	head := gateStatusWordFailure + ": " + countText(n, i18n.Text(language, "gate.status.breach_one"), i18n.Text(language, "gate.status.breach"))
	tail := " (" + b.Severity + ")"
	if place := breachPlace(b.Path, b.Line); place != "" {
		tail += i18n.Format(language, "gate.status.place", place)
	}
	if g.Inconclusive {
		tail += i18n.Text(language, "gate.status.also_inconclusive")
	}
	tail += i18n.Text(language, "gate.status.details")
	title := oneLine(b.Title)
	room := statusDescriptionLimit - utf8.RuneCountInString(head) - utf8.RuneCountInString(tail)
	if utf8.RuneCountInString(title) > room {
		if room < minTitleRunes {
			return capAtWord(head+title+tail, statusDescriptionLimit)
		}
		title = capAtWord(title, room)
	}
	return head + title + tail
}

// breachPlace is "path:line", "path", or "" when the finding has no file.
func breachPlace(path string, line int) string {
	switch {
	case path == "":
		return ""
	case line > 0:
		return fmt.Sprintf("%s:%d", path, line)
	}
	return path
}

// capAtWord returns s cut to at most limit runes at a word boundary, with
// a single "…" in place of what was dropped. A text with no space to cut
// at is cut at the rune.
func capAtWord(s string, limit int) string {
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	if limit <= 1 {
		return string(runes[:limit])
	}
	cut := runes[:limit-1]
	if runes[limit-1] != ' ' {
		if i := lastSpace(cut); i > 0 {
			cut = cut[:i]
		}
	}
	return strings.TrimRight(string(cut), " ,;:—-") + "…"
}

// lastSpace is the index of the last space in runes, or -1.
func lastSpace(runes []rune) int {
	for i := len(runes) - 1; i >= 0; i-- {
		if runes[i] == ' ' {
			return i
		}
	}
	return -1
}
