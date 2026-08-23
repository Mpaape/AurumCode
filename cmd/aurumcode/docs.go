// Command aurumcode's docs subcommand (AUR-426, extended by AUR-447):
//
//	aurumcode docs [--source <dir>] [--output <dir>] [--languages <list>]
//
// generates project documentation from source code the same way
// cmd/regenerate-docs does, but driven by discoverable command-line flags
// instead of requiring the operator to discover AURUMCODE_-prefixed
// environment variables first. `aurumcode docs --help` documents every flag
// and its default.
//
// This subcommand reuses the exact reusable engine cmd/regenerate-docs
// already wires -- internal/pipeline.ExtractorPipeline driving the
// internal/documentation/extractors/* language extractors -- instead of
// reimplementing extraction. cmd/regenerate-docs itself is untouched by this
// card and keeps working exactly as published; this file only adds a second,
// flag-driven caller of the same packages.
//
// Rust and C# (AUR-427/AUR-447): registerDocsExtractors below registers the
// same native, tool-free extractors (internal/documentation/extractors/rust
// and .../csharp's NewNativeExtractor) cmd/regenerate-docs registers BY
// DEFAULT. Neither starts a subprocess or reads a repository-controlled
// build script, so there is nothing to gate behind an opt-in here. Before
// this card, this file kept its own separate registration list that predated
// AUR-427 and never grew the two native extractors cmd/regenerate-docs
// gained -- a real defect a reviewer reproduced: running this command
// against a Rust-only source tree returned exit 1, "no extractor
// registered", zero pages, while cmd/regenerate-docs documented the same
// tree correctly. What stays unwired on purpose is cmd/regenerate-docs's
// SECOND, opt-in path for these two languages: the cargo/dotnet-backed
// RustExtractor/CSharpExtractor behind AURUMCODE_ALLOW_REPO_CODE_EXECUTION
// (cmd/regenerate-docs/repo_code_execution.go), which executes code that
// lives in the documented repository. That logic is defined inside
// cmd/regenerate-docs's own `main` package -- unimportable (Go does not
// allow importing a `main` package) and outside this card's paths -- so it
// necessarily stays a cmd/regenerate-docs-only capability; this file's
// registered set is exactly cmd/regenerate-docs's DEFAULT set (the opt-in
// toolchain off), never a superset of what cmd/regenerate-docs can do.
//
// A genuinely single, shared registration function both binaries call would
// need to live in an importable package (internal/pipeline or
// internal/documentation) AND cmd/regenerate-docs/main.go would need to be
// rewritten to call it instead of keeping its own inline list -- both
// outside this card's paths, and the latter is exactly the "reescrever
// pacote existente que ja cumpre a funcao" this card's Non-goals forbids.
// Absent that, this file's own registerDocsExtractors is kept in exact,
// visible correspondence with cmd/regenerate-docs's registerLanguageExtractors
// default branch instead: the residual risk is that a future language added
// to one list is not mirrored in the other, same as this card's own root
// cause. See docs/specs/AUR-447.md's "Achado e desenho" section.
//
// Welcome-page LLM generation and Jekyll-build validation are likewise out
// of scope: nothing `aurumcode docs --help` documents depends on a model
// call, so the sealed, network-denied acceptance profile always exercises
// the real code path end to end, deterministically.
//
// See docs/specs/AUR-426.md and docs/specs/AUR-447.md for the full flag
// reference, exit codes and offline, secret-free examples.
package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Mpaape/AurumCode/internal/documentation/extractors"
	bashExtractor "github.com/Mpaape/AurumCode/internal/documentation/extractors/bash"
	cppExtractor "github.com/Mpaape/AurumCode/internal/documentation/extractors/cpp"
	csharpExtractor "github.com/Mpaape/AurumCode/internal/documentation/extractors/csharp"
	goExtractor "github.com/Mpaape/AurumCode/internal/documentation/extractors/go"
	javascriptExtractor "github.com/Mpaape/AurumCode/internal/documentation/extractors/javascript"
	powershellExtractor "github.com/Mpaape/AurumCode/internal/documentation/extractors/powershell"
	pythonExtractor "github.com/Mpaape/AurumCode/internal/documentation/extractors/python"
	rustExtractor "github.com/Mpaape/AurumCode/internal/documentation/extractors/rust"
	"github.com/Mpaape/AurumCode/internal/documentation/site"
	"github.com/Mpaape/AurumCode/internal/pipeline"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
)

