package site

import (
	"context"
	"strings"
	"testing"
)

func TestSiteBuilderBuild(t *testing.T) {
	mock := NewMockRunner()

	// Setup Hugo
	mock.WithOutput("hugo version", "hugo v0.134.2+extended")
	mock.WithOutput("hugo", "Built in 100 ms")

	// Setup Pagefind
	mock.WithOutput("npx pagefind --version", "pagefind 1.0.0")
	mock.WithOutput("npx pagefind", "Indexed 10 pages")

	builder := NewSiteBuilder(mock)
	config := &BuildConfig{
		WorkDir:   "/tmp/site",
		OutputDir: "/tmp/site/public",
		BaseURL:   "https://example.com",
		Minify:    true,
	}

	result, err := builder.Build(context.Background(), config)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if !result.Success {
		t.Error("Expected successful build")
	}

	if result.OutputPath != "/tmp/site/public" {
		t.Errorf("OutputPath = %v", result.OutputPath)
	}

	if result.Duration <= 0 {
		t.Error("Expected positive duration")
	}

	// Verify both Hugo and Pagefind were called
	calls := mock.GetCalls()

	hugoFound := false
	pagefindFound := false

	for _, call := range calls {
		if call.Cmd == "hugo" && len(call.Args) > 0 {
			hugoFound = true
		}
		if call.Cmd == "npx" && contains(call.Args, "pagefind") {
			pagefindFound = true
		}
	}

	if !hugoFound {
		t.Error("Hugo was not called")
	}

	if !pagefindFound {
		t.Error("Pagefind was not called")
	}
}

func TestSiteBuilderValidate(t *testing.T) {
	mock := NewMockRunner()
	mock.WithOutput("hugo version", "hugo v0.134.2+extended")
	mock.WithOutput("npx pagefind --version", "pagefind 1.0.0")

	builder := NewSiteBuilder(mock)
	err := builder.Validate(context.Background())
	if err != nil {
		t.Errorf("Validate failed: %v", err)
	}
}

func TestSiteBuilderValidateHugoMissing(t *testing.T) {
	mock := NewMockRunner()
	mock.WithError("hugo version", fmt.Errorf("hugo not found"))
	mock.WithOutput("npx pagefind --version", "pagefind 1.0.0")

	builder := NewSiteBuilder(mock)
	err := builder.Validate(context.Background())
	if err == nil {
		t.Error("Expected validation error when Hugo is missing")
	}

	if !strings.Contains(err.Error(), "hugo") {
		t.Errorf("Error should mention hugo: %v", err)
	}
}

func TestSiteBuilderValidatePagefindMissing(t *testing.T) {
	mock := NewMockRunner()
	mock.WithOutput("hugo version", "hugo v0.134.2+extended")
	mock.WithError("npx pagefind --version", fmt.Errorf("pagefind not found"))

	builder := NewSiteBuilder(mock)
	err := builder.Validate(context.Background())
	if err == nil {
		t.Error("Expected validation error when Pagefind is missing")
	}

	if !strings.Contains(err.Error(), "pagefind") {
		t.Errorf("Error should mention pagefind: %v", err)
	}
}

func TestSiteBuilderBuildHugoOnly(t *testing.T) {
	mock := NewMockRunner()
	mock.WithOutput("hugo", "Built in 100 ms")

	builder := NewSiteBuilder(mock)
	config := &BuildConfig{
		WorkDir: "/tmp/site",
		Minify:  true,
	}

	result, err := builder.BuildHugoOnly(context.Background(), config)
	if err != nil {
		t.Fatalf("BuildHugoOnly failed: %v", err)
	}

	if !result.Success {
		t.Error("Expected successful build")
	}

	// Verify only Hugo was called
	calls := mock.GetCalls()

	for _, call := range calls {
		if call.Cmd == "npx" && contains(call.Args, "pagefind") {
			t.Error("Pagefind should not be called in Hugo-only build")
		}
	}
}

func TestSiteBuilderBuildSearchOnly(t *testing.T) {
	mock := NewMockRunner()
	mock.WithOutput("npx pagefind", "Indexed 10 pages")

	builder := NewSiteBuilder(mock)
	config := &BuildConfig{
		WorkDir:   "/tmp/site",
		OutputDir: "/tmp/site/public",
	}

	result, err := builder.BuildSearchOnly(context.Background(), config)
	if err != nil {
		t.Fatalf("BuildSearchOnly failed: %v", err)
	}

	if !result.Success {
		t.Error("Expected successful build")
	}

	// Verify only Pagefind was called
	calls := mock.GetCalls()

	for _, call := range calls {
		if call.Cmd == "hugo" {
			t.Error("Hugo should not be called in search-only build")
		}
	}
}

func TestSiteBuilderBuildHugoFailure(t *testing.T) {
	mock := NewMockRunner()
	mock.WithError("hugo", fmt.Errorf("hugo build failed"))

	builder := NewSiteBuilder(mock)
	config := &BuildConfig{
		WorkDir: "/tmp/site",
	}

	result, err := builder.Build(context.Background(), config)
	if err == nil {
		t.Error("Expected build error")
	}

	if result.Success {
		t.Error("Build should not be successful")
	}

	if result.Error == nil {
		t.Error("Result should contain error")
	}
}

func TestSiteBuilderBuildPagefindFailure(t *testing.T) {
	mock := NewMockRunner()
	mock.WithOutput("hugo", "Built")
	mock.WithError("npx pagefind", fmt.Errorf("indexing failed"))

	builder := NewSiteBuilder(mock)
	config := &BuildConfig{
		WorkDir: "/tmp/site",
	}

	result, err := builder.Build(context.Background(), config)
	if err == nil {
		t.Error("Expected build error")
	}

	if result.Success {
		t.Error("Build should not be successful")
	}
}
