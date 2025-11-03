package incremental

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/Mpaape/AurumCode/internal/documentation/site"
)

const defaultCachePath = ".aurumcode/cache/incremental.json"

// Manager coordinates incremental documentation builds
type Manager struct {
	detector  *ChangeDetector
	cache     *Cache
	cachePath string
}

// NewManager creates a new incremental documentation manager
func NewManager(runner site.CommandRunner, repoDir string) *Manager {
	return &Manager{
		detector:  NewChangeDetector(runner, repoDir),
		cache:     NewCache(),
		cachePath: defaultCachePath,
	}
}

// NewManagerWithCache creates a manager with custom cache path
func NewManagerWithCache(runner site.CommandRunner, repoDir, cachePath string) *Manager {
	return &Manager{
		detector:  NewChangeDetector(runner, repoDir),
		cache:     NewCache(),
		cachePath: cachePath,
	}
}

// LoadCache loads the cache from disk
func (m *Manager) LoadCache() error {
	return m.cache.Load(m.cachePath)
}

// SaveCache persists the cache to disk
func (m *Manager) SaveCache() error {
	return m.cache.Save(m.cachePath)
}

// GetChangedFiles returns files that changed since last documentation build
func (m *Manager) GetChangedFiles(ctx context.Context) ([]string, error) {
	// Check if we're in a git repository
	if !m.detector.IsGitRepository(ctx) {
		return nil, fmt.Errorf("not a git repository")
	}

	// If no last commit in cache, this is first run - get all uncommitted changes
	if m.cache.LastCommit == "" {
		return m.detector.DetectUnstagedChanges(ctx)
	}

	// Get current commit
	currentCommit, err := m.detector.GetCurrentCommit(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get current commit: %w", err)
	}

	// If commits match, check for unstaged changes
	if !m.cache.HasChanges(currentCommit) {
		return m.detector.DetectUnstagedChanges(ctx)
	}

	// Detect changes since last documented commit
	return m.detector.DetectChangesSinceCommit(ctx, m.cache.LastCommit)
}

// GetAffectedDocumentation returns documentation files that need regeneration
func (m *Manager) GetAffectedDocumentation(ctx context.Context, extensions []string) ([]string, error) {
	// Get changed source files
	changedFiles, err := m.GetChangedFiles(ctx)
	if err != nil {
		return nil, err
	}

	// Filter by language extensions if specified
	if len(extensions) > 0 {
		changedFiles = m.detector.FilterByLanguage(changedFiles, extensions)
	}

	// Get affected documentation files from cache
	return m.cache.GetAffectedDocs(changedFiles), nil
}

// RegisterDocumentation records the mapping between source and documentation files
func (m *Manager) RegisterDocumentation(sourceFile string, docFiles ...string) {
	// Normalize paths to be relative
	sourceFile = filepath.ToSlash(sourceFile)
	var normalized []string
	for _, doc := range docFiles {
		normalized = append(normalized, filepath.ToSlash(doc))
	}

	m.cache.AddMapping(sourceFile, normalized...)
}

// RegisterLanguage associates source files with a programming language
func (m *Manager) RegisterLanguage(language string, sourceFiles ...string) {
	m.cache.AddLanguageMapping(language, sourceFiles...)
}

// UpdateCommit updates the cache with current commit after successful build
func (m *Manager) UpdateCommit(ctx context.Context) error {
	commit, err := m.detector.GetCurrentCommit(ctx)
	if err != nil {
		return fmt.Errorf("failed to get current commit: %w", err)
	}

	m.cache.UpdateCommit(commit)
	return nil
}

// IsFirstRun returns true if this is the first documentation build
func (m *Manager) IsFirstRun() bool {
	return m.cache.IsEmpty() || m.cache.LastCommit == ""
}

// GetCache returns the cache for direct access
func (m *Manager) GetCache() *Cache {
	return m.cache
}

// Reset clears all cache data
func (m *Manager) Reset() {
	m.cache.Clear()
}
