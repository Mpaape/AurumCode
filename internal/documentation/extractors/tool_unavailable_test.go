package extractors_test

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"testing"

	"github.com/Mpaape/AurumCode/internal/documentation/extractors"
	"github.com/Mpaape/AurumCode/internal/documentation/extractors/bash"
	"github.com/Mpaape/AurumCode/internal/documentation/extractors/cpp"
	"github.com/Mpaape/AurumCode/internal/documentation/extractors/csharp"
	goextractor "github.com/Mpaape/AurumCode/internal/documentation/extractors/go"
	"github.com/Mpaape/AurumCode/internal/documentation/extractors/javascript"
	"github.com/Mpaape/AurumCode/internal/documentation/extractors/powershell"
	"github.com/Mpaape/AurumCode/internal/documentation/extractors/python"
	"github.com/Mpaape/AurumCode/internal/documentation/extractors/rust"
	"github.com/Mpaape/AurumCode/internal/documentation/site"
)

// errNotInstalled reproduces the exec lookup failure a host without the tool
// produces, wrapped the way site.DefaultRunner wraps it. Only this shape - an
// error chain reaching exec.ErrNotFound - means the tool is absent; a tool that
// runs and exits non-zero is covered by tool_failure_test.go.
var errNotInstalled = fmt.Errorf("command failed: %w",
	&exec.Error{Name: "tool", Err: exec.ErrNotFound})

// TestExtractorValidate_ReportsMissingToolStructurally pins the contract every
// extractor owes the pipeline: when its external tool is absent, Validate must
// return an error that extractors.MissingTool can classify, naming the executable.
//
// Without this, an extractor may return a plain fmt.Errorf and the pipeline
// silently downgrades the skip from "required tool not in PATH" to the generic
// "extractor validation failed" with an empty Tool - degrading the diagnostic
// while every other test stays green.
func TestExtractorValidate_ReportsMissingToolStructurally(t *testing.T) {
	tests := []struct {
		name string
		// failing is the command the mock runner refuses.
		failing string
		// available are commands that succeed before the failing one is reached.
		available []string
		wantTool  string
		build     func(site.CommandRunner) extractors.Extractor
	}{
		{
			name:     "go/gomarkdoc",
			failing:  "gomarkdoc",
			wantTool: "gomarkdoc",
			build:    func(r site.CommandRunner) extractors.Extractor { return goextractor.NewGoExtractor(r) },
		},
		{
			name:     "python/pydoc-markdown",
			failing:  "pydoc-markdown",
			wantTool: "pydoc-markdown",
			build:    func(r site.CommandRunner) extractors.Extractor { return python.NewPythonExtractor(r) },
		},
		{
			name:     "javascript/typedoc",
			failing:  "typedoc",
			wantTool: "typedoc",
			build:    func(r site.CommandRunner) extractors.Extractor { return javascript.NewJSExtractor(r) },
		},
		{
			name:     "csharp/dotnet",
			failing:  "dotnet",
			wantTool: "dotnet",
			build:    func(r site.CommandRunner) extractors.Extractor { return csharp.NewCSharpExtractor(r) },
		},
		{
			name:      "csharp/xmldocmd",
			failing:   "xmldocmd",
			available: []string{"dotnet"},
			wantTool:  "xmldocmd",
			build:     func(r site.CommandRunner) extractors.Extractor { return csharp.NewCSharpExtractor(r) },
		},
		{
			name:     "cpp/doxygen",
			failing:  "doxygen",
			wantTool: "doxygen",
			build:    func(r site.CommandRunner) extractors.Extractor { return cpp.NewCPPExtractor(r) },
		},
		{
			name:     "rust/cargo",
			failing:  "cargo",
			wantTool: "cargo",
			build:    func(r site.CommandRunner) extractors.Extractor { return rust.NewRustExtractor(r) },
		},
		{
			name:     "bash/bash",
			failing:  "bash",
			wantTool: "bash",
			build:    func(r site.CommandRunner) extractors.Extractor { return bash.NewBashExtractor(r) },
		},
		{
			name:     "powershell/pwsh",
			failing:  "pwsh",
			wantTool: "pwsh",
			build:    func(r site.CommandRunner) extractors.Extractor { return powershell.NewPowerShellExtractor(r) },
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			runner := site.NewMockRunner()
			for _, cmd := range tt.available {
				runner.WithOutput(cmd, "1.0.0")
			}
			runner.WithError(tt.failing, errNotInstalled)

			err := tt.build(runner).Validate(context.Background())
			if err == nil {
				t.Fatalf("Validate returned nil with %q missing, want an error", tt.failing)
			}

			tool, missing := extractors.MissingTool(err)
			if !missing {
				t.Fatalf("MissingTool did not classify %T (%v) as a missing tool; "+
					"Validate must return a *ToolUnavailableError", err, err)
			}

			if tool != tt.wantTool {
				t.Errorf("MissingTool tool = %q, want %q", tool, tt.wantTool)
			}

			if !errors.Is(err, errNotInstalled) {
				t.Errorf("Validate error must wrap the underlying lookup failure, got %v", err)
			}
		})
	}
}

// TestExtractorValidate_SucceedsWhenToolsPresent pins the other side of the
// contract: a host that has the tools must not produce a skip.
func TestExtractorValidate_SucceedsWhenToolsPresent(t *testing.T) {
	runner := site.NewMockRunner()
	for _, cmd := range []string{"gomarkdoc", "pydoc-markdown", "typedoc", "dotnet",
		"xmldocmd", "doxygen", "cargo", "bash", "pwsh"} {
		runner.WithOutput(cmd, "1.0.0")
	}

	all := []extractors.Extractor{
		goextractor.NewGoExtractor(runner),
		python.NewPythonExtractor(runner),
		javascript.NewJSExtractor(runner),
		csharp.NewCSharpExtractor(runner),
		cpp.NewCPPExtractor(runner),
		rust.NewRustExtractor(runner),
		bash.NewBashExtractor(runner),
		powershell.NewPowerShellExtractor(runner),
	}

	for _, extractor := range all {
		validateErr := extractor.Validate(context.Background())
		if validateErr != nil {
			t.Errorf("%s Validate failed with all tools present: %v", extractor.Language(), validateErr)
		}

		if _, missing := extractors.MissingTool(validateErr); missing {
			t.Errorf("%s reported a missing tool although every tool is present", extractor.Language())
		}
	}
}
