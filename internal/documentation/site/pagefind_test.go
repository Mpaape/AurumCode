package site

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestPagefindGetVersion(t *testing.T) {
	mock := NewMockRunner()
	mock.WithOutput("npx pagefind --version", "pagefind 1.0.0")

	pagefind := NewPagefindBuilder(mock)
	version, err := pagefind.GetVersion(context.Background())
	if err != nil {
		t.Fatalf("GetVersion failed: %v", err)
	}

	if version != "pagefind 1.0.0" {
		t.Errorf("Version = %v", version)
	}
}

func TestPagefindValidate(t *testing.T) {
	mock := NewMockRunner()
	mock.WithOutput("npx pagefind --version", "pagefind 1.0.0")

	pagefind := NewPagefindBuilder(mock)
	err := pagefind.Validate(context.Background())
	if err != nil {
		t.Errorf("Validate failed: %v", err)
	}
}

func TestPagefindValidateMissing(t *testing.T) {
	mock := NewMockRunner()
	mock.WithError("npx pagefind --version", fmt.Errorf("pagefind not found"))

	pagefind := NewPagefindBuilder(mock)
	err := pagefind.Validate(context.Background())
	if err == nil {
		t.Error("Expected error when pagefind not found")
	}
}

func TestPagefindBuild(t *testing.T) {
	mock := NewMockRunner()
	mock.WithOutput("npx pagefind --version", "pagefind 1.0.0")
	mock.WithOutput("npx pagefind --source", "Indexed 10 pages")

	pagefind := NewPagefindBuilder(mock)
	err := pagefind.Build(context.Background(), "/tmp/site")
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// Check calls
	calls := mock.GetCalls()
	if len(calls) < 2 {
		t.Errorf("Expected at least 2 calls, got %d", len(calls))
	}

	// Find build call
	var buildCall *MockCall
	for _, call := range calls {
		if call.Cmd == "npx" && len(call.Args) > 0 && call.Args[0] == "pagefind" {
			// Check if it's not the version call
			if !contains(call.Args, "--version") {
				buildCall = &call
				break
			}
		}
	}

	if buildCall == nil {
		t.Fatal("Build call not found")
	}

	if !contains(buildCall.Args, "--source") {
		t.Error("Missing --source argument")
	}
}

func TestPagefindBuildWithSource(t *testing.T) {
	mock := NewMockRunner()
	mock.WithOutput("npx pagefind --source /custom/public", "Indexed")

	pagefind := NewPagefindBuilder(mock)
	err := pagefind.BuildWithSource(context.Background(), "/tmp/site", "/custom/public")
	if err != nil {
		t.Fatalf("BuildWithSource failed: %v", err)
	}

	calls := mock.GetCalls()
	if len(calls) != 1 {
		t.Fatalf("Expected 1 call, got %d", len(calls))
	}

	call := calls[0]
	if call.Cmd != "npx" {
		t.Errorf("Cmd = %v, want npx", call.Cmd)
	}

	if !contains(call.Args, "pagefind") {
		t.Error("Missing pagefind in args")
	}

	if !contains(call.Args, "/custom/public") {
		t.Error("Missing custom source path in args")
	}
}

func TestPagefindBuildWithConfig(t *testing.T) {
	mock := NewMockRunner()
	mock.WithOutput("npx pagefind --source /output", "Indexed")

	pagefind := NewPagefindBuilder(mock)
	config := &BuildConfig{
		WorkDir:   "/tmp/site",
		OutputDir: "/output",
	}

	err := pagefind.BuildWithConfig(context.Background(), config)
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
}

func TestPagefindBuildFailure(t *testing.T) {
	mock := NewMockRunner()
	mock.WithOutput("npx pagefind --version", "pagefind 1.0.0")
	mock.WithError("npx pagefind --source", fmt.Errorf("indexing failed"))

	pagefind := NewPagefindBuilder(mock)
	err := pagefind.Build(context.Background(), "/tmp/site")
	if err == nil {
		t.Error("Expected build error")
	}
}
