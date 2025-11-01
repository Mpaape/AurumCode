package analyzer

import (
	"aurumcode/pkg/types"
	"testing"
)

func TestAnalyzeDiff(t *testing.T) {
	analyzer := NewDiffAnalyzer()

	diff := &types.Diff{
		Files: []types.DiffFile{
			{
				Path: "main.go",
				Hunks: []types.DiffHunk{
					{
						Lines: []string{
							"+package main",
							"+",
							"+func main() {",
							"+\tprintln(\"hello\")",
							"+}",
						},
					},
				},
			},
			{
				Path: "service.go",
				Hunks: []types.DiffHunk{
					{
						Lines: []string{
							" func Service() {",
							"-\told code",
							"+\tnew code",
							" }",
						},
					},
				},
			},
			{
				Path: "handler_test.go",
				Hunks: []types.DiffHunk{
					{
						Lines: []string{
							"+func TestHandler(t *testing.T) {",
							"+}",
						},
					},
				},
			},
			{
				Path: "config.json",
				Hunks: []types.DiffHunk{
					{
						Lines: []string{
							"+{\"key\": \"value\"}",
						},
					},
				},
			},
		},
	}

	metrics := analyzer.AnalyzeDiff(diff)

	if metrics.TotalFiles != 4 {
		t.Errorf("TotalFiles = %d, want 4", metrics.TotalFiles)
	}

	if metrics.LinesAdded != 9 {
		t.Errorf("LinesAdded = %d, want 9", metrics.LinesAdded)
	}

	if metrics.LinesDeleted != 1 {
		t.Errorf("LinesDeleted = %d, want 1", metrics.LinesDeleted)
	}

	if metrics.TestFiles != 1 {
		t.Errorf("TestFiles = %d, want 1", metrics.TestFiles)
	}

	if metrics.ConfigFiles != 1 {
		t.Errorf("ConfigFiles = %d, want 1", metrics.ConfigFiles)
	}

	if metrics.LanguageBreakdown["go"] != 3 {
		t.Errorf("Go files = %d, want 3", metrics.LanguageBreakdown["go"])
	}

	if metrics.LanguageBreakdown["json"] != 1 {
		t.Errorf("JSON files = %d, want 1", metrics.LanguageBreakdown["json"])
	}
}

func TestClassifyFileChange(t *testing.T) {
	analyzer := NewDiffAnalyzer()

	tests := []struct {
		name     string
		file     types.DiffFile
		expected string
	}{
		{
			name: "file added",
			file: types.DiffFile{
				Path: "new.go",
				Hunks: []types.DiffHunk{
					{
						Lines: []string{
							"+package main",
							"+func main() {}",
						},
					},
				},
			},
			expected: "added",
		},
		{
			name: "file deleted",
			file: types.DiffFile{
				Path: "old.go",
				Hunks: []types.DiffHunk{
					{
						Lines: []string{
							"-package main",
							"-func main() {}",
						},
					},
				},
			},
			expected: "deleted",
		},
		{
			name: "file modified",
			file: types.DiffFile{
				Path: "existing.go",
				Hunks: []types.DiffHunk{
					{
						Lines: []string{
							" package main",
							"-old line",
							"+new line",
							" func main() {}",
						},
					},
				},
			},
			expected: "modified",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := analyzer.classifyFileChange(&test.file)
			if result != test.expected {
				t.Errorf("classifyFileChange() = %s, want %s", result, test.expected)
			}
		})
	}
}

func TestExtractChangedFunctions_Go(t *testing.T) {
	analyzer := NewDiffAnalyzer()

	file := &types.DiffFile{
		Path: "service.go",
		Hunks: []types.DiffHunk{
			{
				Lines: []string{
					"+func NewService() *Service {",
					"+\treturn &Service{}",
					"+}",
					" ",
					"+func (s *Service) HandleRequest(req Request) {",
					"+\t// implementation",
					"+}",
				},
			},
		},
	}

	functions := analyzer.ExtractChangedFunctions(file)

	if len(functions) != 2 {
		t.Errorf("expected 2 functions, got %d", len(functions))
	}

	expected := map[string]bool{
		"NewService":    true,
		"HandleRequest": true,
	}

	for _, fn := range functions {
		if !expected[fn] {
			t.Errorf("unexpected function: %s", fn)
		}
	}
}

func TestExtractChangedFunctions_JavaScript(t *testing.T) {
	analyzer := NewDiffAnalyzer()

	file := &types.DiffFile{
		Path: "service.js",
		Hunks: []types.DiffHunk{
			{
				Lines: []string{
					"+function handleRequest(req) {",
					"+  return process(req)",
					"+}",
					" ",
					"+const processData = (data) => {",
					"+  return transform(data)",
					"+}",
				},
			},
		},
	}

	functions := analyzer.ExtractChangedFunctions(file)

	if len(functions) != 2 {
		t.Errorf("expected 2 functions, got %d", len(functions))
	}

	expected := map[string]bool{
		"handleRequest": true,
		"processData":   true,
	}

	for _, fn := range functions {
		if !expected[fn] {
			t.Errorf("unexpected function: %s", fn)
		}
	}
}

