package types

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestNewDefaultConfig(t *testing.T) {
	cfg := NewDefaultConfig()

	if cfg.Version != "2.0" {
		t.Errorf("Expected version 2.0, got %s", cfg.Version)
	}

	if cfg.LLM.Provider != "auto" {
		t.Errorf("Expected provider 'auto', got %s", cfg.LLM.Provider)
	}

	if cfg.LLM.Temperature != 0.3 {
		t.Errorf("Expected temperature 0.3, got %f", cfg.LLM.Temperature)
	}

	if cfg.LLM.MaxTokens != 4000 {
		t.Errorf("Expected max_tokens 4000, got %d", cfg.LLM.MaxTokens)
	}
}

func TestConfigYAMLRoundTrip(t *testing.T) {
	original := NewDefaultConfig()

	// Marshal to YAML
	yamlBytes, err := yaml.Marshal(original)
	if err != nil {
		t.Fatalf("Failed to marshal config: %v", err)
	}

	// Unmarshal back
	var unmarshaled Config
	err = yaml.Unmarshal(yamlBytes, &unmarshaled)
	if err != nil {
		t.Fatalf("Failed to unmarshal config: %v", err)
	}

	// Verify key fields
	if unmarshaled.Version != original.Version {
		t.Errorf("Version mismatch: got %s, want %s", unmarshaled.Version, original.Version)
	}

	if unmarshaled.LLM.Provider != original.LLM.Provider {
		t.Errorf("LLM.Provider mismatch: got %s, want %s", unmarshaled.LLM.Provider, original.LLM.Provider)
	}

	if unmarshaled.LLM.Temperature != original.LLM.Temperature {
		t.Errorf("LLM.Temperature mismatch: got %f, want %f", unmarshaled.LLM.Temperature, original.LLM.Temperature)
	}
}

// TestDocumentationConfigDefaults verifies documentation configuration defaults
func TestDocumentationConfigDefaults(t *testing.T) {
	cfg := NewDefaultConfig()

	// Verify documentation is enabled by default
	if !cfg.Documentation.Enabled {
		t.Error("Expected documentation to be enabled by default")
	}

	// Verify mode
	if cfg.Documentation.Mode != "incremental" {
		t.Errorf("Expected mode 'incremental', got %s", cfg.Documentation.Mode)
	}

	// Verify output directory
	if cfg.Documentation.OutputDirectory != "docs" {
		t.Errorf("Expected output directory 'docs', got %s", cfg.Documentation.OutputDirectory)
	}

	// Verify site generator
	if cfg.Documentation.SiteGenerator != "jekyll" {
		t.Errorf("Expected site generator 'jekyll', got %s", cfg.Documentation.SiteGenerator)
	}

	// Verify theme
	if cfg.Documentation.Theme != "just-the-docs" {
		t.Errorf("Expected theme 'just-the-docs', got %s", cfg.Documentation.Theme)
	}

	// Verify deploy configuration
	if !cfg.Documentation.Deploy.Enabled {
		t.Error("Expected deployment to be enabled by default")
	}

	if cfg.Documentation.Deploy.Target != "github-pages" {
		t.Errorf("Expected deploy target 'github-pages', got %s", cfg.Documentation.Deploy.Target)
	}

	if cfg.Documentation.Deploy.Branch != "gh-pages" {
		t.Errorf("Expected deploy branch 'gh-pages', got %s", cfg.Documentation.Deploy.Branch)
	}

	// Verify all features enabled by default
	if !cfg.Documentation.Features.WelcomePage {
		t.Error("Expected welcome page feature to be enabled")
	}
	if !cfg.Documentation.Features.APIReference {
		t.Error("Expected API reference feature to be enabled")
	}
	if !cfg.Documentation.Features.Tutorials {
		t.Error("Expected tutorials feature to be enabled")
	}
	if !cfg.Documentation.Features.Architecture {
		t.Error("Expected architecture feature to be enabled")
	}
	if !cfg.Documentation.Features.Changelog {
		t.Error("Expected changelog feature to be enabled")
	}
	if !cfg.Documentation.Features.Search {
		t.Error("Expected search feature to be enabled")
	}

	// Verify all categories enabled by default
	if !cfg.Documentation.Categories.API {
		t.Error("Expected API category to be enabled")
	}
	if !cfg.Documentation.Categories.Tutorials {
		t.Error("Expected tutorials category to be enabled")
	}
	if !cfg.Documentation.Categories.Architecture {
		t.Error("Expected architecture category to be enabled")
	}
	if !cfg.Documentation.Categories.Guides {
		t.Error("Expected guides category to be enabled")
	}
	if !cfg.Documentation.Categories.Reference {
		t.Error("Expected reference category to be enabled")
	}
	if !cfg.Documentation.Categories.Roadmap {
		t.Error("Expected roadmap category to be enabled")
	}

	// Verify cache configuration
	if !cfg.Documentation.Cache.Enabled {
		t.Error("Expected cache to be enabled by default")
	}
	if cfg.Documentation.Cache.Directory != ".aurumcode/cache" {
		t.Errorf("Expected cache directory '.aurumcode/cache', got %s", cfg.Documentation.Cache.Directory)
	}
	if cfg.Documentation.Cache.MaxAge != 168 {
		t.Errorf("Expected cache max age 168 hours, got %d", cfg.Documentation.Cache.MaxAge)
	}
}

