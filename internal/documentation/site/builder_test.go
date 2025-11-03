package site

import (
	"context"
	"strings"
	"testing"
)

func TestSiteBuilderBuild(t *testing.T) {
	mock := NewMockRunner()

	// Setup Jekyll
	mock.WithOutput("jekyll --version", "jekyll 4.3.3")
	mock.WithOutput("jekyll", "done in 1.234 seconds")

	// Setup Pagefind
	mock.WithOutput("npx pagefind --version", "pagefind 1.0.0")
	mock.WithOutput("npx pagefind", "Indexed 10 pages")

	builder := NewSiteBuilder(mock)
	config := &BuildConfig{
		WorkDir:   "/tmp/site",
		OutputDir: "/tmp/site/_site",
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

	if result.OutputPath != "/tmp/site/_site" {
		t.Errorf("OutputPath = %v", result.OutputPath)
	}

	if result.Duration <= 0 {
		t.Error("Expected positive duration")
	}

	// Verify both Jekyll and Pagefind were called
	calls := mock.GetCalls()

	jekyllFound := false
	pagefindFound := false

	for _, call := range calls {
		if call.Cmd == "jekyll" && len(call.Args) > 0 {
			jekyllFound = true
		}
		if call.Cmd == "npx" && contains(call.Args, "pagefind") {
			pagefindFound = true
		}
	}

	if !jekyllFound {
		t.Error("Jekyll was not called")
	}

	if !pagefindFound {
		t.Error("Pagefind was not called")
	}
}

func TestSiteBuilderValidate(t *testing.T) {
	mock := NewMockRunner()
	mock.WithOutput("jekyll --version", "jekyll 4.3.3")
	mock.WithOutput("npx pagefind --version", "pagefind 1.0.0")

	builder := NewSiteBuilder(mock)
	err := builder.Validate(context.Background())
	if err != nil {
		t.Errorf("Validate failed: %v", err)
	}
}

func TestSiteBuilderValidateJekyllMissing(t *testing.T) {
	mock := NewMockRunner()
	mock.WithError("jekyll --version", fmt.Errorf("jekyll not found"))
	mock.WithOutput("npx pagefind --version", "pagefind 1.0.0")

	builder := NewSiteBuilder(mock)
	err := builder.Validate(context.Background())
	if err == nil {
		t.Error("Expected validation error when Jekyll is missing")
	}

	if !strings.Contains(err.Error(), "jekyll") {
		t.Errorf("Error should mention jekyll: %v", err)
	}
}

func TestSiteBuilderValidatePagefindMissing(t *testing.T) {
	mock := NewMockRunner()
	mock.WithOutput("jekyll --version", "jekyll 4.3.3")
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

func TestSiteBuilderBuildJekyllOnly(t *testing.T) {
	mock := NewMockRunner()
	mock.WithOutput("jekyll", "done in 1.234 seconds")

	builder := NewSiteBuilder(mock)
	config := &BuildConfig{
		WorkDir: "/tmp/site",
		Minify:  true,
	}

	result, err := builder.BuildJekyllOnly(context.Background(), config)
	if err != nil {
		t.Fatalf("BuildJekyllOnly failed: %v", err)
	}

	if !result.Success {
		t.Error("Expected successful build")
	}

	// Verify only Jekyll was called
	calls := mock.GetCalls()

	for _, call := range calls {
		if call.Cmd == "npx" && contains(call.Args, "pagefind") {
			t.Error("Pagefind should not be called in Jekyll-only build")
		}
	}
}

func TestSiteBuilderBuildSearchOnly(t *testing.T) {
	mock := NewMockRunner()
	mock.WithOutput("npx pagefind", "Indexed 10 pages")

	builder := NewSiteBuilder(mock)
	config := &BuildConfig{
		WorkDir:   "/tmp/site",
		OutputDir: "/tmp/site/_site",
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
		if call.Cmd == "jekyll" {
			t.Error("Jekyll should not be called in search-only build")
		}
	}
}

func TestSiteBuilderBuildJekyllFailure(t *testing.T) {
	mock := NewMockRunner()
	mock.WithError("jekyll", fmt.Errorf("jekyll build failed"))

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
	mock.WithOutput("jekyll", "Built")
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
