package csharp

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
// this extractor cannot document a project without executing code that the
// documented repository controls.
//
// The XML documentation file xmldocmd consumes is a compiler output, so it only
// exists after `dotnet build`, and `dotnet build` is MSBuild. MSBuild executes
// what the project file tells it to execute:
//   - <Exec Command="..."> tasks in the .csproj and in any target it imports;
//   - <UsingTask> tasks, which are arbitrary .NET assemblies loaded into the
//     MSBuild process (including inline task source compiled on the spot);
//   - .props/.targets imported by the project or contributed by restored NuGet
//     packages, plus Directory.Build.props/targets picked up from the tree;
//   - source generators and analyzers referenced by the project, which run
//     inside the compiler.
//
// xmldocmd then loads the assembly the build produced and reflects over it,
// which runs its module initializers and static constructors.
//
// There is no MSBuild-free path to the same artifact, so the decision is
// fail-closed and lives at the composition root: cmd/regenerate-docs does not
// register this extractor unless the consumer names C# in
// AURUMCODE_ALLOW_REPO_CODE_EXECUTION. Constructing it directly is the caller
// asserting the tree is trusted.
const ExecutesRepositoryCode = "dotnet build runs MSBuild, so it executes <Exec> tasks, <UsingTask> assemblies and imported .targets/.props from the repository, and xmldocmd afterwards loads the assembly the build produced"

// CSharpExtractor extracts documentation from C# source code using xmldocmd.
//
// See ExecutesRepositoryCode: every Extract call builds the projects it finds,
// which runs code from the tree being documented.
type CSharpExtractor struct {
	runner site.CommandRunner
}

// NewCSharpExtractor creates a new C# documentation extractor.
//
// The returned extractor executes repository-controlled code when it runs (see
// ExecutesRepositoryCode). It is intentionally absent from the default
// registration in cmd/regenerate-docs; a caller that builds one directly is
// accepting that the tree it documents may run arbitrary code as this process.
func NewCSharpExtractor(runner site.CommandRunner) *CSharpExtractor {
	return &CSharpExtractor{
		runner: runner,
	}
}

// Extract generates documentation from C# source code
func (c *CSharpExtractor) Extract(ctx context.Context, req *extractors.ExtractRequest) (*extractors.ExtractResult, error) {
	// Validate request
	if req.Language != extractors.LanguageCSharp {
		return nil, fmt.Errorf("invalid language: expected %s, got %s", extractors.LanguageCSharp, req.Language)
	}

	// Validate source directory
	if _, err := os.Stat(req.SourceDir); err != nil {
		return nil, fmt.Errorf("invalid source directory: %w", err)
	}

	// Ensure output directory exists
	if err := os.MkdirAll(req.OutputDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	// Find all C# projects
	projects, err := c.findCSharpProjects(req.SourceDir)
	if err != nil {
		return nil, fmt.Errorf("failed to find C# projects: %w", err)
	}

	if len(projects) == 0 {
		return &extractors.ExtractResult{
			Language: extractors.LanguageCSharp,
			Files:    []string{},
			Stats: extractors.ExtractionStats{
				FilesProcessed: 0,
				DocsGenerated:  0,
				LinesProcessed: 0,
			},
		}, nil
	}

	// Extract documentation for each project
	result := &extractors.ExtractResult{
		Language: extractors.LanguageCSharp,
		Files:    []string{},
		Errors:   []error{},
	}

	for _, project := range projects {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		// Build project with documentation
		xmlPath, err := c.buildProjectWithDocs(ctx, project)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("project %s: %w", project, err))
			continue
		}

		// dotnet build exiting 0 is not evidence: without the XML file on disk
		// there is nothing to convert, and that is a failure for this project.
		if xmlPath == "" {
			result.Errors = append(result.Errors, fmt.Errorf(
				"project %s: dotnet build exited successfully but wrote no XML documentation file: %w",
				project, extractors.ErrNoOutputProduced))
			continue
		}

		// Extract documentation using xmldocmd
		projectName := c.getProjectName(project)
		outputPath := filepath.Join(req.OutputDir, projectName)

		err = c.extractWithXmlDocMd(ctx, xmlPath, outputPath)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("xmldocmd %s: %w", projectName, err))
			continue
		}

		// Count generated files. Only what xmldocmd actually wrote counts.
		files, err := c.countGeneratedFiles(outputPath)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("failed to inspect %s: %w", outputPath, err))
			continue
		}

		if confirmErr := extractors.ConfirmOutputFiles("xmldocmd", outputPath, files); confirmErr != nil {
			result.Errors = append(result.Errors, fmt.Errorf("project %s: %w", project, confirmErr))
			continue
		}

		result.Files = append(result.Files, files...)
		result.Stats.DocsGenerated += len(files)

		// Count total lines
		for _, file := range files {
			lines, _ := c.countLines(file)
			result.Stats.LinesProcessed += lines
		}

		result.Stats.FilesProcessed++
	}

	return result, nil
}

