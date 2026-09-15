package review

import (
	"fmt"
	"sort"

	"github.com/Mpaape/AurumCode/pkg/types"
)

// This file is the AUR-435 security pass. The c12d7ab restoration measured
// its missing piece precisely: the scorer already penalized security
// findings (-10 each, internal/review/iso25010 at c12d7ab) and the rules
// already existed as metadata, but no matcher ever connected a rule to the
// code under review. The pass below is that matcher: it scans the ADDED
// lines of the reviewed diff with the patterns the security-category rules
// of the embedded catalog now carry (see rules.go and rules/security.yml)
// and reports each match as a finding.
//
// The pass is deliberately deterministic and model-free. It runs on the RAW
// diff -- matching must see what the repository actually contains, and the
// diff never leaves the process here -- while everything it REPORTS is
// either a trusted catalog string (description, rule citation, standard
// citation) or a diff-derived field (file path, line number) that
// cmd/aurumcode redacts at the sink, so no reviewed content is ever echoed.
//
// Separation from the quality findings is structural, not cosmetic: the
// rubric of this pass is the security-category selection made once inside
// securityScanWithRules, never the quality rubric, and cmd/aurumcode prints
// the result in its own section. MUT-001 attacks exactly that selection.

// SecurityScan runs the project security pass over diff and returns its
// findings. Every finding cites a security-category rule of the embedded
// project catalog -- the AUR-434 rule gate applies to this pass unchanged
// -- plus, when the rule binds one, the standards/security-review rule
// that scopes it. An unloadable catalog is a loud error, never a silent
// zero-finding pass.
func SecurityScan(diff *types.Diff) ([]types.ReviewIssue, error) {
	rules, err := sharedRules()
	if err != nil {
		return nil, fmt.Errorf("review rules unavailable: %w", err)
	}
	return securityScanWithRules(rules, diff), nil
}

// SecurityScanWithCoverage is SecurityScan (see its own doc, unchanged and
// still the entry point tests/unit/AUR-435.go and tests/unit/AUR-442.go
// use) plus the AUR-450 coverage: which security-category rules of the
// embedded catalog carry a matcher (applied, sorted by id) and how many
// the category declares in total (total). Both are derived from the same
// RulesLoader the scan itself uses, loaded once, so a caller can never
// observe the findings and the coverage figures disagreeing because of a
// catalog reload between two separate calls. cmd/aurumcode's --seguranca
// path calls this instead of SecurityScan so it can report, on every run,
// how much of the catalog the pass actually covers -- identically whether
// the pass found something or found nothing, because the coverage figures
// are catalog-derived, never diff-derived.
func SecurityScanWithCoverage(diff *types.Diff) (findings []types.ReviewIssue, applied []string, total int, err error) {
	rules, err := sharedRules()
	if err != nil {
		return nil, nil, 0, fmt.Errorf("review rules unavailable: %w", err)
	}
	findings = securityScanWithRules(rules, diff)
	applied, total = rules.AppliedInCategory("security")
	return findings, applied, total, nil
}

