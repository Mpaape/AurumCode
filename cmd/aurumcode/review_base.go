// The --base review path (AUR-430), split into explicit phases: resolve the
// inputs (review_base_inputs.go), run the analyses (review_base_analysis.go
// and review_base_quality.go), run the shared gate pipeline
// (review_base_gate.go) and publish/exit (review_base_publish.go). The
// state every phase shares lives in baseReview; each phase returns
// (exit code, done) so an early `return N` of the original single function
// stays an identical early return.
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Mpaape/AurumCode/internal/analyzer"
	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/llm"
	"github.com/Mpaape/AurumCode/internal/llm/cost"
	"github.com/Mpaape/AurumCode/internal/memory"
	"github.com/Mpaape/AurumCode/internal/review"
	"github.com/Mpaape/AurumCode/internal/reviewprofile"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// reviewFlags is the parsed command line of `review`. given records which
// flags were explicitly set (fs.Visit), so an explicitly empty value is
// distinguished from an absent one.
type reviewFlags struct {
	base, failOn, modelo, repo, modoPublicacao, limite string
	perfis, perfil, politica, policy, auditoria, sarif string
	seguranca, publicar, naLinha, check                bool
	exigirQualidade, changelog                         bool
	pr                                                 int
	given                                              map[string]bool
}

// parseReviewFlags parses args. ok is false when the command must return
// exit now (help requested, or a usage error).
func parseReviewFlags(args []string, stdout, stderr io.Writer) (f *reviewFlags, exit int, ok bool) {
	// An explicitly requested --help is a fulfilled request (stdout, exit
	// 0); a genuine usage error goes to stderr, exit 2 (AUR-443).
	var helpBuf bytes.Buffer
	fs := flag.NewFlagSet("review", flag.ContinueOnError)
	fs.SetOutput(&helpBuf)
	f = &reviewFlags{given: map[string]bool{}}
	fs.StringVar(&f.base, "base", "", "ref to diff against HEAD (required), e.g. HEAD~1 or a branch name")
	fs.StringVar(&f.failOn, "fail-on", "", "minimum severity that makes the command exit 3: high|error, medium|warning, low|info (default: findings never change the exit code)")
	fs.StringVar(&f.modelo, "modelo", "", "model id that reviews; served offline via AURUMCODE_LLM_FIXTURE or live via LLM_API_KEY and LLM_BASE_URL (default: the endpoint's configured model)")
	fs.BoolVar(&f.seguranca, "seguranca", false, "additionally run the project's security pass: match the diff's added lines against the security rules of the embedded catalog (standards/security-review) and print the findings in their own section (default: off, output unchanged)")
	fs.IntVar(&f.pr, "pr", 0, "pull request number to review (AUR-438); activates the PR path and requires --repo and --publicar (default: off, --base path unchanged)")
	fs.StringVar(&f.repo, "repo", "", "owner/repo of the pull request; required with --pr")
	fs.BoolVar(&f.publicar, "publicar", false, "publish the review on the pull request; required with --pr (default: off)")
	fs.BoolVar(&f.naLinha, "na-linha", false, "include eligible findings as comments on exact changed lines; optional in both publication modes (default: off)")
	fs.StringVar(&f.modoPublicacao, "modo-publicacao", "", "PR publication mode: review (formal GitHub review) or comments (separate comments; default: repository config, otherwise comments)")
	fs.StringVar(&f.limite, "limite", "", "maximum USD this run may spend calling the model; the command estimates the cost before calling it and refuses -- spending nothing -- when the estimate exceeds this value (default: no limit enforced)")
	fs.BoolVar(&f.check, "check", false, "publish a commit status (AUR-439) that fails when a grave (error-severity) finding is present, blocking the pull request's merge (default: off)")
	fs.BoolVar(&f.exigirQualidade, "exigir-qualidade", false, "treat a quality review that did not happen as a failure: exit 1 even when deterministic analysis ran and reported, so a CI job cannot read \"security-only\" as \"fully reviewed\" (default: off -- the published --seguranca-without-a-provider path keeps exit 0)")
	fs.BoolVar(&f.changelog, "changelog", false, "publish the suggested next version and changelog entry derived from the reviewed commit messages (AUR-498 engine); the review.changelog repository setting is the default (default: off)")
	fs.StringVar(&f.perfis, "perfis", "", "comma-separated reviewer profiles to run in the same review (AUR-502); every finding names its source profile and duplicate findings merge once. Profiles are presets over emphasis and rule families only and never change severity, --fail-on, redaction, the cost cap or the security pass (default: review.profiles from config)")
	fs.StringVar(&f.perfil, "profile", "", "alias of --perfis for a single reviewer profile (AUR-502)")
	fs.StringVar(&f.politica, "politica", "", "directory containing a central policy's .aurumcode/config.yml (the directory that HOLDS .aurumcode/, same convention as a repository's own config.yml/skills); its rules, ignore patterns and, when set, review language/publication take precedence over this repository's own (AUR-518; default: the AURUMCODE_POLICY environment variable, otherwise no policy)")
	fs.StringVar(&f.policy, "policy", "", "alias of --politica")
	fs.StringVar(&f.auditoria, "auditoria", "", "path to write AUR-521's compliance audit record (JSON) for this run: policy digest, workflow/reviewed SHA, model, verdict, gate decision, blocking findings, exceptions applied and coverage (default: no audit record written)")
	fs.StringVar(&f.sarif, "sarif", "", "path to write AUR-521's SARIF 2.1.0 document for this run, for a workflow to upload to GitHub code scanning (default: no SARIF written)")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			io.Copy(stdout, &helpBuf) //nolint:errcheck // best-effort; nothing left to report to on failure
			return nil, 0, false
		}
		io.Copy(stderr, &helpBuf) //nolint:errcheck // best-effort; nothing left to report to on failure
		return nil, 2, false
	}
	// Detected via fs.Visit, never by scanning args for a "--" prefix: Go
	// treats "-pr" and "--pr" identically (docs/specs/AUR-438.md).
	fs.Visit(func(fl *flag.Flag) { f.given[fl.Name] = true })
	return f, 0, true
}

