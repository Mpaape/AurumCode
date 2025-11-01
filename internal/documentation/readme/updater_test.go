package readme

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testREADME = `# Test Project

This is a test project.

<!-- aurum:start -->
Old content here
<!-- aurum:end -->

## Installation

Install instructions here.

<!-- aurum:start:badges -->
Old badges
<!-- aurum:end:badges -->

## Usage

Usage instructions.
`

func TestUpdateSection(t *testing.T) {
	tmpDir := t.TempDir()
	readmePath := filepath.Join(tmpDir, "README.md")

	// Create test README
	if err := os.WriteFile(readmePath, []byte(testREADME), 0644); err != nil {
		t.Fatal(err)
	}

	updater := NewUpdater()
	sections := []Section{
		{
			Name:    "default",
			Content: "New content here",
		},
	}

	result, err := updater.Update(readmePath, sections)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	if !result.Updated {
		t.Error("Expected update to occur")
	}

	if len(result.Changes) != 1 {
		t.Errorf("Expected 1 change, got %d", len(result.Changes))
	}

	// Read updated file
	content, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatal(err)
	}

	updated := string(content)

	// Should contain new content
	if !strings.Contains(updated, "New content here") {
		t.Error("Updated content not found")
	}

	// Should not contain old content
	if strings.Contains(updated, "Old content here") {
		t.Error("Old content still present")
	}

	// Should preserve unmarked content
	if !strings.Contains(updated, "## Installation") {
		t.Error("Unmarked content was modified")
	}

	if !strings.Contains(updated, "Install instructions here") {
		t.Error("Unmarked content was modified")
	}
}

func TestUpdateNamedSection(t *testing.T) {
	tmpDir := t.TempDir()
	readmePath := filepath.Join(tmpDir, "README.md")

	if err := os.WriteFile(readmePath, []byte(testREADME), 0644); err != nil {
		t.Fatal(err)
	}

	updater := NewUpdater()
	sections := []Section{
		{
			Name:    "badges",
			Content: "[![Build](badge.svg)](url)",
		},
	}

	result, err := updater.Update(readmePath, sections)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	if !result.Updated {
		t.Error("Expected update to occur")
	}

	// Read file
	content, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatal(err)
	}

	updated := string(content)

	// Should contain new badges
	if !strings.Contains(updated, "[![Build](badge.svg)]") {
		t.Error("New badges not found")
	}

	// Should not contain old badges
	if strings.Contains(updated, "Old badges") {
		t.Error("Old badges still present")
	}

	// Should preserve default section
	if !strings.Contains(updated, "Old content here") {
		t.Error("Other section was modified")
	}
}

func TestUpdateMultipleSections(t *testing.T) {
	tmpDir := t.TempDir()
	readmePath := filepath.Join(tmpDir, "README.md")

	if err := os.WriteFile(readmePath, []byte(testREADME), 0644); err != nil {
		t.Fatal(err)
	}

	updater := NewUpdater()
	sections := []Section{
		{
			Name:    "default",
			Content: "New default content",
		},
		{
			Name:    "badges",
			Content: "New badges content",
		},
	}

	result, err := updater.Update(readmePath, sections)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	if !result.Updated {
		t.Error("Expected update to occur")
	}

	if len(result.Changes) != 2 {
		t.Errorf("Expected 2 changes, got %d", len(result.Changes))
	}

	// Read file
	content, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatal(err)
	}

	updated := string(content)

	// Both sections should be updated
	if !strings.Contains(updated, "New default content") {
		t.Error("Default section not updated")
	}

	if !strings.Contains(updated, "New badges content") {
		t.Error("Badges section not updated")
	}
}

func TestIdempotency(t *testing.T) {
	tmpDir := t.TempDir()
	readmePath := filepath.Join(tmpDir, "README.md")

	if err := os.WriteFile(readmePath, []byte(testREADME), 0644); err != nil {
		t.Fatal(err)
	}

	updater := NewUpdater()
	sections := []Section{
		{
			Name:    "default",
			Content: "New content",
		},
	}

	// First update
	result1, err := updater.Update(readmePath, sections)
	if err != nil {
		t.Fatalf("First update failed: %v", err)
	}

	if !result1.Updated {
		t.Error("First update should have changed content")
	}

	// Second update with same content
	result2, err := updater.Update(readmePath, sections)
	if err != nil {
		t.Fatalf("Second update failed: %v", err)
	}

	if result2.Updated {
		t.Error("Second update should not have changed content (idempotent)")
	}

	if len(result2.Changes) > 0 {
		t.Error("Second update should report no changes")
	}
}

func TestDryRun(t *testing.T) {
	tmpDir := t.TempDir()
	readmePath := filepath.Join(tmpDir, "README.md")

	originalContent := testREADME
	if err := os.WriteFile(readmePath, []byte(originalContent), 0644); err != nil {
		t.Fatal(err)
	}

	updater := NewUpdater().WithDryRun(true)
	sections := []Section{
		{
			Name:    "default",
			Content: "New content",
		},
	}

	result, err := updater.Update(readmePath, sections)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	if !result.Updated {
		t.Error("Dry run should detect changes")
	}

	// File should not be modified
	content, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatal(err)
	}

	if string(content) != originalContent {
		t.Error("Dry run modified the file")
	}
}

func TestMissingMarkers(t *testing.T) {
	tmpDir := t.TempDir()
	readmePath := filepath.Join(tmpDir, "README.md")

	// README without markers
	noMarkers := `# Test Project

No markers here.
`

	if err := os.WriteFile(readmePath, []byte(noMarkers), 0644); err != nil {
		t.Fatal(err)
	}

	updater := NewUpdater()
	sections := []Section{
		{
			Name:    "default",
			Content: "New content",
		},
	}

	_, err := updater.Update(readmePath, sections)
	if err == nil {
		t.Error("Expected error for missing markers")
	}

	if !strings.Contains(err.Error(), "marker not found") {
		t.Errorf("Wrong error message: %v", err)
	}
}

