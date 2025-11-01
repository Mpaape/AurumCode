package analyzer

import (
	"aurumcode/pkg/types"
	"strings"
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
	TotalFiles       int
	LinesAdded       int
	LinesDeleted     int
	FilesModified    int
	FilesAdded       int
	FilesDeleted     int
	TestFiles        int
	ConfigFiles      int
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

// ExtractChangedFunctions extracts function names from changed hunks
func (a *DiffAnalyzer) ExtractChangedFunctions(file *types.DiffFile) []string {
	var functions []string
	seen := make(map[string]bool)

	language := a.languageDetector.DetectLanguage(file.Path)

	for _, hunk := range file.Hunks {
		for _, line := range hunk.Lines {
			// Look for function declarations
			funcName := a.extractFunctionName(line, language)
			if funcName != "" && !seen[funcName] {
				functions = append(functions, funcName)
				seen[funcName] = true
			}
		}
	}

	return functions
}

// extractFunctionName extracts function name from a line based on language
func (a *DiffAnalyzer) extractFunctionName(line, language string) string {
	if len(line) < 2 {
		return ""
	}

	// Skip non-addition lines for function detection
	if line[0] != '+' && line[0] != ' ' {
		return ""
	}

	content := strings.TrimSpace(line[1:])

	switch language {
	case "go":
		return a.extractGoFunction(content)
	case "javascript", "typescript":
		return a.extractJSFunction(content)
	case "python":
		return a.extractPythonFunction(content)
	case "java", "kotlin":
		return a.extractJavaFunction(content)
	default:
		return ""
	}
}

// extractGoFunction extracts Go function names
func (a *DiffAnalyzer) extractGoFunction(line string) string {
	if !strings.HasPrefix(line, "func ") {
		return ""
	}

	// func FunctionName( or func (receiver) FunctionName(
	parts := strings.Fields(line)
	if len(parts) < 2 {
		return ""
	}

	// Check for method receiver
	if strings.HasPrefix(parts[1], "(") {
		// Method: func (r *Receiver) MethodName(
		if len(parts) < 4 {
			return ""
		}
		funcName := parts[3]
		if idx := strings.Index(funcName, "("); idx != -1 {
			return funcName[:idx]
		}
		return funcName
	}

	// Regular function: func FunctionName(
	funcName := parts[1]
	if idx := strings.Index(funcName, "("); idx != -1 {
		return funcName[:idx]
	}

	return funcName
}

// extractJSFunction extracts JavaScript/TypeScript function names
func (a *DiffAnalyzer) extractJSFunction(line string) string {
	// function name( or const name = ( or name: function( or name() {
	if strings.Contains(line, "function ") {
		parts := strings.Split(line, "function ")
		if len(parts) > 1 {
			funcPart := strings.TrimSpace(parts[1])
			if idx := strings.Index(funcPart, "("); idx != -1 {
				return funcPart[:idx]
			}
		}
	}

	// Arrow functions: const name = ( or name = (
	if strings.Contains(line, " = ") && strings.Contains(line, "=>") {
		parts := strings.Split(line, " = ")
		if len(parts) > 0 {
			namePart := strings.TrimSpace(parts[0])
			namePart = strings.TrimPrefix(namePart, "const ")
			namePart = strings.TrimPrefix(namePart, "let ")
			namePart = strings.TrimPrefix(namePart, "var ")
			if namePart != "" {
				return namePart
			}
		}
	}

	return ""
}

// extractPythonFunction extracts Python function names
func (a *DiffAnalyzer) extractPythonFunction(line string) string {
	if !strings.HasPrefix(line, "def ") {
		return ""
	}

	parts := strings.Fields(line)
	if len(parts) < 2 {
		return ""
	}

	funcName := parts[1]
	if idx := strings.Index(funcName, "("); idx != -1 {
		return funcName[:idx]
	}

	return funcName
}

// extractJavaFunction extracts Java/Kotlin function names
func (a *DiffAnalyzer) extractJavaFunction(line string) string {
	// Look for method signatures: public void methodName( or fun methodName(
	if strings.Contains(line, "fun ") {
		// Kotlin function
		parts := strings.Split(line, "fun ")
		if len(parts) > 1 {
			funcPart := strings.TrimSpace(parts[1])
			if idx := strings.Index(funcPart, "("); idx != -1 {
				return funcPart[:idx]
			}
		}
	}

	// Java method - look for pattern: visibility returnType methodName(
	words := strings.Fields(line)
	for i, word := range words {
		if strings.Contains(word, "(") && i > 0 {
			// Previous word might be the method name
			methodName := word
			if idx := strings.Index(methodName, "("); idx != -1 {
				methodName = methodName[:idx]
			}
			// Skip constructors and keywords
			if methodName != "if" && methodName != "for" && methodName != "while" && methodName != "switch" {
				return methodName
			}
		}
	}

	return ""
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
