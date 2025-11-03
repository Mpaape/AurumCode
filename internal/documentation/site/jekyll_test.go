package site

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// MockRunner for testing
type MockJekyllRunner struct {
	runFunc func(ctx context.Context, cmd string, args []string, workdir string, env map[string]string) (string, error)
}

func (m *MockJekyllRunner) Run(ctx context.Context, cmd string, args []string, workdir string, env map[string]string) (string, error) {
	if m.runFunc != nil {
		return m.runFunc(ctx, cmd, args, workdir, env)
	}
	return "", nil
}

func TestJekyllBuilder_GetVersion(t *testing.T) {
	tests := []struct {
		name    string
		output  string
		wantErr bool
	}{
		{
			name:    "valid version",
			output:  "jekyll 4.3.3",
			wantErr: false,
		},
		{
			name:    "command not found",
			output:  "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := &MockJekyllRunner{
				runFunc: func(ctx context.Context, cmd string, args []string, workdir string, env map[string]string) (string, error) {
					if tt.wantErr {
						return "", errors.New("command not found")
					}
					return tt.output, nil
				},
			}

			builder := NewJekyllBuilder(runner)
			version, err := builder.GetVersion(context.Background())

			if tt.wantErr && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if !tt.wantErr && !strings.Contains(version, "jekyll") {
				t.Errorf("expected version to contain 'jekyll', got: %s", version)
			}
		})
	}
}

func TestJekyllBuilder_Validate(t *testing.T) {
	tests := []struct {
		name    string
		version string
		wantErr bool
	}{
		{
			name:    "valid jekyll 4.3",
			version: "jekyll 4.3.3",
			wantErr: false,
		},
		{
			name:    "invalid version",
			version: "jekyll 3.9.0",
			wantErr: true,
		},
		{
			name:    "jekyll not found",
			version: "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := &MockJekyllRunner{
				runFunc: func(ctx context.Context, cmd string, args []string, workdir string, env map[string]string) (string, error) {
					if tt.version == "" {
						return "", errors.New("command not found")
					}
					return tt.version, nil
				},
			}

			builder := NewJekyllBuilder(runner)
			err := builder.Validate(context.Background())

			if tt.wantErr && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestJekyllBuilder_Build(t *testing.T) {
	tests := []struct {
		name       string
		buildOut   string
		versionOut string
		wantErr    bool
	}{
		{
			name:       "successful build",
			buildOut:   "done in 1.234 seconds",
			versionOut: "jekyll 4.3.3",
			wantErr:    false,
		},
		{
			name:       "build failure",
			buildOut:   "",
			versionOut: "jekyll 4.3.3",
			wantErr:    true,
		},
		{
			name:       "jekyll not found",
			buildOut:   "",
			versionOut: "",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := &MockJekyllRunner{
				runFunc: func(ctx context.Context, cmd string, args []string, workdir string, env map[string]string) (string, error) {
					if cmd == "jekyll" && len(args) > 0 && args[0] == "--version" {
						if tt.versionOut == "" {
							return "", errors.New("command not found")
						}
						return tt.versionOut, nil
					}
					if cmd == "jekyll" && len(args) > 0 && args[0] == "build" {
						if tt.buildOut == "" {
							return "", errors.New("build failed")
						}
						return tt.buildOut, nil
					}
					return "", nil
				},
			}

			builder := NewJekyllBuilder(runner)
			err := builder.Build(context.Background(), ".")

			if tt.wantErr && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestJekyllBuilder_BuildWithConfig(t *testing.T) {
	runner := &MockJekyllRunner{
		runFunc: func(ctx context.Context, cmd string, args []string, workdir string, env map[string]string) (string, error) {
			// Validate command and args
			if cmd != "jekyll" {
				t.Errorf("expected command 'jekyll', got: %s", cmd)
			}
			if len(args) == 0 || args[0] != "build" {
				t.Errorf("expected first arg 'build', got: %v", args)
			}

			// Check for destination flag
			hasDestination := false
			for i, arg := range args {
				if arg == "--destination" && i+1 < len(args) {
					hasDestination = true
					if args[i+1] != "custom-output" {
						t.Errorf("expected destination 'custom-output', got: %s", args[i+1])
					}
				}
				if arg == "--baseurl" && i+1 < len(args) {
					if args[i+1] != "/myproject" {
						t.Errorf("expected baseurl '/myproject', got: %s", args[i+1])
					}
				}
			}

			if !hasDestination {
				t.Error("expected --destination flag in args")
			}

			return "done in 1.234 seconds", nil
		},
	}

	builder := NewJekyllBuilder(runner)
	config := &BuildConfig{
		WorkDir:   "test-dir",
		OutputDir: "custom-output",
		BaseURL:   "/myproject",
	}

	err := builder.BuildWithConfig(context.Background(), config)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestJekyllBuilder_WithRequiredVersion(t *testing.T) {
	runner := &MockJekyllRunner{}
	builder := NewJekyllBuilder(runner).WithRequiredVersion("4.2")

	if builder.requiredVersion != "4.2" {
		t.Errorf("expected required version '4.2', got: %s", builder.requiredVersion)
	}
}