func TestGetSection(t *testing.T) {
	tmpDir := t.TempDir()
	readmePath := filepath.Join(tmpDir, "README.md")

	if err := os.WriteFile(readmePath, []byte(testREADME), 0644); err != nil {
		t.Fatal(err)
	}

	updater := NewUpdater()

	// Get default section
	content, err := updater.GetSection(readmePath, "default")
	if err != nil {
		t.Fatalf("GetSection failed: %v", err)
	}

	if !strings.Contains(content, "Old content here") {
		t.Errorf("Wrong content extracted: %s", content)
	}

	// Get badges section
	badges, err := updater.GetSection(readmePath, "badges")
	if err != nil {
		t.Fatalf("GetSection failed: %v", err)
	}

	if !strings.Contains(badges, "Old badges") {
		t.Errorf("Wrong badges extracted: %s", badges)
	}
}

func TestHasSection(t *testing.T) {
	tmpDir := t.TempDir()
	readmePath := filepath.Join(tmpDir, "README.md")

	if err := os.WriteFile(readmePath, []byte(testREADME), 0644); err != nil {
		t.Fatal(err)
	}

	updater := NewUpdater()

	// Check existing sections
	exists, err := updater.HasSection(readmePath, "default")
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Error("Default section should exist")
	}

	exists, err = updater.HasSection(readmePath, "badges")
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Error("Badges section should exist")
	}

	// Check non-existing section
	exists, err = updater.HasSection(readmePath, "nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	if exists {
		t.Error("Non-existent section should not exist")
	}
}

func TestDiff(t *testing.T) {
	tmpDir := t.TempDir()
	readmePath := filepath.Join(tmpDir, "README.md")

	if err := os.WriteFile(readmePath, []byte(testREADME), 0644); err != nil {
		t.Fatal(err)
	}

	updater := NewUpdater()
	sections := []Section{
		{
			Name:    "default",
			Content: "New content",
		},
		{
			Name:    "badges",
			Content: "New badges",
		},
	}

	diff, err := updater.Diff(readmePath, sections)
	if err != nil {
		t.Fatalf("Diff failed: %v", err)
	}

	// Should show both changes
	if !strings.Contains(diff, "Section: default") {
		t.Error("Diff missing default section")
	}

	if !strings.Contains(diff, "Section: badges") {
		t.Error("Diff missing badges section")
	}

	if !strings.Contains(diff, "Old content here") {
		t.Error("Diff missing old content")
	}

	if !strings.Contains(diff, "New content") {
		t.Error("Diff missing new content")
	}
}

func TestInsertSection(t *testing.T) {
	tmpDir := t.TempDir()
	readmePath := filepath.Join(tmpDir, "README.md")

	initial := `# Test Project

Introduction here.

## Features

Feature list.
`

	if err := os.WriteFile(readmePath, []byte(initial), 0644); err != nil {
		t.Fatal(err)
	}

	updater := NewUpdater()
	section := Section{
		Name:    "api-status",
		Content: "API Status: Online",
	}

	err := updater.InsertSection(readmePath, section, "## Features")
	if err != nil {
		t.Fatalf("InsertSection failed: %v", err)
	}

	// Read file
	content, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatal(err)
	}

	updated := string(content)

	// Should contain new section
	if !strings.Contains(updated, "<!-- aurum:start:api-status -->") {
		t.Error("Start marker not found")
	}

	if !strings.Contains(updated, "API Status: Online") {
		t.Error("Section content not found")
	}

	if !strings.Contains(updated, "<!-- aurum:end:api-status -->") {
		t.Error("End marker not found")
	}

	// Original content should be preserved
	if !strings.Contains(updated, "Introduction here") {
		t.Error("Original content was lost")
	}
}

func TestPreserveUnmarkedContent(t *testing.T) {
	tmpDir := t.TempDir()
	readmePath := filepath.Join(tmpDir, "README.md")

	complex := `# Project

Introduction paragraph.

## Overview

<!-- aurum:start:status -->
Old status
<!-- aurum:end:status -->

Middle paragraph should be preserved.

## Details

Detail content here.

<!-- aurum:start:badges -->
Old badges
<!-- aurum:end:badges -->

Footer content.
`

	if err := os.WriteFile(readmePath, []byte(complex), 0644); err != nil {
		t.Fatal(err)
	}

	updater := NewUpdater()
	sections := []Section{
		{
			Name:    "status",
			Content: "New status",
		},
		{
			Name:    "badges",
			Content: "New badges",
		},
	}

	_, err := updater.Update(readmePath, sections)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	// Read file
	content, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatal(err)
	}

	updated := string(content)

	// All unmarked content should be preserved
	mustContain := []string{
		"# Project",
		"Introduction paragraph",
		"## Overview",
		"Middle paragraph should be preserved",
		"## Details",
		"Detail content here",
		"Footer content",
	}

	for _, expected := range mustContain {
		if !strings.Contains(updated, expected) {
			t.Errorf("Unmarked content lost: %s", expected)
		}
	}

	// New content should be present
	if !strings.Contains(updated, "New status") {
		t.Error("New status not found")
	}

	if !strings.Contains(updated, "New badges") {
		t.Error("New badges not found")
	}

	// Old content should be gone
	if strings.Contains(updated, "Old status") {
		t.Error("Old status still present")
	}

	if strings.Contains(updated, "Old badges") {
		t.Error("Old badges still present")
	}
}
