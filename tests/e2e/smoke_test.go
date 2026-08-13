// Package e2e drives the regenerate-docs binary against a throwaway fixture
// repository. Every other test in this repository stops at a fake
// CommandRunner, so this is the only place where source discovery, extraction,
// the external documentation tool, markdown normalization and the process exit
// status are exercised as one chain.
package e2e

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	documentedSymbol = "AddMoney"
	documentedText   = "AddMoney sums two monetary amounts expressed in cents."
	runTimeout       = 5 * time.Minute
)

var (
	buildOnce   sync.Once
	builtBinary string
	buildErr    error

	toolOnce sync.Once
	toolDir  string
	toolKind string
	toolErr  error
)

type runResult struct {
	exitCode int
	output   string
}

// TestSmokeGeneratesDocsAndSkipsUnsupportedLanguage is the whole chain: the
// fixture holds one Go package and one Java file, so a single run must produce
// real markdown for Go and report Java as skipped instead of failing outright
// or claiming a clean success.
func TestSmokeGeneratesDocsAndSkipsUnsupportedLanguage(t *testing.T) {
	repo := copyFixture(t)
	result := runGenerator(t, repo, nil, toolPATH(t))

	if result.exitCode != 0 {
		t.Fatalf("partial run must exit 0, got %d\n%s", result.exitCode, result.output)
	}

	docPath := filepath.Join(repo, ".aurumcode", "go", "ledger.md")
	content := readFile(t, docPath)

	if !strings.Contains(content, documentedSymbol) {
		t.Errorf("%s does not document %s\n%s", docPath, documentedSymbol, content)
	}
	if !strings.Contains(content, documentedText) {
		t.Errorf("%s lost the doc comment %q\n%s", docPath, documentedText, content)
	}

	// The normalizer is the last link before publication: without its front
	// matter Jekyll renders the page as raw text.
	if !strings.HasPrefix(content, "---\n") || !strings.Contains(content, "layout: default") {
		t.Errorf("%s has no Jekyll front matter\n%s", docPath, content)
	}

	assertContains(t, result.output, "PARTIAL SUCCESS")
	assertContains(t, result.output, "skipped: java: no extractor registered")
	assertContains(t, result.output,
		"aurumcode: result=partial docs=1 skipped=1 failed=0 languages_skipped=java")

	if strings.Contains(result.output, "result=ok") {
		t.Errorf("a run that skipped a language must not report result=ok\n%s", result.output)
	}
}

// TestSmokeCleanRunReportsFullSuccess keeps the partial verdict meaningful: the
// same fixture without the unsupported file has to report a clean success.
func TestSmokeCleanRunReportsFullSuccess(t *testing.T) {
	repo := copyFixture(t)
	if err := os.Remove(filepath.Join(repo, "Manifest.java")); err != nil {
		t.Fatalf("remove java file: %v", err)
	}

	result := runGenerator(t, repo, nil, toolPATH(t))

	if result.exitCode != 0 {
		t.Fatalf("clean run must exit 0, got %d\n%s", result.exitCode, result.output)
	}

	assertContains(t, result.output,
		"aurumcode: result=ok docs=1 skipped=0 failed=0 languages_skipped=none")

	content := readFile(t, filepath.Join(repo, ".aurumcode", "go", "ledger.md"))
	if !strings.Contains(content, documentedSymbol) {
		t.Errorf("clean run did not document %s\n%s", documentedSymbol, content)
	}
}

// TestSmokeMissingToolchainFailsLoudly pins the degradation floor: when no
// language can be documented at all, the run must fail instead of reporting an
// empty success.
//
// The fixture is adjusted rather than the expectation. Until AUR-424 the
// tool-less language driving this case was Go, whose extractor shelled out to
// gomarkdoc; Go now documents with the standard library and can no longer be
// made undocumentable by emptying PATH, so it would turn this run into a
// partial success and delete the floor. Python takes its place: python is
// registered in registerLanguageExtractors and still invokes pydoc-markdown,
// so with an empty PATH nothing in this repository can be documented and the
// original diagnostic -- "required tool not in PATH" -- is still the reason.
// The seven extractors that still depend on an external tool remain covered.
func TestSmokeMissingToolchainFailsLoudly(t *testing.T) {
	repo := copyFixture(t)

	// Go would document successfully with no PATH at all, so it is removed
	// from this copy: the case is about a repository whose every language
	// needs a tool that is not there.
	if err := os.RemoveAll(filepath.Join(repo, "ledger")); err != nil {
		t.Fatalf("remove go package: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "mod.py"), []byte("\"\"\"Fixture module.\"\"\"\n"), 0o644); err != nil {
		t.Fatalf("write python file: %v", err)
	}

	// The PATH is replaced instead of extended: the point of the case is that
	// no documentation tool is reachable at all.
	result := runGenerator(t, repo, nil, t.TempDir())

	if result.exitCode == 0 {
		t.Fatalf("run without a documentation tool must fail, got exit 0\n%s", result.output)
	}

	assertContains(t, result.output, "skipped: python: required tool not in PATH")
	assertContains(t, result.output, "pydoc-markdown")
	assertContains(t, result.output, "produced no documentation")
	assertContains(t, result.output,
		"aurumcode: result=failed docs=0 skipped=2 failed=0 languages_skipped=java,python")

	if _, err := os.Stat(filepath.Join(repo, ".aurumcode", "python")); !os.IsNotExist(err) {
		t.Errorf("no markdown may exist after a failed run, stat error was %v", err)
	}
}

