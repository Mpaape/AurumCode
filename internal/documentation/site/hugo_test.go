package site

import (
	"context"
	"fmt"
	"testing"
)

func TestHugoGetVersion(t *testing.T) {
	mock := NewMockRunner()
	mock.WithOutput("hugo version", "hugo v0.134.2+extended linux/amd64")

	hugo := NewHugoBuilder(mock)
	version, err := hugo.GetVersion(context.Background())
	if err != nil {
		t.Fatalf("GetVersion failed: %v", err)
	}

	if version != "hugo v0.134.2+extended linux/amd64" {
		t.Errorf("Version = %v", version)
	}
}

func TestHugoValidate(t *testing.T) {
	mock := NewMockRunner()
	mock.WithOutput("hugo version", "hugo v0.134.2+extended")

	hugo := NewHugoBuilder(mock).WithRequiredVersion("0.134.2")
	err := hugo.Validate(context.Background())
	if err != nil {
		t.Errorf("Validate failed: %v", err)
	}
}

func TestHugoValidateVersionMismatch(t *testing.T) {
	mock := NewMockRunner()
	mock.WithOutput("hugo version", "hugo v0.100.0+extended")

	hugo := NewHugoBuilder(mock).WithRequiredVersion("0.134.2")
	err := hugo.Validate(context.Background())
	if err == nil {
		t.Error("Expected version mismatch error")
	}
}

func TestHugoValidateMissing(t *testing.T) {
	mock := NewMockRunner()
	mock.WithError("hugo version", fmt.Errorf("hugo: command not found"))

	hugo := NewHugoBuilder(mock)
	err := hugo.Validate(context.Background())
	if err == nil {
		t.Error("Expected error when hugo not found")
	}
}

func TestHugoBuild(t *testing.T) {
	mock := NewMockRunner()
	mock.WithOutput("hugo version", "hugo v0.134.2+extended")
	mock.WithOutput("hugo --minify", "Total in 123 ms\nBuilt in 100 ms")

	hugo := NewHugoBuilder(mock).WithRequiredVersion("0.134.2")
	err := hugo.Build(context.Background(), "/tmp/site")
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// Check calls
	calls := mock.GetCalls()
	if len(calls) != 2 {
		t.Errorf("Expected 2 calls (version + build), got %d", len(calls))
	}

	buildCall := calls[1]
	if buildCall.Cmd != "hugo" {
		t.Errorf("Cmd = %v, want hugo", buildCall.Cmd)
	}

	if buildCall.Workdir != "/tmp/site" {
		t.Errorf("Workdir = %v, want /tmp/site", buildCall.Workdir)
	}
}

func TestHugoBuildWithConfig(t *testing.T) {
	mock := NewMockRunner()
	mock.WithOutput("hugo --destination /out --baseURL https://example.com --minify", "Built")

	hugo := NewHugoBuilder(mock)
	config := &BuildConfig{
		WorkDir:   "/tmp/site",
		OutputDir: "/out",
		BaseURL:   "https://example.com",
		Minify:    true,
	}

	err := hugo.BuildWithConfig(context.Background(), config)
	if err != nil {
		t.Fatalf("BuildWithConfig failed: %v", err)
	}

	calls := mock.GetCalls()
	if len(calls) != 1 {
		t.Fatalf("Expected 1 call, got %d", len(calls))
	}

	call := calls[0]
	if call.Workdir != "/tmp/site" {
		t.Errorf("Workdir = %v", call.Workdir)
	}

	// Check args contain expected values
	argsStr := fmt.Sprintf("%v", call.Args)
	if !(contains(call.Args, "--destination") && contains(call.Args, "/out")) {
		t.Error("Missing destination args")
	}

	if !(contains(call.Args, "--baseURL") && contains(call.Args, "https://example.com")) {
		t.Error("Missing baseURL args")
	}

	if !contains(call.Args, "--minify") {
		t.Error("Missing minify arg")
	}
}

func TestHugoBuildFailure(t *testing.T) {
	mock := NewMockRunner()
	mock.WithOutput("hugo version", "hugo v0.134.2+extended")
	mock.WithError("hugo --minify", fmt.Errorf("build error: template not found"))

	hugo := NewHugoBuilder(mock).WithRequiredVersion("0.134.2")
	err := hugo.Build(context.Background(), "/tmp/site")
	if err == nil {
		t.Error("Expected build error")
	}
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