func TestExtractChangedFunctions_Python(t *testing.T) {
	analyzer := NewDiffAnalyzer()

	file := &types.DiffFile{
		Path: "service.py",
		Hunks: []types.DiffHunk{
			{
				Lines: []string{
					"+def handle_request(req):",
					"+    return process(req)",
					" ",
					"+def process_data(data):",
					"+    return transform(data)",
				},
			},
		},
	}

	functions := analyzer.ExtractChangedFunctions(file)

	if len(functions) != 2 {
		t.Errorf("expected 2 functions, got %d", len(functions))
	}

	expected := map[string]bool{
		"handle_request": true,
		"process_data":   true,
	}

	for _, fn := range functions {
		if !expected[fn] {
			t.Errorf("unexpected function: %s", fn)
		}
	}
}

func TestGetComplexityScore(t *testing.T) {
	analyzer := NewDiffAnalyzer()

	tests := []struct {
		name     string
		metrics  *DiffMetrics
		expected int
	}{
		{
			name: "low complexity",
			metrics: &DiffMetrics{
				LinesAdded:        10,
				LinesDeleted:      5,
				FilesModified:     1,
				LanguageBreakdown: map[string]int{"go": 1},
			},
			expected: 0,
		},
		{
			name: "medium complexity",
			metrics: &DiffMetrics{
				LinesAdded:        100,
				LinesDeleted:      50,
				FilesModified:     3,
				LanguageBreakdown: map[string]int{"go": 2, "javascript": 1},
			},
			expected: 3,
		},
		{
			name: "high complexity",
			metrics: &DiffMetrics{
				LinesAdded:        300,
				LinesDeleted:      200,
				FilesModified:     15,
				ConfigFiles:       2,
				LanguageBreakdown: map[string]int{"go": 5, "javascript": 5, "python": 3, "rust": 2},
			},
			expected: 9,
		},
		{
			name: "with config files",
			metrics: &DiffMetrics{
				LinesAdded:        50,
				LinesDeleted:      20,
				FilesModified:     2,
				ConfigFiles:       1,
				LanguageBreakdown: map[string]int{"yaml": 1, "go": 1},
			},
			expected: 3,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			score := analyzer.GetComplexityScore(test.metrics)
			if score != test.expected {
				t.Errorf("GetComplexityScore() = %d, want %d", score, test.expected)
			}
		})
	}
}

func TestExtractGoFunction_Methods(t *testing.T) {
	analyzer := NewDiffAnalyzer()

	tests := []struct {
		line     string
		expected string
	}{
		{"func NewService() *Service {", "NewService"},
		{"func (s *Service) Start() error {", "Start"},
		{"func (s Service) Stop() {", "Stop"},
		{"func HandleRequest(req Request) Response {", "HandleRequest"},
		{"  not a function", ""},
	}

	for _, test := range tests {
		result := analyzer.extractGoFunction(test.line)
		if result != test.expected {
			t.Errorf("extractGoFunction(%q) = %q, want %q", test.line, result, test.expected)
		}
	}
}

func TestExtractJSFunction_ArrowFunctions(t *testing.T) {
	analyzer := NewDiffAnalyzer()

	tests := []struct {
		line     string
		expected string
	}{
		{"const handleClick = () => {", "handleClick"},
		{"let processData = (data) => {", "processData"},
		{"var transform = x => x * 2", "transform"},
		{"function normalFunc() {", "normalFunc"},
		{"  not a function", ""},
	}

	for _, test := range tests {
		result := analyzer.extractJSFunction(test.line)
		if result != test.expected {
			t.Errorf("extractJSFunction(%q) = %q, want %q", test.line, result, test.expected)
		}
	}
}

func TestAnalyzeDiff_EmptyDiff(t *testing.T) {
	analyzer := NewDiffAnalyzer()

	diff := &types.Diff{
		Files: []types.DiffFile{},
	}

	metrics := analyzer.AnalyzeDiff(diff)

	if metrics.TotalFiles != 0 {
		t.Errorf("TotalFiles = %d, want 0", metrics.TotalFiles)
	}

	if metrics.LinesAdded != 0 {
		t.Errorf("LinesAdded = %d, want 0", metrics.LinesAdded)
	}

	if len(metrics.LanguageBreakdown) != 0 {
		t.Errorf("LanguageBreakdown should be empty, got %d entries", len(metrics.LanguageBreakdown))
	}
}