// TestSmokeGoDocumentsWithoutAnyToolOnPATH is the end-to-end fact AUR-424
// delivers, and the reason the case above had to change fixture.
//
// Before the fix, this exact run -- the real binary, a real Go package, an
// empty PATH -- produced zero pages and exit 0: gomarkdoc could not be found,
// the pipeline classified that as a skip, and a consumer got no documentation
// and no failure. The Go extractor now parses with go/parser and documents
// with go/doc, so the run must produce the real page with the real doc
// comment, with nothing but the binary itself on the machine.
func TestSmokeGoDocumentsWithoutAnyToolOnPATH(t *testing.T) {
	repo := copyFixture(t)

	// Only the Go package is left, so the verdict is about Go alone.
	if err := os.Remove(filepath.Join(repo, "Manifest.java")); err != nil {
		t.Fatalf("remove java file: %v", err)
	}

	// An empty directory as the entire PATH: gomarkdoc, and every other
	// documentation tool, is unreachable.
	result := runGenerator(t, repo, nil, t.TempDir())

	if result.exitCode != 0 {
		t.Fatalf("Go must document with an empty PATH, got exit %d\n%s", result.exitCode, result.output)
	}

	assertContains(t, result.output,
		"aurumcode: result=ok docs=1 skipped=0 failed=0 languages_skipped=none")

	content := readFile(t, filepath.Join(repo, ".aurumcode", "go", "ledger.md"))
	if !strings.Contains(content, documentedSymbol) {
		t.Errorf("page does not document %s\n%s", documentedSymbol, content)
	}
	if !strings.Contains(content, documentedText) {
		t.Errorf("page lost the doc comment %q\n%s", documentedText, content)
	}

	// The old failure mode was a silent skip, so its absence is part of the
	// contract: nothing may report Go as unavailable.
	if strings.Contains(result.output, "skipped: go") {
		t.Errorf("Go was skipped although it needs no external tool\n%s", result.output)
	}
}

// TestSmokeHonoursOutputOverrideAndIgnoresSourceDir covers the knobs this
// binary exposes without letting them collide with the composite action, which
// resolves its own source-dir input by changing directory before the launch.
func TestSmokeHonoursOutputOverrideAndIgnoresSourceDir(t *testing.T) {
	repo := copyFixture(t)
	workdir := t.TempDir()
	outputDir := filepath.Join(workdir, "generated")

	result := runGenerator(t, workdir, []string{
		"AURUMCODE_SOURCE_DIR=" + repo,
		"AURUMCODE_OUTPUT_DIR=" + outputDir,
		"SOURCE_DIR=/nonexistent-must-be-ignored",
	}, toolPATH(t))

	if result.exitCode != 0 {
		t.Fatalf("override run must exit 0, got %d\n%s", result.exitCode, result.output)
	}

	content := readFile(t, filepath.Join(outputDir, "go", "ledger.md"))
	if !strings.Contains(content, documentedSymbol) {
		t.Errorf("override run did not document %s\n%s", documentedSymbol, content)
	}

	if _, err := os.Stat(filepath.Join(repo, ".aurumcode")); !os.IsNotExist(err) {
		t.Errorf("output override was ignored, .aurumcode appeared in the source tree (stat error %v)", err)
	}
}

// TestSmokeRefusesUnimplementedDeploy guards the one switch whose pipeline step
// is still a placeholder: it must be rejected rather than reported as deployed.
func TestSmokeRefusesUnimplementedDeploy(t *testing.T) {
	repo := copyFixture(t)
	result := runGenerator(t, repo, []string{"AURUMCODE_DEPLOY_GH_PAGES=true"}, toolPATH(t))

	if result.exitCode == 0 {
		t.Fatalf("requesting an unimplemented deploy must fail, got exit 0\n%s", result.output)
	}
	assertContains(t, result.output, "gh-pages deployment is not implemented")
}

