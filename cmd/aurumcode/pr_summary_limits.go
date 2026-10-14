// The limitations the parecer shows: only what a reader of the decision
// needs. The gate's own decision lines and the triage bookkeeping repeat
// the findings and the headline, so they stay on stderr and in the audit;
// a reduced codebase context is said once, as a note that the reviewed code
// itself was read whole.
package main

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/Mpaape/AurumCode/pkg/types"
)

// findingGateLine is a gate line about one finding (gate.FindingLine):
// "policy gate: <rule> - <message> (severidade …, limiar …, origem …)". The
// finding is already listed with its decision, so the line repeats it. A
// gate line without a finding (a numeric breach, an inconclusive reason)
// stays: it is the only account of why the gate decided.
var findingGateLine = regexp.MustCompile(`^policy gate: .* \((?:severidade|severity) [^,]*, (?:limiar|threshold) `)

// readerLimitations is result's limitations as the parecer shows them:
// without the engine's decision lines, without internal codes, each once,
// and with the context note when the codebase context was reduced.
func readerLimitations(result *types.ReviewResult, copy reviewCopy) []string {
	seen := map[string]bool{}
	var out []string
	for _, line := range result.Limitations {
		if isEngineLine(line) {
			continue
		}
		text := readableText(strings.TrimPrefix(strings.TrimPrefix(withoutIgnoredNames(line, copy), triagePrefix), gatePrefix))
		if text == "" || seen[text] {
			continue
		}
		seen[text] = true
		out = append(out, text)
	}
	if result.Metadata[metaContextReduced] != "" {
		out = append(out, copy.contextReducedNote)
	}
	return out
}

// isEngineLine reports a line written for the terminal and the audit, not
// for the reader: a gate line about a finding, or a finding the model's
// triage refuted (a refuted finding leaves the parecer).
func isEngineLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	return findingGateLine.MatchString(trimmed) || strings.HasPrefix(trimmed, "gate.triage (")
}

// triagePrefix opens the triage notices; the reader needs the sentence,
// not the configuration key.
const triagePrefix = "gate.triage: "

// gatePrefix opens the gate's lines; in the parecer the sentence suffices.
const gatePrefix = "policy gate: "

// writeRulesSection lists, collapsed in the details, the rules the findings
// cite and their origin, each once: what an exception or a policy change
// names, kept out of the body the author reads first. A rule cited once
// names its place; a rule cited more often, its count.
func writeRulesSection(b *strings.Builder, result *types.ReviewResult, copy reviewCopy) {
	issues := sortedIssues(result.Issues)
	if len(issues) == 0 {
		return
	}
	type cited struct {
		rule, origin, place string
		count               int
	}
	var order []string
	byKey := map[string]*cited{}
	for _, issue := range issues {
		key := issue.RuleID + "\x00" + issue.Origin
		if c, ok := byKey[key]; ok {
			c.count++
			continue
		}
		byKey[key] = &cited{rule: issue.RuleID, origin: issue.Origin, place: fmt.Sprintf("%s:%d", issue.File, issue.Line), count: 1}
		order = append(order, key)
	}
	fmt.Fprintf(b, "#### %s\n\n", copy.rulesHeading)
	for _, key := range order {
		c := byKey[key]
		fmt.Fprintf(b, "- `%s`", c.rule)
		if c.origin != "" {
			fmt.Fprintf(b, " (%s)", fmt.Sprintf(copy.ruleOrigin, c.origin))
		}
		if c.count == 1 {
			fmt.Fprintf(b, ": `%s`\n", c.place)
		} else {
			fmt.Fprintf(b, ": %s\n", fmt.Sprintf(copy.ruleCount, c.count))
		}
	}
	b.WriteString("\n")
}

// withoutIgnoredNames is the coverage block with the ignored files counted
// but not named: they are the repository's own ignore configuration, and
// the parecer of a large change listed dozens of them. The terminal keeps
// the names.
func withoutIgnoredNames(block string, copy reviewCopy) string {
	if !strings.Contains(block, "\n") {
		return block
	}
	markers := []string{afterCount(copy.coverageIgnored, "%d"), afterCount(copy.coverageIgnoredOne, "1")}
	var out []string
	skipping := false
	for _, line := range strings.Split(block, "\n") {
		if skipping && strings.HasPrefix(line, "  ") {
			continue
		}
		skipping = false
		for _, marker := range markers {
			if marker != "" && strings.HasPrefix(line, "- ") && strings.Contains(line, marker) {
				skipping = true
			}
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

// afterCount is template's text after its count, the part every count
// shares.
func afterCount(template, count string) string {
	if _, rest, ok := strings.Cut(template, count); ok {
		return strings.TrimSpace(rest)
	}
	return ""
}
