package gate

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// Run carries the inputs every contributor shares. A path (--base, --pr)
// fills it from its own diff source and configuration; contributors read it
// and may replace the redaction filter and the output writers (a secret
// learned mid-run), which the path then reads back.
type Run struct {
	Ctx    context.Context
	Cfg    *config.Config
	Diff   *types.Diff
	Review *types.ReviewResult
	// Extra holds deterministic findings kept apart from Review.Issues
	// (the --base security pass) that still count toward the gate.
	Extra []types.ReviewIssue
	// Security is the --seguranca pass's findings, handed to the gate on both
	// paths (on --pr they are also merged into Review.Issues). They count
	// toward fail_on whatever the inconclusive mode (AUR-569).
	Security []types.ReviewIssue

	Language string
	Filter   *redaction.Filter
	Stdout   io.Writer
	Stderr   io.Writer

	// RepoIdentity is the verified "owner/repo" of the reviewed code;
	// RepoIdentityKnown is false when it could not be verified.
	RepoIdentity      string
	RepoIdentityKnown bool

	// Now is the clock exceptions are judged against.
	Now func() time.Time

	// Triage is what the model's assessment may change (triage.go);
	// Demoted collects the findings it demoted in this run.
	Triage  Triage
	Demoted []Demotion

	flushers []func()
}

// IssuesForGate is Review.Issues followed by Extra, as a fresh slice.
func (r *Run) IssuesForGate() []types.ReviewIssue {
	out := make([]types.ReviewIssue, 0, len(r.Review.Issues)+len(r.Extra))
	out = append(out, r.Review.Issues...)
	return append(out, r.Extra...)
}

// Clock returns the run's current time.
func (r *Run) Clock() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

// OnFlush registers a function the path must call (Flush) before it
// returns, so redaction writers installed by a contributor drain.
func (r *Run) OnFlush(f func()) { r.flushers = append(r.flushers, f) }

// Flush runs every registered flush function once, last registered first
// (the order deferred calls would run in).
func (r *Run) Flush() {
	for i := len(r.flushers) - 1; i >= 0; i-- {
		r.flushers[i]()
	}
	r.flushers = nil
}

// Contributor is one source of gate evidence.
type Contributor interface {
	// Name identifies the contributor in the pipeline (unique per pipeline).
	Name() string
	// Origin labels the findings this contributor adds.
	Origin() string
	// Apply returns the contributor's own partial decision; the pipeline
	// merges it (Result.Merge) into the run's result. prior is the result
	// so far, to read (its inconclusive reason), never to rewrite.
	// Returning a plain error makes the run inconclusive by the policy's
	// mode; wrapping it with Fatal aborts the pipeline (invalid
	// configuration).
	Apply(ctx context.Context, run *Run, prior Result) (Result, error)
}

// FatalError marks an error the pipeline must not absorb as inconclusive.
type FatalError struct{ Err error }

func (e *FatalError) Error() string { return e.Err.Error() }
func (e *FatalError) Unwrap() error { return e.Err }

// Fatal wraps err as a FatalError.
func Fatal(err error) error {
	if err == nil {
		return nil
	}
	return &FatalError{Err: err}
}

// Pipeline applies contributors in the order they were declared.
type Pipeline struct {
	contributors []Contributor
}

// NewPipeline declares a pipeline; order is the order given.
func NewPipeline(contributors ...Contributor) *Pipeline {
	return &Pipeline{contributors: append([]Contributor(nil), contributors...)}
}

// Contributors returns the declared contributors, in order.
func (p *Pipeline) Contributors() []Contributor {
	return append([]Contributor(nil), p.contributors...)
}

// Names returns the declared contributor names, in order.
func (p *Pipeline) Names() []string {
	names := make([]string, 0, len(p.contributors))
	for _, c := range p.contributors {
		names = append(names, c.Name())
	}
	return names
}

// Run merges every contributor's partial decision into res in declared
// order and records each one's footprint in res.Trail. A contributor that returns an ordinary
// error leaves the run inconclusive: it is marked Active and Inconclusive
// and its reason is joined to res.Reason. After every contributor the
// effective inconclusive mode is applied (ApplyInconclusiveMode), the same
// for every source, so a failing contributor and a scanner that could not
// run end the same way. A FatalError aborts and is returned.
func (p *Pipeline) Run(ctx context.Context, run *Run, res *Result) error {
	for _, c := range p.contributors {
		lines, findings := len(res.Lines), len(res.BlockingFindings)
		part, err := c.Apply(ctx, run, *res)
		res.Merge(part)
		entry := Contribution{Name: c.Name(), Origin: c.Origin()}
		if err != nil {
			var fatal *FatalError
			if errors.As(err, &fatal) {
				return fatal.Err
			}
			entry.Err = err.Error()
			markContributorFailure(res, c, err)
		}
		entry.Lines = len(res.Lines) - lines
		entry.Findings = len(res.BlockingFindings) - findings
		res.Trail = append(res.Trail, entry)
		ApplyInconclusiveMode(run, res)
	}
	return nil
}

func markContributorFailure(res *Result, c Contributor, err error) {
	res.Active = true
	res.Inconclusive = true
	res.AddReason("contributor_error:" + c.Name())
	res.Lines = append(res.Lines, fmt.Sprintf("gate contributor %s failed (%v)", c.Name(), err))
}

// ApplyInconclusiveMode is the one place an inconclusive result becomes a
// failure: under the effective gate.inconclusive mode "block" (the default
// whenever a gate or a scanner is configured, config.Config.InconclusiveMode)
// an inconclusive result fails. Sources only report Inconclusive; none of
// them decides the mode on its own. The pipeline applies it after every
// contributor, and a path that marks the result inconclusive after the
// pipeline (a compliance artifact that could not be written) applies it
// again.
func ApplyInconclusiveMode(run *Run, res *Result) {
	if run == nil {
		return
	}
	mode, err := run.Cfg.InconclusiveMode()
	ApplyInconclusiveModeValue(mode, err, res)
}

// ApplyInconclusiveModeValue applies an already-resolved mode (a command
// that holds only the gate section resolves it itself). An unreadable mode
// never weakens the gate: it is treated as block.
func ApplyInconclusiveModeValue(mode string, modeErr error, res *Result) {
	if res.Inconclusive && (modeErr != nil || mode == config.InconclusiveBlock) {
		res.Fail = true
	}
}
