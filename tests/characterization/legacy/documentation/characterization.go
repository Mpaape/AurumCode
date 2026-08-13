package documentationcharacterization

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Mpaape/AurumCode/internal/documentation/extractors"
	bashextractor "github.com/Mpaape/AurumCode/internal/documentation/extractors/bash"
	cppextractor "github.com/Mpaape/AurumCode/internal/documentation/extractors/cpp"
	csharpextractor "github.com/Mpaape/AurumCode/internal/documentation/extractors/csharp"
	goextractor "github.com/Mpaape/AurumCode/internal/documentation/extractors/go"
	jsextractor "github.com/Mpaape/AurumCode/internal/documentation/extractors/javascript"
	powershellextractor "github.com/Mpaape/AurumCode/internal/documentation/extractors/powershell"
	pythonextractor "github.com/Mpaape/AurumCode/internal/documentation/extractors/python"
	rustextractor "github.com/Mpaape/AurumCode/internal/documentation/extractors/rust"
	"github.com/Mpaape/AurumCode/internal/documentation/incremental"
	"github.com/Mpaape/AurumCode/internal/documentation/normalizer"
	"github.com/Mpaape/AurumCode/internal/documentation/site"
)

const ManifestPath = "tests/characterization/legacy/documentation/classification.tsv"

var allowedDispositions = map[string]bool{
	"keep": true, "migrate": true, "replace": true, "delete": true,
}

// Classification binds one existing package to an explicit disposition and to
// the exact behavioral test that justifies it.
type Classification struct {
	Package     string `json:"package"`
	Disposition string `json:"disposition"`
	Test        string `json:"test"`
	Observation string `json:"observation"`
}

// ProbeResult is emitted only after a package entrypoint was called and its
// observable output or effect was asserted.
type ProbeResult struct {
	Package     string
	Test        string
	Observation string
}

type commandCall struct {
	command string
	args    []string
	workdir string
}

type scriptedRunner struct {
	calls []commandCall
	hook  func(command string, args []string, workdir string) error
}

func (r *scriptedRunner) Run(_ context.Context, command string, args []string, workdir string, _ map[string]string) (string, error) {
	r.calls = append(r.calls, commandCall{command: command, args: append([]string(nil), args...), workdir: workdir})
	if r.hook != nil {
		if err := r.hook(command, args, workdir); err != nil {
			return "", err
		}
	}
	return "ok", nil
}

type stubExtractor struct{ language extractors.Language }

func (s stubExtractor) Extract(context.Context, *extractors.ExtractRequest) (*extractors.ExtractResult, error) {
	return &extractors.ExtractResult{Language: s.language}, nil
}
func (s stubExtractor) Validate(context.Context) error { return nil }
func (s stubExtractor) Language() extractors.Language  { return s.language }

// ExpectedPackages is the AUR-001-era package inventory in the card's declared
// scope. Acceptance compares it with a fresh go-list result before trusting it.
func ExpectedPackages() []string {
	return []string{
		"internal/documentation/extractors",
		"internal/documentation/extractors/bash",
		"internal/documentation/extractors/cpp",
		"internal/documentation/extractors/csharp",
		"internal/documentation/extractors/go",
		"internal/documentation/extractors/javascript",
		"internal/documentation/extractors/powershell",
		"internal/documentation/extractors/python",
		"internal/documentation/extractors/rust",
		"internal/documentation/incremental",
		"internal/documentation/normalizer",
		"internal/documentation/site",
	}
}

func LoadClassifications(path string) ([]Classification, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("classification manifest unreadable: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1024), 64*1024)
	if !scanner.Scan() || scanner.Text() != "package\tdisposition\ttest\tobservation" {
		return nil, errors.New("classification manifest header is invalid")
	}

	var rows []Classification
	for scanner.Scan() {
		fields := strings.Split(scanner.Text(), "\t")
		if len(fields) != 4 {
			return nil, errors.New("classification manifest row is invalid")
		}
		row := Classification{Package: fields[0], Disposition: fields[1], Test: fields[2], Observation: fields[3]}
		if row.Package == "" || !allowedDispositions[row.Disposition] || row.Test == "" || row.Observation == "" {
			return nil, fmt.Errorf("classification row for %q is incomplete", row.Package)
		}
		rows = append(rows, row)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("classification manifest read failed: %w", err)
	}
	return rows, nil
}

func LoadPackageList(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("package inventory unreadable: %w", err)
	}
	defer file.Close()

	var packages []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		value := strings.TrimSpace(scanner.Text())
		if value != "" {
			packages = append(packages, value)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("package inventory read failed: %w", err)
	}
	return packages, nil
}

