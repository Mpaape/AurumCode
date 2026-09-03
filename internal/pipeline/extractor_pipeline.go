package pipeline

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Mpaape/AurumCode/internal/documentation/extractors"
	"github.com/Mpaape/AurumCode/internal/documentation/incremental"
	"github.com/Mpaape/AurumCode/internal/documentation/normalizer"
	docsreview "github.com/Mpaape/AurumCode/internal/documentation/review"
	"github.com/Mpaape/AurumCode/internal/documentation/site"
	"github.com/Mpaape/AurumCode/internal/documentation/welcome"
	"github.com/Mpaape/AurumCode/internal/llm"
)

// SkipReason classifies why a detected language produced no documentation without
// that counting as an extraction failure.
type SkipReason string

// Skip reasons reported by the pipeline.
const (
	SkipNoExtractor      SkipReason = "no extractor registered"
	SkipToolUnavailable  SkipReason = "required tool not in PATH"
	SkipValidationFailed SkipReason = "extractor validation failed"
)

// LanguageSkip records a language the pipeline deliberately did not extract, along
// with the reason and how many source files went undocumented because of it.
type LanguageSkip struct {
	Language extractors.Language
	Reason   SkipReason
	Tool     string
	Detail   string
	Files    int
}

func (s LanguageSkip) String() string {
	msg := fmt.Sprintf("%s: %s", s.Language, s.Reason)
	// Detail carries external tool output. It is redacted and bounded here as
	// well as at the assignment site: every path that renders a skip is a path
	// into the public Action log.
	if s.Detail != "" {
		msg += " (" + site.Redact(s.Detail) + ")"
	}

	return fmt.Sprintf("%s [%d file(s)]", msg, s.Files)
}

// ExtractionError reports an extraction run that did not fully succeed. Partial is
// false when no documentation at all was produced and true when documentation was
// produced but at least one language failed or was skipped.
type ExtractionError struct {
	Partial        bool
	SourceFiles    int
	FilesProcessed int
	DocsGenerated  int
	Errors         []error
	Skipped        []LanguageSkip
}

func (e *ExtractionError) Error() string {
	outcome := "produced no documentation"
	if e.Partial {
		outcome = "was partial"
	}

	msg := fmt.Sprintf("documentation extraction %s: %d source files, %d processed, %d docs generated, %d error(s), %d language(s) skipped",
		outcome, e.SourceFiles, e.FilesProcessed, e.DocsGenerated, len(e.Errors), len(e.Skipped))

	if len(e.Skipped) > 0 {
		skips := make([]string, 0, len(e.Skipped))
		for _, skip := range e.Skipped {
			skips = append(skips, skip.String())
		}
		msg += " {skipped: " + strings.Join(skips, "; ") + "}"
	}

	if len(e.Errors) == 0 {
		return msg
	}

	causes := make([]string, 0, len(e.Errors))
	for _, err := range e.Errors {
		// Causes wrap extractor errors that embed tool stderr.
		causes = append(causes, site.Redact(err.Error()))
	}

	return msg + ": " + strings.Join(causes, "; ")
}

// Unwrap exposes the individual extractor failures to errors.Is and errors.As.
func (e *ExtractionError) Unwrap() []error { return e.Errors }

// ExtractorPipelineConfig configures the documentation extraction pipeline
type ExtractorPipelineConfig struct {
	SourceDir       string   // Root directory of source code
	OutputDir       string   // Output directory for documentation
	DocsDir         string   // Jekyll docs directory (e.g., "docs/")
	Languages       []string // Languages to extract (empty = all)
	Incremental     bool     // Enable incremental mode
	GenerateWelcome bool     // Generate LLM-powered welcome page
	// GenerateDocsReview runs the bounded editorial pass after the deterministic
	// site scaffold. It never rewrites API pages.
	GenerateDocsReview bool
	// DocsReviewRequired turns a missing provider or failed editorial pass into
	// a failed run. Automatic mode leaves deterministic docs available.
	DocsReviewRequired bool
	DocsReviewLanguage string
	ValidateJekyll     bool // Validate Jekyll site after generation
	DeployGHPages      bool // Deploy to gh-pages branch

	// BaseURL is the path a project Pages site is published under, e.g.
	// "/my-repo" for owner.github.io/my-repo/. It is normalized before use.
	BaseURL string

	// BaseURLDeclared records whether the caller supplied BaseURL at all.
	//
	// It exists because "" is a meaningful value: a site on a custom domain
	// publishes at the root, and nothing in the CI environment says so, so the
	// caller must be able to declare the root and outrank the derivation below.
	// Without this flag, "publish at the root" and "decide for me" are the same
	// bytes and the custom-domain case is unreachable.
	BaseURLDeclared bool

	// IncludeTests, when true, disables the default exclusion of test and
	// fixture files (see IsTestScopePath) from documentation scope: the
	// pipeline documents the repository's own test code the same as it
	// always did before AUR-483.
	//
	// The default (false) is deliberate: on an unconfigured repository the
	// generated site should describe the product, not enumerate its test
	// scaffolding (AUR-483). A caller that explicitly wants test files
	// documented sets this field to true, or sets the environment variable
	// named by envIncludeTestDocs -- read directly by this package so the
	// capability reaches every entrypoint that runs this pipeline without
	// each one having to wire a new flag.
	IncludeTests bool
}

