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
						if re.MatchString(body) {
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