// VerifyClassifications rejects inventory drift, duplicate rows, invalid
// dispositions, and—critically—a classification whose named test did not run.
func VerifyClassifications(rows []Classification, probes []ProbeResult, discovered []string) error {
	if len(rows) == 0 || len(rows) != len(discovered) {
		return fmt.Errorf("classification count %d does not match discovered package count %d", len(rows), len(discovered))
	}

	discoveredSet := make(map[string]bool, len(discovered))
	for _, pkg := range discovered {
		if discoveredSet[pkg] {
			return fmt.Errorf("discovered package %s is duplicated", pkg)
		}
		discoveredSet[pkg] = true
	}
	probeByTest := make(map[string]ProbeResult, len(probes))
	for _, probe := range probes {
		probeByTest[probe.Test] = probe
	}

	seenPackages := make(map[string]bool, len(rows))
	seenTests := make(map[string]bool, len(rows))
	previous := ""
	for _, row := range rows {
		if previous != "" && previous >= row.Package {
			return errors.New("classification manifest is not strictly package-sorted")
		}
		previous = row.Package
		if !discoveredSet[row.Package] {
			return fmt.Errorf("classification names undiscovered package %s", row.Package)
		}
		if seenPackages[row.Package] {
			return fmt.Errorf("classification package %s is duplicated", row.Package)
		}
		seenPackages[row.Package] = true
		if !allowedDispositions[row.Disposition] {
			return fmt.Errorf("classification package %s has invalid disposition %s", row.Package, row.Disposition)
		}
		if seenTests[row.Test] {
			return fmt.Errorf("classification test %s is reused", row.Test)
		}
		seenTests[row.Test] = true

		probe, ok := probeByTest[row.Test]
		if !ok || probe.Package != row.Package {
			return fmt.Errorf("classification package %s references unexecuted test %s", row.Package, row.Test)
		}
		if probe.Observation != row.Observation {
			return fmt.Errorf("classification package %s observation differs from executed test %s", row.Package, row.Test)
		}
	}
	for _, pkg := range discovered {
		if !seenPackages[pkg] {
			return fmt.Errorf("discovered package %s has no classification", pkg)
		}
	}
	return nil
}

// RunAllProbes calls every package behavior in the closed AUR-308 inventory.
func RunAllProbes() ([]ProbeResult, error) {
	probes := []func() (ProbeResult, error){
		probeExtractorCore,
		probeBash,
		probeCPP,
		probeCSharp,
		probeGo,
		probeJavaScript,
		probePowerShell,
		probePython,
		probeRust,
		probeIncremental,
		probeNormalizer,
		probeSite,
	}
	results := make([]ProbeResult, 0, len(probes))
	for _, probe := range probes {
		result, err := probe()
		if err != nil {
			return nil, fmt.Errorf("%s: %w", result.Package, err)
		}
		results = append(results, result)
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Package < results[j].Package })
	return results, nil
}

func ProbePackage(pkg string) (ProbeResult, error) {
	probes := map[string]func() (ProbeResult, error){
		"internal/documentation/extractors":            probeExtractorCore,
		"internal/documentation/extractors/bash":       probeBash,
		"internal/documentation/extractors/cpp":        probeCPP,
		"internal/documentation/extractors/csharp":     probeCSharp,
		"internal/documentation/extractors/go":         probeGo,
		"internal/documentation/extractors/javascript": probeJavaScript,
		"internal/documentation/extractors/powershell": probePowerShell,
		"internal/documentation/extractors/python":     probePython,
		"internal/documentation/extractors/rust":       probeRust,
		"internal/documentation/incremental":           probeIncremental,
		"internal/documentation/normalizer":            probeNormalizer,
		"internal/documentation/site":                  probeSite,
	}
	probe, ok := probes[pkg]
	if !ok {
		return ProbeResult{Package: pkg}, errors.New("package is outside the closed characterization inventory")
	}
	return probe()
}

func newProbeDir() (string, func(), error) {
	dir, err := os.MkdirTemp("", "aurum-a308-probe-")
	if err != nil {
		return "", nil, err
	}
	return dir, func() { _ = os.RemoveAll(dir) }, nil
}

func writeFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

func probeExtractorCore() (ProbeResult, error) {
	result := ProbeResult{Package: "internal/documentation/extractors", Test: "TestExtractorCoreRegistryAndDetector", Observation: "detects supported source while excluding vendored source and refuses duplicate registration"}
	dir, cleanup, err := newProbeDir()
	if err != nil {
		return result, err
	}
	defer cleanup()
	if err := writeFile(filepath.Join(dir, "main.go"), "package main\nfunc main() {}\n"); err != nil {
		return result, err
	}
	if err := writeFile(filepath.Join(dir, "vendor", "hidden.py"), "print('hidden')\n"); err != nil {
		return result, err
	}
	detected, err := extractors.NewDetector().Detect(context.Background(), dir)
	if err != nil {
		return result, err
	}
	stats, ok := detected.GetStats(extractors.LanguageGo)
	if !ok || stats.FileCount != 1 || stats.LineCount != 2 || detected.TotalFiles != 1 || detected.HasLanguage(extractors.LanguagePython) {
		return result, errors.New("detector did not execute the bounded include/exclude behavior")
	}
	registry := extractors.NewRegistry()
	ext := stubExtractor{language: extractors.LanguageGo}
	if err := registry.Register(ext); err != nil {
		return result, err
	}
	if err := registry.Register(ext); err == nil || registry.Count() != 1 {
		return result, errors.New("registry accepted a duplicate extractor")
	}
	if got, err := registry.Get(extractors.LanguageGo); err != nil || got.Language() != extractors.LanguageGo {
		return result, errors.New("registry did not return the registered extractor")
	}
	return result, nil
}

func probeBash() (ProbeResult, error) {
	result := ProbeResult{Package: "internal/documentation/extractors/bash", Test: "TestBashExtractorComments", Observation: "renders script comments into a confirmed markdown output"}
	dir, cleanup, err := newProbeDir()
	if err != nil {
		return result, err
	}
	defer cleanup()
	source, output := filepath.Join(dir, "source"), filepath.Join(dir, "output")
	if err := writeFile(filepath.Join(source, "deploy.sh"), "#!/usr/bin/env bash\n# Deploy safely\nprintf done\n"); err != nil {
		return result, err
	}
	got, err := bashextractor.NewBashExtractor(&scriptedRunner{}).Extract(context.Background(), &extractors.ExtractRequest{Language: extractors.LanguageBash, SourceDir: source, OutputDir: output})
	if err != nil || got.Stats.DocsGenerated != 1 || len(got.Files) != 1 || len(got.Errors) != 0 {
		return result, fmt.Errorf("bash extraction result is incomplete: %v", err)
	}
	content, err := os.ReadFile(got.Files[0])
	if err != nil || !strings.Contains(string(content), "Deploy safely") || strings.Contains(string(content), "/usr/bin/env") {
		return result, errors.New("bash extraction did not render the reachable comment behavior")
	}
	return result, nil
}

func quotedDoxyfileValue(content, key string) (string, error) {
	prefix := key + " = \""
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(line, prefix) && strings.HasSuffix(line, "\"") {
			return strings.TrimSuffix(strings.TrimPrefix(line, prefix), "\""), nil
		}
	}
	return "", fmt.Errorf("Doxyfile has no %s", key)
}

func probeCPP() (ProbeResult, error) {
	result := ProbeResult{Package: "internal/documentation/extractors/cpp", Test: "TestCPPExtractorDoxygenOutput", Observation: "invokes doxygen with a private configuration and counts only observed XML output"}
	dir, cleanup, err := newProbeDir()
	if err != nil {
		return result, err
	}
	defer cleanup()
	source, output := filepath.Join(dir, "source"), filepath.Join(dir, "output")
	if err := writeFile(filepath.Join(source, "api.cpp"), "int answer() { return 42; }\n"); err != nil {
		return result, err
	}
	runner := &scriptedRunner{hook: func(command string, args []string, _ string) error {
		if command != "doxygen" || len(args) != 1 {
			return errors.New("unexpected C++ extractor command")
		}
		data, err := os.ReadFile(args[0])
		if err != nil {
			return err
		}
		destination, err := quotedDoxyfileValue(string(data), "OUTPUT_DIRECTORY")
		if err != nil {
			return err
		}
		return writeFile(filepath.Join(destination, "xml", "index.xml"), "<doxygen/>\n")
	}}
	got, err := cppextractor.NewCPPExtractor(runner).Extract(context.Background(), &extractors.ExtractRequest{Language: extractors.LanguageCPP, SourceDir: source, OutputDir: output})
	if err != nil || len(runner.calls) != 1 || got.Stats.FilesProcessed != 1 || got.Stats.DocsGenerated != 1 || len(got.Errors) != 0 {
		return result, fmt.Errorf("C++ extraction did not confirm Doxygen output: %v", err)
	}
	return result, nil
}

