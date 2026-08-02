package rust

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Mpaape/AurumCode/internal/documentation/extractors"
	"github.com/Mpaape/AurumCode/internal/documentation/site"
)

// ExecutesRepositoryCode states the constraint the code cannot show on its own:
// this extractor cannot document a crate without executing code that the
// documented repository controls.
//
// `cargo doc` is not a reader - it compiles. Compiling a crate runs its
// build.rs, runs the build.rs of every dependency it has to build, and runs
// proc-macro crates while expanding macros. Cargo also honours the checkout's
// .cargo/config.toml, so the repository can redirect the toolchain
// (rustc-wrapper, runner, target linker) at the same time.
//
// The flags already in use do not change any of that:
//   - --no-deps only limits which crates get HTML, not which crates get built;
//   - --offline only forbids network fetches, not execution.
//
// Cargo has no flag that disables build scripts. The execution-free command,
// `cargo metadata --no-deps --offline`, yields manifest data, not documentation.
// Because there is no safe mode, the decision is fail-closed and lives at the
// composition root: cmd/regenerate-docs does not register this extractor unless
// the consumer names Rust in AURUMCODE_ALLOW_REPO_CODE_EXECUTION. Constructing
// it directly is the caller asserting the tree is trusted.
const ExecutesRepositoryCode = "cargo doc compiles the crate, so it executes build.rs from the repository and from its dependencies, and executes proc-macro crates during macro expansion"

// RustExtractor extracts documentation from Rust source code using cargo doc.
//
// See ExecutesRepositoryCode: every Extract call hands the source tree to a
// compiler that runs code from that tree.
type RustExtractor struct {
	runner site.CommandRunner
}

// NewRustExtractor creates a new Rust documentation extractor.
//
// The returned extractor executes repository-controlled code when it runs (see
// ExecutesRepositoryCode). It is intentionally absent from the default
// registration in cmd/regenerate-docs; a caller that builds one directly is
// accepting that the tree it documents may run arbitrary code as this process.
func NewRustExtractor(runner site.CommandRunner) *RustExtractor {
	return &RustExtractor{
		runner: runner,
	}
}

// Extract generates documentation from Rust source code
func (r *RustExtractor) Extract(ctx context.Context, req *extractors.ExtractRequest) (*extractors.ExtractResult, error) {
	if req.Language != extractors.LanguageRust {
		return nil, fmt.Errorf("invalid language: expected %s, got %s", extractors.LanguageRust, req.Language)
	}

	if _, err := os.Stat(req.SourceDir); err != nil {
		return nil, fmt.Errorf("invalid source directory: %w", err)
	}

	// Find Cargo.toml
	cargoPath := filepath.Join(req.SourceDir, "Cargo.toml")
	if _, err := os.Stat(cargoPath); err != nil {
		return &extractors.ExtractResult{
			Language: extractors.LanguageRust,
			Files:    []string{},
			Stats:    extractors.ExtractionStats{},
			Errors:   []error{fmt.Errorf("no Cargo.toml found")},
		}, nil
	}

	// Run cargo doc.
	//
	// This line executes consumer code: cargo compiles the crate in req.SourceDir,
	// which runs that crate's build.rs and its proc-macro dependencies. --no-deps
	// narrows the output, not the execution. See ExecutesRepositoryCode.
	args := []string{"doc", "--no-deps"}
	_, err := r.runner.Run(ctx, "cargo", args, req.SourceDir, nil)
	if err != nil {
		return &extractors.ExtractResult{
			Language: extractors.LanguageRust,
			Files:    []string{},
			Stats:    extractors.ExtractionStats{},
			Errors:   []error{fmt.Errorf("cargo doc failed: %w", err)},
		}, nil
	}

	// Find generated HTML docs. cargo doc exiting 0 is not evidence: only the
	// files it left under target/doc count as documentation.
	docDir := filepath.Join(req.SourceDir, "target", "doc")
	files, _ := r.countHTMLFiles(docDir)
	if err := extractors.ConfirmOutputFiles("cargo doc", docDir, files); err != nil {
		return &extractors.ExtractResult{
			Language: extractors.LanguageRust,
			Files:    []string{},
			Stats:    extractors.ExtractionStats{},
			Errors:   []error{err},
		}, nil
	}

	result := &extractors.ExtractResult{
		Language: extractors.LanguageRust,
		Files:    files,
		Stats: extractors.ExtractionStats{
			FilesProcessed: 1,
			DocsGenerated:  len(files),
		},
	}

	return result, nil
}

// Validate checks if Rust and Cargo are available.
//
// Only a genuine lookup failure means cargo is unavailable; an installed cargo
// that exits non-zero is an extraction error, not a skip.
func (r *RustExtractor) Validate(ctx context.Context) error {
	_, err := r.runner.Run(ctx, "cargo", []string{"--version"}, ".", nil)
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return extractors.NewToolUnavailableError("cargo", "please install from https://rustup.rs", err)
		}
		return fmt.Errorf("cargo is installed but failed: %w", err)
	}
	return nil
}

// Language returns the language this extractor handles
func (r *RustExtractor) Language() extractors.Language {
	return extractors.LanguageRust
}

func (r *RustExtractor) countHTMLFiles(dir string) ([]string, error) {
	files := []string{}
	if _, err := os.Stat(dir); err != nil {
		return files, nil
	}
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && strings.HasSuffix(path, ".html") {
			files = append(files, path)
		}
		return nil
	})
	return files, nil
}