// envIncludeTestDocs is the AURUMCODE_-prefixed override that restores the
// pre-AUR-483 behaviour of documenting test and fixture files. It is read
// directly by this package (see IncludeTests) rather than plumbed through
// every caller, so the opt-in reaches cmd/regenerate-docs and `aurumcode
// docs` alike without either needing a code change of its own.
const envIncludeTestDocs = "AURUMCODE_INCLUDE_TEST_DOCS"

// IncludeTests reports whether test/fixture files should be documented:
// either the caller set config.IncludeTests explicitly, or the operator set
// envIncludeTestDocs in the process environment.
func (p *ExtractorPipeline) IncludeTests() bool {
	if p.config.IncludeTests {
		return true
	}
	return envFlagEnabled(os.Getenv(envIncludeTestDocs))
}

// envFlagEnabled parses an AURUMCODE_-style boolean environment variable.
// Unset or unrecognized values are "false" -- the exclusion this card adds
// stays the default until an operator opts out in a form this recognizes.
func envFlagEnabled(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// testScopeComponents lists path components that mark a file as test or
// fixture scope for AUR-483's default exclusion.
//
// Matching happens on a WHOLE path component (see IsTestScopePath), never on
// a substring of the path: internal/attestation and cmd/latest each contain
// the letters "test" but neither has a path component that equals one of
// these names, so neither is scope-excluded. A substring match would swallow
// legitimate product code; this repository's own internal/attestation and
// cmd/latest are exactly that trap (AUR-483 AC-003).
var testScopeComponents = map[string]struct{}{
	"tests":    {},
	"testdata": {},
	"fixtures": {},
}

// IsTestScopePath reports whether path is test or fixture scope: some
// directory component of path is exactly "tests", "testdata" or "fixtures",
// or the file's own name (stem, extension stripped) ends in "_test".
//
// Both checks compare a whole path component/stem, never a substring, so
// contest.go, internal/attestation and cmd/latest are never caught: their
// names merely contain the letters "test", they do not equal or end in it as
// a distinct component.
func IsTestScopePath(path string) bool {
	clean := filepath.ToSlash(filepath.Clean(path))
	dir, base := filepath.Split(clean)

	stem := strings.TrimSuffix(base, filepath.Ext(base))
	if strings.HasSuffix(stem, "_test") {
		return true
	}

	for _, component := range strings.Split(dir, "/") {
		if _, skip := testScopeComponents[component]; skip {
			return true
		}
	}

	return false
}

// buildScopedSourceView copies sourceDir into a fresh temporary directory,
// excluding shouldSkipPath's existing directories and AUR-483's test/fixture
// scope (IsTestScopePath), and returns the copy's root plus a cleanup func
// that removes it.
//
// A copy, not a symlink tree: some extractors' own directory walks (e.g.
// go/build's package matcher) may not treat a symlink DirEntry as a regular
// file, which would silently under-document the mirror. A real copy makes
// the mirror behave identically to a real checkout for every extractor,
// with no per-extractor code to touch or trust.
//
// Only the file CONTENT and its path relative to sourceDir matter to a
// caller here: every extractor computes relative paths via
// filepath.Rel(req.SourceDir, ...) for its own output naming, so a
// same-shaped mirror produces byte-identical output filenames to the real
// tree it stands in for.
func buildScopedSourceView(sourceDir string) (root string, cleanup func(), err error) {
	mirror, err := os.MkdirTemp("", "aurumcode-docscope-*")
	if err != nil {
		return "", func() {}, err
	}
	cleanup = func() { _ = os.RemoveAll(mirror) }

	walkErr := filepath.Walk(sourceDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil || path == sourceDir {
			return nil
		}

		if shouldSkipPath(path) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		rel, relErr := filepath.Rel(sourceDir, path)
		if relErr != nil {
			return nil
		}

		if info.IsDir() {
			if _, skip := testScopeComponents[info.Name()]; skip {
				return filepath.SkipDir
			}
			return os.MkdirAll(filepath.Join(mirror, rel), 0o755)
		}

		if IsTestScopePath(rel) {
			return nil
		}

		dest := filepath.Join(mirror, rel)
		if mkErr := os.MkdirAll(filepath.Dir(dest), 0o755); mkErr != nil {
			return mkErr
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		return os.WriteFile(dest, data, 0o644)
	})
	if walkErr != nil {
		cleanup()
		return "", func() {}, walkErr
	}

	return mirror, cleanup, nil
}

// ErrBasePathConflict reports that the caller and the on-disk _config.yml
// disagree about where the site is published.
var ErrBasePathConflict = errors.New("base path conflict")

// normalizeBasePath reduces whatever a caller supplied to either "" (root) or
// "/segment[/segment...]".
//
// site.Scaffold.applyBaseURL only trims one trailing slash, so every other shape
// has to be resolved here. The normalization deliberately does NOT live in the
// scaffold: that package round-trips hostile values on purpose to prove its YAML
// escaping, and sanitizing its input would delete that coverage.
//
// The absolute-URL case matters because actions/configure-pages emits a full
// "https://owner.github.io/repo" as its base_url output, and a consumer will
// wire that output straight into this input. The protocol-relative case matters
// because "//host" is not a path at all: a browser reads it as another origin
// and leaves the site entirely. Both are reduced to a path on this site.
func normalizeBasePath(raw string) string {
	trimmed := strings.TrimSpace(raw)

	if u, err := url.Parse(trimmed); err == nil && u.Scheme != "" && u.Host != "" {
		trimmed = u.Path
	}

	// One Trim handles "", "/", "//", "/x/", "//x//" and a bare "x" alike.
	trimmed = strings.Trim(trimmed, "/")
	if trimmed == "" {
		return ""
	}

	return "/" + trimmed
}

// deriveBasePathFromRepository guesses the base path from GITHUB_REPOSITORY.
//
// It is a guess, which is why it is the lowest-priority source: correct for a
// project site (owner.github.io/repo/), correct for a user or organisation site
// (owner.github.io/, no prefix), and wrong for a custom domain, about which the
// environment says nothing at all.
func deriveBasePathFromRepository(repository string) string {
	owner, name, found := strings.Cut(strings.TrimSpace(repository), "/")
	if !found || owner == "" || name == "" {
		return ""
	}

	// A user/org site is served from the domain root, so it takes no prefix.
	if strings.EqualFold(name, owner+".github.io") {
		return ""
	}

	return normalizeBasePath(name)
}

// configuredBaseURL reads the baseurl key out of an existing _config.yml.
//
// A missing key and a key set to "" are different answers: the latter is a
// consumer stating that the site is published at the root, which is the only
// signal a custom-domain deployment can give from disk.
func configuredBaseURL(configPath string) (value string, declared bool) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return "", false
	}

	var parsed struct {
		BaseURL *string `yaml:"baseurl"`
	}
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		// An unparseable consumer config is not this pipeline's to reject; it
		// simply carries no usable answer.
		return "", false
	}

	if parsed.BaseURL == nil {
		return "", false
	}

	return *parsed.BaseURL, true
}