func probeCSharp() (ProbeResult, error) {
	result := ProbeResult{Package: "internal/documentation/extractors/csharp", Test: "TestCSharpExtractorExecutesBuild", Observation: "invokes dotnet build and xmldocmd so repository controlled build code is reachable"}
	dir, cleanup, err := newProbeDir()
	if err != nil {
		return result, err
	}
	defer cleanup()
	source, output := filepath.Join(dir, "source"), filepath.Join(dir, "output")
	project := filepath.Join(source, "demo.csproj")
	if err := writeFile(project, "<Project Sdk=\"Microsoft.NET.Sdk\"/>\n"); err != nil {
		return result, err
	}
	runner := &scriptedRunner{hook: func(command string, args []string, workdir string) error {
		switch command {
		case "dotnet":
			if len(args) < 2 || args[0] != "build" || args[1] != project || workdir != source {
				return errors.New("dotnet build boundary was not reached as declared")
			}
			return writeFile(filepath.Join(source, "bin", "Debug", "net8.0", "demo.xml"), "<doc/>\n")
		case "xmldocmd":
			if len(args) != 2 {
				return errors.New("xmldocmd boundary was malformed")
			}
			return writeFile(filepath.Join(args[1], "api.md"), "# Demo API\n")
		default:
			return errors.New("unexpected C# extractor command")
		}
	}}
	got, err := csharpextractor.NewCSharpExtractor(runner).Extract(context.Background(), &extractors.ExtractRequest{Language: extractors.LanguageCSharp, SourceDir: source, OutputDir: output})
	if err != nil || len(runner.calls) != 2 || got.Stats.DocsGenerated != 1 || len(got.Errors) != 0 || csharpextractor.ExecutesRepositoryCode == "" {
		return result, fmt.Errorf("C# build execution was not observed: %v", err)
	}
	return result, nil
}

// probeGo records the Go extractor's behaviour as AUR-424 intentionally
// changed it. The legacy observation was "invokes gomarkdoc and counts only a
// nonempty output observed on disk": the extractor shelled out to the
// third-party gomarkdoc binary, which does not exist in the offline sandbox,
// and the lookup failure became a silent language skip. The extractor now
// parses with go/parser and documents with go/doc, so the observable fact to
// characterize is that it reaches no external tool at all and still confirms
// real output on disk.
//
// This is a deliberate re-characterization of one package, not a relaxation of
// the matrix: the runner below still fails every call it receives, so any
// reintroduced subprocess is caught here exactly as before. The other seven
// language probes are untouched and still assert their external-tool boundary.
func probeGo() (ProbeResult, error) {
	result := ProbeResult{Package: "internal/documentation/extractors/go", Test: "TestGoExtractorWritesConfirmedOutput", Observation: "documents with the Go standard library, reaching no external tool, and counts only a nonempty output observed on disk"}
	dir, cleanup, err := newProbeDir()
	if err != nil {
		return result, err
	}
	defer cleanup()
	source, output := filepath.Join(dir, "source"), filepath.Join(dir, "output")
	if err := writeFile(filepath.Join(source, "api.go"), "// Package api is a probe fixture.\npackage api\n\n// Answer returns the answer.\nfunc Answer() int { return 42 }\n"); err != nil {
		return result, err
	}
	runner := &scriptedRunner{hook: func(command string, args []string, _ string) error {
		return errors.New("no external tool may be reached: " + command)
	}}
	got, err := goextractor.NewGoExtractor(runner).Extract(context.Background(), &extractors.ExtractRequest{Language: extractors.LanguageGo, SourceDir: source, OutputDir: output})
	if err != nil || len(runner.calls) != 0 || got.Stats.DocsGenerated != 1 || len(got.Errors) != 0 {
		return result, fmt.Errorf("Go extractor did not document without an external tool: external_calls=%d generated=%d errors=%v: %v",
			len(runner.calls), got.Stats.DocsGenerated, got.Errors, err)
	}
	// The counted page must really be on disk with the real symbol in it: a
	// confirmed count is the half of the legacy observation that survives.
	page, err := os.ReadFile(got.Files[0])
	if err != nil || !strings.Contains(string(page), "Answer") {
		return result, fmt.Errorf("Go extractor counted a page it cannot show: %v", err)
	}
	return result, nil
}

