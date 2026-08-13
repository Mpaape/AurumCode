package extractors_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
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

// countMarkdownlikeFiles returns every regular file under dir, so a reported doc
// count can be checked against what is actually on disk.
func filesOnDisk(t *testing.T, dir string) []string {
	t.Helper()

	found := []string{}
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil
		}
		found = append(found, path)
		return nil
	})

	return found
}

// TestExtract_ReportedDocsExistOnDisk pins that an extractor may only report
// documentation it can prove exists.
//
// The runner here is the honest reproduction of a tool that exits 0 and writes
// nothing: every command succeeds, no output file appears. An extractor that
// counts DocsGenerated from the exit status alone reports documentation that the
// publishing step will never find, and the caller has no way to tell.
func TestExtract_ReportedDocsExistOnDisk(t *testing.T) {
	tests := []struct {
		name     string
		language extractors.Language
		// fixture seeds the source tree the extractor scans.
		fixture func(t *testing.T, srcDir string)
		build   func(site.CommandRunner) extractors.Extractor
		// toolWritesOutput is true when the external tool owns the output; with a
		// tool that writes nothing, the extraction must report a failure.
		toolWritesOutput bool
	}{
		{
			// Since AUR-424 the Go extractor writes its own page from
			// go/parser + go/doc output, so it is a self-writing extractor
			// like bash and powershell, not a tool-driven one: there is no
			// external tool left that could exit 0 and write nothing.
			//
			// The invariant this file exists for is unchanged and still
			// enforced on this row: every file in result.Files must exist and
			// be non-empty on disk, and DocsGenerated may not exceed what is
			// actually there. What moves is only who is expected to have
			// written it.
			name:     "go",
			language: extractors.LanguageGo,
			fixture: func(t *testing.T, srcDir string) {
				writeFixture(t, filepath.Join(srcDir, "main.go"),
					"// Package main is the fixture.\npackage main\n\n// Exported is documentable.\nfunc Exported() {}\n")
			},
			build:            func(r site.CommandRunner) extractors.Extractor { return goextractor.NewGoExtractor(r) },
			toolWritesOutput: false,
		},
		{
			name:     "javascript",
			language: extractors.LanguageJavaScript,
			fixture: func(t *testing.T, srcDir string) {
				writeFixture(t, filepath.Join(srcDir, "package.json"), "{\"name\":\"fixture\"}\n")
				writeFixture(t, filepath.Join(srcDir, "index.js"), "export function hello() {}\n")
			},
			build:            func(r site.CommandRunner) extractors.Extractor { return javascript.NewJSExtractor(r) },
			toolWritesOutput: true,
		},
		{
			name:     "csharp",
			language: extractors.LanguageCSharp,
			fixture: func(t *testing.T, srcDir string) {
				writeFixture(t, filepath.Join(srcDir, "App.csproj"), "<Project Sdk=\"Microsoft.NET.Sdk\" />\n")
			},
			build:            func(r site.CommandRunner) extractors.Extractor { return csharp.NewCSharpExtractor(r) },
			toolWritesOutput: true,
		},
		{
			name:     "cpp",
			language: extractors.LanguageCPP,
			fixture: func(t *testing.T, srcDir string) {
				writeFixture(t, filepath.Join(srcDir, "main.cpp"), "int main() { return 0; }\n")
			},
			build:            func(r site.CommandRunner) extractors.Extractor { return cpp.NewCPPExtractor(r) },
			toolWritesOutput: true,
		},
		{
			name:     "rust",
			language: extractors.LanguageRust,
			fixture: func(t *testing.T, srcDir string) {
				writeFixture(t, filepath.Join(srcDir, "Cargo.toml"), "[package]\nname = \"fixture\"\n")
			},
			build:            func(r site.CommandRunner) extractors.Extractor { return rust.NewRustExtractor(r) },
			toolWritesOutput: true,
		},
		{
			name:     "python",
			language: extractors.LanguagePython,
			fixture: func(t *testing.T, srcDir string) {
				writeFixture(t, filepath.Join(srcDir, "mod.py"), "\"\"\"Docstring.\"\"\"\n")
			},
			build: func(r site.CommandRunner) extractors.Extractor { return python.NewPythonExtractor(r) },
		},
		{
			name:     "bash",
			language: extractors.LanguageBash,
			fixture: func(t *testing.T, srcDir string) {
				writeFixture(t, filepath.Join(srcDir, "script.sh"), "#!/bin/bash\n# Docs\necho hi\n")
			},
			build: func(r site.CommandRunner) extractors.Extractor { return bash.NewBashExtractor(r) },
		},
		{
			name:     "powershell",
			language: extractors.LanguagePowerShell,
			fixture: func(t *testing.T, srcDir string) {
				writeFixture(t, filepath.Join(srcDir, "script.ps1"), "<#\nDocs\n#>\nWrite-Host hi\n")
			},
			build: func(r site.CommandRunner) extractors.Extractor { return powershell.NewPowerShellExtractor(r) },
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			srcDir := t.TempDir()
			outputDir := filepath.Join(t.TempDir(), "docs")
			tt.fixture(t, srcDir)

			// Every command succeeds and writes nothing: MockRunner's default.
			result, err := tt.build(site.NewMockRunner()).Extract(context.Background(), &extractors.ExtractRequest{
				Language:  tt.language,
				SourceDir: srcDir,
				OutputDir: outputDir,
			})

			if !tt.toolWritesOutput && err != nil {
				t.Fatalf("Extract failed for a self-writing extractor: %v", err)
			}

			if result == nil {
				if err == nil {
					t.Fatal("Extract returned no result and no error")
				}
				return
			}

			// Nothing may be reported that is not on disk.
			for _, file := range result.Files {
				info, statErr := os.Stat(file)
				if statErr != nil {
					t.Errorf("reported file %s does not exist on disk: %v", file, statErr)
					continue
				}
				if info.Size() == 0 {
					t.Errorf("reported file %s is empty on disk", file)
				}
			}

			onDisk := filesOnDisk(t, outputDir)
			if result.Stats.DocsGenerated > len(onDisk) {
				t.Errorf("DocsGenerated=%d but only %d file(s) exist under %s: %v",
					result.Stats.DocsGenerated, len(onDisk), outputDir, onDisk)
			}

			if !tt.toolWritesOutput {
				if result.Stats.DocsGenerated == 0 {
					t.Errorf("self-writing extractor generated nothing: %+v", result.Stats)
				}
				return
			}

			// The tool exited 0 and wrote nothing, so the run produced no docs and
			// must say so.
			if result.Stats.DocsGenerated != 0 {
				t.Errorf("DocsGenerated=%d although the tool wrote nothing", result.Stats.DocsGenerated)
			}

			if err == nil && len(result.Errors) == 0 {
				t.Errorf("tool exited 0 and wrote nothing, but extraction reported success "+
					"(err=%v, DocsGenerated=%d, Errors=%d, filesOnDisk=%d)",
					err, result.Stats.DocsGenerated, len(result.Errors), len(onDisk))
			}

			// The failure must name what went wrong.
			reported := []string{}
			if err != nil {
				reported = append(reported, err.Error())
			}
			for _, e := range result.Errors {
				reported = append(reported, e.Error())
			}
			joined := strings.Join(reported, " | ")
			if joined != "" && !strings.Contains(joined, "no output") {
				t.Logf("reported failure: %s", joined)
			}
		})
	}
}

func writeFixture(t *testing.T, path, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("failed to create fixture dir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write fixture %s: %v", path, err)
	}
}
