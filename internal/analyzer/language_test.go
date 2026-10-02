package analyzer

import (
	"testing"
)

func TestDetectLanguage(t *testing.T) {
	detector := NewLanguageDetector()

	tests := []struct {
		filePath string
		expected string
	}{
		// Go
		{"main.go", "go"},
		{"service/handler.go", "go"},
		{"go.mod", "gomod"},
		{"go.sum", "unknown"}, // no grammar: the review reads the text

		// JavaScript/TypeScript
		{"app.js", "javascript"},
		{"component.jsx", "javascript"},
		{"service.ts", "typescript"},
		{"Component.tsx", "tsx"},
		{"index.mjs", "javascript"},

		// Python
		{"script.py", "python"},
		{"module.pyx", "unknown"},

		// Java/Kotlin
		{"Main.java", "java"},
		{"Service.kt", "kotlin"},

		// C/C++
		{"main.c", "c"},
		{"header.h", "c"},
		{"main.cpp", "cpp"},
		{"header.hpp", "cpp"},

		// C#
		{"Program.cs", "c_sharp"},

		// Rust
		{"main.rs", "rust"},

		// Ruby
		{"app.rb", "ruby"},

		// Web
		{"index.html", "html"},
		{"style.css", "css"},
		{"style.scss", "scss"},

		// Config
		{"config.json", "json"},
		{"config.yaml", "yaml"},
		{"config.yml", "yaml"},
		{"Cargo.toml", "toml"},
	}

	for _, test := range tests {
		t.Run(test.filePath, func(t *testing.T) {
			result := detector.DetectLanguage(test.filePath)
			if result != test.expected {
				t.Errorf("DetectLanguage(%s) = %s, want %s", test.filePath, result, test.expected)
			}
		})
	}
}

func TestGetLanguageCategory(t *testing.T) {
	detector := NewLanguageDetector()

	tests := []struct {
		language string
		expected string
	}{
		// Backend
		{"go", "backend"},
		{"python", "backend"},
		{"java", "backend"},
		{"rust", "backend"},

		// Frontend
		{"javascript", "frontend"},
		{"typescript", "frontend"},
		{"html", "frontend"},
		{"css", "frontend"},

		// Database
		{"sql", "database"},

		// Infrastructure
		{"bash", "infrastructure"},
		{"dockerfile", "infrastructure"},

		// Config
		{"json", "config"},
		{"yaml", "config"},
		{"toml", "config"},

		// Documentation
		{"markdown", "documentation"},

		// Other
		{"unknown", "other"},
	}

	for _, test := range tests {
		t.Run(test.language, func(t *testing.T) {
			result := detector.GetLanguageCategory(test.language)
			if result != test.expected {
				t.Errorf("GetLanguageCategory(%s) = %s, want %s", test.language, result, test.expected)
			}
		})
	}
}

func TestDetectLanguage_CaseInsensitive(t *testing.T) {
	detector := NewLanguageDetector()

	// Should handle uppercase extensions
	tests := []struct {
		filePath string
		expected string
	}{
		{"Main.GO", "go"},
		{"App.JS", "javascript"},
		{"Service.PY", "python"},
	}

	for _, test := range tests {
		result := detector.DetectLanguage(test.filePath)
		if result != test.expected {
			t.Errorf("DetectLanguage(%s) = %s, want %s", test.filePath, result, test.expected)
		}
	}
}

// The detector holds no table: a name resolves only through the runtime, and a
// brand-new extension is "unknown", never an error.
func TestDetectLanguage_UnknownIsNotAnError(t *testing.T) {
	d := NewLanguageDetector()
	for _, p := range []string{"notes.zzqx", "weird", "a/b/c.unheardof"} {
		if got := d.DetectLanguage(p); got != "unknown" {
			t.Errorf("DetectLanguage(%q) = %q, want unknown", p, got)
		}
	}
	for _, p := range []string{"Dockerfile", "main.tf", "x.kt", "x.php", "x.rb", "x.java"} {
		if got := d.DetectLanguage(p); got == "unknown" {
			t.Errorf("DetectLanguage(%q) is unknown; the runtime has a grammar for it", p)
		}
	}
}