func probeJavaScript() (ProbeResult, error) {
	result := ProbeResult{Package: "internal/documentation/extractors/javascript", Test: "TestJavaScriptExtractorTypeDocOutput", Observation: "invokes typedoc for a TypeScript project and counts observed markdown output"}
	dir, cleanup, err := newProbeDir()
	if err != nil {
		return result, err
	}
	defer cleanup()
	source, output := filepath.Join(dir, "source"), filepath.Join(dir, "output")
	if err := writeFile(filepath.Join(source, "tsconfig.json"), "{}\n"); err != nil {
		return result, err
	}
	if err := writeFile(filepath.Join(source, "index.ts"), "export const answer = 42;\n"); err != nil {
		return result, err
	}
	runner := &scriptedRunner{hook: func(command string, args []string, _ string) error {
		if command != "typedoc" {
			return errors.New("typedoc boundary was not reached")
		}
		for index, arg := range args {
			if arg == "--out" && index+1 < len(args) {
				return writeFile(filepath.Join(args[index+1], "index.md"), "# TypeScript API\n")
			}
		}
		return errors.New("typedoc output argument was absent")
	}}
	got, err := jsextractor.NewJSExtractor(runner).Extract(context.Background(), &extractors.ExtractRequest{Language: extractors.LanguageTypeScript, SourceDir: source, OutputDir: output})
	if err != nil || len(runner.calls) != 1 || got.Stats.DocsGenerated != 1 || len(got.Errors) != 0 {
		return result, fmt.Errorf("JavaScript extractor did not observe TypeDoc output: %v", err)
	}
	return result, nil
}

func probePowerShell() (ProbeResult, error) {
	result := ProbeResult{Package: "internal/documentation/extractors/powershell", Test: "TestPowerShellExtractorComments", Observation: "renders line and block comments into a confirmed markdown output"}
	dir, cleanup, err := newProbeDir()
	if err != nil {
		return result, err
	}
	defer cleanup()
	source, output := filepath.Join(dir, "source"), filepath.Join(dir, "output")
	if err := writeFile(filepath.Join(source, "deploy.ps1"), "<#\nDeploy safely\n#>\n# Confirm first\nWrite-Output done\n"); err != nil {
		return result, err
	}
	got, err := powershellextractor.NewPowerShellExtractor(&scriptedRunner{}).Extract(context.Background(), &extractors.ExtractRequest{Language: extractors.LanguagePowerShell, SourceDir: source, OutputDir: output})
	if err != nil || got.Stats.DocsGenerated != 1 || len(got.Errors) != 0 {
		return result, fmt.Errorf("PowerShell extraction result is incomplete: %v", err)
	}
	data, readErr := os.ReadFile(got.Files[0])
	if readErr != nil || !strings.Contains(string(data), "Deploy safely") || !strings.Contains(string(data), "Confirm first") {
		return result, errors.New("PowerShell comment behavior was not observed")
	}
	return result, nil
}

func probePython() (ProbeResult, error) {
	result := ProbeResult{Package: "internal/documentation/extractors/python", Test: "TestPythonExtractorCapturesNonDocstringLiteral", Observation: "captures an assigned triple quoted string that is not a Python docstring"}
	dir, cleanup, err := newProbeDir()
	if err != nil {
		return result, err
	}
	defer cleanup()
	source, output := filepath.Join(dir, "source"), filepath.Join(dir, "output")
	if err := writeFile(filepath.Join(source, "module.py"), "def value():\n    answer = 42\n    \"\"\"not API documentation\"\"\"\n    return answer\n"); err != nil {
		return result, err
	}
	got, err := pythonextractor.NewPythonExtractor(&scriptedRunner{}).Extract(context.Background(), &extractors.ExtractRequest{Language: extractors.LanguagePython, SourceDir: source, OutputDir: output})
	if err != nil || got.Stats.DocsGenerated != 1 || len(got.Errors) != 0 {
		return result, fmt.Errorf("Python extractor did not execute: %v", err)
	}
	data, readErr := os.ReadFile(got.Files[0])
	if readErr != nil || !strings.Contains(string(data), "not API documentation") {
		return result, errors.New("Python extractor no longer demonstrates the characterized false-positive")
	}
	return result, nil
}

