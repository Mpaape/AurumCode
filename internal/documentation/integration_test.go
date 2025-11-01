package documentation

import (
	"aurumcode/internal/documentation/api"
	"aurumcode/internal/documentation/changelog"
	"aurumcode/internal/documentation/readme"
	"aurumcode/internal/documentation/site"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestDocumentationPipeline tests the complete documentation generation flow
func TestDocumentationPipeline(t *testing.T) {
	// Create temporary repository
	tmpDir := t.TempDir()

	// Step 1: Setup git repository with commits
	setupGitRepo(t, tmpDir)

	// Step 2: Create README with markers
	setupREADME(t, tmpDir)

	// Step 3: Create OpenAPI spec
	setupOpenAPISpec(t, tmpDir)

	// Step 4: Generate changelog
	t.Run("GenerateChangelog", func(t *testing.T) {
		testChangelogGeneration(t, tmpDir)
	})

	// Step 5: Update README
	t.Run("UpdateREADME", func(t *testing.T) {
		testREADMEUpdate(t, tmpDir)
	})

	// Step 6: Generate API docs
	t.Run("GenerateAPIDoc", func(t *testing.T) {
		testAPIDocGeneration(t, tmpDir)
	})

	// Step 7: Verify all docs exist
	t.Run("VerifyDocs", func(t *testing.T) {
		verifyDocsExist(t, tmpDir)
	})

	// Step 8: Test idempotency
	t.Run("TestIdempotency", func(t *testing.T) {
		testIdempotency(t, tmpDir)
	})
}

func setupGitRepo(t *testing.T, tmpDir string) {
	// Create sample git history (simulated)
	// In real usage, this would use actual git commands
	// For testing, we create commit log manually
	t.Helper()
}

func setupREADME(t *testing.T, tmpDir string) {
	t.Helper()

	readmeContent := `# Test Project

This is a test project for documentation generation.

## Status

<!-- aurum:start:status -->
Status will be updated here
<!-- aurum:end:status -->

## Installation

Install the project using npm or go.

<!-- aurum:start:badges -->
Badges will be here
<!-- aurum:end:badges -->

## Usage

Usage instructions.
`

	readmePath := filepath.Join(tmpDir, "README.md")
	if err := os.WriteFile(readmePath, []byte(readmeContent), 0644); err != nil {
		t.Fatal(err)
	}
}

func setupOpenAPISpec(t *testing.T, tmpDir string) {
	t.Helper()

	specContent := `openapi: 3.0.0
info:
  title: Test API
  version: 1.0.0
  description: Test API for integration testing
  contact:
    name: Test Team
    email: test@example.com
servers:
  - url: https://api.test.example.com
    description: Test server
paths:
  /users:
    get:
      tags:
        - users
      summary: List users
      description: Get all users
  /users/{id}:
    get:
      tags:
        - users
      summary: Get user
      description: Get a single user by ID
    delete:
      tags:
        - users
      summary: Delete user
      description: Delete a user
tags:
  - name: users
    description: User management
`

	specPath := filepath.Join(tmpDir, "openapi.yaml")
	if err := os.WriteFile(specPath, []byte(specContent), 0644); err != nil {
		t.Fatal(err)
	}
}

func testChangelogGeneration(t *testing.T, tmpDir string) {
	t.Helper()

	// Create sample commits
	commits := []changelog.Commit{
		{
			Hash:    "abc123",
			Type:    changelog.TypeFeat,
			Scope:   "api",
			Subject: "add user authentication",
			Date:    time.Now().Add(-48 * time.Hour),
			Author:  "Alice",
		},
		{
			Hash:    "def456",
			Type:    changelog.TypeFix,
			Subject: "correct validation error",
			Date:    time.Now().Add(-24 * time.Hour),
			Author:  "Bob",
		},
		{
			Hash:    "ghi789",
			Type:    changelog.TypeDocs,
			Subject: "update README",
			Date:    time.Now(),
			Author:  "Charlie",
		},
	}

	// Create tags
	tags := map[string]time.Time{
		"v1.0.0": time.Now().Add(-36 * time.Hour),
	}

	// Generate changelog
	writer := changelog.NewWriter()
	err := writer.UpdateChangelog(tmpDir, commits, tags)
	if err != nil {
		t.Fatalf("UpdateChangelog failed: %v", err)
	}

	// Verify changelog exists
	changelogPath := filepath.Join(tmpDir, "docs", "CHANGELOG.md")
	if _, err := os.Stat(changelogPath); os.IsNotExist(err) {
		t.Fatal("CHANGELOG.md was not created")
	}

	// Read and verify content
	content, err := os.ReadFile(changelogPath)
	if err != nil {
		t.Fatal(err)
	}

	changelogStr := string(content)

	// Check for expected sections
	expectedSections := []string{
		"# Changelog",
		"## [Unreleased]",
		"## [1.0.0]",
		"### Features",
		"### Bug Fixes",
		"### Documentation",
		"add user authentication",
		"correct validation error",
		"update README",
	}

	for _, section := range expectedSections {
		if !strings.Contains(changelogStr, section) {
			t.Errorf("CHANGELOG missing expected section: %s", section)
		}
	}
}

func testREADMEUpdate(t *testing.T, tmpDir string) {
	t.Helper()

	readmePath := filepath.Join(tmpDir, "README.md")

	updater := readme.NewUpdater()
	sections := []readme.Section{
		{
			Name:    "status",
			Content: "✅ **Status:** All systems operational",
		},
		{
			Name:    "badges",
			Content: "[![Build](https://img.shields.io/badge/build-passing-green)](url)\n[![Tests](https://img.shields.io/badge/tests-100%25-green)](url)",
		},
	}

	result, err := updater.Update(readmePath, sections)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	if !result.Updated {
		t.Error("README should have been updated")
	}

	// Read and verify
	content, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatal(err)
	}

	readmeStr := string(content)

	// Check new content is present
	if !strings.Contains(readmeStr, "All systems operational") {
		t.Error("Status not updated")
	}

	if !strings.Contains(readmeStr, "build-passing-green") {
		t.Error("Badges not updated")
	}

	// Check original content is preserved
	if !strings.Contains(readmeStr, "## Installation") {
		t.Error("Original content was lost")
	}

	if !strings.Contains(readmeStr, "## Usage") {
		t.Error("Original content was lost")
	}
}

