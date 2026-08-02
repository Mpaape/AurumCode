package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/documentation/extractors"
	"github.com/Mpaape/AurumCode/internal/documentation/site"
	"github.com/Mpaape/AurumCode/internal/pipeline"
)

// This file reproduces the attack the Rust and C# extractors expose, at the
// only layer that decides whether it happens: the default registration of this
// binary.
//
// The attack needs no exotic input. A contributor opens a pull request that
// adds a build.rs (Rust) or an <Exec> task to a .csproj (C#). The workflow
// checks that branch out and runs this binary against it. cargo and MSBuild
// then execute that file on the maintainer's runner.
//
// The stand-in tools below are not "mocks that make the test pass": each one
// reproduces the single documented behaviour of the real tool that turns
// repository content into execution, and nothing else. That keeps the test
// runnable on a host with no Rust and no .NET toolchain while still failing for
// exactly the reason the real toolchain would. Every path the fixtures touch is
// baked into the script text, so the reproduction does not depend on the
// runner's environment policy.

const (
	cargoInvokedCanary  = "cargo-doc-invoked.canary"
	dotnetInvokedCanary = "dotnet-build-invoked.canary"
	buildRSCanary       = "rust-build-rs-executed.canary"
	msbuildExecCanary   = "csharp-msbuild-exec-executed.canary"
)

// cargoStub stands in for cargo. `cargo doc` compiles the crate, and compiling a
// crate executes its build.rs - that is the whole of what this stub reproduces.
// The real cargo compiles build.rs with rustc first and then runs the resulting
// binary; the stub runs it with /bin/sh so the test needs no Rust toolchain. The
// attacker capability is identical: arbitrary code, working directory inside the
// checkout.
const cargoStubTemplate = `#!/bin/sh
case "$1" in
  --version) echo "cargo 1.70.0 (test stand-in)" ; exit 0 ;;
  doc)
    : > "%[1]s/` + cargoInvokedCanary + `"
    if [ -f build.rs ]; then sh build.rs; fi
    mkdir -p target/doc
    echo "<html><body>doc</body></html>" > target/doc/index.html
    exit 0 ;;
esac
exit 0
`

// dotnetStubTemplate stands in for the .NET SDK. `dotnet build` runs MSBuild,
// and MSBuild executes the <Exec> tasks declared by the project file it is
// building. The stub extracts that command and runs it, which is what MSBuild
// does.
const dotnetStubTemplate = `#!/bin/sh
case "$1" in
  --version) echo "8.0.100" ; exit 0 ;;
  build)
    : > "%[1]s/` + dotnetInvokedCanary + `"
    cmd=$(sed -n 's/.*<Exec Command="\([^"]*\)".*/\1/p' "$2")
    if [ -n "$cmd" ]; then sh -c "$cmd"; fi
    exit 0 ;;
esac
exit 0
`

// xmldocmdStub only has to exist and answer --version: the C# extractor probes
// it during validation, and the execution has already happened by then.
const xmldocmdStub = `#!/bin/sh
echo "1.0.0"
exit 0
`

// hostileBuildRS is the payload a pull request would add. It writes a canary
// instead of doing anything harmful; a real one would read the workspace and
// the runner's credentials.
const hostileBuildRSTemplate = `#!/bin/sh
: > "%[1]s/` + buildRSCanary + `"
`

// hostileCsprojTemplate is the C# equivalent: a project file whose <Exec> task
// MSBuild runs during `dotnet build`.
const hostileCsprojTemplate = `<Project Sdk="Microsoft.NET.Sdk">
  <Target Name="Payload" BeforeTargets="Build">
    <Exec Command="sh %[1]s/payload.sh" />
  </Target>
</Project>
`

const hostileCsprojPayloadTemplate = `#!/bin/sh
: > "%[1]s/` + msbuildExecCanary + `"
`

