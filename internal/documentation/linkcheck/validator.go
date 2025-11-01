package linkcheck

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Validator validates links
type Validator struct {
	baseDir        string
	checkExternal  bool
	timeout        time.Duration
	httpClient     *http.Client
}

// NewValidator creates a new link validator
func NewValidator(baseDir string) *Validator {
	return &Validator{
		baseDir:       baseDir,
		checkExternal: false,
		timeout:       5 * time.Second,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 3 {
					return fmt.Errorf("too many redirects")
				}
				return nil
			},
		},
	}
}

// WithExternalCheck enables external link checking
func (v *Validator) WithExternalCheck(enabled bool) *Validator {
	v.checkExternal = enabled
	return v
}

// WithTimeout sets the HTTP timeout
func (v *Validator) WithTimeout(timeout time.Duration) *Validator {
	v.timeout = timeout
	v.httpClient.Timeout = timeout
	return v
}

// Validate validates a link
func (v *Validator) Validate(ctx context.Context, link Link) LinkResult {
	switch link.Type {
	case LinkTypeInternal:
		return v.validateInternal(link)
	case LinkTypeAnchor:
		return v.validateAnchor(link)
	case LinkTypeExternal:
		return v.validateExternal(ctx, link)
	default:
		return LinkResult{
			Link:    link,
			Status:  LinkStatusSkipped,
			Message: "unknown link type",
		}
	}
}

// validateInternal validates internal links
func (v *Validator) validateInternal(link Link) LinkResult {
	// Resolve relative path
	sourceDir := filepath.Dir(link.SourceFile)
	targetPath := filepath.Join(sourceDir, link.URL)

	// Clean path
	targetPath = filepath.Clean(targetPath)

	// Check if file exists
	if _, err := os.Stat(targetPath); os.IsNotExist(err) {
		// Try relative to base dir
		targetPath = filepath.Join(v.baseDir, link.URL)
		if _, err := os.Stat(targetPath); os.IsNotExist(err) {
			return LinkResult{
				Link:    link,
				Status:  LinkStatusBroken,
				Message: "file not found",
			}
		}
	}

	return LinkResult{
		Link:   link,
		Status: LinkStatusOK,
	}
}

// validateAnchor validates anchor links
func (v *Validator) validateAnchor(link Link) LinkResult {
	// Extract anchor
	anchor := strings.TrimPrefix(link.URL, "#")
	if anchor == "" {
		return LinkResult{
			Link:    link,
			Status:  LinkStatusOK,
			Message: "empty anchor (top of page)",
		}
	}

	// Read source file
	content, err := os.ReadFile(link.SourceFile)
	if err != nil {
		return LinkResult{
			Link:    link,
			Status:  LinkStatusBroken,
			Message: fmt.Sprintf("cannot read source file: %v", err),
		}
	}

	// Check if anchor exists in the file
	// Markdown anchors are generated from headers
	// Format: # Header -> #header
	contentStr := strings.ToLower(string(content))
	anchorLower := strings.ToLower(anchor)

	// Simple check: look for header with this text
	headerPatterns := []string{
		fmt.Sprintf("# %s", anchorLower),
		fmt.Sprintf("## %s", anchorLower),
		fmt.Sprintf("### %s", anchorLower),
		fmt.Sprintf("#### %s", anchorLower),
		fmt.Sprintf("<a name=\"%s\"", anchorLower),
		fmt.Sprintf("id=\"%s\"", anchorLower),
	}

	for _, pattern := range headerPatterns {
		if strings.Contains(contentStr, pattern) {
			return LinkResult{
				Link:   link,
				Status: LinkStatusOK,
			}
		}
	}

	return LinkResult{
		Link:    link,
		Status:  LinkStatusBroken,
		Message: "anchor not found in file",
	}
}

// validateExternal validates external links
func (v *Validator) validateExternal(ctx context.Context, link Link) LinkResult {
	// Skip if external checking is disabled
	if !v.checkExternal {
		return LinkResult{
			Link:    link,
			Status:  LinkStatusSkipped,
			Message: "external link checking disabled",
		}
	}

	// Skip mailto, tel, etc
	if !strings.HasPrefix(link.URL, "http://") && !strings.HasPrefix(link.URL, "https://") {
		return LinkResult{
			Link:    link,
			Status:  LinkStatusSkipped,
			Message: "non-HTTP scheme",
		}
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, "HEAD", link.URL, nil)
	if err != nil {
		return LinkResult{
			Link:    link,
			Status:  LinkStatusBroken,
			Message: fmt.Sprintf("invalid URL: %v", err),
		}
	}

	// Set user agent
	req.Header.Set("User-Agent", "AurumCode-LinkChecker/1.0")

	// Make request
	resp, err := v.httpClient.Do(req)
	if err != nil {
		return LinkResult{
			Link:    link,
			Status:  LinkStatusBroken,
			Message: fmt.Sprintf("request failed: %v", err),
		}
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		return LinkResult{
			Link:   link,
			Status: LinkStatusOK,
		}
	}

	return LinkResult{
		Link:    link,
		Status:  LinkStatusBroken,
		Message: fmt.Sprintf("HTTP %d", resp.StatusCode),
	}
}