// resolvePolicyDir picks the central policy directory (AUR-518):
// --politica/--policy, else AURUMCODE_POLICY, else "". An explicitly empty
// value is a usage error, never a silent fallback.
func (f *reviewFlags) resolvePolicyDir(stderr io.Writer) (dir string, exit int, ok bool) {
	dir = strings.TrimSpace(f.politica)
	if dir == "" {
		dir = strings.TrimSpace(f.policy)
	}
	politicaGiven := f.given["politica"] || f.given["policy"]
	if politicaGiven && dir == "" {
		fmt.Fprintln(stderr, "aurumcode review: --politica/--policy: directory must not be empty")
		return "", 2, false
	}
	if !politicaGiven {
		dir = strings.TrimSpace(os.Getenv("AURUMCODE_POLICY"))
	}
	return dir, 0, true
}

// prOptions maps the flags onto the --pr path's options.
func (f *reviewFlags) prOptions(policyDir string) prReviewOptions {
	return prReviewOptions{
		seguranca:       f.seguranca,
		failOnSet:       f.given["fail-on"],
		failOn:          f.failOn,
		modeloSet:       f.given["modelo"],
		modelo:          f.modelo,
		limiteSet:       f.given["limite"],
		limite:          f.limite,
		publicationSet:  f.given["modo-publicacao"],
		publication:     f.modoPublicacao,
		changelog:       f.changelog,
		exigirQualidade: f.exigirQualidade,
		policyDir:       policyDir,
		auditoriaPath:   f.auditoria,
		sarifPath:       f.sarif,
	}
}

// baseReview is the state of one --base run, shared by every phase.
type baseReview struct {
	f              *reviewFlags
	stdout, stderr io.Writer
	filter         *redaction.Filter
	run            *gateRun // flushed by runReview when the run ends

	policyDir     string
	threshold     int
	thresholdName string
	limiteSet     bool
	limiteUSD     float64

	cwd              string
	diff             *types.Diff
	notices          []analyzer.DiffNotice
	rawDiffFileCount int
	ignoredPaths     []string
	repoCfg          *config.Config
	centralCfg       *config.Config
	policyWarnings   []config.ProviderWarning
	reviewLanguage   string

	profileRes      *reviewprofile.MultiResult
	profilesApplied bool
	profileIdentity string

	codebaseContextText string
	memoryStore         memory.Store
	memoryNotes         []memory.Note
	memoryNotesText     string
	changelogText       string
	changelogLimitation string

	provider     llm.Provider
	providerErr  error
	providerVia  string
	tracker      *cost.Tracker
	result       *types.ReviewResult
	dynamicRules map[string]review.Rule

	ruleCatalogIDs     []string
	ruleCatalogDigest  string
	contextBlockDigest string
	baseModelIdentity  string

	qualitySkipped bool
	qualityFailed  bool

	securityFindings []types.ReviewIssue
	rawIssues        []types.ReviewIssue // AUR-524 v2 snapshot, before rule config
	sastIssues       []types.ReviewIssue
	sastReason       string
	sastOrigin       string

	coverage     reviewCoverageBreakdown
	coverageText string

	gateRes *gateDecision
}

// flush drains the redaction writers a gate contributor installed.
func (b *baseReview) flush() { b.run.Flush() }

// runReview is the --base entry point (and the --pr dispatcher).
func runReview(args []string, stdout, stderr io.Writer, filter *redaction.Filter) int {
	f, exit, ok := parseReviewFlags(args, stdout, stderr)
	if !ok {
		return exit
	}
	policyDir, exit, ok := f.resolvePolicyDir(stderr)
	if !ok {
		return exit
	}
	if f.given["pr"] {
		// AUR-502: --perfis selects the --base passes only; refused loudly
		// with --pr as a usage error.
		if f.given["perfis"] || f.given["profile"] {
			fmt.Fprintln(stderr, "aurumcode review: --perfis/--profile are not supported with --pr (they select the --base review passes only)")
			return 2
		}
		return runPRReview(stdout, stderr, f.pr, f.repo, f.publicar, f.naLinha, f.check, filter, f.prOptions(policyDir))
	}
	b := &baseReview{f: f, stdout: stdout, stderr: stderr, filter: filter, policyDir: policyDir, run: &gateRun{}}
	defer b.flush()
	for _, phase := range []func() (int, bool){b.resolveInputs, b.analyze, b.decideGate, b.publish} {
		if code, done := phase(); done {
			return code
		}
	}
	return 0
}
