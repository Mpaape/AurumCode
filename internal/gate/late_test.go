package gate

import (
	"context"
	"testing"

	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/review"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// TestLateFindingNeverCountsInTheGate: a finding round convergence
// published as an observation does not fail the gate even when its rule's
// own severity floor reaches fail_on; the same finding not late does.
func TestLateFindingNeverCountsInTheGate(t *testing.T) {
	dynamic := map[string]review.Rule{"repo/erro": {ID: "repo/erro", Title: "Erro", Origin: OriginRepo, Severity: "error"}}
	for _, late := range []bool{false, true} {
		cfg := &config.Config{}
		cfg.Gate = config.GateConfig{FailOn: []string{"error"}, Inconclusive: "warn"}
		run := &Run{Cfg: cfg, Review: &types.ReviewResult{Issues: []types.ReviewIssue{{File: "a.go", Line: 3, Severity: "info", RuleID: "repo/erro", Late: late}}}}
		res, err := PolicySkillsContributor{AcceptedOrigin: OriginRepo, Dynamic: dynamic}.Apply(context.Background(), run, Result{})
		if err != nil {
			t.Fatal(err)
		}
		if res.Fail == late || len(res.BlockingFindings) == 0 != late {
			t.Fatalf("late=%v: gate fail=%v blocking=%v", late, res.Fail, res.BlockingFindings)
		}
	}
}