// hostileTree writes a source tree a malicious pull request could produce and
// returns (sourceDir, canaryDir). It also puts the tool stand-ins on PATH, so
// the binary resolves them exactly as it would resolve the real toolchain.
func hostileTree(t *testing.T) (string, string) {
	t.Helper()

	srcDir := t.TempDir()
	canaryDir := t.TempDir()
	binDir := t.TempDir()

	write := func(path, content string, mode os.FileMode) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte(content), mode); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	// Rust side: a crate whose build script is the payload.
	write(filepath.Join(srcDir, "Cargo.toml"), "[package]\nname = \"fixture\"\nversion = \"0.1.0\"\n", 0o644)
	write(filepath.Join(srcDir, "src", "lib.rs"), "//! Fixture crate.\npub fn hello() {}\n", 0o644)
	write(filepath.Join(srcDir, "build.rs"), fmt.Sprintf(hostileBuildRSTemplate, canaryDir), 0o755)

	// C# side: a project whose <Exec> task is the payload.
	write(filepath.Join(srcDir, "App.csproj"), fmt.Sprintf(hostileCsprojTemplate, canaryDir), 0o644)
	write(filepath.Join(srcDir, "App.cs"), "/// <summary>Fixture.</summary>\npublic class App {}\n", 0o644)
	write(filepath.Join(canaryDir, "payload.sh"), fmt.Sprintf(hostileCsprojPayloadTemplate, canaryDir), 0o755)

	// Tool stand-ins.
	write(filepath.Join(binDir, "cargo"), fmt.Sprintf(cargoStubTemplate, canaryDir), 0o755)
	write(filepath.Join(binDir, "dotnet"), fmt.Sprintf(dotnetStubTemplate, canaryDir), 0o755)
	write(filepath.Join(binDir, "xmldocmd"), xmldocmdStub, 0o755)

	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	return srcDir, canaryDir
}

// runPipelineWithDefaultRegistration runs the real pipeline over srcDir with the
// real command runner and this binary's registration. The pipeline's own outcome
// is deliberately ignored: what is under test is what the run executed, not what
// it documented.
func runPipelineWithDefaultRegistration(t *testing.T, srcDir string) {
	t.Helper()

	outputDir := filepath.Join(t.TempDir(), "out")
	config := &pipeline.ExtractorPipelineConfig{
		SourceDir: srcDir,
		OutputDir: outputDir,
		DocsDir:   outputDir,
	}

	runner := site.NewDefaultRunner()
	p := pipeline.NewExtractorPipeline(config, runner, nil)
	if err := registerLanguageExtractors(p, runner); err != nil {
		t.Fatalf("registerLanguageExtractors: %v", err)
	}

	_ = p.Run(context.Background())
}

func canaryFired(t *testing.T, canaryDir, name string) bool {
	t.Helper()

	_, err := os.Stat(filepath.Join(canaryDir, name))
	return err == nil
}

// TestDefaultRegistration_NeverExecutesRepositoryCode is the reproduction.
//
// With the default registration - no opt-in of any kind - the run must never
// hand the hostile tree to cargo or to MSBuild. Each canary is a distinct step
// of the attack chain, so a failure names the step that still fires.
func TestDefaultRegistration_NeverExecutesRepositoryCode(t *testing.T) {
	srcDir, canaryDir := hostileTree(t)

	runPipelineWithDefaultRegistration(t, srcDir)

	forbidden := []struct {
		canary string
		what   string
	}{
		{cargoInvokedCanary, "cargo doc was invoked with the working directory inside the hostile tree"},
		{dotnetInvokedCanary, "dotnet build was invoked on a project file from the hostile tree"},
		{buildRSCanary, "build.rs from the repository was EXECUTED"},
		{msbuildExecCanary, "the <Exec> task from the repository's .csproj was EXECUTED"},
	}

	for _, f := range forbidden {
		if canaryFired(t, canaryDir, f.canary) {
			t.Errorf("default run executed repository-controlled code: %s (canary %s)", f.what, f.canary)
		}
	}
}

// TestOptIn_RunsTheExtractorAndItsToolchain is the other side of the gate, and
// it is what keeps the test above honest.
//
// The same fixture, the same stand-in tools, the same pipeline - only the opt-in
// changes. Every canary fires here, which proves two things at once: the opt-in
// really does register and run the extractors, and the absence of those canaries
// in the default run is the gate working rather than a fixture that never
// worked. It also shows precisely what the consumer accepts by opting in.
func TestOptIn_RunsTheExtractorAndItsToolchain(t *testing.T) {
	srcDir, canaryDir := hostileTree(t)
	t.Setenv(envAllowRepoCodeExecution, "rust,csharp")

	runPipelineWithDefaultRegistration(t, srcDir)

	expected := []struct {
		canary string
		what   string
	}{
		{cargoInvokedCanary, "cargo doc was not invoked although rust was opted in"},
		{dotnetInvokedCanary, "dotnet build was not invoked although csharp was opted in"},
		{buildRSCanary, "build.rs did not run, so the fixture cannot prove the default run suppressed it"},
		{msbuildExecCanary, "the <Exec> task did not run, so the fixture cannot prove the default run suppressed it"},
	}

	for _, e := range expected {
		if !canaryFired(t, canaryDir, e.canary) {
			t.Errorf("%s (canary %s missing)", e.what, e.canary)
		}
	}
}