// Validate checks if dotnet and xmldocmd are available.
//
// Only a genuine lookup failure means a tool is unavailable; a tool that is
// installed and exits non-zero is an extraction error, not a skip.
func (c *CSharpExtractor) Validate(ctx context.Context) error {
	// Check for dotnet
	_, err := c.runner.Run(ctx, "dotnet", []string{"--version"}, ".", nil)
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return extractors.NewToolUnavailableError("dotnet", "please install .NET SDK from https://dot.net", err)
		}
		return fmt.Errorf("dotnet is installed but failed: %w", err)
	}

	// Check for xmldocmd
	_, err = c.runner.Run(ctx, "xmldocmd", []string{"--version"}, ".", nil)
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return extractors.NewToolUnavailableError("xmldocmd",
				"please install with 'dotnet tool install -g xmldocmd'", err)
		}
		return fmt.Errorf("xmldocmd is installed but failed: %w", err)
	}

	return nil
}

// Language returns the language this extractor handles
func (c *CSharpExtractor) Language() extractors.Language {
	return extractors.LanguageCSharp
}

// findCSharpProjects finds all .csproj files in the source directory
func (c *CSharpExtractor) findCSharpProjects(rootDir string) ([]string, error) {
	projects := []string{}

	err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		// Skip directories
		if info.IsDir() {
			// Skip common excluded directories
			dirName := filepath.Base(path)
			if dirName == "bin" || dirName == "obj" || dirName == "node_modules" ||
				dirName == ".git" || dirName == ".taskmaster" || strings.HasPrefix(dirName, ".") {
				return filepath.SkipDir
			}
			return nil
		}

		// Check for .csproj files
		if strings.HasSuffix(path, ".csproj") {
			projects = append(projects, path)
		}

		return nil
	})

	return projects, err
}

// getProjectName extracts project name from .csproj path
func (c *CSharpExtractor) getProjectName(projectPath string) string {
	normalized := strings.ReplaceAll(projectPath, "\\", "/")
	base := filepath.Base(normalized)
	return strings.TrimSuffix(base, ".csproj")
}

// buildProjectWithDocs builds the C# project with XML documentation enabled
func (c *CSharpExtractor) buildProjectWithDocs(ctx context.Context, projectPath string) (string, error) {
	// Get project directory
	projectDir := filepath.Dir(projectPath)
	projectName := c.getProjectName(projectPath)

	// Build with documentation generation
	args := []string{
		"build",
		projectPath,
		"/p:GenerateDocumentationFile=true",
	}

	// This line executes consumer code: MSBuild runs the tasks and imports the
	// .csproj declares, with the working directory inside the project's own
	// directory. See ExecutesRepositoryCode.
	_, err := c.runner.Run(ctx, "dotnet", args, projectDir, nil)
	if err != nil {
		return "", fmt.Errorf("dotnet build failed: %w", err)
	}

	// Find the generated XML file
	// It should be in bin/Debug/netX.0/ProjectName.xml or bin/Release/netX.0/ProjectName.xml
	xmlPath := c.findXmlDocFile(projectDir, projectName)
	return xmlPath, nil
}

// findXmlDocFile finds the generated XML documentation file
func (c *CSharpExtractor) findXmlDocFile(projectDir, projectName string) string {
	// Common output paths
	patterns := []string{
		filepath.Join(projectDir, "bin", "Debug", "*", projectName+".xml"),
		filepath.Join(projectDir, "bin", "Release", "*", projectName+".xml"),
		filepath.Join(projectDir, "bin", "Debug", projectName+".xml"),
		filepath.Join(projectDir, "bin", "Release", projectName+".xml"),
	}

	for _, pattern := range patterns {
		matches, _ := filepath.Glob(pattern)
		if len(matches) > 0 {
			return matches[0]
		}
	}

	return ""
}

// extractWithXmlDocMd uses xmldocmd to convert XML to markdown
func (c *CSharpExtractor) extractWithXmlDocMd(ctx context.Context, xmlPath, outputPath string) error {
	// Ensure output directory exists
	if err := os.MkdirAll(outputPath, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Run xmldocmd
	args := []string{
		xmlPath,
		outputPath,
	}

	_, err := c.runner.Run(ctx, "xmldocmd", args, ".", nil)
	if err != nil {
		return fmt.Errorf("xmldocmd failed: %w", err)
	}

	return nil
}

// countGeneratedFiles counts markdown files in output directory
func (c *CSharpExtractor) countGeneratedFiles(outputDir string) ([]string, error) {
	files := []string{}

	err := filepath.Walk(outputDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		if !info.IsDir() && strings.HasSuffix(path, ".md") {
			files = append(files, path)
		}

		return nil
	})

	return files, err
}

// countLines counts lines in a file
func (c *CSharpExtractor) countLines(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}

	count := 0
	for _, b := range data {
		if b == '\n' {
			count++
		}
	}

	if len(data) > 0 && data[len(data)-1] != '\n' {
		count++
	}

	return count, nil
}