func probeRust() (ProbeResult, error) {
	result := ProbeResult{Package: "internal/documentation/extractors/rust", Test: "TestRustExtractorExecutesCargoDoc", Observation: "invokes cargo doc so repository build scripts and proc macros are reachable"}
	dir, cleanup, err := newProbeDir()
	if err != nil {
		return result, err
	}
	defer cleanup()
	source := filepath.Join(dir, "source")
	if err := writeFile(filepath.Join(source, "Cargo.toml"), "[package]\nname='demo'\nversion='0.1.0'\n"); err != nil {
		return result, err
	}
	runner := &scriptedRunner{hook: func(command string, args []string, workdir string) error {
		if command != "cargo" || len(args) != 2 || args[0] != "doc" || workdir != source {
			return errors.New("cargo doc boundary was not reached")
		}
		return writeFile(filepath.Join(source, "target", "doc", "index.html"), "<html>docs</html>\n")
	}}
	got, err := rustextractor.NewRustExtractor(runner).Extract(context.Background(), &extractors.ExtractRequest{Language: extractors.LanguageRust, SourceDir: source, OutputDir: filepath.Join(dir, "unused")})
	if err != nil || len(runner.calls) != 1 || got.Stats.DocsGenerated != 1 || len(got.Errors) != 0 || rustextractor.ExecutesRepositoryCode == "" {
		return result, fmt.Errorf("Rust build execution was not observed: %v", err)
	}
	return result, nil
}

func probeIncremental() (ProbeResult, error) {
	result := ProbeResult{Package: "internal/documentation/incremental", Test: "TestIncrementalCacheFollowsOutputSymlink", Observation: "cache save follows a preexisting output symlink and overwrites its target"}
	dir, cleanup, err := newProbeDir()
	if err != nil {
		return result, err
	}
	defer cleanup()
	target := filepath.Join(dir, "outside.json")
	link := filepath.Join(dir, "cache.json")
	if err := os.WriteFile(target, []byte("outside sentinel\n"), 0o600); err != nil {
		return result, err
	}
	if err := os.Symlink(target, link); err != nil {
		return result, err
	}
	cache := incremental.NewCache()
	cache.AddMapping("src/api.go", "docs/api.md")
	if err := cache.Save(link); err != nil {
		return result, fmt.Errorf("legacy cache no longer follows the characterized symlink: %w", err)
	}
	data, err := os.ReadFile(target)
	if err != nil || !strings.Contains(string(data), "src/api.go") || !strings.Contains(string(data), "docs/api.md") {
		return result, errors.New("cache did not overwrite the symlink target with normalized mappings")
	}
	return result, nil
}

func probeNormalizer() (ProbeResult, error) {
	result := ProbeResult{Package: "internal/documentation/normalizer", Test: "TestNormalizerPreservesFrontMatter", Observation: "preserves unknown front matter and converges to byte stable markdown"}
	dir, cleanup, err := newProbeDir()
	if err != nil {
		return result, err
	}
	defer cleanup()
	path := filepath.Join(dir, "_api", "hello_go.md")
	content := "---\ntitle: Existing title\ncustom_key: preserved\n---\n\n# API\n"
	if err := writeFile(path, content); err != nil {
		return result, err
	}
	n := normalizer.NewNormalizer(dir)
	if err := n.NormalizeFile(path); err != nil {
		return result, err
	}
	first, err := os.ReadFile(path)
	if err != nil {
		return result, err
	}
	if err := n.NormalizeFile(path); err != nil {
		return result, err
	}
	second, err := os.ReadFile(path)
	if err != nil || string(first) != string(second) || !strings.Contains(string(second), "custom_key: preserved") || !strings.Contains(string(second), "title: Existing title") {
		return result, errors.New("normalizer did not preserve unknown front matter byte-stably")
	}
	return result, nil
}