// runDocs parses docs subcommand flags and, on success, runs the
// documentation pipeline. It returns the process exit code:
//
//	0  the command did what --help promises: help printed, or documentation
//	   generated (fully, or partially with at least one page produced)
//	1  the run failed: an invalid --source, a pipeline failure, or a run
//	   that produced zero documentation pages
//	2  usage error: an unknown flag or a value a flag cannot parse
//
// See docs/specs/AUR-426.md's "Exit codes" table, which this function's
// behavior is pinned to.
func runDocs(args []string, stdout, stderr io.Writer, filter *redaction.Filter) int {
	var helpBuf bytes.Buffer
	fs := flag.NewFlagSet("docs", flag.ContinueOnError)
	fs.SetOutput(&helpBuf)

	source := fs.String("source", ".", "directory to scan for documentable source code")
	output := fs.String("output", ".aurumcode", "directory to write generated markdown and site files (index.md, _config.yml)")
	languages := fs.String("languages", "", "comma-separated language filter, exact lowercase names, e.g. go,python (default: all supported languages; an unrecognized or wrongly-cased name matches nothing)")

	fs.Usage = func() {
		fmt.Fprintln(&helpBuf, "usage: aurumcode docs [flags]")
		fmt.Fprintln(&helpBuf)
		fmt.Fprintln(&helpBuf, "Generate project documentation from source code. Reuses the same")
		fmt.Fprintln(&helpBuf, "internal/pipeline and internal/documentation/extractors packages")
		fmt.Fprintln(&helpBuf, "cmd/regenerate-docs uses, driven by flags instead of environment")
		fmt.Fprintln(&helpBuf, "variables.")
		fmt.Fprintln(&helpBuf)
		fmt.Fprintln(&helpBuf, "Flags:")
		fs.PrintDefaults()
	}

	err := fs.Parse(args)
	if errors.Is(err, flag.ErrHelp) {
		// --help is a request, not an error: the usage text is the promised
		// result, printed to stdout, exit 0. (Distinct from a genuine usage
		// error below, which prints to stderr and exits 2; see
		// docs/specs/AUR-426.md's note on this command's own convention.)
		io.Copy(stdout, &helpBuf) //nolint:errcheck // best-effort; nothing left to report to on failure
		return 0
	}
	if err != nil {
		io.Copy(stderr, &helpBuf) //nolint:errcheck // best-effort; nothing left to report to on failure
		return 2
	}

	absSource, err := filepath.Abs(*source)
	if err != nil {
		fmt.Fprintf(stderr, "aurumcode docs: %v\n", err)
		return 1
	}
	info, err := os.Stat(absSource)
	if err != nil {
		fmt.Fprintf(stderr, "aurumcode docs: source directory %s: %v\n", filter.Redact(absSource), err)
		return 1
	}
	if !info.IsDir() {
		fmt.Fprintf(stderr, "aurumcode docs: source directory %s is not a directory\n", filter.Redact(absSource))
		return 1
	}

	cfg := &pipeline.ExtractorPipelineConfig{
		SourceDir:       absSource,
		OutputDir:       *output,
		DocsDir:         *output,
		Languages:       splitDocsLanguages(*languages),
		Incremental:     false,
		GenerateWelcome: false,
		ValidateJekyll:  false,
		DeployGHPages:   false,
	}

	runner := site.NewDefaultRunner()
	extractorPipeline := pipeline.NewExtractorPipeline(cfg, runner, nil)
	if err := registerDocsExtractors(extractorPipeline, runner); err != nil {
		fmt.Fprintf(stderr, "aurumcode docs: %v\n", err)
		return 1
	}

	// internal/pipeline logs its own progress through the standard "log"
	// package, which by default writes straight to the real os.Stderr --
	// bypassing the redaction writer stderr is otherwise wrapped in
	// (AUR-432) and carrying a non-deterministic timestamp prefix that
	// would break the "same input, same output" contract this command
	// promises. Silencing it here and reporting our own curated,
	// filtered, timestamp-free summary from the pipeline's returned result
	// keeps both properties.
	prevOutput := log.Writer()
	prevFlags := log.Flags()
	log.SetOutput(io.Discard)
	defer func() {
		log.SetOutput(prevOutput)
		log.SetFlags(prevFlags)
	}()

	runErr := extractorPipeline.Run(context.Background())

	docs, walkErr := collectDocsMarkdown(*output)
	if walkErr != nil {
		fmt.Fprintf(stderr, "aurumcode docs: inspecting output %s: %v\n", filter.Redact(*output), walkErr)
		return 1
	}

	var extractionErr *pipeline.ExtractionError
	structured := errors.As(runErr, &extractionErr)

	if runErr != nil && (!structured || len(docs) == 0) {
		fmt.Fprintf(stderr, "aurumcode docs: %s\n", filter.Redact(runErr.Error()))
		return 1
	}

	printDocsSummary(stdout, filter, *output, docs)

	if len(docs) == 0 {
		fmt.Fprintln(stderr, "aurumcode docs: no documentation page was generated")
		return 1
	}

	if structured && extractionErr.Partial {
		fmt.Fprintf(stderr, "aurumcode docs: partial run: %d extraction error(s), %d language(s) skipped\n",
			len(extractionErr.Errors), len(extractionErr.Skipped))
	}

	return 0
}