func testAPIDocGeneration(t *testing.T, tmpDir string) {
	t.Helper()

	// Generate API docs
	err := api.GenerateAPI(tmpDir)
	if err != nil {
		t.Fatalf("GenerateAPI failed: %v", err)
	}

	// Verify API.md exists
	apiPath := filepath.Join(tmpDir, "docs", "API.md")
	if _, err := os.Stat(apiPath); os.IsNotExist(err) {
		t.Fatal("API.md was not created")
	}

	// Read and verify content
	content, err := os.ReadFile(apiPath)
	if err != nil {
		t.Fatal(err)
	}

	apiStr := string(content)

	// Check for expected content
	expectedContent := []string{
		"# Test API",
		"**Version:** 1.0.0",
		"Test API for integration testing",
		"## Contact",
		"test@example.com",
		"## Servers",
		"https://api.test.example.com",
		"## Endpoints",
		"### Users",
		"`GET /users`",
		"List users",
		"`DELETE /users/{id}`",
	}

	for _, expected := range expectedContent {
		if !strings.Contains(apiStr, expected) {
			t.Errorf("API.md missing expected content: %s", expected)
		}
	}
}

func verifyDocsExist(t *testing.T, tmpDir string) {
	t.Helper()

	// Check all expected files exist
	expectedFiles := []string{
		"docs/CHANGELOG.md",
		"docs/API.md",
		"README.md",
	}

	for _, file := range expectedFiles {
		path := filepath.Join(tmpDir, file)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("Expected file not found: %s", file)
		}
	}

	// Verify docs directory structure
	docsDir := filepath.Join(tmpDir, "docs")
	entries, err := os.ReadDir(docsDir)
	if err != nil {
		t.Fatalf("Failed to read docs dir: %v", err)
	}

	if len(entries) < 2 {
		t.Errorf("Expected at least 2 files in docs/, got %d", len(entries))
	}
}

func testIdempotency(t *testing.T, tmpDir string) {
	t.Helper()

	// Read current state
	changelogPath := filepath.Join(tmpDir, "docs", "CHANGELOG.md")
	changelog1, err := os.ReadFile(changelogPath)
	if err != nil {
		t.Fatal(err)
	}

	apiPath := filepath.Join(tmpDir, "docs", "API.md")
	api1, err := os.ReadFile(apiPath)
	if err != nil {
		t.Fatal(err)
	}

	readmePath := filepath.Join(tmpDir, "README.md")
	readme1, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatal(err)
	}

	// Run generation again
	testAPIDocGeneration(t, tmpDir)

	// Update README again with same content
	updater := readme.NewUpdater()
	sections := []readme.Section{
		{
			Name:    "status",
			Content: "✅ **Status:** All systems operational",
		},
	}

	result, err := updater.Update(readmePath, sections)
	if err != nil {
		t.Fatal(err)
	}

	if result.Updated {
		t.Error("Second update should be idempotent (no changes)")
	}

	// Verify content hasn't changed
	changelog2, _ := os.ReadFile(changelogPath)
	api2, _ := os.ReadFile(apiPath)
	readme2, _ := os.ReadFile(readmePath)

	if string(changelog1) != string(changelog2) {
		t.Error("Changelog changed on second run (not idempotent)")
	}

	if string(api1) != string(api2) {
		t.Error("API docs changed on second run (not idempotent)")
	}

	if string(readme1) != string(readme2) {
		t.Error("README changed on second run (not idempotent)")
	}
}

// TestSiteBuildWithMocks tests the site build with mocked commands
func TestSiteBuildWithMocks(t *testing.T) {
	tmpDir := t.TempDir()

	// Setup Hugo content
	contentDir := filepath.Join(tmpDir, "content")
	if err := os.MkdirAll(contentDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create sample markdown
	sampleContent := `---
title: "Test Page"
---

# Test Content

This is a test page.
`

	contentPath := filepath.Join(contentDir, "test.md")
	if err := os.WriteFile(contentPath, []byte(sampleContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create mock runner
	mock := site.NewMockRunner()
	mock.WithOutput("hugo version", "hugo v0.134.2+extended")
	mock.WithOutput("hugo", "Built in 100 ms")
	mock.WithOutput("npx pagefind --version", "pagefind 1.0.0")
	mock.WithOutput("npx pagefind", "Indexed 1 page")

	// Build site
	builder := site.NewSiteBuilder(mock)
	config := &site.BuildConfig{
		WorkDir:   tmpDir,
		OutputDir: filepath.Join(tmpDir, "public"),
		Minify:    true,
	}

	result, err := builder.Build(context.Background(), config)
	if err != nil {
		t.Fatalf("Site build failed: %v", err)
	}

	if !result.Success {
		t.Error("Expected successful build")
	}

	// Verify commands were called
	calls := mock.GetCalls()
	if len(calls) < 2 {
		t.Errorf("Expected at least 2 command calls, got %d", len(calls))
	}
}
