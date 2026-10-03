// Package session is the one review session --base and --pr both run: a
// fixed, typed list of phases in a single order. A source (a local diff or
// a verified pull request) implements the phases; the order, the exit
// policy and what each model outcome means for that source are declared
// here, once.
package session

// Phase names one step of a review session.
type Phase string

const (
	// PhaseResolve validates the invocation and resolves the inputs: diff,
	// effective configuration, context, memory.
	PhaseResolve Phase = "resolve"
	// PhaseModel runs the model's quality pass.
	PhaseModel Phase = "model"
	// PhaseEvidence runs the deterministic passes: security, static
	// analysis, rule configuration, SAST, coverage.
	PhaseEvidence Phase = "evidence"
	// PhaseGate runs the shared gate pipeline.
	PhaseGate Phase = "gate"
	// PhasePublish publishes the result and decides the exit code.
	PhasePublish Phase = "publish"
)

// Order is the one order every review runs its phases in. Changing when
// the model runs relative to the evidence is a change to this list only.
var Order = []Phase{PhaseResolve, PhaseModel, PhaseEvidence, PhaseGate, PhasePublish}

// Step is what a phase returns: done ends the session with exit.
type Step func() (exit int, done bool)

// Phases is what a source provides: one step per phase.
type Phases interface {
	Step(Phase) Step
}

// Run executes the phases in Order. A phase that reports done ends the
// session with its exit code, so each early exit keeps its own code; a
// session that runs every phase without one exits 0.
func Run(p Phases) int {
	for _, phase := range Order {
		if exit, done := p.Step(phase)(); done {
			return exit
		}
	}
	return 0
}
