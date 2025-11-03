package incremental

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Cache stores mappings between source files and generated documentation
type Cache struct {
	// LastCommit is the last processed git commit hash
	LastCommit string `json:"last_commit"`

	// LastUpdate is the timestamp of last cache update
	LastUpdate time.Time `json:"last_update"`

	// Mappings maps source file paths to their documentation file paths
	Mappings map[string][]string `json:"mappings"`

	// LanguageMappings maps languages to their source files
	LanguageMappings map[string][]string `json:"language_mappings"`
}

// NewCache creates a new empty cache
func NewCache() *Cache {
	return &Cache{
		Mappings:         make(map[string][]string),
		LanguageMappings: make(map[string][]string),
		LastUpdate:       time.Now(),
	}
}

// AddMapping adds a mapping from source file to documentation file(s)
func (c *Cache) AddMapping(sourceFile string, docFiles ...string) {
	if c.Mappings == nil {
		c.Mappings = make(map[string][]string)
	}

	// Normalize paths
	sourceFile = filepath.ToSlash(sourceFile)
	var normalizedDocs []string
	for _, doc := range docFiles {
		normalizedDocs = append(normalizedDocs, filepath.ToSlash(doc))
	}

	c.Mappings[sourceFile] = normalizedDocs
	c.LastUpdate = time.Now()
}

// AddLanguageMapping associates a language with source files
func (c *Cache) AddLanguageMapping(language string, sourceFiles ...string) {
	if c.LanguageMappings == nil {
		c.LanguageMappings = make(map[string][]string)
	}

	// Normalize paths
	var normalized []string
	for _, file := range sourceFiles {
		normalized = append(normalized, filepath.ToSlash(file))
	}

	existing := c.LanguageMappings[language]
	c.LanguageMappings[language] = append(existing, normalized...)
	c.LastUpdate = time.Now()
}

// GetDocFiles returns documentation files for a source file
func (c *Cache) GetDocFiles(sourceFile string) []string {
	sourceFile = filepath.ToSlash(sourceFile)
	return c.Mappings[sourceFile]
}

// GetAffectedDocs returns all documentation files affected by changed source files
func (c *Cache) GetAffectedDocs(changedFiles []string) []string {
	affectedSet := make(map[string]bool)

	for _, srcFile := range changedFiles {
		docFiles := c.GetDocFiles(srcFile)
		for _, doc := range docFiles {
			affectedSet[doc] = true
		}
	}

	// Convert set to slice
	var affected []string
	for doc := range affectedSet {
		affected = append(affected, doc)
	}

	return affected
}

// GetSourcesByLanguage returns all source files for a language
func (c *Cache) GetSourcesByLanguage(language string) []string {
	return c.LanguageMappings[language]
}

// UpdateCommit updates the last processed commit
func (c *Cache) UpdateCommit(commit string) {
	c.LastCommit = commit
	c.LastUpdate = time.Now()
}

// HasChanges checks if cache has changes since last commit
func (c *Cache) HasChanges(currentCommit string) bool {
	return c.LastCommit != currentCommit
}

// IsEmpty returns true if cache has no mappings
func (c *Cache) IsEmpty() bool {
	return len(c.Mappings) == 0
}

// Clear removes all mappings from cache
func (c *Cache) Clear() {
	c.Mappings = make(map[string][]string)
	c.LanguageMappings = make(map[string][]string)
	c.LastCommit = ""
	c.LastUpdate = time.Now()
}

// Save persists cache to a JSON file
func (c *Cache) Save(path string) error {
	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
	}

	// Marshal to JSON
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal cache: %w", err)
	}

	// Write to file
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write cache file: %w", err)
	}

	return nil
}

// Load reads cache from a JSON file
func (c *Cache) Load(path string) error {
	// Read file
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// Cache doesn't exist yet, use empty cache
			return nil
		}
		return fmt.Errorf("failed to read cache file: %w", err)
	}

	// Unmarshal JSON
	if err := json.Unmarshal(data, c); err != nil {
		return fmt.Errorf("failed to unmarshal cache: %w", err)
	}

	// Ensure maps are initialized
	if c.Mappings == nil {
		c.Mappings = make(map[string][]string)
	}
	if c.LanguageMappings == nil {
		c.LanguageMappings = make(map[string][]string)
	}

	return nil
}

// LoadFromFile loads a cache from file, returns new cache if file doesn't exist
func LoadFromFile(path string) (*Cache, error) {
	cache := NewCache()
	if err := cache.Load(path); err != nil {
		return nil, err
	}
	return cache, nil
}
