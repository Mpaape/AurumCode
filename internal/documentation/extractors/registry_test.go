package extractors

import (
	"context"
	"sync"
	"testing"
)

// MockExtractor for testing
type MockExtractor struct {
	lang Language
}

func (m *MockExtractor) Extract(ctx context.Context, req *ExtractRequest) (*ExtractResult, error) {
	return &ExtractResult{Language: m.lang}, nil
}

func (m *MockExtractor) Validate(ctx context.Context) error {
	return nil
}

func (m *MockExtractor) Language() Language {
	return m.lang
}

func TestRegistry_Register(t *testing.T) {
	t.Run("successful registration", func(t *testing.T) {
		registry := NewRegistry()
		extractor := &MockExtractor{lang: LanguageGo}

		err := registry.Register(extractor)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if registry.Count() != 1 {
			t.Errorf("expected 1 extractor, got %d", registry.Count())
		}
	})

	t.Run("nil extractor", func(t *testing.T) {
		registry := NewRegistry()

		err := registry.Register(nil)
		if err == nil {
			t.Error("expected error for nil extractor")
		}
	})

	t.Run("duplicate registration", func(t *testing.T) {
		registry := NewRegistry()
		extractor1 := &MockExtractor{lang: LanguageGo}
		extractor2 := &MockExtractor{lang: LanguageGo}

		registry.Register(extractor1)
		err := registry.Register(extractor2)
		if err == nil {
			t.Error("expected error for duplicate registration")
		}
	})
}

func TestRegistry_Get(t *testing.T) {
	t.Run("get existing extractor", func(t *testing.T) {
		registry := NewRegistry()
		extractor := &MockExtractor{lang: LanguagePython}

		registry.Register(extractor)

		retrieved, err := registry.Get(LanguagePython)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if retrieved.Language() != LanguagePython {
			t.Errorf("expected Python extractor, got %s", retrieved.Language())
		}
	})

	t.Run("get non-existent extractor", func(t *testing.T) {
		registry := NewRegistry()

		_, err := registry.Get(LanguageRust)
		if err == nil {
			t.Error("expected error for non-existent extractor")
		}
	})
}

func TestRegistry_Has(t *testing.T) {
	registry := NewRegistry()
	extractor := &MockExtractor{lang: LanguageTypeScript}

	registry.Register(extractor)

	if !registry.Has(LanguageTypeScript) {
		t.Error("expected registry to have TypeScript extractor")
	}

	if registry.Has(LanguageJava) {
		t.Error("expected registry to not have Java extractor")
	}
}

func TestRegistry_List(t *testing.T) {
	registry := NewRegistry()

	registry.Register(&MockExtractor{lang: LanguageGo})
	registry.Register(&MockExtractor{lang: LanguagePython})
	registry.Register(&MockExtractor{lang: LanguageJavaScript})

	extractors := registry.List()
	if len(extractors) != 3 {
		t.Errorf("expected 3 extractors, got %d", len(extractors))
	}
}

func TestRegistry_Languages(t *testing.T) {
	registry := NewRegistry()

	registry.Register(&MockExtractor{lang: LanguageGo})
	registry.Register(&MockExtractor{lang: LanguageRust})

	langs := registry.Languages()
	if len(langs) != 2 {
		t.Errorf("expected 2 languages, got %d", len(langs))
	}

	// Verify both languages are in the result
	hasGo := false
	hasRust := false
	for _, lang := range langs {
		if lang == LanguageGo {
			hasGo = true
		}
		if lang == LanguageRust {
			hasRust = true
		}
	}

	if !hasGo || !hasRust {
		t.Error("expected Go and Rust in languages list")
	}
}

func TestRegistry_Clear(t *testing.T) {
	registry := NewRegistry()

	registry.Register(&MockExtractor{lang: LanguageGo})
	registry.Register(&MockExtractor{lang: LanguagePython})

	if registry.Count() != 2 {
		t.Errorf("expected 2 extractors before clear, got %d", registry.Count())
	}

	registry.Clear()

	if registry.Count() != 0 {
		t.Errorf("expected 0 extractors after clear, got %d", registry.Count())
	}
}

func TestRegistry_ThreadSafety(t *testing.T) {
	registry := NewRegistry()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			// Determine language based on index
			langs := []Language{LanguageGo, LanguagePython, LanguageJavaScript, LanguageRust}
			lang := langs[idx%len(langs)]

			// Try to register (may fail for duplicates, that's ok)
			registry.Register(&MockExtractor{lang: lang})

			// Try to get
			registry.Get(lang)

			// Try list and languages
			registry.List()
			registry.Languages()

			// Check has
			registry.Has(lang)
		}(i)
	}

	wg.Wait()

	// Registry should still be valid after concurrent access
	count := registry.Count()
	if count > 4 || count < 0 {
		t.Errorf("unexpected count after concurrent operations: %d", count)
	}
}

func TestLanguage_IsValid(t *testing.T) {
	tests := []struct {
		lang  Language
		valid bool
	}{
		{LanguageGo, true},
		{LanguagePython, true},
		{LanguageJavaScript, true},
		{Language("invalid"), false},
		{Language(""), false},
	}

	for _, tt := range tests {
		t.Run(string(tt.lang), func(t *testing.T) {
			result := tt.lang.IsValid()
			if result != tt.valid {
				t.Errorf("expected IsValid()=%v for %s, got %v", tt.valid, tt.lang, result)
			}
		})
	}
}

func TestAllLanguages(t *testing.T) {
	langs := AllLanguages()

	// Should have 10 languages
	if len(langs) != 10 {
		t.Errorf("expected 10 languages, got %d", len(langs))
	}

	// Check that all expected languages are present
	expected := map[Language]bool{
		LanguageGo:         false,
		LanguageJavaScript: false,
		LanguageTypeScript: false,
		LanguagePython:     false,
		LanguageCSharp:     false,
		LanguageCPP:        false,
		LanguageRust:       false,
		LanguageBash:       false,
		LanguagePowerShell: false,
		LanguageJava:       false,
	}

	for _, lang := range langs {
		if _, ok := expected[lang]; !ok {
			t.Errorf("unexpected language in AllLanguages: %s", lang)
		}
		expected[lang] = true
	}

	for lang, found := range expected {
		if !found {
			t.Errorf("missing language in AllLanguages: %s", lang)
		}
	}
}
