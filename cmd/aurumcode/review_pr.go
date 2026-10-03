// The --pr review path as explicit phases: validate the invocation, resolve
// its inputs (diff, effective config, checkout, memory), run the analyses
// (model, deterministic passes, SAST, coverage), run the shared gate
// pipeline (review_gate.go), then publish and decide the exit code. State
// the phases share lives in prReview; each phase returns (exit, done) so an
// early exit keeps the exact code the single function used to return.
package main

import (
	"context"
	"fmt"
	"io"

	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/git/githubclient"
	"github.com/Mpaape/AurumCode/internal/llm"
	"github.com/Mpaape/AurumCode/internal/llm/cost"
	"github.com/Mpaape/AurumCode/internal/memory"
	"github.com/Mpaape/AurumCode/internal/review"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// prReview is the state of one --pr run.
type prReview struct {
	ctx            context.Context
	stdout, stderr io.Writer
	filter         *redaction.Filter
	opts           prReviewOptions

	prNumber         int
	repoFlag         string
	owner, repoName  string
	publicar, inline bool // inline: --na-linha as given
	check            bool

	threshold     int
	thresholdName string
	limiteUSD     float64

	client *githubclient.Client

	// resolved inputs
	diff                *types.Diff
	cfg                 *config.Config
	reviewLanguage      string
	centralCfg          *config.Config
	policyWarnings      []config.ProviderWarning
	publication         string
	inlineComments      bool
	ignoredPaths        []string
	rawDiffFileCount    int
	changelogText       string
	changelogLimitation string
	codebaseText        string
	codebaseLimitation  string
	verifiedDir         string
	checkoutMismatch    string
	memoryStore         memory.Store
	memoryNotes         []memory.Note
	memoryNotesText     string

	// model and context
	provider          llm.Provider
	baseModelIdentity string
	contextRef        string
	contextBlockDig   string
	dynamicRules      map[string]review.Rule
	ruleCatalogIDs    []string
	ruleCatalogDig    string
	tracker           *cost.Tracker
	reviewer          *review.Reviewer
	history           string
	historyErr        error

	// analysis outputs
	result            *types.ReviewResult
	qualityDegraded   bool
	providerFailed    bool
	gateDeclared      bool
	securityFindings  []types.ReviewIssue
	rawIssuesSnapshot []types.ReviewIssue
	sastOrigin        string
	sastIssues        []types.ReviewIssue
	sastReason        string
	coverage          reviewCoverageBreakdown

	// gate
	run     *gateRun
	gateRes *gateDecision

	// publication
	issues   []types.ReviewIssue
	commitID string
}

// runPRReview is reached only when --pr was explicitly given (see the
// fs.Visit dispatch in runReview); every other flag's published behavior
// is therefore untouched by this function's existence. --repo and
// --publicar are always required; see validate.
func runPRReview(stdout, stderr io.Writer, prNumber int, repoFlag string, publicar, naLinha, check bool, filter *redaction.Filter, opts prReviewOptions) int {
	p := &prReview{
		ctx: context.Background(), stdout: stdout, stderr: stderr, filter: filter, opts: opts,
		prNumber: prNumber, repoFlag: repoFlag, publicar: publicar, inline: naLinha, check: check,
	}
	defer func() {
		if p.run != nil {
			p.run.Flush()
		}
	}()
	phases := []func() (int, bool){
		p.validate, p.resolveInputs, p.analyze, p.runGate, p.publish,
	}
	for _, phase := range phases {
		if code, done := phase(); done {
			return code
		}
	}
	return 0
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
