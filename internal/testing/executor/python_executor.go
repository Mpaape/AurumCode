package executor

import (
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// PythonExecutor runs Python tests
type PythonExecutor struct{}

// NewPythonExecutor creates a new Python test executor
func NewPythonExecutor() *PythonExecutor {
	return &PythonExecutor{}
}

// Language returns the language
func (e *PythonExecutor) Language() Language {
	return LanguagePython
}

// Run executes pytest with coverage
func (e *PythonExecutor) Run(workdir string) (*TestResult, error) {
	start := time.Now()

	coverageFile := filepath.Join(workdir, "coverage.xml")

	// Run: pytest --cov --cov-report=xml
	cmd := exec.Command("pytest", "--cov", "--cov-report=xml")
	cmd.Dir = workdir

	output, err := cmd.CombinedOutput()
	duration := time.Since(start).Milliseconds()

	result := &TestResult{
		Language:     LanguagePython,
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

	e.parseTestCounts(string(output), result)

	return result, nil
}

// parseTestCounts extracts test counts from pytest output
func (e *PythonExecutor) parseTestCounts(output string, result *TestResult) {
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Match pytest summary line like "5 passed, 2 failed, 1 skipped in 0.50s"
		if strings.Contains(line, "passed") || strings.Contains(line, "failed") || strings.Contains(line, "skipped") {
			// Parse counts from summary
			parts := strings.Fields(line)
			for i := 0; i < len(parts)-1; i++ {
				count := 0
				if _, err := fmt.Sscanf(parts[i], "%d", &count); err == nil {
					switch parts[i+1] {
					case "passed", "passed,":
						result.Passed = count
					case "failed", "failed,":
						result.Failed = count
					case "skipped", "skipped,":
						result.Skipped = count
					}
				}
			}
		}
	}
}

// coverageXML represents Python coverage.xml structure
type coverageXML struct {
	XMLName  xml.Name `xml:"coverage"`
	LineRate float64  `xml:"line-rate,attr"`
	Packages struct {
		Package []struct {
			Classes struct {
				Class []struct {
					Name       string  `xml:"name,attr"`
					Filename   string  `xml:"filename,attr"`
					LineRate   float64 `xml:"line-rate,attr"`
					BranchRate float64 `xml:"branch-rate,attr"`
					Lines      struct {
						Line []struct {
							Number int `xml:"number,attr"`
							Hits   int `xml:"hits,attr"`
						} `xml:"line"`
					} `xml:"lines"`
				} `xml:"class"`
			} `xml:"classes"`
		} `xml:"package"`
	} `xml:"packages"`
}

// ParseCoverage parses Python coverage XML
func (e *PythonExecutor) ParseCoverage(coveragePath string) (*Coverage, error) {
	coverage := &Coverage{
		Language: LanguagePython,
		Files:    make(map[string]FileCoverage),
	}

	// Read coverage XML file
	data, err := os.ReadFile(coveragePath)
	if err != nil {
		if os.IsNotExist(err) {
			return coverage, nil
		}
		return nil, fmt.Errorf("read coverage file: %w", err)
	}

	// Parse XML
	var cov coverageXML
	if err := xml.Unmarshal(data, &cov); err != nil {
		return nil, fmt.Errorf("parse coverage XML: %w", err)
	}

	// Extract file coverage
	for _, pkg := range cov.Packages.Package {
		for _, class := range pkg.Classes.Class {
			totalLines := len(class.Lines.Line)
			coveredLines := 0
			for _, line := range class.Lines.Line {
				if line.Hits > 0 {
					coveredLines++
				}
			}

			fc := FileCoverage{
				Path:         class.Filename,
				TotalLines:   totalLines,
				CoveredLines: coveredLines,
			}
			if totalLines > 0 {
				fc.LineCoverage = float64(coveredLines) / float64(totalLines) * 100.0
			}
			coverage.Files[class.Filename] = fc

			coverage.TotalLines += totalLines
			coverage.CoveredLines += coveredLines
		}
	}

	// Calculate overall line coverage
	if coverage.TotalLines > 0 {
		coverage.LineCoverage = float64(coverage.CoveredLines) / float64(coverage.TotalLines) * 100.0
	}

	// Use overall branch rate from XML if available
	if cov.LineRate > 0 {
		coverage.BranchCoverage = cov.LineRate * 100.0
	}

	return coverage, nil
}
