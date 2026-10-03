package session

import "github.com/Mpaape/AurumCode/internal/gate"

// Source is what distinguishes the two ways a review is fed and published.
// Everything that differs between --base and --pr and is not "where the
// diff comes from" or "how the result is published" is declared here as
// data, so a divergence is visible in one table instead of in two copies of
// code.
type Source struct {
	// Label names the path in diagnostics ("--base", "--pr").
	Label string
	// NotReviewed maps each model outcome to whether it closes the run as
	// "not reviewed" (ExitInputs.NotReviewed).
	NotReviewed gate.NotReviewedRules
	// SecurityInIssues: the security pass's findings join the review's
	// issues (published as review comments) instead of being kept in their
	// own section. Either way they reach the gate and the verdict snapshot
	// exactly once.
	SecurityInIssues bool
	// NotReviewedNotice is printed when "not reviewed" decides the exit;
	// empty when the source already said so in its report.
	NotReviewedNotice string
}

// LocalDiff reviews a local diff and prints a report. Any model failure
// (including a required quality review that was skipped) always closes the
// run: the report it prints covers deterministic analysis only.
var LocalDiff = Source{
	Label: "--base",
	NotReviewed: gate.NotReviewedRules{
		gate.ModelProviderFailed: gate.AlwaysNotReviewed,
		gate.ModelParseFailed:    gate.AlwaysNotReviewed,
	},
}

// PullRequest reviews a verified pull request and publishes on it. A
// provider failure or an unparseable answer degrades to the deterministic
// half (the gate decides), and closes the run only when the caller required
// a quality review.
var PullRequest = Source{
	Label: "--pr",
	NotReviewed: gate.NotReviewedRules{
		gate.ModelProviderFailed: gate.NotReviewedWhenRequired,
		gate.ModelParseFailed:    gate.NotReviewedWhenRequired,
	},
	SecurityInIssues:  true,
	NotReviewedNotice: "aurumcode review: --exigir-qualidade: the model review was inconclusive; the published deterministic findings do not approve this pull request",
}
