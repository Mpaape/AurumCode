package types

import (
	"strings"
	"testing"
)

func TestConfig_Validate_AcceptsDefaults(t *testing.T) {
	if err := NewDefaultConfig().Validate(); err != nil {
		t.Fatalf("NewDefaultConfig() must be valid, got: %v", err)
	}
}

func TestConfig_Validate_NilReceiver(t *testing.T) {
	var cfg *Config
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected an error for a nil config, got nil")
	}
}

func TestConfig_Validate_Rejects(t *testing.T) {
	tests := []struct {
		name        string
		mutate      func(*Config)
		wantMention string
	}{
		{"missing version", func(c *Config) { c.Version = "" }, "version"},
		{"missing provider", func(c *Config) { c.LLM.Provider = "" }, "llm.provider"},
		{"temperature below range", func(c *Config) { c.LLM.Temperature = -0.1 }, "llm.temperature"},
		{"temperature above range", func(c *Config) { c.LLM.Temperature = 2.5 }, "llm.temperature"},
		{"zero max tokens", func(c *Config) { c.LLM.MaxTokens = 0 }, "llm.max_tokens"},
		{"negative max tokens", func(c *Config) { c.LLM.MaxTokens = -1 }, "llm.max_tokens"},

		{"unknown mode", func(c *Config) { c.Documentation.Mode = "sideways" }, "documentation.mode"},
		{"empty output directory", func(c *Config) { c.Documentation.OutputDirectory = "" }, "documentation.output_directory"},
		{"absolute output directory", func(c *Config) { c.Documentation.OutputDirectory = "/etc" }, "documentation.output_directory"},
		{"escaping output directory", func(c *Config) { c.Documentation.OutputDirectory = "../../etc" }, "documentation.output_directory"},
		{"escaping via segments", func(c *Config) { c.Documentation.OutputDirectory = "docs/../../etc" }, "documentation.output_directory"},

		{"empty cache directory", func(c *Config) { c.Documentation.Cache.Directory = "" }, "documentation.cache.directory"},
		{"escaping cache directory", func(c *Config) { c.Documentation.Cache.Directory = "../outside" }, "documentation.cache.directory"},
		{"negative cache age", func(c *Config) { c.Documentation.Cache.MaxAge = -1 }, "documentation.cache.max_age"},

		{"deploy without target", func(c *Config) { c.Documentation.Deploy.Target = "" }, "documentation.deploy.target"},
		{"deploy without branch", func(c *Config) { c.Documentation.Deploy.Branch = "" }, "documentation.deploy.branch"},
		{"deploy with relative base url", func(c *Config) { c.Documentation.Deploy.BaseURL = "example.com/docs" }, "documentation.deploy.base_url"},
		{"deploy with unparsable base url", func(c *Config) { c.Documentation.Deploy.BaseURL = "http://%zz" }, "documentation.deploy.base_url"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := NewDefaultConfig()
			tt.mutate(cfg)

			err := cfg.Validate()
			if err == nil {
				t.Fatalf("expected an error mentioning %q, got nil", tt.wantMention)
			}
			if !strings.Contains(err.Error(), tt.wantMention) {
				t.Errorf("error %q does not mention %q", err, tt.wantMention)
			}
		})
	}
}

// TestConfig_Validate_SkipsDisabledSections pins that a section switched off
// is not held to the rules that only apply when it runs.
func TestConfig_Validate_SkipsDisabledSections(t *testing.T) {
	cfg := NewDefaultConfig()
	cfg.Documentation.Enabled = false
	cfg.Documentation.Mode = "sideways"
	cfg.Documentation.OutputDirectory = ""
	cfg.Documentation.Deploy.Target = ""

	if err := cfg.Validate(); err != nil {
		t.Fatalf("disabled documentation must not be validated, got: %v", err)
	}

	cfg = NewDefaultConfig()
	cfg.Documentation.Deploy.Enabled = false
	cfg.Documentation.Deploy.Target = ""
	cfg.Documentation.Deploy.Branch = ""
	cfg.Documentation.Cache.Enabled = false
	cfg.Documentation.Cache.Directory = ""

	if err := cfg.Validate(); err != nil {
		t.Fatalf("disabled deploy and cache must not be validated, got: %v", err)
	}
}

// TestConfig_Validate_ReportsEveryProblem pins that validation is not a
// first-failure guard: a broken file should be fixable in one pass.
func TestConfig_Validate_ReportsEveryProblem(t *testing.T) {
	cfg := NewDefaultConfig()
	cfg.Version = ""
	cfg.LLM.Provider = ""
	cfg.LLM.MaxTokens = -1
	cfg.Documentation.Mode = "sideways"

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected an error, got nil")
	}

	for _, want := range []string{"version", "llm.provider", "llm.max_tokens", "documentation.mode"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not mention %q:\n%v", want, err)
		}
	}
}

// TestConfig_Validate_AcceptsBoundaries keeps the range checks from being
// tightened into rejecting legitimate values.
func TestConfig_Validate_AcceptsBoundaries(t *testing.T) {
	for _, temperature := range []float64{0, 1, 2} {
		cfg := NewDefaultConfig()
		cfg.LLM.Temperature = temperature
		if err := cfg.Validate(); err != nil {
			t.Errorf("temperature %v must be accepted, got: %v", temperature, err)
		}
	}

	cfg := NewDefaultConfig()
	cfg.Documentation.Mode = "full"
	cfg.Documentation.OutputDirectory = "docs/generated"
	cfg.Documentation.Cache.MaxAge = 0
	cfg.Documentation.Deploy.BaseURL = "https://example.com/docs/"
	if err := cfg.Validate(); err != nil {
		t.Errorf("valid configuration rejected: %v", err)
	}
}
