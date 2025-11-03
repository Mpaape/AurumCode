package powershell

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Mpaape/AurumCode/internal/documentation/extractors"
	"github.com/Mpaape/AurumCode/internal/documentation/site"
)

func TestNewPowerShellExtractor(t *testing.T) {
	runner := site.NewMockRunner()
	extractor := NewPowerShellExtractor(runner)

	if extractor == nil {
		t.Fatal("NewPowerShellExtractor returned nil")
	}

	if extractor.Language() != extractors.LanguagePowerShell {
		t.Errorf("expected language %s, got %s", extractors.LanguagePowerShell, extractor.Language())
	}
}

func TestPowerShellExtractor_Validate(t *testing.T) {
	tests := []struct {
		name      string
		output    string
		err       error
		wantError bool
	}{
		{"pwsh installed", "PowerShell 7.0", nil, false},
		{"pwsh not found", "", errors.New("not found"), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := site.NewMockRunner()
			if tt.err != nil {
				runner.WithError("pwsh -Version", tt.err)
			} else {
				runner.WithOutput("pwsh -Version", tt.output)
			}

			extractor := NewPowerShellExtractor(runner)
			err := extractor.Validate(context.Background())

			if tt.wantError && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.wantError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestPowerShellExtractor_findPowerShellScripts(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test scripts
	scripts := []string{"script.ps1", "module.psm1"}
	for _, script := range scripts {
		path := filepath.Join(tmpDir, script)
		if err := os.WriteFile(path, []byte("# PowerShell script"), 0644); err != nil {
			t.Fatalf("failed to create script: %v", err)
		}
	}

	runner := site.NewMockRunner()
	extractor := NewPowerShellExtractor(runner)

	found, err := extractor.findPowerShellScripts(tmpDir)
	if err != nil {
		t.Fatalf("findPowerShellScripts failed: %v", err)
	}

	if len(found) != len(scripts) {
		t.Errorf("expected %d scripts, got %d", len(scripts), len(found))
	}
}
