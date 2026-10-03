// The state and the steps both review sources share. A source (--base's
// local diff, --pr's verified pull request) embeds reviewState, fills it in
// its own resolve and model phases, and hands it to the same evidence, gate
// and exit steps; the phase order itself is internal/review/session's.
package main

import (
	"context"
	"io"
	"os"
	"time"

	"github.com/Mpaape/AurumCode/internal/config"
	codebasectx "github.com/Mpaape/AurumCode/internal/context"
	"github.com/Mpaape/AurumCode/internal/llm"
	"github.com/Mpaape/AurumCode/internal/llm/cost"
	"github.com/Mpaape/AurumCode/internal/memory"
	"github.com/Mpaape/AurumCode/internal/prompt"
	"github.com/Mpaape/AurumCode/internal/review"
	"github.com/Mpaape/AurumCode/internal/review/session"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// reviewEnv is the process environment a review reads, captured once at
// the command's edge so no phase reads os.Getenv on its own.
type reviewEnv struct {
	githubSHA      string // GITHUB_SHA: the reviewed commit
	baseSHA        string // AURUMCODE_BASE_SHA: the trusted base commit
	repository     string // GITHUB_REPOSITORY
	llmModel       string // LLM_MODEL
	outputFile     string // AURUMCODE_OUTPUT_FILE
	permissionMode string // AURUMCODE_PR_PERMISSION_MODE
}

// readReviewEnv snapshots the environment.
func readReviewEnv() reviewEnv {
	return reviewEnv{
		githubSHA:      os.Getenv("GITHUB_SHA"),
		baseSHA:        os.Getenv("AURUMCODE_BASE_SHA"),
		repository:     os.Getenv("GITHUB_REPOSITORY"),
		llmModel:       os.Getenv("LLM_MODEL"),
		outputFile:     os.Getenv("AURUMCODE_OUTPUT_FILE"),
		permissionMode: os.Getenv("AURUMCODE_PR_PERMISSION_MODE"),
	}
}

// semgrepRunner executes Semgrep in dir.
type semgrepRunner = func(ctx context.Context, dir string, args ...string) (stdout, stderr string, err error)

// codebaseResolver reads the codebase context of exactly files under dir.
type codebaseResolver = func(resolver *codebasectx.Resolver, dir string, changed, files []string) (*codebasectx.Pack, error)

// reviewDeps are the collaborators a review session is given instead of
// reaching for package state: the clock exceptions are judged against, the
// SAST scanner, an observer of the gate pipeline, the codebase resolver,
// the prompt builder whose fixed content versions the caches, and the
// environment snapshot. A zero field takes its production default.
type reviewDeps struct {
	clock         func() time.Time
	semgrep       semgrepRunner
	gateObserver  func(label string, names []string)
	resolveFiles  codebaseResolver
	digestBuilder func() *prompt.PromptBuilder
	env           *reviewEnv
}

// withDefaults fills every unset dependency with production's.
func (d reviewDeps) withDefaults() reviewDeps {
	if d.clock == nil {
		d.clock = time.Now
	}
	if d.semgrep == nil {
		d.semgrep = realSemgrepRunner
	}
	if d.gateObserver == nil {
		d.gateObserver = func(string, []string) {}
	}
	if d.resolveFiles == nil {
		d.resolveFiles = func(resolver *codebasectx.Resolver, dir string, changed, files []string) (*codebasectx.Pack, error) {
			return resolver.ResolveWithFiles(dir, changed, files)
		}
	}
	if d.digestBuilder == nil {
		d.digestBuilder = prompt.NewPromptBuilder
	}
	if d.env == nil {
		env := readReviewEnv()
		d.env = &env
	}
	return d
}

// reviewIO is where a review writes and the dependencies it runs with.
type reviewIO struct {
	stdout, stderr io.Writer
	filter         *redaction.Filter
	deps           reviewDeps
}

// reviewState is everything both sources compute, under one name each.
type reviewState struct {
	source session.Source
	ctx    context.Context
	stdout io.Writer
	stderr io.Writer
	filter *redaction.Filter
	deps   reviewDeps
	run    *gateRun // flushed when the session ends

	// invocation
	policyDir                    string
	threshold                    int
	thresholdName                string
	limiteUSD                    float64
	modelFlag                    string
	seguranca, exigirQualidade   bool
	auditoriaPath, sarifPath     string
	repoIdentity                 string
	repoIdentityKnown            bool
	artifactRepo, artifactCommit string

	// resolved inputs
	diff                *types.Diff
	cfg                 *config.Config
	centralCfg          *config.Config
	reviewLanguage      string
	policyWarnings      []config.ProviderWarning
	skillNotices        []string
	ignoredPaths        []string
	rawDiffFileCount    int
	changelogText       string
	changelogLimitation string
	codebaseText        string
	memoryStore         memory.Store
	memoryNotes         []memory.Note
	memoryNotesText     string
	profileIdentity     string

	// model pass
	provider           llm.Provider
	baseModelIdentity  string
	contextBlockDigest string
	dynamicRules       map[string]review.Rule
	ruleCatalogIDs     []string
	ruleCatalogDigest  string
	tracker            *cost.Tracker
	result             *types.ReviewResult
	model              modelOutcome

	// evidence, collected before the model and offered to it
	securityFindings []types.ReviewIssue
	securityApplied  []string
	securityTotal    int
	analysisIssues   []types.ReviewIssue
	evidence         []prompt.EvidenceItem
	evidenceIDs      map[string]string // evidenceKey -> id shown in the prompt
	// repositoryContext is the configured context providers' block, sent in
	// its own prompt slot.
	repositoryContext  string
	proposedExceptions string
	rawIssues          []types.ReviewIssue // verdict-reuse snapshot, before rule config
	sastOrigin         string
	sastIssues         []types.ReviewIssue
	sastReason         string
	coverage           reviewCoverageBreakdown

	gateRes *gateDecision
}

// newReviewState starts the shared state of one session.
func newReviewState(source session.Source, rio reviewIO) reviewState {
	deps := rio.deps.withDefaults()
	return reviewState{
		source: source, ctx: context.Background(),
		stdout: rio.stdout, stderr: rio.stderr, filter: rio.filter,
		deps: deps, run: &gateRun{},
	}
}

// env is the environment snapshot taken at the edge.
func (s *reviewState) env() reviewEnv { return *s.deps.env }

// flush drains the redaction writers a gate contributor installed.
func (s *reviewState) flush() { s.run.Flush() }

// modelDegraded reports a model pass that produced no trustworthy answer.
func (s *reviewState) modelDegraded() bool {
	return s.model == modelProviderFailed || s.model == modelParseFailed
}

// qualityRequirement is --exigir-qualidade as the gate's typed value.
func (s *reviewState) qualityRequirement() qualityRequirement {
	if s.exigirQualidade {
		return qualityRequired
	}
	return qualityOptional
}

// notReviewed applies this source's outcome table.
func (s *reviewState) notReviewed() bool {
	return s.source.NotReviewed.NotReviewed(s.model, s.qualityRequirement())
}
