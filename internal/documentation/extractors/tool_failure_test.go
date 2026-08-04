package extractors_test

import (
	"context"
	"os"
	"path/filepath"
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

// realToolCase describes one extractor and the executables its Validate looks up.
type realToolCase struct {
	name string
	// tools are every executable Validate may invoke, in lookup order.
	tools []string
	// firstTool is the executable Validate reaches first.
	firstTool string
	build     func(site.CommandRunner) extractors.Extractor
}

func realToolCases() []realToolCase {
	return []realToolCase{
		{
			name:      "go/gomarkdoc",
			tools:     []string{"gomarkdoc"},
			firstTool: "gomarkdoc",
			build:     func(r site.CommandRunner) extractors.Extractor { return goextractor.NewGoExtractor(r) },
		},
		{
			name:      "python/pydoc-markdown",
			tools:     []string{"pydoc-markdown"},
			firstTool: "pydoc-markdown",
			build:     func(r site.CommandRunner) extractors.Extractor { return python.NewPythonExtractor(r) },
		},
		{
			name:      "javascript/typedoc",
			tools:     []string{"typedoc"},
			firstTool: "typedoc",
			build:     func(r site.CommandRunner) extractors.Extractor { return javascript.NewJSExtractor(r) },
		},
		{
			name:      "csharp/dotnet+xmldocmd",
			tools:     []string{"dotnet", "xmldocmd"},
			firstTool: "dotnet",
			build:     func(r site.CommandRunner) extractors.Extractor { return csharp.NewCSharpExtractor(r) },
		},
		{
			name:      "cpp/doxygen",
			tools:     []string{"doxygen"},
			firstTool: "doxygen",
			build:     func(r site.CommandRunner) extractors.Extractor { return cpp.NewCPPExtractor(r) },
		},
		{
			name:      "rust/cargo",
			tools:     []string{"cargo"},
			firstTool: "cargo",
			build:     func(r site.CommandRunner) extractors.Extractor { return rust.NewRustExtractor(r) },
		},
		{
			name:      "bash/bash",
			tools:     []string{"bash"},
			firstTool: "bash",
			build:     func(r site.CommandRunner) extractors.Extractor { return bash.NewBashExtractor(r) },
		},
		{
			name:      "powershell/pwsh",
			tools:     []string{"pwsh"},
			firstTool: "pwsh",
			build:     func(r site.CommandRunner) extractors.Extractor { return powershell.NewPowerShellExtractor(r) },
		},
	}
}

// installStubs writes an executable stub for each tool into a fresh directory and
// returns that directory. The stub is a real program on PATH, so exec finds it and
// the failure the runner reports is a genuine non-zero exit, not a lookup failure.
func installStubs(t *testing.T, tools []string, script string) string {
	t.Helper()

	binDir := t.TempDir()
	for _, tool := range tools {
		path := filepath.Join(binDir, tool)
		if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
			t.Fatalf("failed to install stub %s: %v", tool, err)
		}
	}

	return binDir
}

// TestExtractorValidate_ToolPresentButFailingIsAnError pins the distinction the
// degradation path depends on: a tool that is installed and exits non-zero is an
// extraction ERROR, never a "missing tool" skip.
//
// Classifying every runner failure as ToolUnavailable turns a broken toolchain
// into a silent partial success: the pipeline skips the language, the binary
// exits 0, and the consumer gets no documentation and no failure.
func TestExtractorValidate_ToolPresentButFailingIsAnError(t *testing.T) {
	for _, tc := range realToolCases() {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			// The stub exists and is executable; it just fails, like a tool with a
			// broken install or an unsupported flag.
			binDir := installStubs(t, tc.tools, "#!/bin/sh\necho 'boom' >&2\nexit 1\n")
			t.Setenv("PATH", binDir)

			// Sanity: the executable really is resolvable on this PATH.
			if _, statErr := os.Stat(filepath.Join(binDir, tc.firstTool)); statErr != nil {
				t.Fatalf("stub %s was not installed: %v", tc.firstTool, statErr)
			}

			err := tc.build(site.NewDefaultRunner()).Validate(context.Background())
			if err == nil {
				t.Fatalf("Validate returned nil although %s exits 1", tc.firstTool)
			}

			if tool, missing := extractors.MissingTool(err); missing {
				t.Fatalf("Validate classified an installed-but-failing %s as missing tool %q: %v",
					tc.firstTool, tool, err)
			}
		})
	}
}

// TestExtractorValidate_ToolAbsentIsSkippable pins the other half: with the
// executable genuinely absent from PATH, Validate must produce a classifiable
// ToolUnavailableError naming the executable.
func TestExtractorValidate_ToolAbsentIsSkippable(t *testing.T) {
	for _, tc := range realToolCases() {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			// Empty PATH directory: no stub at all, so exec reports ErrNotFound.
			t.Setenv("PATH", t.TempDir())

			err := tc.build(site.NewDefaultRunner()).Validate(context.Background())
			if err == nil {
				t.Fatalf("Validate returned nil with %s absent from PATH", tc.firstTool)
			}

			tool, missing := extractors.MissingTool(err)
			if !missing {
				t.Fatalf("MissingTool did not classify %v as a missing tool", err)
			}

			if tool != tc.firstTool {
				t.Errorf("MissingTool tool = %q, want %q", tool, tc.firstTool)
			}
		})
	}
}