// resolveBasePath applies the precedence ladder that decides where the site is
// published:
//
//  1. what the caller declared (AURUMCODE_BASE_URL / the action's base-path
//     input) - the operator's intent, and the only channel that can say "root"
//     on a custom domain;
//  2. a baseurl already present in the consumer's _config.yml - that file is
//     what Jekyll actually reads, and writeConfig never overwrites it, so it is
//     a fact rather than a guess;
//  3. a derivation from GITHUB_REPOSITORY - a guess, right for the default
//     GitHub Pages project-site case that used to 404 on every link;
//  4. "" - the root, which is also what a local run with no CI environment gets,
//     so behaviour off CI is unchanged.
//
// Rung 3 losing to rung 2 is silent by design: disk is fact, derivation is
// guess. Rungs 1 and 2 disagreeing is not silent - the two are both assertions
// of intent, and publishing under one while the theme resolves assets through
// the other yields a broken site with a green run.
func (p *ExtractorPipeline) resolveBasePath(docsDir string) (string, error) {
	onDisk, onDiskDeclared := configuredBaseURL(filepath.Join(docsDir, "_config.yml"))

	if p.config.BaseURLDeclared {
		declared := normalizeBasePath(p.config.BaseURL)

		if onDiskDeclared && normalizeBasePath(onDisk) != declared {
			return "", fmt.Errorf("%w: caller declared base path %q but %s already declares %q; "+
				"reconcile them, or drop the caller's value to keep the one on disk",
				ErrBasePathConflict, p.config.BaseURL,
				filepath.ToSlash(filepath.Join(docsDir, "_config.yml")), onDisk)
		}

		return declared, nil
	}

	if onDiskDeclared {
		return normalizeBasePath(onDisk), nil
	}

	return deriveBasePathFromRepository(os.Getenv("GITHUB_REPOSITORY")), nil
}

