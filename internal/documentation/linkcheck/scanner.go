package linkcheck

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	// Markdown link patterns
	markdownLinkRegex = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
	htmlLinkRegex     = regexp.MustCompile(`<a\s+[^>]*href=["']([^"']+)["'][^>]*>`)
)

// Scanner scans files for links
type Scanner struct {
	ignorePatterns []string
}

// NewScanner creates a new link scanner
func NewScanner() *Scanner {
	return &Scanner{
		ignorePatterns: []string{},
	}
}

// WithIgnorePatterns sets patterns to ignore
func (s *Scanner) WithIgnorePatterns(patterns []string) *Scanner {
	s.ignorePatterns = patterns
	return s
}

// ScanDirectory scans a directory for links
func (s *Scanner) ScanDirectory(dir string) ([]Link, error) {
	var links []Link

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			// Skip hidden directories
			if strings.HasPrefix(info.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}

		// Only scan markdown and HTML files
		ext := filepath.Ext(path)
		if ext != ".md" && ext != ".html" && ext != ".markdown" {
			return nil
		}

		fileLinks, err := s.ScanFile(path)
		if err != nil {
			// Log error but continue
			return nil
		}

		links = append(links, fileLinks...)
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("walk directory: %w", err)
	}

	return links, nil
}

// ScanFile scans a single file for links
func (s *Scanner) ScanFile(filePath string) ([]Link, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}
	defer file.Close()

	var links []Link
	scanner := bufio.NewScanner(file)
	lineNumber := 0

	for scanner.Scan() {
		lineNumber++
		line := scanner.Text()

		// Extract Markdown links
		mdLinks := markdownLinkRegex.FindAllStringSubmatch(line, -1)
		for _, match := range mdLinks {
			if len(match) >= 3 {
				url := strings.TrimSpace(match[2])
				if s.shouldIgnore(url) {
					continue
				}

				links = append(links, Link{
					URL:        url,
					Type:       s.detectLinkType(url),
					SourceFile: filePath,
					LineNumber: lineNumber,
				})
			}
		}

		// Extract HTML links
		htmlLinks := htmlLinkRegex.FindAllStringSubmatch(line, -1)
		for _, match := range htmlLinks {
			if len(match) >= 2 {
				url := strings.TrimSpace(match[1])
				if s.shouldIgnore(url) {
					continue
				}

				links = append(links, Link{
					URL:        url,
					Type:       s.detectLinkType(url),
					SourceFile: filePath,
					LineNumber: lineNumber,
				})
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan file: %w", err)
	}

	return links, nil
}

// detectLinkType determines the type of link
func (s *Scanner) detectLinkType(url string) LinkType {
	// Anchor links
	if strings.HasPrefix(url, "#") {
		return LinkTypeAnchor
	}

	// External links
	if strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://") {
		return LinkTypeExternal
	}

	// mailto, tel, etc
	if strings.Contains(url, ":") {
		return LinkTypeExternal
	}

	// Internal links (relative paths)
	return LinkTypeInternal
}

// shouldIgnore checks if a URL should be ignored
func (s *Scanner) shouldIgnore(url string) bool {
	for _, pattern := range s.ignorePatterns {
		if strings.Contains(url, pattern) {
			return true
		}
	}
	return false
}
