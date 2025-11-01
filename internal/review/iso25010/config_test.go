package iso25010

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	// Create temp config file
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "iso25010-weights.yml")

	testYAML := `weights:
  functionality: 0.15
  reliability: 0.15
  usability: 0.10
  efficiency: 0.12
  maintainability: 0.18
  portability: 0.08
  security: 0.17
  compatibility: 0.05

thresholds:
  excellent: 90
  good: 75
  acceptable: 60
  poor: 40
  critical: 0

static_signals:
  complexity_increase: -5
  complexity_decrease: 3
  todo_comments: -2
  fixme_comments: -3
  code_smells: -4
  test_coverage_increase: 5
  test_coverage_decrease: -10
  missing_docs: -2
  added_docs: 3
`

	if err := os.WriteFile(configFile, []byte(testYAML), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	// Load config
	config, err := LoadConfig(configFile)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify weights
	if config.Weights.Functionality != 0.15 {
		t.Errorf("Expected functionality weight 0.15, got %.2f", config.Weights.Functionality)
	}

	if config.Weights.Security != 0.17 {
		t.Errorf("Expected security weight 0.17, got %.2f", config.Weights.Security)
	}

	// Verify thresholds
	if config.Thresholds.Excellent != 90 {
		t.Errorf("Expected excellent threshold 90, got %d", config.Thresholds.Excellent)
	}

	// Verify static signals
	if config.StaticSignals.TodoComments != -2 {
		t.Errorf("Expected todo_comments -2, got %d", config.StaticSignals.TodoComments)
	}
}

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name      string
		config    Config
		wantError bool
	}{
		{
			name: "valid weights",
			config: Config{
				Weights: Weights{
					Functionality:   0.15,
					Reliability:     0.15,
					Usability:       0.10,
					Efficiency:      0.12,
					Maintainability: 0.18,
					Portability:     0.08,
					Security:        0.17,
					Compatibility:   0.05,
				},
			},
			wantError: false,
		},
		{
			name: "weights sum too high",
			config: Config{
				Weights: Weights{
					Functionality:   0.20,
					Reliability:     0.20,
					Usability:       0.20,
					Efficiency:      0.20,
					Maintainability: 0.20,
					Portability:     0.20,
					Security:        0.20,
					Compatibility:   0.20,
				},
			},
			wantError: true,
		},
		{
			name: "weights sum too low",
			config: Config{
				Weights: Weights{
					Functionality: 0.10,
					Reliability:   0.10,
				},
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantError {
				t.Errorf("Validate() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}

func TestGetQualityLevel(t *testing.T) {
	config := &Config{
		Thresholds: Thresholds{
			Excellent:  90,
			Good:       75,
			Acceptable: 60,
			Poor:       40,
			Critical:   0,
		},
	}

	tests := []struct {
		score int
		want  string
	}{
		{95, "excellent"},
		{90, "excellent"},
		{85, "good"},
		{75, "good"},
		{65, "acceptable"},
		{60, "acceptable"},
		{50, "poor"},
		{40, "poor"},
		{30, "critical"},
		{0, "critical"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := config.GetQualityLevel(tt.score)
			if got != tt.want {
				t.Errorf("GetQualityLevel(%d) = %s, want %s", tt.score, got, tt.want)
			}
		})
	}
}
