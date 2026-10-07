// The conversion between a profile pass's findings and the merged report:
// every field a single-profile review publishes survives the merge.
package main

import (
	"strings"

	"github.com/Mpaape/AurumCode/internal/i18n"

	"github.com/Mpaape/AurumCode/internal/reviewprofile"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// profileFinding maps one issue of a profile pass onto the merge's value.
// ref is the issue's index in the caller's originals slice, so the merged
// issue is rebuilt from the whole original (ID, Origin and Assessment
// included) and never from a field-by-field subset.
func profileFinding(profile string, ref int, issue types.ReviewIssue) reviewprofile.Finding {
	return reviewprofile.Finding{
		Profile:      profile,
		RuleID:       issue.RuleID,
		File:         issue.File,
		Line:         issue.Line,
		Message:      issue.Message,
		Severity:     issue.Severity,
		Side:         issue.Side,
		Impact:       issue.Impact,
		Evidence:     issue.Evidence,
		Suggestion:   issue.Suggestion,
		Verification: issue.Verification,
		Ref:          ref,
	}
}

// attributedIssues converts merged, profile-attributed findings back into the
// report's ReviewIssue shape. Each issue starts from the kept occurrence's
// original (originals[f.Ref]), takes the merged detail (a duplicate may
// have filled an empty field or added evidence) and folds the attribution
// into the message: the first profile, then every profile that agreed. The
// severity the finding already carried never changes.
func attributedIssues(findings []reviewprofile.Finding, originals []types.ReviewIssue, language string) []types.ReviewIssue {
	if len(findings) == 0 {
		return nil
	}
	out := make([]types.ReviewIssue, 0, len(findings))
	for _, f := range findings {
		issue := types.ReviewIssue{File: f.File, Line: f.Line, Severity: f.Severity, RuleID: f.RuleID}
		if f.Ref >= 0 && f.Ref < len(originals) {
			issue = originals[f.Ref]
		}
		issue.Side = f.Side
		issue.Impact = f.Impact
		issue.Evidence = f.Evidence
		issue.Suggestion = f.Suggestion
		issue.Verification = f.Verification
		issue.Message = profileAttribution(f, language)
		out = append(out, issue)
	}
	return out
}

// profileAttribution is the finding's message with the profiles that
// reported it, in the review's language: the first profile, then every
// profile that agreed, so a collapsed duplicate stays traceable.
func profileAttribution(f reviewprofile.Finding, language string) string {
	if f.Profile == "" || strings.Contains(f.Message, i18n.Format(language, "review.profile_attribution", f.Profile)) {
		return f.Message
	}
	if len(f.AlsoFrom) == 0 {
		return f.Message + " " + i18n.Format(language, "review.profile_attribution", f.Profile)
	}
	return f.Message + " " + i18n.Format(language, "review.profile_attribution_also", f.Profile, strings.Join(f.AlsoFrom, ", "))
}
