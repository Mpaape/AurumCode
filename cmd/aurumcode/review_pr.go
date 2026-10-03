// The --pr review path: the pull-request source of the review session
// (internal/review/session). It validates the invocation and resolves its
// inputs (diff, effective config, checkout, memory), runs the model pass
// and the evidence, hands the shared state to the session's gate
// (review_gate.go), then publishes on GitHub. Each step returns (exit,
// done) so an early exit keeps its code.
package main

import (
	"fmt"

	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/git/githubclient"
	"github.com/Mpaape/AurumCode/internal/review"
	"github.com/Mpaape/AurumCode/internal/review/session"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// prReview is the pull-request source of a review session: the shared
// reviewState plus what only a pull request has.
type prReview struct {
	reviewState
	opts prReviewOptions

	prNumber         int
	repoFlag         string
	owner, repoName  string
	publicar, inline bool // inline: --na-linha as given
	check            bool

	client *githubclient.Client

	publication        string
	inlineComments     bool
	codebaseLimitation string
	verifiedDir        string
	checkoutMismatch   string
	contextRef         string
	reviewer           *review.Reviewer
	history            string
	historyErr         error
	gateDeclared       bool

	// publication
	issues   []types.ReviewIssue
	commitID string
}

// Step maps each session phase onto this source's steps.
func (p *prReview) Step(phase session.Phase) session.Step {
	switch phase {
	case session.PhaseResolve:
		return p.resolve
	case session.PhaseModel:
		return p.runModelPass
	case session.PhaseEvidence:
		return p.collectEvidence
	case session.PhaseGate:
		return p.runGate
	}
	return p.publish
}

// runPRReview is reached only when --pr was explicitly given (see the
// fs.Visit dispatch in runReview); every other flag's published behavior
// is therefore untouched by this function's existence. --repo and
// --publicar are always required; see validate.
func runPRReview(rio reviewIO, opts prReviewOptions) int {
	p := &prReview{
		reviewState: newReviewState(session.PullRequest, rio), opts: opts,
		prNumber: opts.prNumber, repoFlag: opts.repo, publicar: opts.publicar, inline: opts.naLinha, check: opts.check,
	}
	p.policyDir, p.modelFlag, p.seguranca, p.exigirQualidade = opts.policyDir, opts.modelo, opts.seguranca, opts.exigirQualidade
	p.auditoriaPath, p.sarifPath = opts.auditoriaPath, opts.sarifPath
	defer p.flush()
	return session.Run(p)
}

// resolve validates the invocation, then resolves the inputs.
func (p *prReview) resolve() (int, bool) {
	if code, done := p.validate(); done {
		return code, true
	}
	return p.resolveInputs()
}

// validate refuses a malformed invocation before any permission check,
// diff fetch or model call (AUR-438/451): every refusal is a usage error.
func (p *prReview) validate() (int, bool) {
	stderr, opts := p.stderr, p.opts
	if p.repoFlag == "" {
		fmt.Fprintln(stderr, "aurumcode review: --repo is required with --pr")
		return 2, true
	}
	// Keep the legacy command spelling stable. A caller that wants the new
	// formal mode explicitly selects it with --modo-publicacao review; the
	// historical comments command still requires --na-linha unless --check
	// is the requested publication.
	if !p.inline && !p.check && !opts.publicationSet {
		fmt.Fprintln(stderr, "aurumcode review: --na-linha is required with --pr in the legacy comments mode; use --modo-publicacao review for a formal review")
		return 2, true
	}
	if !p.publicar {
		fmt.Fprintln(stderr, "aurumcode review: --publicar is required with --pr")
		return 2, true
	}
	if p.prNumber <= 0 {
		fmt.Fprintln(stderr, "aurumcode review: --pr must be a positive pull request number")
		return 2, true
	}
	var err error
	p.owner, p.repoName, err = parseOwnerRepo(p.repoFlag)
	if err != nil {
		fmt.Fprintf(stderr, "aurumcode review: --repo: %v\n", err)
		return 2, true
	}
	// The repository identity is the verified "owner/repo" of the pull
	// request, never anything derived from its author-controlled diff or
	// checkout.
	p.repoIdentity, p.repoIdentityKnown = p.owner+"/"+p.repoName, true
	p.artifactRepo = p.repoIdentity
	return p.validateFlags()
}

// validateFlags mirrors the --base path's validation of --fail-on, --modelo,
// --limite and --modo-publicacao: an explicitly empty or unparsable value
// is a usage error, never a silently disabled flag (AUR-451).
func (p *prReview) validateFlags() (int, bool) {
	stderr, opts := p.stderr, p.opts
	if opts.failOnSet {
		var err error
		p.threshold, p.thresholdName, err = parseFailOnLevel(opts.failOn)
		if err != nil {
			fmt.Fprintf(stderr, "aurumcode review: %v\n", err)
			return 2, true
		}
	}
	if opts.modeloSet && opts.modelo == "" {
		fmt.Fprintln(stderr, "aurumcode review: --modelo: model name must not be empty")
		return 2, true
	}
	if opts.limiteSet {
		if opts.limite == "" {
			fmt.Fprintln(stderr, "aurumcode review: --limite: value must not be empty")
			return 2, true
		}
		var err error
		p.limiteUSD, err = parseLimiteUSD(opts.limite)
		if err != nil {
			fmt.Fprintf(stderr, "aurumcode review: %v\n", err)
			return 2, true
		}
	}
	if opts.publicationSet {
		if _, err := config.NormalizeReviewPublication(opts.publication); err != nil {
			fmt.Fprintf(stderr, "aurumcode review: --modo-publicacao: %v\n", err)
			return 2, true
		}
	}
	return 0, false
}
