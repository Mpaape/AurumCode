package executor

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// GoExecutor runs Go tests
type GoExecutor struct{}

// NewGoExecutor creates a new Go test executor
func NewGoExecutor() *GoExecutor {
	return &GoExecutor{}
}

// Language returns the language
func (e *GoExecutor) Language() Language {
	return LanguageGo
}

// Run executes Go tests with coverage
func (e *GoExecutor) Run(workdir string) (*TestResult, error) {
	start := time.Now()

	coverageFile := filepath.Join(workdir, "coverage.out")

	// Run: go test ./... -coverprofile=coverage.out
	cmd := exec.Command("go", "test", "./...", "-coverprofile="+coverageFile)
	cmd.Dir = workdir

	output, err := cmd.CombinedOutput()
	duration := time.Since(start).Milliseconds()

	result := &TestResult{
		Language:     LanguageGo,
		Duration:     duration,
		CoveragePath: coverageFile,
		Output:       string(output),
	}

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			result.ExitCode = 1
		}
	}

	// Parse test counts from output
	e.parseTestCounts(string(output), result)

	return result, nil
}

// parseTestCounts extracts test counts from output
func (e *GoExecutor) parseTestCounts(output string, result *TestResult) {
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Match lines like "--- PASS: TestFoo (0.00s)" or "--- FAIL: TestBar (0.01s)"
		if strings.HasPrefix(line, "--- PASS:") {
			result.Passed++
		} else if strings.HasPrefix(line, "--- FAIL:") {
			result.Failed++
		} else if strings.HasPrefix(line, "--- SKIP:") {
			result.Skipped++
		}
	}
}

// ParseCoverage parses Go coverage output
func (e *GoExecutor) ParseCoverage(coveragePath string) (*Coverage, error) {
	coverage := &Coverage{
		Language: LanguageGo,
		Files:    make(map[string]FileCoverage),
	}

	// Read coverage file
	data, err := os.ReadFile(coveragePath)
	if err != nil {
		// If coverage file doesn't exist, return empty coverage
		if os.IsNotExist(err) {
			return coverage, nil
		}
		return nil, fmt.Errorf("read coverage file: %w", err)
	}

	lines := strings.Split(string(data), "\n")
	if len(lines) == 0 {
		return coverage, nil
	}

	// Skip mode line (e.g., "mode: set")
	for i := 1; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}

		// Format: file.go:start.col,end.col statements count
		parts := strings.Fields(line)
		if len(parts) < 3 {
			continue
		}

		// Extract file path
		colonIdx := strings.LastIndex(parts[0], ":")
		if colonIdx == -1 {
			continue
		}
		filePath := parts[0][:colonIdx]

		// Parse statement count and coverage count
		stmtCount := 0
		covCount := 0
		fmt.Sscanf(parts[1], "%d", &stmtCount)
		fmt.Sscanf(parts[2], "%d", &covCount)

		// Update file coverage
		fc := coverage.Files[filePath]
		fc.Path = filePath
		fc.TotalLines += stmtCount
		if covCount > 0 {
			fc.CoveredLines += stmtCount
		}
		coverage.Files[filePath] = fc

		// Update totals
		coverage.TotalLines += stmtCount
		if covCount > 0 {
			coverage.CoveredLines += stmtCount
		}
	}

	// Calculate line coverage percentage
	if coverage.TotalLines > 0 {
		coverage.LineCoverage = float64(coverage.CoveredLines) / float64(coverage.TotalLines) * 100.0
	}

	// Update per-file percentages
	for path, fc := range coverage.Files {
		if fc.TotalLines > 0 {
			fc.LineCoverage = float64(fc.CoveredLines) / float64(fc.TotalLines) * 100.0
			coverage.Files[path] = fc
		}
	}

	return coverage, nil
}
