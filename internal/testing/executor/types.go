package executor

// Language represents a test language
type Language string

const (
	LanguageGo     Language = "go"
	LanguagePython Language = "python"
	LanguageJS     Language = "javascript"
)

// TestResult represents the result of running tests
type TestResult struct {
	Language       Language `json:"language"`
	Passed         int      `json:"passed"`
	Failed         int      `json:"failed"`
	Skipped        int      `json:"skipped"`
	Duration       int64    `json:"duration_ms"`
	CoveragePath   string   `json:"coverage_path,omitempty"`
	ExitCode       int      `json:"exit_code"`
	Output         string   `json:"output,omitempty"`
}

// Coverage represents test coverage metrics
type Coverage struct {
	Language       Language          `json:"language"`
	LineCoverage   float64           `json:"line_coverage"`
	BranchCoverage float64           `json:"branch_coverage,omitempty"`
	TotalLines     int               `json:"total_lines"`
	CoveredLines   int               `json:"covered_lines"`
	Files          map[string]FileCoverage `json:"files,omitempty"`
}

// FileCoverage represents coverage for a single file
type FileCoverage struct {
	Path           string  `json:"path"`
	LineCoverage   float64 `json:"line_coverage"`
	CoveredLines   int     `json:"covered_lines"`
	TotalLines     int     `json:"total_lines"`
}

// Executor runs tests for a specific language
type Executor interface {
	Language() Language
	Run(workdir string) (*TestResult, error)
	ParseCoverage(coveragePath string) (*Coverage, error)
}
