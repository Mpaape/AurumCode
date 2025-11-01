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
		{"go.mod", "go"},
		{"go.sum", "go"},

		// JavaScript/TypeScript
		{"app.js", "javascript"},
		{"component.jsx", "javascript"},
		{"service.ts", "typescript"},
		{"Component.tsx", "typescript"},
		{"index.mjs", "javascript"},

		// Python
		{"script.py", "python"},
		{"module.pyx", "python"},

		// Java/Kotlin
		{"Main.java", "java"},
		{"Service.kt", "kotlin"},

		// C/C++
		{"main.c", "c"},
		{"header.h", "c"},
		{"main.cpp", "cpp"},
		{"header.hpp", "cpp"},

		// C#
		{"Program.cs", "csharp"},

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

		// Markdown
		{"README.md", "markdown"},

		// Docker
		{"Dockerfile", "docker"},
		{"Makefile", "make"},

		// Unknown
		{"file.xyz", "unknown"},
		{"no_extension", "unknown"},
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

func TestIsTestFile(t *testing.T) {
	detector := NewLanguageDetector()

	tests := []struct {
		filePath string
		expected bool
	}{
		// Go test files
		{"handler_test.go", true},
		{"service_test.go", true},
		{"main.go", false},

		// JavaScript/TypeScript test files
		{"component.test.js", true},
		{"service.spec.ts", true},
		{"util.test.tsx", true},
		{"app.js", false},

		// Python test files
		{"test_service.py", true},
		{"handler_test.py", true},
		{"service.py", false},

		// Java test files
		{"ServiceTest.java", true},
		{"HandlerTests.java", true},
		{"Service.java", false},

		// Test directories
		{"src/test/handler.go", true},
		{"src/tests/service.js", true},
		{"src/__tests__/component.tsx", true},
		{"src/spec/util.rb", true},
		{"src/main/service.java", false},
	}

	for _, test := range tests {
		t.Run(test.filePath, func(t *testing.T) {
			result := detector.IsTestFile(test.filePath)
			if result != test.expected {
				t.Errorf("IsTestFile(%s) = %v, want %v", test.filePath, result, test.expected)
			}
		})
	}
}

func TestIsConfigFile(t *testing.T) {
	detector := NewLanguageDetector()

	tests := []struct {
		filePath string
		expected bool
	}{
		// Config files by name
		{"package.json", true},
		{"tsconfig.json", true},
		{"go.mod", true},
		{"go.sum", true},
		{"Dockerfile", true},
		{"docker-compose.yml", true},
		{"Makefile", true},
		{".gitignore", true},
		{".env", true},

		// Config files by extension
		{"config.json", true},
		{"settings.yaml", true},
		{"app.toml", true},
		{"database.ini", true},

		// Non-config files
		{"main.go", false},
		{"service.js", false},
		{"handler.py", false},
	}

	for _, test := range tests {
		t.Run(test.filePath, func(t *testing.T) {
			result := detector.IsConfigFile(test.filePath)
			if result != test.expected {
				t.Errorf("IsConfigFile(%s) = %v, want %v", test.filePath, result, test.expected)
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
		{"shell", "infrastructure"},
		{"docker", "infrastructure"},

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
