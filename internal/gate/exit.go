package gate

// The review command's exit codes. 0 is a clean run; ExitBehavioral is the
// existing "the review did not produce a trustworthy answer" code (a run
// that did not review, a publication or artifact that failed, a gate that
// blocks an inconclusive run); ExitFindings is "the review ran and found
// something at or above the threshold".
const (
	ExitClean      = 0
	ExitBehavioral = 1
	ExitFindings   = 3
)

// ExitCause names which rule of the ExitPolicy decided the code.
type ExitCause int

const (
	CauseNone ExitCause = iota
	CausePublishFailure
	CauseNotReviewed
	CauseStatusFailure
	CauseGateBreach
	CauseGateBlock
	CauseArtifactMissing
	CauseFailOn
	CauseCheckStatus
)

// ExitInputs is everything the exit decision reads. A source that does not
// publish (the local diff) leaves the publication fields zero.
type ExitInputs struct {
	// PublishFailures counts review comments that could not be posted.
	PublishFailures int
	// NotReviewed is the source's NotReviewedRules applied to the model
	// outcome.
	NotReviewed bool
	// CheckStatusExit and GateStatusExit are what publishing the commit
	// statuses returned (0 when no status was published); ExitBehavioral
	// from either is a status that could not be published.
	CheckStatusExit int
	GateStatusExit  int
	// Gate is the pipeline's decision; nil means no gate ran.
	Gate *Result
	// ArtifactsMissing: a requested audit record or SARIF was not written.
	ArtifactsMissing bool
	// FindingsAtThreshold counts findings at or above --fail-on (0 when
	// --fail-on is off).
	FindingsAtThreshold int
}

// ExitDecision is the exit code and the rule that produced it.
type ExitDecision struct {
	Code  int
	Cause ExitCause
}

// exitRule is one rung of the ladder: it either decides or passes.
type exitRule func(ExitInputs) (ExitDecision, bool)

// exitLadder is the one precedence both review sources share, highest
// first. A comment that could not be posted outranks everything (the run
// did not deliver what it claimed); "did not review" outranks a status that
// could not be published; the policy gate (a breach is ExitFindings even
// when also inconclusive; a block without a breach is ExitBehavioral)
// outranks a missing compliance artifact, which outranks --fail-on, which
// outranks the --check status's own code.
var exitLadder = []exitRule{
	func(in ExitInputs) (ExitDecision, bool) {
		return ExitDecision{ExitBehavioral, CausePublishFailure}, in.PublishFailures > 0
	},
	func(in ExitInputs) (ExitDecision, bool) {
		return ExitDecision{ExitBehavioral, CauseNotReviewed}, in.NotReviewed
	},
	func(in ExitInputs) (ExitDecision, bool) {
		failed := in.CheckStatusExit == ExitBehavioral || in.GateStatusExit == ExitBehavioral
		return ExitDecision{ExitBehavioral, CauseStatusFailure}, failed
	},
	func(in ExitInputs) (ExitDecision, bool) {
		return ExitDecision{ExitFindings, CauseGateBreach}, in.Gate != nil && in.Gate.Breach
	},
	func(in ExitInputs) (ExitDecision, bool) {
		return ExitDecision{ExitBehavioral, CauseGateBlock}, in.Gate != nil && in.Gate.Fail
	},
	func(in ExitInputs) (ExitDecision, bool) {
		return ExitDecision{ExitBehavioral, CauseArtifactMissing}, in.ArtifactsMissing
	},
	func(in ExitInputs) (ExitDecision, bool) {
		return ExitDecision{ExitFindings, CauseFailOn}, in.FindingsAtThreshold > 0
	},
	func(in ExitInputs) (ExitDecision, bool) {
		return ExitDecision{in.CheckStatusExit, CauseCheckStatus}, in.CheckStatusExit != ExitClean
	},
}

// ExitPolicy is the one exit decision of a review: the first rung of the ladder that applies
// decides; none applying is a clean exit.
func ExitPolicy(in ExitInputs) ExitDecision {
	for _, rule := range exitLadder {
		if d, ok := rule(in); ok {
			return d
		}
	}
	return ExitDecision{ExitClean, CauseNone}
}