// ExtractorPipeline orchestrates complete documentation extraction and site generation
type ExtractorPipeline struct {
	config         *ExtractorPipelineConfig
	registry       *extractors.Registry
	runner         site.CommandRunner
	incrementalMgr *incremental.Manager
	normalizer     *normalizer.Normalizer
	welcomeGen     *welcome.Generator
	docsReviewer   *docsreview.Reviewer
	llmOrch        *llm.Orchestrator

	// extractionSourceDir, when set, is what each language extractor is
	// actually pointed at (see sourceDirForExtraction), instead of
	// config.SourceDir. AUR-483 sets it to a scoped mirror of SourceDir --
	// same tree, minus test/fixture scope -- because every extractor
	// discovers its own files by walking its given SourceDir directly
	// (ExtractRequest carries no file list), so excluding test/fixture
	// files earlier, only from determineFilesToProcess's own accounting,
	// would never change what an extractor actually reads.
	extractionSourceDir string
}

// sourceDirForExtraction is what gets handed to every extractor as
// ExtractRequest.SourceDir: extractionSourceDir when AUR-483's scope filter
// built one, config.SourceDir (unchanged, pre-AUR-483 behavior) otherwise --
// in particular whenever IncludeTests() is true.
func (p *ExtractorPipeline) sourceDirForExtraction() string {
	if p.extractionSourceDir != "" {
		return p.extractionSourceDir
	}
	return p.config.SourceDir
}

// NewExtractorPipeline creates a new documentation extraction pipeline
func NewExtractorPipeline(
	config *ExtractorPipelineConfig,
	runner site.CommandRunner,
	llmOrch *llm.Orchestrator,
) *ExtractorPipeline {
	registry := extractors.NewRegistry()

	// Register all extractors (assuming they're already registered in init())
	// This would be done in the main package or via extractors.RegisterAll()

	return &ExtractorPipeline{
		config:         config,
		registry:       registry,
		runner:         runner,
		incrementalMgr: incremental.NewManager(runner, config.SourceDir),
		normalizer:     normalizer.NewNormalizer(config.DocsDir),
		welcomeGen:     welcome.NewGenerator(llmOrch),
		docsReviewer:   docsreview.NewReviewer(llmOrch),
		llmOrch:        llmOrch,
	}
}

// RegisterExtractor registers a language extractor with the pipeline registry.
func (p *ExtractorPipeline) RegisterExtractor(extractor extractors.Extractor) error {
	return p.registry.Register(extractor)
}

