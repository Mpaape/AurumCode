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
	// Apply folds the contributor's evidence into res. Returning a plain
	// error makes the run inconclusive by the policy's mode; wrapping it
	// with Fatal aborts the pipeline (invalid configuration).
	Apply(ctx context.Context, run *Run, res *Result) error
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

// Run applies every contributor to res in declared order and records each
// one's footprint in res.Trail. A contributor that returns an ordinary
// error leaves the run inconclusive by the policy's mode: it is marked
// Active and Inconclusive, its reason is joined to res.Reason, and under
// gate.inconclusive: block the result fails -- a failing contributor never
// yields an approval. A FatalError aborts and is returned.
func (p *Pipeline) Run(ctx context.Context, run *Run, res *Result) error {
	for _, c := range p.contributors {
		lines, findings := len(res.Lines), len(res.BlockingFindings)
		err := c.Apply(ctx, run, res)
		entry := Contribution{Name: c.Name(), Origin: c.Origin()}
		if err != nil {
			var fatal *FatalError
			if errors.As(err, &fatal) {
				return fatal.Err
			}
			entry.Err = err.Error()
			markContributorFailure(run, res, c, err)
		}
		entry.Lines = len(res.Lines) - lines
		entry.Findings = len(res.BlockingFindings) - findings
		res.Trail = append(res.Trail, entry)
	}
	return nil
}

func markContributorFailure(run *Run, res *Result, c Contributor, err error) {
	res.Active = true
	res.Inconclusive = true
	res.AddReason("contributor_error:" + c.Name())
	res.Lines = append(res.Lines, fmt.Sprintf("gate contributor %s failed (%v)", c.Name(), err))
	if run != nil && run.Cfg != nil {
		if mode, modeErr := run.Cfg.Gate.InconclusiveMode(); modeErr == nil && mode == "block" {
			res.Fail = true
		}
	}
}
