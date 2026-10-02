package analyzer

import (
	"github.com/Mpaape/AurumCode/internal/grammar"
	"strings"

	"github.com/Mpaape/AurumCode/pkg/types"
)

// DiffAnalyzer analyzes code diffs and extracts metrics
type DiffAnalyzer struct {
	languageDetector *LanguageDetector
}

// NewDiffAnalyzer creates a new diff analyzer
func NewDiffAnalyzer() *DiffAnalyzer {
	return &DiffAnalyzer{
		languageDetector: NewLanguageDetector(),
	}
}

// DiffMetrics contains metrics extracted from a diff
type DiffMetrics struct {
	TotalFiles        int
	LinesAdded        int
	LinesDeleted      int
	FilesModified     int
	FilesAdded        int
	FilesDeleted      int
	TestFiles         int
	ConfigFiles       int
	LanguageBreakdown map[string]int
}

// AnalyzeDiff analyzes a diff and extracts metrics
func (a *DiffAnalyzer) AnalyzeDiff(diff *types.Diff) *DiffMetrics {
	metrics := &DiffMetrics{
		LanguageBreakdown: make(map[string]int),
	}

	for _, file := range diff.Files {
		metrics.TotalFiles++

		// Detect language
		language := a.languageDetector.DetectLanguage(file.Path)
		metrics.LanguageBreakdown[language]++

		// Check if test or config file
		if a.languageDetector.IsTestFile(file.Path) {
			metrics.TestFiles++
		}
		if a.languageDetector.IsConfigFile(file.Path) {
			metrics.ConfigFiles++
		}

		// Classify file change type
		changeType := a.classifyFileChange(&file)
		switch changeType {
		case "added":
			metrics.FilesAdded++
		case "deleted":
			metrics.FilesDeleted++
		case "modified":
			metrics.FilesModified++
		}

		// Count added and deleted lines
		for _, hunk := range file.Hunks {
			for _, line := range hunk.Lines {
				if len(line) == 0 {
					continue
				}
				switch line[0] {
				case '+':
					metrics.LinesAdded++
				case '-':
					metrics.LinesDeleted++
				}
			}
		}
	}

	return metrics
}

// classifyFileChange determines if a file was added, deleted, or modified
func (a *DiffAnalyzer) classifyFileChange(file *types.DiffFile) string {
	if len(file.Hunks) == 0 {
		return "modified"
	}

	// Check if file was deleted (all lines are deletions)
	allDeletions := true
	allAdditions := true
	hasLines := false

	for _, hunk := range file.Hunks {
		for _, line := range hunk.Lines {
			if len(line) == 0 {
				continue
			}
			hasLines = true
			if line[0] != '-' {
				allDeletions = false
			}
			if line[0] != '+' {
				allAdditions = false
			}
		}
	}

	if !hasLines {
		return "modified"
	}

	if allDeletions {
		return "deleted"
	}

	if allAdditions {
		return "added"
	}

	return "modified"
}

// ExtractChangedFunctions returns the symbols the grammar runtime finds in the
// new side of each changed hunk (added and context lines), in encounter order.
// The names come from the grammar of the file, not from per-language patterns;
// a file with no grammar yields none, and the caller treats that as "no
// structural context" rather than as "no functions".
func (a *DiffAnalyzer) ExtractChangedFunctions(file *types.DiffFile) []string {
	var functions []string
	seen := make(map[string]bool)
	for _, hunk := range file.Hunks {
		var src strings.Builder
		for _, line := range hunk.Lines {
			if len(line) < 1 || (line[0] != '+' && line[0] != ' ') {
				continue
			}
			src.WriteString(line[1:])
			src.WriteByte('\n')
		}
		for _, name := range grammar.Analyze(file.Path, []byte(src.String())).Symbols {
			if !seen[name] {
				seen[name] = true
				functions = append(functions, name)
			}
		}
	}
	return functions
}

// GetComplexityScore estimates complexity based on metrics
func (a *DiffAnalyzer) GetComplexityScore(metrics *DiffMetrics) int {
	score := 0

	// Base score from lines changed
	totalLines := metrics.LinesAdded + metrics.LinesDeleted
	if totalLines > 500 {
		score += 5
	} else if totalLines > 200 {
		score += 3
	} else if totalLines > 50 {
		score += 1
	}

	// Files modified
	if metrics.FilesModified > 10 {
		score += 3
	} else if metrics.FilesModified > 5 {
		score += 2
	} else if metrics.FilesModified > 2 {
		score += 1
	}

	// Multiple languages
	if len(metrics.LanguageBreakdown) > 3 {
		score += 2
	} else if len(metrics.LanguageBreakdown) > 1 {
		score += 1
	}

	// Config files modified (higher risk)
	if metrics.ConfigFiles > 0 {
		score += 1
	}

	// Cap at 10
	if score > 10 {
		score = 10
	}

	return score
}