// Run executes the complete documentation pipeline
func (p *ExtractorPipeline) Run(ctx context.Context) error {
	log.Printf("[Pipeline] Starting documentation extraction pipeline")
	log.Printf("[Pipeline] Source: %s, Output: %s", p.config.SourceDir, p.config.OutputDir)

	// Step 1: Determine what needs to be extracted
	filesToProcess, err := p.determineFilesToProcess(ctx)
	if err != nil {
		return fmt.Errorf("failed to determine files to process: %w", err)
	}

	// AUR-483: build a scoped mirror of SourceDir -- same tree, minus
	// test/fixture scope -- for every extractor to read from instead of the
	// real SourceDir. Skipped entirely (extractionSourceDir stays "", so
	// sourceDirForExtraction falls back to config.SourceDir, byte-for-byte
	// the pre-AUR-483 path) when the operator opted back into documenting
	// tests, or in incremental mode, whose change-detection already
	// operates on a caller-supplied file list rather than a directory walk.
	if !p.IncludeTests() && !p.config.Incremental {
		mirror, cleanup, mirrorErr := buildScopedSourceView(p.config.SourceDir)
		if mirrorErr != nil {
			return fmt.Errorf("building scoped source view: %w", mirrorErr)
		}
		defer cleanup()
		p.extractionSourceDir = mirror
	}

	if len(filesToProcess) == 0 {
		log.Printf("[Pipeline] No files to process")
		// A run with nothing to document must still leave a servable root:
		// returning here without a scaffold publishes a 404.
		if _, err := p.writeSiteScaffold(); err != nil {
			return fmt.Errorf("site scaffold generation failed: %w", err)
		}
		return nil
	}

	sourceFiles := 0
	for _, files := range filesToProcess {
		sourceFiles += len(files)
	}

	log.Printf("[Pipeline] Found %d files to process", sourceFiles)

	// Step 2: Extract documentation for each language
	stats, extractionErrors, skipped := p.extractDocumentation(ctx, filesToProcess)

	// Log statistics
	log.Printf("[Pipeline] Extraction complete: %d files processed, %d docs generated, %d language(s) skipped",
		stats.FilesProcessed, stats.DocsGenerated, len(skipped))

	if len(extractionErrors) > 0 {
		log.Printf("[Pipeline] %d extraction errors occurred", len(extractionErrors))
		for _, err := range extractionErrors {
			log.Printf("[Pipeline] Error: %s", site.Redact(err.Error()))
		}
	}

	if stats.DocsGenerated == 0 {
		return &ExtractionError{
			SourceFiles:    sourceFiles,
			FilesProcessed: stats.FilesProcessed,
			DocsGenerated:  stats.DocsGenerated,
			Errors:         extractionErrors,
			Skipped:        skipped,
		}
	}

	// Step 3: Normalize markdown files with Jekyll front matter
	if stats.DocsGenerated > 0 {
		log.Printf("[Pipeline] Normalizing markdown files...")
		normalized, normErrors := p.normalizer.NormalizeDir(p.config.OutputDir)
		log.Printf("[Pipeline] Normalized %d markdown files", normalized)

		if len(normErrors) > 0 {
			log.Printf("[Pipeline] %d normalization errors occurred", len(normErrors))
		}
	}

	// Step 4: Generate LLM-powered welcome page if enabled
	if p.config.GenerateWelcome && p.llmOrch != nil {
		log.Printf("[Pipeline] Generating welcome page...")
		if err := p.generateWelcomePage(ctx); err != nil {
			log.Printf("[Pipeline] Warning: Welcome page generation failed: %v", err)
		} else {
			log.Printf("[Pipeline] Welcome page generated successfully")
		}
	}

	// Step 5: Write the deterministic site scaffold.
	//
	// This is what makes the run's artifact publishable: extraction leaves
	// markdown files behind, and markdown files alone are not a site. Without an
	// index.md the published root is a 404, and without a _config.yml the host
	// has no reason to render the pages. Both are derived from what is on disk,
	// so they exist whether or not an LLM provider was configured. It runs after
	// the welcome page on purpose: an LLM-written introduction is then kept as
	// the index intro instead of being overwritten by it.
	log.Printf("[Pipeline] Writing site scaffold...")
	scaffoldResult, err := p.writeSiteScaffold()
	if err != nil {
		return fmt.Errorf("site scaffold generation failed: %w", err)
	}
	log.Printf("[Pipeline] Site scaffold: %s (%d page(s) listed), %s",
		scaffoldResult.IndexPath, len(scaffoldResult.Pages), scaffoldResult.ConfigPath)

	// Step 6: Run the optional editorial review only after the deterministic
	// scaffold exists. The report lives under reviews/, outside the API listing,
	// and is the only site content the model may write.
	if p.config.GenerateDocsReview || p.config.DocsReviewRequired {
		log.Printf("[Pipeline] Reviewing published documentation...")
		if err := p.generateDocsReview(ctx); err != nil {
			if p.config.DocsReviewRequired {
				return fmt.Errorf("documentation review required: %w", err)
			}
			log.Printf("[Pipeline] Warning: documentation review skipped: %v", err)
		} else {
			log.Printf("[Pipeline] Documentation review written")
		}
	}

	// Step 7: Validate Jekyll site if enabled
	if p.config.ValidateJekyll {
		log.Printf("[Pipeline] Validating Jekyll site...")
		if err := p.validateJekyllSite(ctx); err != nil {
			log.Printf("[Pipeline] Warning: Jekyll validation failed: %v", err)
		} else {
			log.Printf("[Pipeline] Jekyll site validation successful")
		}
	}

	// Step 8: Deploy to gh-pages if enabled
	if p.config.DeployGHPages {
		log.Printf("[Pipeline] Deploying to gh-pages...")
		if err := p.deployToGHPages(ctx); err != nil {
			return fmt.Errorf("gh-pages deployment failed: %w", err)
		}
		log.Printf("[Pipeline] Deployed to gh-pages successfully")
	}

	// Step 9: Update incremental cache
	if p.config.Incremental {
		log.Printf("[Pipeline] Updating incremental cache...")
		if err := p.incrementalMgr.UpdateCommit(ctx); err != nil {
			log.Printf("[Pipeline] Warning: Failed to update cache: %v", err)
		}
		if err := p.incrementalMgr.SaveCache(); err != nil {
			log.Printf("[Pipeline] Warning: Failed to save cache: %v", err)
		}
	}

	if len(extractionErrors) > 0 || len(skipped) > 0 {
		log.Printf("[Pipeline] Documentation pipeline completed PARTIALLY: %d extraction error(s), %d language(s) skipped",
			len(extractionErrors), len(skipped))
		return &ExtractionError{
			Partial:        true,
			SourceFiles:    sourceFiles,
			FilesProcessed: stats.FilesProcessed,
			DocsGenerated:  stats.DocsGenerated,
			Errors:         extractionErrors,
			Skipped:        skipped,
		}
	}

	log.Printf("[Pipeline] Documentation pipeline completed successfully")
	return nil
}