func probeSite() (ProbeResult, error) {
	result := ProbeResult{Package: "internal/documentation/site", Test: "TestSiteScaffoldPreservesConfig", Observation: "preserves consumer config reports hidden pages and emits a stable redacted scaffold"}
	dir, cleanup, err := newProbeDir()
	if err != nil {
		return result, err
	}
	defer cleanup()
	page := filepath.Join(dir, "go", "api.md")
	if err := writeFile(page, "---\ntitle: Go API\npermalink: /go/api/\n---\n\n# Go API\n\n## Answer\n"); err != nil {
		return result, err
	}
	config := "title: Consumer docs\nexclude:\n  - go/\n"
	if err := writeFile(filepath.Join(dir, "_config.yml"), config); err != nil {
		return result, err
	}
	scaffold := site.NewScaffold(site.ScaffoldConfig{DocsDir: dir, OutputDir: dir, Title: "Docs", GeneratedPages: []string{"go/api.md"}})
	first, err := scaffold.Generate()
	if err != nil {
		return result, err
	}
	firstIndex, err := os.ReadFile(first.IndexPath)
	if err != nil {
		return result, err
	}
	second, err := scaffold.Generate()
	if err != nil {
		return result, err
	}
	secondIndex, err := os.ReadFile(second.IndexPath)
	if err != nil {
		return result, err
	}
	keptConfig, err := os.ReadFile(filepath.Join(dir, "_config.yml"))
	secretShape := "s" + "k-" + strings.Repeat("x", 20)
	redacted := site.Redact("tool said " + secretShape)
	if err != nil || first.ConfigCreated || second.ConfigCreated || len(first.ExcludedPages) != 1 || first.ExcludedPages[0] != "go/api.md" || string(keptConfig) != config || string(firstIndex) != string(secondIndex) || strings.Contains(redacted, secretShape) || !strings.Contains(redacted, site.RedactionMarker) {
		return result, errors.New("site scaffold did not preserve config, report exclusion, stabilize output, and redact diagnostics")
	}
	return result, nil
}

type artifact struct {
	Schema              string           `json:"schema"`
	Version             int              `json:"version"`
	Card                string           `json:"card"`
	Scenario            string           `json:"scenario"`
	Result              string           `json:"result"`
	InputManifestDigest string           `json:"input_manifest_digest"`
	ClassificationHash  string           `json:"classification_digest"`
	TestsExecuted       int              `json:"tests_executed"`
	Classifications     []Classification `json:"classifications"`
}

func classificationDigest(rows []Classification) string {
	var data strings.Builder
	for _, row := range rows {
		fmt.Fprintf(&data, "%s\t%s\t%s\t%s\n", row.Package, row.Disposition, row.Test, row.Observation)
	}
	sum := sha256.Sum256([]byte(data.String()))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func pathWithin(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

// WriteArtifact writes only beneath a real staging directory and refuses an
// existing path or symlink. This is the boundary exercised by MUT-002.
func WriteArtifact(stagingRoot, outputPath, inputManifestDigest string, rows []Classification, probes []ProbeResult, discovered []string) ([]byte, error) {
	if err := VerifyClassifications(rows, probes, discovered); err != nil {
		return nil, err
	}
	root, err := filepath.Abs(stagingRoot)
	if err != nil {
		return nil, err
	}
	output, err := filepath.Abs(outputPath)
	if err != nil {
		return nil, err
	}
	if !pathWithin(root, output) {
		return nil, errors.New("artifact output escapes staging")
	}
	rootInfo, err := os.Lstat(root)
	if err != nil || !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("artifact staging root is not a real directory")
	}
	parent := filepath.Dir(output)
	resolvedParent, err := filepath.EvalSymlinks(parent)
	if err != nil || !pathWithin(root, resolvedParent) {
		return nil, errors.New("artifact output parent escapes staging through a symlink")
	}
	if info, statErr := os.Lstat(output); statErr == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, errors.New("artifact output is a symlink")
		}
		return nil, errors.New("artifact output already exists")
	} else if !os.IsNotExist(statErr) {
		return nil, statErr
	}

	document := artifact{
		Schema: "aurum.legacy-documentation-characterization", Version: 1,
		Card: "AUR-308", Scenario: "AC-001", Result: "pass",
		InputManifestDigest: inputManifestDigest,
		ClassificationHash:  classificationDigest(rows),
		TestsExecuted:       len(probes), Classifications: rows,
	}
	data, err := json.Marshal(document)
	if err != nil {
		return nil, err
	}
	data = append(data, '\n')
	file, err := os.OpenFile(output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, err
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		_ = os.Remove(output)
		return nil, err
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(output)
		return nil, err
	}
	return data, nil
}
