package iso25010

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Weights represents ISO/IEC 25010 characteristic weights
type Weights struct {
	Functionality   float64 `yaml:"functionality"`
	Reliability     float64 `yaml:"reliability"`
	Usability       float64 `yaml:"usability"`
	Efficiency      float64 `yaml:"efficiency"`
	Maintainability float64 `yaml:"maintainability"`
	Portability     float64 `yaml:"portability"`
	Security        float64 `yaml:"security"`
	Compatibility   float64 `yaml:"compatibility"`
}

// Thresholds defines quality score thresholds
type Thresholds struct {
	Excellent  int `yaml:"excellent"`
	Good       int `yaml:"good"`
	Acceptable int `yaml:"acceptable"`
	Poor       int `yaml:"poor"`
	Critical   int `yaml:"critical"`
}

// StaticSignals defines weights for static analysis signals
type StaticSignals struct {
	ComplexityIncrease    int `yaml:"complexity_increase"`
	ComplexityDecrease    int `yaml:"complexity_decrease"`
	TodoComments          int `yaml:"todo_comments"`
	FixmeComments         int `yaml:"fixme_comments"`
	CodeSmells            int `yaml:"code_smells"`
	TestCoverageIncrease  int `yaml:"test_coverage_increase"`
	TestCoverageDecrease  int `yaml:"test_coverage_decrease"`
	MissingDocs           int `yaml:"missing_docs"`
	AddedDocs             int `yaml:"added_docs"`
}

// Config represents the complete ISO/IEC 25010 configuration
type Config struct {
	Weights       Weights       `yaml:"weights"`
	Thresholds    Thresholds    `yaml:"thresholds"`
	StaticSignals StaticSignals `yaml:"static_signals"`
}

// LoadConfig loads ISO/IEC 25010 configuration from YAML file
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Validate weights sum to approximately 1.0
	if err := config.Validate(); err != nil {
		return nil, err
	}

	return &config, nil
}

// Validate validates the configuration
func (c *Config) Validate() error {
	// Check weights sum to 1.0 (with tolerance)
	sum := c.Weights.Functionality +
		c.Weights.Reliability +
		c.Weights.Usability +
		c.Weights.Efficiency +
		c.Weights.Maintainability +
		c.Weights.Portability +
		c.Weights.Security +
		c.Weights.Compatibility

	const tolerance = 0.01
	if sum < 1.0-tolerance || sum > 1.0+tolerance {
		return fmt.Errorf("weights must sum to 1.0, got %.3f", sum)
	}

	return nil
}

// GetQualityLevel returns the quality level for a given score
func (c *Config) GetQualityLevel(score int) string {
	switch {
	case score >= c.Thresholds.Excellent:
		return "excellent"
	case score >= c.Thresholds.Good:
		return "good"
	case score >= c.Thresholds.Acceptable:
		return "acceptable"
	case score >= c.Thresholds.Poor:
		return "poor"
	default:
		return "critical"
	}
}