// determineFilesToProcess determines which files need documentation extraction
func (p *ExtractorPipeline) determineFilesToProcess(ctx context.Context) (map[extractors.Language][]string, error) {
	files := make(map[extractors.Language][]string)
	includeTests := p.IncludeTests()
	scopeExcluded := 0

	if p.config.Incremental {
		// Load existing cache
		if err := p.incrementalMgr.LoadCache(); err != nil {
			log.Printf("[Pipeline] Warning: Failed to load cache: %v", err)
		}

		// Get changed files
		changedFiles, err := p.incrementalMgr.GetChangedFiles(ctx)
		if err != nil {
			return nil, err
		}

		log.Printf("[Pipeline] Incremental mode: %d changed files detected", len(changedFiles))

		if !includeTests {
			kept := make([]string, 0, len(changedFiles))
			for _, path := range changedFiles {
				if IsTestScopePath(path) {
					scopeExcluded++
					continue
				}
				kept = append(kept, path)
			}
			changedFiles = kept
		}

		// Group by language
		files = p.groupFilesByLanguage(changedFiles)
	} else {
		// Full extraction mode - find all source files
		log.Printf("[Pipeline] Full extraction mode")

		var allFiles []string
		err := filepath.Walk(p.config.SourceDir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}

			// Skip common non-source directories
			if shouldSkipPath(path) {
				return nil
			}

			// Skip test and fixture scope by default (AUR-483): a
			// generated site should describe the product, not enumerate
			// its own test scaffolding. IsTestScopePath matches whole
			// path components, never a substring, so internal/attestation
			// and cmd/latest are never caught by this.
			//
			// The check runs against the path RELATIVE to SourceDir, not
			// the raw walked path: SourceDir itself can sit under a
			// directory that happens to be named tests/testdata/fixtures
			// (this card's own fixture tree does, deliberately, at
			// tests/fixtures/docs/AUR-483), and that parent name must
			// never decide the scope of files inside it.
			scopeCheckPath := path
			if rel, relErr := filepath.Rel(p.config.SourceDir, path); relErr == nil {
				scopeCheckPath = rel
			}
			if !includeTests && IsTestScopePath(scopeCheckPath) {
				scopeExcluded++
				return nil
			}

			allFiles = append(allFiles, path)
			return nil
		})

		if err != nil {
			return nil, err
		}

		files = p.groupFilesByLanguage(allFiles)
	}

	// AUR-483 AC-001: the run declares how many files it excluded by scope,
	// rather than silently narrowing what gets documented.
	if includeTests {
		log.Printf("[Pipeline] scope: test/fixture exclusion disabled by config (%s=true) - documenting test and fixture files", envIncludeTestDocs)
	} else {
		log.Printf("[Pipeline] scope: excluded %d file(s) as test or fixture scope (tests/, testdata/, fixtures/ directories and *_test files); set %s=true to include them", scopeExcluded, envIncludeTestDocs)
	}

	// Filter by configured languages if specified
	if len(p.config.Languages) > 0 {
		filtered := make(map[extractors.Language][]string)
		for _, langStr := range p.config.Languages {
			lang := extractors.Language(langStr)
			if fileList, ok := files[lang]; ok {
				filtered[lang] = fileList
			}
		}
		files = filtered
	}

	return files, nil
}

// groupFilesByLanguage groups files by their programming language
func (p *ExtractorPipeline) groupFilesByLanguage(files []string) map[extractors.Language][]string {
	grouped := make(map[extractors.Language][]string)

	for _, file := range files {
		lang := detectLanguageFromFile(file)
		if lang != "" {
			grouped[lang] = append(grouped[lang], file)
		}
	}

	return grouped
}