// toolPATH is the PATH of a correctly provisioned run: the documentation tool
// plus the toolchain the tool itself shells out to.
func toolPATH(t *testing.T) string {
	t.Helper()
	return docToolDir(t) + string(os.PathListSeparator) + os.Getenv("PATH")
}

func runGenerator(t *testing.T, workdir string, extraEnv []string, pathValue string) runResult {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), runTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, buildGenerator(t))
	cmd.Dir = workdir
	cmd.Env = append([]string{
		"PATH=" + pathValue,
		"HOME=" + t.TempDir(),
		"GOCACHE=" + cacheDir(t, "gocache"),
		"GOMODCACHE=" + cacheDir(t, "gomodcache"),
		"GOPATH=" + cacheDir(t, "gopath"),
	}, extraEnv...)

	output, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("generator timed out after %s\n%s", runTimeout, output)
	}

	result := runResult{output: string(output)}
	if err != nil {
		exitErr, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("running generator: %v\n%s", err, output)
		}
		result.exitCode = exitErr.ExitCode()
	}

	t.Logf("generator exit=%d workdir=%s doc-tool=%s\n%s", result.exitCode, workdir, toolKind, result.output)
	return result
}

// cacheDir keeps the Go build caches out of the fixture's HOME so a run that
// shells out to the toolchain does not fail on an unwritable default location.
func cacheDir(t *testing.T, name string) string {
	t.Helper()

	dir := filepath.Join(os.TempDir(), "aurumcode-e2e-"+name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("create %s: %v", dir, err)
	}
	return dir
}

func buildGenerator(t *testing.T) string {
	t.Helper()

	buildOnce.Do(func() {
		binary := filepath.Join(os.TempDir(), "aurumcode-e2e-regenerate-docs")
		cmd := exec.Command("go", "build", "-o", binary, "./cmd/regenerate-docs")
		cmd.Dir = repoRoot(t)
		if output, err := cmd.CombinedOutput(); err != nil {
			buildErr = err
			t.Logf("building regenerate-docs failed: %v\n%s", err, output)
			return
		}
		builtBinary = binary
	})

	if buildErr != nil {
		t.Fatalf("regenerate-docs did not build: %v", buildErr)
	}
	return builtBinary
}

// docToolDir returns a directory holding an executable named gomarkdoc. The
// real tool is preferred; when it is absent the stand-in from testdata is built
// instead, and the substitution is logged on every run rather than silently
// skipping the test.
func docToolDir(t *testing.T) string {
	t.Helper()

	toolOnce.Do(func() {
		if path, err := exec.LookPath("gomarkdoc"); err == nil {
			toolDir = filepath.Dir(path)
			toolKind = "gomarkdoc (real)"
			return
		}

		dir := filepath.Join(os.TempDir(), "aurumcode-e2e-doctool")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			toolErr = err
			return
		}

		cmd := exec.Command("go", "build", "-o", filepath.Join(dir, "gomarkdoc"),
			filepath.Join(repoRoot(t), "tests", "e2e", "testdata", "docgen", "main.go"))
		cmd.Dir = repoRoot(t)
		if output, err := cmd.CombinedOutput(); err != nil {
			toolErr = err
			t.Logf("building docgen stand-in failed: %v\n%s", err, output)
			return
		}

		toolDir = dir
		toolKind = "docgen stand-in (real gomarkdoc not installed)"
	})

	if toolErr != nil {
		t.Fatalf("no documentation tool available for the e2e run: %v", toolErr)
	}

	t.Logf("documentation tool: %s", toolKind)
	return toolDir
}

func copyFixture(t *testing.T) string {
	t.Helper()

	source := filepath.Join(repoRoot(t), "tests", "e2e", "testdata", "tinyrepo")
	destination := filepath.Join(t.TempDir(), "tinyrepo")

	err := filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)

		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		t.Fatalf("copy fixture: %v", err)
	}

	return destination
}

func repoRoot(t *testing.T) string {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate the test source file")
	}
	return filepath.Join(filepath.Dir(file), "..", "..")
}

func readFile(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected documentation at %s: %v", path, err)
	}
	return string(data)
}

func assertContains(t *testing.T, haystack, needle string) {
	t.Helper()

	if !strings.Contains(haystack, needle) {
		t.Errorf("run output is missing %q\n%s", needle, haystack)
	}
}