// TestDocumentationConfigYAMLParsing tests YAML parsing of documentation config
func TestDocumentationConfigYAMLParsing(t *testing.T) {
	yamlContent := `
version: "2.0"
documentation:
  enabled: true
  mode: "full"
  output_directory: "custom-docs"
  languages: ["go", "typescript"]
  site_generator: "jekyll"
  theme: "custom-theme"
  deploy:
    enabled: false
    target: "netlify"
    branch: "docs"
    base_url: "https://example.com"
  features:
    welcome_page: false
    api_reference: true
    tutorials: true
    architecture: false
    changelog: true
    search: false
  categories:
    api: true
    tutorials: false
    architecture: true
    guides: false
    reference: true
    roadmap: false
  cache:
    enabled: true
    directory: "/tmp/cache"
    max_age: 24
`

	var cfg Config
	err := yaml.Unmarshal([]byte(yamlContent), &cfg)
	if err != nil {
		t.Fatalf("Failed to unmarshal YAML: %v", err)
	}

	// Verify parsed values
	if !cfg.Documentation.Enabled {
		t.Error("Expected documentation enabled to be true")
	}

	if cfg.Documentation.Mode != "full" {
		t.Errorf("Expected mode 'full', got %s", cfg.Documentation.Mode)
	}

	if cfg.Documentation.OutputDirectory != "custom-docs" {
		t.Errorf("Expected output directory 'custom-docs', got %s", cfg.Documentation.OutputDirectory)
	}

	if len(cfg.Documentation.Languages) != 2 {
		t.Errorf("Expected 2 languages, got %d", len(cfg.Documentation.Languages))
	}

	if cfg.Documentation.Deploy.Enabled {
		t.Error("Expected deployment to be disabled")
	}

	if cfg.Documentation.Deploy.Target != "netlify" {
		t.Errorf("Expected deploy target 'netlify', got %s", cfg.Documentation.Deploy.Target)
	}

	if cfg.Documentation.Features.WelcomePage {
		t.Error("Expected welcome page to be disabled")
	}

	if !cfg.Documentation.Features.APIReference {
		t.Error("Expected API reference to be enabled")
	}

	if cfg.Documentation.Categories.Tutorials {
		t.Error("Expected tutorials category to be disabled")
	}

	if cfg.Documentation.Cache.MaxAge != 24 {
		t.Errorf("Expected cache max age 24, got %d", cfg.Documentation.Cache.MaxAge)
	}
}

// TestBackwardCompatibility ensures configs without documentation section still work
func TestBackwardCompatibility(t *testing.T) {
	// Old config without documentation section
	yamlContent := `
version: "2.0"
llm:
  provider: "anthropic"
  model: "claude-3-5-sonnet"
  temperature: 0.5
  max_tokens: 8000
`

	var cfg Config
	err := yaml.Unmarshal([]byte(yamlContent), &cfg)
	if err != nil {
		t.Fatalf("Failed to unmarshal YAML: %v", err)
	}

	// Verify config loads without error
	if cfg.Version != "2.0" {
		t.Errorf("Expected version 2.0, got %s", cfg.Version)
	}

	if cfg.LLM.Provider != "anthropic" {
		t.Errorf("Expected provider 'anthropic', got %s", cfg.LLM.Provider)
	}

	// Documentation section should be zero-valued (disabled)
	if cfg.Documentation.Enabled {
		t.Error("Expected documentation to be disabled in legacy config")
	}
}

// TestDocumentationConfigRoundTrip tests full round-trip of documentation config
func TestDocumentationConfigRoundTrip(t *testing.T) {
	original := NewDefaultConfig()

	// Marshal to YAML
	yamlBytes, err := yaml.Marshal(original)
	if err != nil {
		t.Fatalf("Failed to marshal config: %v", err)
	}

	// Unmarshal back
	var unmarshaled Config
	err = yaml.Unmarshal(yamlBytes, &unmarshaled)
	if err != nil {
		t.Fatalf("Failed to unmarshal config: %v", err)
	}

	// Verify documentation configuration survived round-trip
	if unmarshaled.Documentation.Enabled != original.Documentation.Enabled {
		t.Error("Documentation.Enabled mismatch after round-trip")
	}

	if unmarshaled.Documentation.Mode != original.Documentation.Mode {
		t.Errorf("Documentation.Mode mismatch: got %s, want %s",
			unmarshaled.Documentation.Mode, original.Documentation.Mode)
	}

	if unmarshaled.Documentation.SiteGenerator != original.Documentation.SiteGenerator {
		t.Errorf("Documentation.SiteGenerator mismatch: got %s, want %s",
			unmarshaled.Documentation.SiteGenerator, original.Documentation.SiteGenerator)
	}

	if unmarshaled.Documentation.Deploy.Target != original.Documentation.Deploy.Target {
		t.Errorf("Documentation.Deploy.Target mismatch: got %s, want %s",
			unmarshaled.Documentation.Deploy.Target, original.Documentation.Deploy.Target)
	}

	if unmarshaled.Documentation.Cache.MaxAge != original.Documentation.Cache.MaxAge {
		t.Errorf("Documentation.Cache.MaxAge mismatch: got %d, want %d",
			unmarshaled.Documentation.Cache.MaxAge, original.Documentation.Cache.MaxAge)
	}
}