// extractDocumentation extracts documentation for all files. Languages the pipeline
// cannot handle here and now - no extractor, or an extractor whose external tool is
// missing - are reported as skips rather than errors, so one uncovered language in a
// mixed repository cannot fail the languages that are covered.
func (p *ExtractorPipeline) extractDocumentation(
	ctx context.Context,
	filesByLanguage map[extractors.Language][]string,
) (extractors.ExtractionStats, []error, []LanguageSkip) {

	totalStats := extractors.ExtractionStats{}
	var allErrors []error
	var allSkips []LanguageSkip

	for _, lang := range sortedLanguages(filesByLanguage) {
		files := filesByLanguage[lang]

		log.Printf("[Pipeline] Extracting %s documentation (%d files)...", lang, len(files))

		skip := LanguageSkip{Language: lang, Files: len(files)}

		extractor, err := p.registry.Get(lang)
		if err != nil {
			skip.Reason = SkipNoExtractor
			log.Printf("[Pipeline] SKIP %s", skip)
			allSkips = append(allSkips, skip)
			continue
		}

		// Validate reports whether the extractor's external dependencies are present.
		if err := extractor.Validate(ctx); err != nil {
			skip.Detail = site.Redact(err.Error())
			if tool, missing := extractors.MissingTool(err); missing {
				skip.Reason = SkipToolUnavailable
				skip.Tool = tool
			} else {
				skip.Reason = SkipValidationFailed
			}

			log.Printf("[Pipeline] SKIP %s", skip)
			allSkips = append(allSkips, skip)
			continue
		}

		// Extract documentation
		request := &extractors.ExtractRequest{
			Language:  lang,
			SourceDir: p.sourceDirForExtraction(),
			OutputDir: filepath.Join(p.config.OutputDir, string(lang)),
		}

		result, err := extractor.Extract(ctx, request)
		if err != nil {
			errMsg := fmt.Errorf("%s extraction failed: %w", lang, err)
			log.Printf("[Pipeline] ⚠️  ERROR: %s", site.Redact(errMsg.Error()))
			allErrors = append(allErrors, errMsg)
			continue
		}

		// Aggregate statistics
		totalStats.FilesProcessed += result.Stats.FilesProcessed
		totalStats.DocsGenerated += result.Stats.DocsGenerated

		// Track errors
		allErrors = append(allErrors, result.Errors...)

		// Register in incremental cache
		if p.config.Incremental {
			for _, file := range files {
				p.incrementalMgr.RegisterDocumentation(file, result.Files...)
			}
			p.incrementalMgr.RegisterLanguage(string(lang), files...)
		}

		log.Printf("[Pipeline] %s: %d files processed, %d docs generated",
			lang, result.Stats.FilesProcessed, result.Stats.DocsGenerated)
	}

	return totalStats, allErrors, allSkips
}

// sortedLanguages orders the languages so diagnostics are stable across runs.
func sortedLanguages(filesByLanguage map[extractors.Language][]string) []extractors.Language {
	langs := make([]extractors.Language, 0, len(filesByLanguage))
	for lang := range filesByLanguage {
		langs = append(langs, lang)
	}

	sort.Slice(langs, func(i, j int) bool { return langs[i] < langs[j] })

	return langs
}

// generateWelcomePage generates LLM-powered welcome page from README
func (p *ExtractorPipeline) generateWelcomePage(ctx context.Context) error {
	readmePath := filepath.Join(p.config.SourceDir, "README.md")
	if _, err := os.Stat(readmePath); os.IsNotExist(err) {
		return fmt.Errorf("README.md not found")
	}

	opts := welcome.GenerateOptions{
		ReadmePath: readmePath,
		OutputPath: filepath.Join(p.config.DocsDir, "index.md"),
		ProjectDir: p.config.SourceDir,
		Title:      "Home",
		NavOrder:   1,
	}

	_, err := p.welcomeGen.Generate(ctx, opts)
	return err
}

// generateDocsReview writes only the editorial report. Generated API pages are
// never sent back to disk through the model's response.
func (p *ExtractorPipeline) generateDocsReview(ctx context.Context) error {
	docsDir := p.config.DocsDir
	if docsDir == "" {
		docsDir = p.config.OutputDir
	}
	language := p.config.DocsReviewLanguage
	if language == "" {
		language = "pt-BR"
	}
	_, err := p.docsReviewer.Generate(ctx, docsreview.GenerateOptions{
		ProjectDir:   p.config.SourceDir,
		SiteDir:      docsDir,
		GeneratedDir: p.config.OutputDir,
		OutputPath:   filepath.Join(docsDir, "reviews", "docs-review.md"),
		Language:     language,
	})
	return err
}