// securityScanWithRules is SecurityScan over an explicit catalog.
func securityScanWithRules(rules *RulesLoader, diff *types.Diff) []types.ReviewIssue {
	// The security rubric: the security-category rules of the embedded
	// catalog, sorted by ID for determinism. Reusing the quality rubric
	// here is MUT-001, and it must make the expected finding vanish.
	rubric := rules.GetByCategory("security")

	var found []types.ReviewIssue
	for _, file := range diff.Files {
		for _, hunk := range file.Hunks {
			// Track the new-file line number the way a unified diff reader
			// does: context and added lines advance it, removed lines do
			// not exist in the new file.
			newLine := hunk.NewStart
			for _, line := range hunk.Lines {
				marker, body := splitDiffMarker(line)
				if marker == "-" {
					// Removed code is not part of the reviewed result and
					// must never produce a finding.
					continue
				}
				if marker == "+" {
					for _, rule := range rubric {
						re, ok := rules.PatternFor(rule.ID)
						if !ok {
							continue // metadata-only rule, never matched
						}
						if loc := re.FindStringIndex(body); loc != nil {
							// AUR-503: the four command-injection branches
							// AUR-486 added are deliberately unanchored, so a
							// REAL invocation embedded in an expression
							// (`x := exec.Command(...)`, `if err :=
							// exec.Command(...)`, `return exec.Command(...)`,
							// `var p = Process.Start(...)`, `$r = iex ...`)
							// still matches. A line regexp cannot tell a `;`
							// statement separator from a `;` inside a comment
							// (`// foo; exec.Command(...)`), so the context
							// decision is structural here: a
							// security/command-injection match whose first
							// byte sits inside a comment or string literal is
							// a mention, not a defect. Scoped to this one rule
							// so security/hardcoded-secret, whose true
							// positives are legitimately inside string
							// literals, is unchanged.
							if rule.ID == "security/command-injection" && !codeMask(body)[loc[0]] {
								continue
							}
							msg := rule.Description
							if rule.Standard != "" {
								msg = fmt.Sprintf("%s [standards/security-review %s]", msg, rule.Standard)
							}
							found = append(found, types.ReviewIssue{
								File:     file.Path,
								Line:     newLine,
								Severity: rule.Severity,
								RuleID:   rule.ID,
								Message:  msg,
							})
						}
					}
				}
				newLine++
			}
		}
	}

	// The AUR-434 gate applies to this pass exactly as it does to model
	// findings: every finding must cite a resolvable catalog rule, and the
	// citation " (rule <id>: <title>)" is appended by the same code path.
	// By construction nothing here can be rejected -- the findings were
	// generated FROM catalog rules -- but the gate stays in the path so a
	// future defect fails closed instead of shipping an uncited finding.
	result := &types.ReviewResult{Issues: found}
	enforceRuleCitations(rules, result)

	sort.SliceStable(result.Issues, func(i, j int) bool {
		if result.Issues[i].File != result.Issues[j].File {
			return result.Issues[i].File < result.Issues[j].File
		}
		if result.Issues[i].Line != result.Issues[j].Line {
			return result.Issues[i].Line < result.Issues[j].Line
		}
		return result.Issues[i].RuleID < result.Issues[j].RuleID
	})
	return result.Issues
}

// codeMask marks each byte of one added-line body as CODE (true) or as part
// of a comment / string literal (false). It exists for the AUR-503
// command-injection context filter: those branches are unanchored so a real
// call in an expression context still matches, and this is what keeps a
// textual mention inside `//`/`#`/`--`/`/* */` or a `"`/`'`/backtick span
// from being reported. Both the opening marker byte and the whole span are
// marked non-code, so a match starting on the marker itself is also
// dropped.
//
// The scan is deliberately line-local and lexical, not a parser: it is a
// single left-to-right state machine so a comment marker inside a string
// (or a quote inside a comment) is handled in the right order. `--` only
// opens a comment after whitespace or at the start of the line, so Go's
// `x--` decrement is not treated as a SQL comment. An unterminated block
// comment or quote masks the rest of the line, which is the conservative
// choice for this filter (it can only suppress a mention, never invent
// one).
func codeMask(body string) []bool {
	mask := make([]bool, len(body))
	for i := range mask {
		mask[i] = true
	}
	i := 0
	for i < len(body) {
		c := body[i]
		switch {
		case c == '/' && i+1 < len(body) && body[i+1] == '/':
			for ; i < len(body); i++ {
				mask[i] = false
			}
		case c == '#':
			for ; i < len(body); i++ {
				mask[i] = false
			}
		case c == '-' && i+1 < len(body) && body[i+1] == '-' && (i == 0 || body[i-1] == ' ' || body[i-1] == '\t'):
			for ; i < len(body); i++ {
				mask[i] = false
			}
		case c == '/' && i+1 < len(body) && body[i+1] == '*':
			mask[i] = false
			mask[i+1] = false
			i += 2
			for i < len(body) {
				if body[i] == '*' && i+1 < len(body) && body[i+1] == '/' {
					mask[i] = false
					mask[i+1] = false
					i += 2
					break
				}
				mask[i] = false
				i++
			}
		case c == '"' || c == '\'' || c == '`':
			quote := c
			mask[i] = false
			i++
			for i < len(body) {
				if quote != '`' && body[i] == '\\' {
					mask[i] = false
					i++
					if i < len(body) {
						mask[i] = false
						i++
					}
					continue
				}
				if body[i] == quote {
					mask[i] = false
					i++
					break
				}
				mask[i] = false
				i++
			}
		default:
			i++
		}
	}
	return mask
}
