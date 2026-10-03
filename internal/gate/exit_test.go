package gate

import "testing"

// TestExitPolicyPrecedence pins the one ladder both review sources share:
// each row sets two conditions at once and the higher rung must win.
func TestExitPolicyPrecedence(t *testing.T) {
	breach := &Result{Breach: true, Fail: true}
	block := &Result{Fail: true, Inconclusive: true}
	cases := []struct {
		name string
		in   ExitInputs
		want ExitDecision
	}{
		{"clean", ExitInputs{}, ExitDecision{ExitClean, CauseNone}},
		{"publish failure over not reviewed", ExitInputs{PublishFailures: 1, NotReviewed: true}, ExitDecision{ExitBehavioral, CausePublishFailure}},
		{"not reviewed over gate breach", ExitInputs{NotReviewed: true, Gate: breach}, ExitDecision{ExitBehavioral, CauseNotReviewed}},
		{"status failure over gate breach", ExitInputs{CheckStatusExit: ExitBehavioral, Gate: breach}, ExitDecision{ExitBehavioral, CauseStatusFailure}},
		{"gate status failure over gate breach", ExitInputs{GateStatusExit: ExitBehavioral, Gate: breach}, ExitDecision{ExitBehavioral, CauseStatusFailure}},
		{"gate breach over artifact missing", ExitInputs{Gate: breach, ArtifactsMissing: true}, ExitDecision{ExitFindings, CauseGateBreach}},
		{"gate block over artifact missing", ExitInputs{Gate: block, ArtifactsMissing: true}, ExitDecision{ExitBehavioral, CauseGateBlock}},
		{"artifact missing over fail-on", ExitInputs{ArtifactsMissing: true, FindingsAtThreshold: 2}, ExitDecision{ExitBehavioral, CauseArtifactMissing}},
		{"fail-on over check status", ExitInputs{FindingsAtThreshold: 1, CheckStatusExit: ExitFindings}, ExitDecision{ExitFindings, CauseFailOn}},
		{"check status alone", ExitInputs{CheckStatusExit: ExitFindings}, ExitDecision{ExitFindings, CauseCheckStatus}},
		{"inactive gate passes", ExitInputs{Gate: &Result{}}, ExitDecision{ExitClean, CauseNone}},
	}
	for _, c := range cases {
		if got := ExitPolicy(c.in); got != c.want {
			t.Errorf("%s: ExitPolicy = %+v, want %+v", c.name, got, c.want)
		}
	}
}

// TestRankReasonOrder pins the inconclusive ranking: each row sets every
// lower motive too, so only the top one may be named.
func TestRankReasonOrder(t *testing.T) {
	all := func(m ModelOutcome) ReasonInputs {
		return ReasonInputs{Model: m, DegradedParse: true, SASTReason: "sast_unavailable", PartialCoverage: true}
	}
	cases := []struct {
		in   ReasonInputs
		want Reason
	}{
		{all(ModelProviderFailed), ReasonProviderFailure},
		{all(ModelSkipped), ReasonQualitySkipped},
		{all(ModelParseFailed), ReasonModelParseFailure},
		{all(ModelReviewed), ReasonDegradedParse},
		{ReasonInputs{SASTReason: "sast_unavailable", PartialCoverage: true}, Reason("sast_unavailable")},
		{ReasonInputs{PartialCoverage: true}, ReasonPartialCoverage},
		{ReasonInputs{}, ReasonNone},
	}
	for _, c := range cases {
		if got := RankReason(c.in); got != c.want {
			t.Errorf("RankReason(%+v) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestNotReviewedRules covers the three rules of the per-source table.
func TestNotReviewedRules(t *testing.T) {
	rules := NotReviewedRules{ModelProviderFailed: AlwaysNotReviewed, ModelParseFailed: NotReviewedWhenRequired}
	checks := []struct {
		outcome ModelOutcome
		quality QualityRequirement
		want    bool
	}{
		{ModelProviderFailed, QualityOptional, true},
		{ModelParseFailed, QualityOptional, false},
		{ModelParseFailed, QualityRequired, true},
		{ModelReviewed, QualityRequired, false},
	}
	for _, c := range checks {
		if got := rules.NotReviewed(c.outcome, c.quality); got != c.want {
			t.Errorf("NotReviewed(%v, %v) = %v, want %v", c.outcome, c.quality, got, c.want)
		}
	}
}
