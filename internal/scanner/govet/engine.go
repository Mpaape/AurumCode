// Package govet is the lint engine of the scanner registry: the Go
// toolchain's own `go vet`, run over the reviewed module and kept only on
// the lines the reviewed commit range added. A diagnostic on a line the
// pull request did not touch is the repository's history, not this change,
// and never blocks it.
//
// The engine installs nothing: `go` must already be on PATH (a missing
// binary is lint_unavailable). It never downloads a toolchain or a module
// (GOTOOLCHAIN=local, GOPROXY=off): a module whose dependencies are not
// already in the module cache or vendored fails to load, and the scan is
// lint_execution_error, never "zero findings".
package govet

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Mpaape/AurumCode/internal/analysis"
	"github.com/Mpaape/AurumCode/internal/scanner"
)

const (
	// Name is the engine's registered name and its typed origin.
	Name = "govet"
	// Category is the gate source every linter engine answers to.
	Category = "lint"
	// goBinary is the executable looked up on PATH.
	goBinary = "go"
	// goModFile marks the module the engine vets: the review root's own.
	goModFile = "go.mod"
)

// ErrNoModule: the review root holds no go.mod. go vet would then resolve
// another module (a parent's) whose paths the diff of the root cannot map,
// and an unmapped finding must never read as "clean".
var ErrNoModule = errors.New("govet: no go.mod at the review root")

// ErrNoRange: the caller gave no commit range, so no finding can be
// anchored to the change; vetting the whole tree instead would accuse the
// pull request of the repository's history.
var ErrNoRange = errors.New("govet: no reviewed commit range")

// Environment is what `go` receives beyond scanner.BaseEnvironment: the
// caller's build and module caches, and fixed values that forbid any
// download. GOFLAGS is deliberately absent (a -toolexec there would run an
// arbitrary program). CGO_ENABLED=0: the pull request controls the #cgo
// directives (CFLAGS, LDFLAGS), so vet must never invoke the C toolchain.
var Environment = scanner.Environment{
	Pass:  []string{"GOCACHE", "GOPATH", "GOMODCACHE", "GOROOT"},
	Fixed: []string{"GOTOOLCHAIN=local", "GOPROXY=off", "CGO_ENABLED=0"},
	Extra: scanner.GitSafeDirectories,
}

func init() {
	scanner.Register(scanner.Engine{
		Scanner:     Engine{},
		Category:    Category,
		Origin:      Name,
		Validate:    validateOptions,
		Environment: Environment,
	})
}

// Engine runs go vet.
type Engine struct{}

// Name is the engine's registered name.
func (Engine) Name() string { return Name }

// Run vets the module at req.Root and keeps the findings on lines added by
// req.Range. Any failure is an error for the executor's single
// inconclusive rule; a missing binary keeps exec.ErrNotFound.
func (Engine) Run(ctx context.Context, req scanner.Request) (scanner.Report, error) {
	run := req.Command
	if run == nil {
		return scanner.Report{}, errors.New("govet: nil command runner")
	}
	if req.Range.Empty() {
		return scanner.Report{}, ErrNoRange
	}
	if info, err := os.Stat(filepath.Join(req.Root, goModFile)); err != nil || !info.Mode().IsRegular() {
		return scanner.Report{}, ErrNoModule
	}
	version, err := goVersion(ctx, run, req.Root)
	if err != nil {
		return scanner.Report{}, err
	}
	added, err := addedLines(ctx, run, req.Root, req.Range)
	if err != nil {
		return scanner.Report{}, err
	}
	goRun := func(ctx context.Context, dir string, args ...string) (string, string, error) {
		return run(ctx, dir, goBinary, args...)
	}
	findings, err := analysis.NewRunner().Vet(ctx, req.Root, goRun)
	if err != nil {
		return scanner.Report{}, fmt.Errorf("govet: %w", err)
	}
	return scanner.Report{Findings: added.keep(findings), Complete: true, Version: "go vet " + version}, nil
}

// goVersion is the toolchain's own version, the engine identity recorded
// in Report.Version.
func goVersion(ctx context.Context, run scanner.Command, root string) (string, error) {
	stdout, _, err := run(ctx, root, goBinary, "env", "GOVERSION")
	if err != nil {
		return "", fmt.Errorf("govet: go env: %w", err)
	}
	version := strings.TrimSpace(stdout)
	if version == "" || strings.ContainsAny(version, " \n") {
		return "", fmt.Errorf("govet: go env GOVERSION %q: %w", version, scanner.ErrInvalidOutput)
	}
	return version, nil
}

// validateOptions: the engine takes no option.
func validateOptions(opts scanner.Options) error {
	for key := range opts {
		return fmt.Errorf("unknown option %q (govet takes no option)", key)
	}
	return nil
}