// TestOptIn_IsPerLanguage pins that enabling one language does not enable the
// other. A boolean opt-in would have made "I need Rust docs" silently accept
// MSBuild execution too.
func TestOptIn_IsPerLanguage(t *testing.T) {
	srcDir, canaryDir := hostileTree(t)
	t.Setenv(envAllowRepoCodeExecution, "rust")

	runPipelineWithDefaultRegistration(t, srcDir)

	if !canaryFired(t, canaryDir, buildRSCanary) {
		t.Error("rust was opted in but build.rs never ran")
	}
	if canaryFired(t, canaryDir, dotnetInvokedCanary) || canaryFired(t, canaryDir, msbuildExecCanary) {
		t.Error("opting in to rust also let MSBuild execute code from the repository")
	}
}

// TestParseRepoCodeExecutionOptIn_FailsClosed pins the parser's two failure
// directions: nothing is enabled by default, and a value that is not exactly a
// known language name is refused instead of being guessed at.
func TestParseRepoCodeExecutionOptIn_FailsClosed(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    []string
		wantErr bool
	}{
		{name: "unset", raw: "", want: nil},
		{name: "whitespace only", raw: "   ,  ,", want: nil},
		{name: "rust", raw: "rust", want: []string{"rust"}},
		{name: "csharp", raw: "csharp", want: []string{"csharp"}},
		{name: "both", raw: "rust,csharp", want: []string{"csharp", "rust"}},
		{name: "case and spacing", raw: " RUST , CSharp ", want: []string{"csharp", "rust"}},
		{name: "boolean true is not a language", raw: "true", wantErr: true},
		{name: "1 is not a language", raw: "1", wantErr: true},
		{name: "all is not a language", raw: "all", wantErr: true},
		{name: "unknown language", raw: "java", wantErr: true},
		{name: "wrong separator", raw: "rust;csharp", wantErr: true},
		{name: "one valid one invalid", raw: "rust,jaba", wantErr: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			enabled, err := parseRepoCodeExecutionOptIn(tt.raw)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("parse(%q) accepted the value, want an error; enabled=%v", tt.raw, enabled)
				}
				return
			}

			if err != nil {
				t.Fatalf("parse(%q) failed: %v", tt.raw, err)
			}

			got := make([]string, 0, len(enabled))
			for token := range enabled {
				got = append(got, token)
			}
			sort.Strings(got)

			want := tt.want
			if want == nil {
				want = []string{}
			}

			if strings.Join(got, ",") != strings.Join(want, ",") {
				t.Errorf("parse(%q) enabled %v, want %v", tt.raw, got, want)
			}
		})
	}
}

// TestRegisterRepoCodeExecutingExtractors_RejectedValueRegistersNothing pins
// that a partially valid opt-in registers nothing at all. Registering the valid
// half and then failing would leave a caller that ignores the error running the
// code-executing extractor it never managed to configure.
func TestRegisterRepoCodeExecutingExtractors_RejectedValueRegistersNothing(t *testing.T) {
	registered := []string{}
	register := func(ext extractors.Extractor) error {
		registered = append(registered, string(ext.Language()))
		return nil
	}

	err := registerRepoCodeExecutingExtractors(register, site.NewMockRunner(), "rust,definitely-not-a-language")
	if err == nil {
		t.Fatal("registerRepoCodeExecutingExtractors accepted an invalid opt-in value")
	}

	if len(registered) != 0 {
		t.Errorf("invalid opt-in still registered %v", registered)
	}
}

// TestRegisterRepoCodeExecutingExtractors_DefaultRegistersNothing pins the
// default at the registration boundary itself, independently of the pipeline.
func TestRegisterRepoCodeExecutingExtractors_DefaultRegistersNothing(t *testing.T) {
	registered := []string{}
	register := func(ext extractors.Extractor) error {
		registered = append(registered, string(ext.Language()))
		return nil
	}

	if err := registerRepoCodeExecutingExtractors(register, site.NewMockRunner(), ""); err != nil {
		t.Fatalf("default registration failed: %v", err)
	}

	if len(registered) != 0 {
		t.Errorf("default registration registered %v, want nothing", registered)
	}
}