// splitDocsLanguages mirrors cmd/regenerate-docs's splitLanguages: an empty
// filter means "every supported language", never a slice containing one
// empty string.
func splitDocsLanguages(raw string) []string {
	languages := []string{}
	for _, part := range strings.Split(raw, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			languages = append(languages, trimmed)
		}
	}
	return languages
}

// printDocsSummary reports what this run actually produced. output and each
// doc path derive from the --source tree and the --output flag -- both
// repository- or operator-controlled input -- so they pass the AUR-432
// redaction filter before reaching stdout, exactly like printNotices does
// for `aurumcode review`.
func printDocsSummary(stdout io.Writer, filter *redaction.Filter, output string, docs []string) {
	sorted := make([]string, len(docs))
	copy(sorted, docs)
	sort.Strings(sorted)

	if len(sorted) == 0 {
		fmt.Fprintf(stdout, "No documentation pages were generated in %s.\n", filter.Redact(output))
		return
	}

	fmt.Fprintf(stdout, "Generated %d documentation page(s) in %s:\n", len(sorted), filter.Redact(output))
	for _, doc := range sorted {
		fmt.Fprintf(stdout, "  - %s\n", filter.Redact(doc))
	}
}

// collectDocsMarkdown lists the markdown pages a docs run actually left on
// disk under outputDir. It mirrors cmd/regenerate-docs's own collectMarkdown:
// a missing output directory means nothing was generated (not an error), and
// the site landing page (index.md, written by the pipeline's own scaffold
// step for every run that generates at least one page) is excluded, since
// counting it would let an empty run claim it documented something.
func collectDocsMarkdown(outputDir string) ([]string, error) {
	landingPage := filepath.Clean(filepath.Join(outputDir, "index.md"))

	var docs []string
	err := filepath.Walk(outputDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.EqualFold(filepath.Ext(path), ".md") {
			return nil
		}
		if filepath.Clean(path) == landingPage {
			return nil
		}
		docs = append(docs, filepath.ToSlash(path))
		return nil
	})
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return docs, nil
}