// writeSiteScaffold emits the index page and the Jekyll configuration that turn
// the generated markdown into a servable site. It needs no LLM provider: the
// listing is derived from the pages that really exist under OutputDir.
func (p *ExtractorPipeline) writeSiteScaffold() (*site.ScaffoldResult, error) {
	docsDir := p.config.DocsDir
	if docsDir == "" {
		docsDir = p.config.OutputDir
	}

	title := "Documentation"
	if base := filepath.Base(p.config.SourceDir); base != "." && base != string(filepath.Separator) && base != "" {
		title = base + " documentation"
	}

	// Without this the generated index links every page at "/go/pkg/", which is
	// a 404 on any site published under a base path - the default for a GitHub
	// Pages project site. kramdown copies a markdown destination into the href
	// verbatim, so Jekyll's baseurl cannot rescue it after the fact: the prefix
	// has to be in the markdown this pipeline writes.
	basePath, err := p.resolveBasePath(docsDir)
	if err != nil {
		return nil, err
	}
	if basePath != "" {
		log.Printf("[Pipeline] Publishing under base path %s", basePath)
	}

	scaffold := site.NewScaffold(site.ScaffoldConfig{
		DocsDir:     docsDir,
		OutputDir:   p.config.OutputDir,
		Title:       title,
		Description: "API documentation generated by AurumCode.",
		BaseURL:     basePath,
	})

	result, err := scaffold.Generate()
	if err != nil {
		return nil, err
	}

	p.warnIfConfigMissesBaseURL(result, basePath)

	return result, nil
}

// warnIfConfigMissesBaseURL covers the one case the scaffold deliberately cannot
// fix. writeConfig never overwrites a consumer-owned _config.yml, so a repository
// that already ships one gets its links prefixed but not its baseurl key, and the
// theme's own assets - which resolve through relative_url - would still be
// requested off the base path. Silently publishing a half-configured site is
// worse than saying which line to add.
func (p *ExtractorPipeline) warnIfConfigMissesBaseURL(result *site.ScaffoldResult, basePath string) {
	if basePath == "" || result == nil || result.ConfigCreated {
		return
	}

	if _, declared := configuredBaseURL(result.ConfigPath); declared {
		return
	}

	log.Printf("[Pipeline] Warning: %s already exists and declares no baseurl; "+
		"the generated links use %s but the theme's assets will not. "+
		"Add this line to it: baseurl: %q",
		filepath.ToSlash(result.ConfigPath), basePath, basePath)
}

// validateJekyllSite validates the Jekyll site can be built
func (p *ExtractorPipeline) validateJekyllSite(ctx context.Context) error {
	// Check if Jekyll is available
	_, err := p.runner.Run(ctx, "bundle", []string{"--version"}, p.config.DocsDir, nil)
	if err != nil {
		return fmt.Errorf("bundler not available")
	}

	// Try to build the site
	_, err = p.runner.Run(ctx, "bundle", []string{"exec", "jekyll", "build"}, p.config.DocsDir, nil)
	if err != nil {
		return fmt.Errorf("jekyll build failed: %w", err)
	}

	return nil
}

// deployToGHPages deploys documentation to gh-pages branch
func (p *ExtractorPipeline) deployToGHPages(ctx context.Context) error {
	// This would implement gh-pages deployment logic
	// For now, just a placeholder
	log.Printf("[Pipeline] gh-pages deployment not yet implemented")
	return nil
}

// detectLanguageFromFile detects language from file extension
func detectLanguageFromFile(file string) extractors.Language {
	ext := filepath.Ext(file)

	switch ext {
	case ".go":
		return extractors.LanguageGo
	case ".js", ".mjs":
		return extractors.LanguageJavaScript
	case ".ts":
		return extractors.LanguageTypeScript
	case ".py":
		return extractors.LanguagePython
	case ".cs":
		return extractors.LanguageCSharp
	case ".java":
		return extractors.LanguageJava
	case ".cpp", ".cc", ".cxx", ".h", ".hpp":
		return extractors.LanguageCPP
	case ".rs":
		return extractors.LanguageRust
	case ".sh":
		return extractors.LanguageBash
	case ".ps1", ".psm1":
		return extractors.LanguagePowerShell
	default:
		return ""
	}
}

// shouldSkipPath checks if path should be skipped during file discovery
func shouldSkipPath(path string) bool {
	skipDirs := map[string]struct{}{
		"node_modules": {},
		".git":         {},
		".github":      {},
		"vendor":       {},
		"target":       {},
		"dist":         {},
		"build":        {},
		"_site":        {},
		".taskmaster":  {},
		".aurumcode":   {},
	}

	clean := filepath.Clean(path)
	for {
		base := filepath.Base(clean)
		if _, skip := skipDirs[base]; skip {
			return true
		}

		parent := filepath.Dir(clean)
		if parent == clean || parent == "." || parent == string(filepath.Separator) {
			break
		}
		clean = parent
	}

	return false
}
