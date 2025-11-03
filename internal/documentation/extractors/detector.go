package extractors

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Detector scans projects to detect programming languages
type Detector struct {
	excludedDirs map[string]bool
	extensions   map[string]Language
}

// DetectionResult contains the results of language detection
type DetectionResult struct {
	// Languages maps each detected language to its statistics
	Languages map[Language]*LanguageStats

	// TotalFiles is the total number of files scanned
	TotalFiles int

	// TotalLines is the total lines of code across all files
	TotalLines int
}

// LanguageStats contains statistics for a detected language
type LanguageStats struct {
	// Language is the programming language
	Language Language

	// FileCount is the number of files for this language
	FileCount int

	// Files is the list of file paths for this language
	Files []string

	// LineCount is the total lines of code for this language
	LineCount int
}

// NewDetector creates a new language detector
func NewDetector() *Detector {
	d := &Detector{
		excludedDirs: make(map[string]bool),
		extensions:   make(map[string]Language),
	}

	// Set default excluded directories
	d.excludedDirs["vendor"] = true
	d.excludedDirs["node_modules"] = true
	d.excludedDirs[".git"] = true
	d.excludedDirs["bin"] = true
	d.excludedDirs["obj"] = true
	d.excludedDirs["_site"] = true
	d.excludedDirs[".taskmaster"] = true
	d.excludedDirs["dist"] = true
	d.excludedDirs["build"] = true
	d.excludedDirs["target"] = true
	d.excludedDirs[".next"] = true
	d.excludedDirs["__pycache__"] = true

	// Map file extensions to languages
	d.extensions[".go"] = LanguageGo
	d.extensions[".js"] = LanguageJavaScript
	d.extensions[".jsx"] = LanguageJavaScript
	d.extensions[".mjs"] = LanguageJavaScript
	d.extensions[".cjs"] = LanguageJavaScript
	d.extensions[".ts"] = LanguageTypeScript
	d.extensions[".tsx"] = LanguageTypeScript
	d.extensions[".py"] = LanguagePython
	d.extensions[".pyw"] = LanguagePython
	d.extensions[".cs"] = LanguageCSharp
	d.extensions[".cpp"] = LanguageCPP
	d.extensions[".cc"] = LanguageCPP
	d.extensions[".cxx"] = LanguageCPP
	d.extensions[".c"] = LanguageCPP
	d.extensions[".h"] = LanguageCPP
	d.extensions[".hpp"] = LanguageCPP
	d.extensions[".rs"] = LanguageRust
	d.extensions[".sh"] = LanguageBash
	d.extensions[".bash"] = LanguageBash
	d.extensions[".ps1"] = LanguagePowerShell
	d.extensions[".psm1"] = LanguagePowerShell
	d.extensions[".java"] = LanguageJava

	return d
}

// WithExcludedDirs adds additional directories to exclude
func (d *Detector) WithExcludedDirs(dirs ...string) *Detector {
	for _, dir := range dirs {
		d.excludedDirs[dir] = true
	}
	return d
}

// WithExtensions adds custom file extension mappings
func (d *Detector) WithExtensions(extMap map[string]Language) *Detector {
	for ext, lang := range extMap {
		d.extensions[ext] = lang
	}
	return d
}

// Detect scans the root directory and detects all languages in use
func (d *Detector) Detect(ctx context.Context, rootDir string) (*DetectionResult, error) {
	// Validate root directory
	info, err := os.Stat(rootDir)
	if err != nil {
		return nil, fmt.Errorf("invalid root directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("path is not a directory: %s", rootDir)
	}

	result := &DetectionResult{
		Languages: make(map[Language]*LanguageStats),
	}

	// Walk the directory tree
	err = filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			// Skip files/directories that cause errors
			return nil
		}

		// Check context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Skip directories in exclusion list
		if info.IsDir() {
			dirName := filepath.Base(path)
			if d.excludedDirs[dirName] {
				return filepath.SkipDir
			}
			return nil
		}

		// Check file extension
		ext := strings.ToLower(filepath.Ext(path))
		lang, ok := d.extensions[ext]
		if !ok {
			// Unknown extension, skip
			return nil
		}

		// Count lines in file
		lineCount, err := d.countLines(path)
		if err != nil {
			// If we can't read the file, skip it
			return nil
		}

		// Update statistics
		stats, exists := result.Languages[lang]
		if !exists {
			stats = &LanguageStats{
				Language: lang,
				Files:    []string{},
			}
			result.Languages[lang] = stats
		}

		stats.FileCount++
		stats.Files = append(stats.Files, path)
		stats.LineCount += lineCount

		result.TotalFiles++
		result.TotalLines += lineCount

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("directory scan failed: %w", err)
	}

	return result, nil
}

// countLines counts the number of lines in a file
func (d *Detector) countLines(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}

	// Count newlines
	count := 0
	for _, b := range data {
		if b == '\n' {
			count++
		}
	}

	// If file doesn't end with newline, add 1
	if len(data) > 0 && data[len(data)-1] != '\n' {
		count++
	}

	return count, nil
}

// GetLanguages returns a slice of all detected languages
func (r *DetectionResult) GetLanguages() []Language {
	langs := make([]Language, 0, len(r.Languages))
	for lang := range r.Languages {
		langs = append(langs, lang)
	}
	return langs
}

// HasLanguage checks if a specific language was detected
func (r *DetectionResult) HasLanguage(lang Language) bool {
	_, ok := r.Languages[lang]
	return ok
}

// GetStats returns statistics for a specific language
func (r *DetectionResult) GetStats(lang Language) (*LanguageStats, bool) {
	stats, ok := r.Languages[lang]
	return stats, ok
}