// docsExtractorAlias lets one extractor implementation answer for a second
// extractors.Language under its own identity -- exactly the mechanism
// cmd/regenerate-docs's own extractorAlias provides for TypeScript riding
// the JavaScript extractor. Duplicated here rather than imported because
// cmd/regenerate-docs's extractorAlias is unexported in a separate main
// package this card must not modify.
type docsExtractorAlias struct {
	base extractors.Extractor
	lang extractors.Language
}

func (a *docsExtractorAlias) Extract(ctx context.Context, req *extractors.ExtractRequest) (*extractors.ExtractResult, error) {
	reqCopy := *req
	reqCopy.Language = a.lang
	return a.base.Extract(ctx, &reqCopy)
}

func (a *docsExtractorAlias) Validate(ctx context.Context) error {
	return a.base.Validate(ctx)
}

func (a *docsExtractorAlias) Language() extractors.Language {
	return a.lang
}

// registerDocsExtractors registers the same language extractors
// cmd/regenerate-docs registers BY DEFAULT (its opt-in
// AURUMCODE_ALLOW_REPO_CODE_EXECUTION toolchain off): every extractor that
// starts no subprocess against repository-controlled input outside its own
// declared tool, plus (AUR-427/AUR-447) the native, tool-free Rust and C#
// doc-comment parsers. It deliberately does NOT register
// cmd/regenerate-docs's second, opt-in cargo/dotnet-backed
// RustExtractor/CSharpExtractor path: that logic lives inside a separate
// `main` package this card cannot import and must not modify (see the
// package doc above), and wiring a second, repository-code-executing path
// here is outside what this card's --help promises.
func registerDocsExtractors(p *pipeline.ExtractorPipeline, runner site.CommandRunner) error {
	register := func(ext extractors.Extractor) error {
		if err := p.RegisterExtractor(ext); err != nil {
			return fmt.Errorf("register %s extractor: %w", ext.Language(), err)
		}
		return nil
	}

	if err := register(goExtractor.NewGoExtractor(runner)); err != nil {
		return err
	}

	// AUR-463: mirrors cmd/regenerate-docs/main.go's own registration exactly
	// (see that file's comment): SelectExtractor probes for typedoc once and
	// registers the typedoc-backed JSExtractor when present (byte-identical
	// output when the tool is installed, AC-003) or the native, tool-free
	// JSDoc reader when it is not (real documentation instead of
	// internal/pipeline's unconditional "required tool not in PATH" skip).
	jsExtractor := javascriptExtractor.SelectExtractor(context.Background(), runner)
	if err := register(jsExtractor); err != nil {
		return err
	}
	if err := register(&docsExtractorAlias{base: jsExtractor, lang: extractors.LanguageTypeScript}); err != nil {
		return err
	}

	if err := register(pythonExtractor.NewPythonExtractor(runner)); err != nil {
		return err
	}

	if err := register(cppExtractor.NewCPPExtractor(runner)); err != nil {
		return err
	}

	// AUR-447: this constant is the MUT-001 target. It is a bare constant,
	// not an environment opt-in like cmd/regenerate-docs/
	// repo_code_execution.go's AURUMCODE_ALLOW_REPO_CODE_EXECUTION, because
	// there is no repository-code-executing alternative wired here for it to
	// gate -- flipping it to false only proves this registration is load
	// bearing, the same way turning off cmd/regenerate-docs's own default
	// registration does for AUR-427's MUT-001.
	const registerNativeRustAndCSharp = true
	if registerNativeRustAndCSharp {
		if err := register(rustExtractor.NewNativeExtractor()); err != nil {
			return err
		}
		if err := register(csharpExtractor.NewNativeExtractor()); err != nil {
			return err
		}
	}

	if err := register(bashExtractor.NewBashExtractor(runner)); err != nil {
		return err
	}

	if err := register(powershellExtractor.NewPowerShellExtractor(runner)); err != nil {
		return err
	}

	return nil
}
