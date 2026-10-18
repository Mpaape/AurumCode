// The changelog check's gate source: the verdict the local review takes
// from the same check as `aurumcode changelog` (cmd/aurumcode).
package gate

import (
	"context"

	"github.com/Mpaape/AurumCode/internal/gate/facts"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// ContributorChangelog is the changelog check's contributor name.
const ContributorChangelog = "changelog"

// OriginChangelog is the origin of the changelog check's finding.
const OriginChangelog = "changelog"

// ChangelogCheck is the verdict of the changelog check the local review
// ran (the same check as `aurumcode changelog`). Finding is what the review
// publishes: a refused entry, or the observation of the suggest mode.
// Blocks is true only for an entry the required mode refuses; Breach is
// false when the refusal is an indeterminate reading, not a bad entry.
// Line is the gate line that states the verdict.
type ChangelogCheck struct {
	Finding *types.ReviewIssue
	Blocks  bool
	Breach  bool
	Line    string
}

// ChangelogContributor gates the changelog entry. Check is nil when the
// path does not run the check (the --pr path, where the changelog workflow
// decides on its own, or a repository without an active changelog_check):
// the contributor then adds nothing. It runs last, so its finding is never
// offered to the model, replayed by verdict reuse or judged by another
// source.
type ChangelogContributor struct {
	Check *ChangelogCheck
}

func (ChangelogContributor) Name() string   { return ContributorChangelog }
func (ChangelogContributor) Origin() string { return OriginChangelog }
func (c ChangelogContributor) Apply(_ context.Context, run *Run, _ Result) (Result, error) {
	if c.Check == nil {
		return Result{}, nil
	}
	if f := c.Check.Finding; f != nil {
		run.Review.Issues = append(run.Review.Issues, *f)
	}
	if !c.Check.Blocks || c.Check.Finding == nil {
		return Result{}, nil
	}
	f := *c.Check.Finding
	part := Result{Active: true, Fail: true, Breach: c.Check.Breach}
	part.Lines = append(part.Lines, c.Check.Line)
	part.BlockingFindings = append(part.BlockingFindings, facts.AuditFinding{
		RuleID: f.RuleID, Path: f.File, Line: f.Line, Severity: f.Severity, Origin: OriginChangelog,
	})
	if c.Check.Breach {
		part.noteBreach(f.Message, f)
	}
	return part, nil
}
