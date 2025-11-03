package bash

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Mpaape/AurumCode/internal/documentation/extractors"
	"github.com/Mpaape/AurumCode/internal/documentation/site"
)

func TestNewBashExtractor(t *testing.T) {
	runner := site.NewMockRunner()
	extractor := NewBashExtractor(runner)

	if extractor == nil {
		t.Fatal("NewBashExtractor returned nil")
	}

	if extractor.Language() != extractors.LanguageBash {
		t.Errorf("expected language %s, got %s", extractors.LanguageBash, extractor.Language())
	}
}

func TestBashExtractor_Validate(t *testing.T) {
	tests := []struct {
		name      string
		output    string
		err       error
		wantError bool
	}{
		{"bash installed", "GNU bash 5.0", nil, false},
		{"bash not found", "", errors.New("not found"), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := site.NewMockRunner()
			if tt.err != nil {
				runner.WithError("bash --version", tt.err)
			} else {
				runner.WithOutput("bash --version", tt.output)
			}

			extractor := NewBashExtractor(runner)
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

func TestBashExtractor_findBashScripts(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test scripts
	scripts := []string{"script.sh", "util.bash"}
	for _, script := range scripts {
		path := filepath.Join(tmpDir, script)
		if err := os.WriteFile(path, []byte("#!/bin/bash\n# Comment"), 0644); err != nil {
			t.Fatalf("failed to create script: %v", err)
		}
	}

	runner := site.NewMockRunner()
	extractor := NewBashExtractor(runner)

	found, err := extractor.findBashScripts(tmpDir)
	if err != nil {
		t.Fatalf("findBashScripts failed: %v", err)
	}

	if len(found) != len(scripts) {
		t.Errorf("expected %d scripts, got %d", len(scripts), len(found))
	}
}
