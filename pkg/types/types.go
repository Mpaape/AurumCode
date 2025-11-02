package types

// Event represents a Git provider event (webhook payload)
type Event struct {
	Repo       string `json:"repo" yaml:"repo"`
	RepoOwner  string `json:"repo_owner" yaml:"repo_owner"`
	Provider   string `json:"provider" yaml:"provider"` // "github", "gitea", "git"
	EventType  string `json:"event_type" yaml:"event_type"`
	Action     string `json:"action" yaml:"action"`         // PR action: opened, synchronize, closed, etc.
	DeliveryID string `json:"delivery_id" yaml:"delivery_id"`
	PRNumber   int    `json:"pr_number" yaml:"pr_number"`
	CommitSHA  string `json:"commit_sha" yaml:"commit_sha"`
	Branch     string `json:"branch" yaml:"branch"`
	Merged     bool   `json:"merged" yaml:"merged"` // For PR closed events
	Payload    []byte `json:"payload" yaml:"payload"`
	Signature  string `json:"signature" yaml:"signature"`
}

// DiffFile represents a single file in a diff
type DiffFile struct {
	Path  string     `json:"path" yaml:"path"`
	Lang  string     `json:"lang" yaml:"lang"`
	Hunks []DiffHunk `json:"hunks" yaml:"hunks"`
}

// DiffHunk represents a single hunk within a file diff
type DiffHunk struct {
	OldStart int      `json:"old_start" yaml:"old_start"`
	OldLines int      `json:"old_lines" yaml:"old_lines"`
	NewStart int      `json:"new_start" yaml:"new_start"`
	NewLines int      `json:"new_lines" yaml:"new_lines"`
	Lines    []string `json:"lines" yaml:"lines"`
}

// Diff represents the complete parsed diff
type Diff struct {
	Files []DiffFile `json:"files" yaml:"files"`
}

// ReviewIssue represents a single finding in a code review
type ReviewIssue struct {
	ID         string `json:"id" yaml:"id"`
	File       string `json:"file" yaml:"file"`
	Line       int    `json:"line" yaml:"line"`
	Severity   string `json:"severity" yaml:"severity"` // "error", "warning", "info"
	RuleID     string `json:"rule_id" yaml:"rule_id"`
	Message    string `json:"message" yaml:"message"`
	Suggestion string `json:"suggestion,omitempty" yaml:"suggestion,omitempty"`
}

// ISOScores represents ISO/IEC 25010 quality characteristics
type ISOScores struct {
	Functionality   int `json:"functionality" yaml:"functionality"`
	Reliability     int `json:"reliability" yaml:"reliability"`
	Usability       int `json:"usability" yaml:"usability"`
	Efficiency      int `json:"efficiency" yaml:"efficiency"`
	Maintainability int `json:"maintainability" yaml:"maintainability"`
	Portability     int `json:"portability" yaml:"portability"`
	Security        int `json:"security" yaml:"security"`
	Compatibility   int `json:"compatibility" yaml:"compatibility"`
}

// ReviewResult represents the complete output of a code review
type ReviewResult struct {
	Issues           []ReviewIssue    `json:"issues" yaml:"issues"`
	ISOScores        *ISOScores       `json:"iso_scores,omitempty" yaml:"iso_scores,omitempty"`
	Summary          string           `json:"summary" yaml:"summary"`
	OverallScore     float64          `json:"overall_score" yaml:"overall_score"`
	LineComments     []ReviewComment  `json:"line_comments" yaml:"line_comments"`         // Line-by-line comments
	FileComments     []ReviewComment  `json:"file_comments" yaml:"file_comments"`         // File-level summaries
	CommitComment    string           `json:"commit_comment" yaml:"commit_comment"`       // Overall PR/commit summary
	Metadata         map[string]string `json:"metadata,omitempty" yaml:"metadata,omitempty"`
}

// ReviewComment represents a comment to post on a PR
type ReviewComment struct {
	Path     string `json:"path" yaml:"path"`         // Empty for general PR comment
	Line     int    `json:"line" yaml:"line"`         // 0 for general comment
	Body     string `json:"body" yaml:"body"`
	CommitID string `json:"commit_id" yaml:"commit_id"`
}

// QAArtifacts tracks generated test and quality artifacts
type QAArtifacts struct {
	CoverageReportPath string            `json:"coverage_report_path,omitempty" yaml:"coverage_report_path,omitempty"`
	SARIFPath          string            `json:"sarif_path,omitempty" yaml:"sarif_path,omitempty"`
	SBOMPath           string            `json:"sbom_path,omitempty" yaml:"sbom_path,omitempty"`
	ChangelogPath      string            `json:"changelog_path,omitempty" yaml:"changelog_path,omitempty"`
	Coverage           *CoverageReport   `json:"coverage,omitempty" yaml:"coverage,omitempty"`
}

// CoverageReport represents test coverage metrics
type CoverageReport struct {
	LineCoverage   float64 `json:"line_coverage" yaml:"line_coverage"`
	BranchCoverage float64 `json:"branch_coverage" yaml:"branch_coverage"`
	TotalLines     int     `json:"total_lines" yaml:"total_lines"`
	CoveredLines   int     `json:"covered_lines" yaml:"covered_lines"`
}

