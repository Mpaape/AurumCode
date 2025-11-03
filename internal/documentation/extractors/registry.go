package extractors

import (
	"fmt"
	"sync"
)

// Registry manages registered documentation extractors
type Registry struct {
	mu         sync.RWMutex
	extractors map[Language]Extractor
}

// NewRegistry creates a new extractor registry
func NewRegistry() *Registry {
	return &Registry{
		extractors: make(map[Language]Extractor),
	}
}

// Register adds an extractor to the registry
func (r *Registry) Register(extractor Extractor) error {
	if extractor == nil {
		return fmt.Errorf("extractor cannot be nil")
	}

	lang := extractor.Language()
	if !lang.IsValid() {
		return fmt.Errorf("invalid language: %s", lang)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.extractors[lang]; exists {
		return fmt.Errorf("extractor for %s already registered", lang)
	}

	r.extractors[lang] = extractor
	return nil
}

// Get retrieves an extractor for a specific language
func (r *Registry) Get(lang Language) (Extractor, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	extractor, ok := r.extractors[lang]
	if !ok {
		return nil, fmt.Errorf("no extractor registered for %s", lang)
	}

	return extractor, nil
}

// Has checks if an extractor is registered for a language
func (r *Registry) Has(lang Language) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, ok := r.extractors[lang]
	return ok
}

// List returns all registered extractors
func (r *Registry) List() []Extractor {
	r.mu.RLock()
	defer r.mu.RUnlock()

	extractors := make([]Extractor, 0, len(r.extractors))
	for _, ext := range r.extractors {
		extractors = append(extractors, ext)
	}

	return extractors
}

// Languages returns all languages with registered extractors
func (r *Registry) Languages() []Language {
	r.mu.RLock()
	defer r.mu.RUnlock()

	langs := make([]Language, 0, len(r.extractors))
	for lang := range r.extractors {
		langs = append(langs, lang)
	}

	return langs
}

// Count returns the number of registered extractors
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.extractors)
}

// Clear removes all registered extractors
func (r *Registry) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.extractors = make(map[Language]Extractor)
}
