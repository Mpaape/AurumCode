// The gate phase both review paths (--base and --pr) share. A path resolves
// its inputs, runs its analyses, then hands the same declared pipeline to
// executeGate; only the diff source and the publication differ. Adding a
// gate source means one Contributor (review_gate_contributors.go) and one
// line in assembleGatePipeline.
package main

import (
	"fmt"

	"github.com/Mpaape/AurumCode/internal/gate"
	"github.com/Mpaape/AurumCode/internal/llm"
	"github.com/Mpaape/AurumCode/internal/review"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// gatePipelineInputs is everything the contributors capture that is not
// part of the shared gate.Run: results of earlier phases of the path.
type gatePipelineInputs struct {
	// Provider and VerdictKey/RawIssues feed verdict reuse (AUR-524).
	Provider   llm.Provider
	VerdictKey gateVerdictKeyInputs
	RawIssues  []types.ReviewIssue

	// AcceptedOrigin is the dynamic-rule origin that counts toward the
	// skills gate: gateOriginPolicy when a central policy is active,
	// otherwise gateOriginRepo.
	AcceptedOrigin string
	DynamicRules   map[string]review.Rule

	// SAST* are runSASTPass's outputs and the origin its section came from.
	SASTOrigin string
	SASTIssues []types.ReviewIssue
	SASTReason string
}

// assembleGatePipeline declares the one gate pipeline, in the order the
// contributors apply. Both --base and --pr call exactly this function.
func assembleGatePipeline(in gatePipelineInputs) *gate.Pipeline {
	return gate.NewPipeline(
		exceptionsContributor{},
		verdictReuseContributor{provider: in.Provider, key: in.VerdictKey, raw: in.RawIssues},
		policySkillsContributor{origin: in.AcceptedOrigin, dynamic: in.DynamicRules},
		sastContributor{origin: in.SASTOrigin, issues: in.SASTIssues, reason: in.SASTReason},
		embeddedAnalysisContributor{},
		analysisDataContributor{},
		dependencyTrackContributor{},
	)
}

// gatePipelineObserver, when set (tests), receives the contributor names of
// every pipeline a path executes, labeled by path.
var gatePipelineObserver func(label string, names []string)

// executeGate runs pipeline against run with reason as the initial
// inconclusive motive. A configuration error ends the review (exit 2): the
// message is printed here and ok is false.
func executeGate(label string, pipeline *gate.Pipeline, run *gate.Run, reason string) (res *gate.Result, ok bool) {
	if gatePipelineObserver != nil {
		gatePipelineObserver(label, pipeline.Names())
	}
	res = &gate.Result{Reason: reason}
	if err := pipeline.Run(run.Ctx, run, res); err != nil {
		fmt.Fprintf(run.Stderr, "aurumcode review: gate: %v\n", err)
		return res, false
	}
	return res, true
}

// gateExitCode maps a failed gate to the shared exit codes: a real severity
// breach is exitFindings regardless of inconclusiveness; Fail without a
// breach is gate.inconclusive: block, exitQualityNotReviewed. closed is
// false when the gate does not close the run.
func gateExitCode(res *gate.Result) (code int, closed bool) {
	if res.Breach {
		return exitFindings, true
	}
	if res.Fail {
		return exitQualityNotReviewed, true
	}
	return 0, false
}
