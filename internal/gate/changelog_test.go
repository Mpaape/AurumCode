package gate

import (
	"context"
	"io"
	"testing"

	"github.com/Mpaape/AurumCode/pkg/types"
)

func changelogRun() *Run {
	return &Run{Review: &types.ReviewResult{}, Stdout: io.Discard, Stderr: io.Discard}
}

// A refused entry is a breach (exit 3); an indeterminate reading fails the
// gate without one (exit 1); no check adds nothing.
func TestChangelogContributorExits(t *testing.T) {
	finding := types.ReviewIssue{File: "CHANGELOG.md", Line: 3, Severity: "error", RuleID: "changelog/entrada_longa", Message: "changelog: reprovado (entrada_longa)", Origin: OriginChangelog}
	for name, tc := range map[string]struct {
		check *ChangelogCheck
		exit  int
	}{
		"reprovada":     {&ChangelogCheck{Finding: &finding, Blocks: true, Breach: true, Line: "l"}, ExitFindings},
		"indeterminado": {&ChangelogCheck{Finding: &finding, Blocks: true, Line: "l"}, ExitBehavioral},
		"sem check":     {nil, ExitClean},
	} {
		t.Run(name, func(t *testing.T) {
			run := changelogRun()
			res, err := ChangelogContributor{Check: tc.check}.Apply(context.Background(), run, Result{})
			if err != nil {
				t.Fatal(err)
			}
			if got := ExitPolicy(ExitInputs{Gate: &res}).Code; got != tc.exit {
				t.Fatalf("exit=%d, want %d (%+v)", got, tc.exit, res)
			}
			if tc.check != nil && (len(run.Review.Issues) != 1 || len(res.BlockingFindings) != 1) {
				t.Fatalf("the refusal must be one published and one blocking finding: issues=%v blocking=%v", run.Review.Issues, res.BlockingFindings)
			}
		})
	}
}
