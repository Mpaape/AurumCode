package changelog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWriterGenerate(t *testing.T) {
	writer := NewWriter()

	changelog := &Changelog{
		Unreleased: []Commit{
			{
				Type:    TypeFeat,
				Subject: "add new feature",
				Hash:    "abc1234",
			},
		},
		Releases: []Release{
			{
				Version: "1.0.0",
				Date:    mustParseTime("2024-01-15T10:00:00Z"),
				Tag:     "v1.0.0",
				Commits: []Commit{
					{
						Type:    TypeFeat,
						Scope:   "api",
						Subject: "add authentication",
						Hash:    "def5678",
					},
					{
						Type:    TypeFix,
						Subject: "correct validation bug",
						Hash:    "ghi9012",
					},
				},
			},
		},
	}

	content := writer.Generate(changelog)

	// Check header
	if !strings.Contains(content, "# Changelog") {
		t.Error("Missing changelog header")
	}

	// Check unreleased section
	if !strings.Contains(content, "## [Unreleased]") {
		t.Error("Missing unreleased section")
	}

	// Check release section
	if !strings.Contains(content, "## [1.0.0] - 2024-01-15") {
		t.Error("Missing release section")
	}

	// Check features section
	if !strings.Contains(content, "### Features") {
		t.Error("Missing features section")
	}

	// Check bug fixes section
	if !strings.Contains(content, "### Bug Fixes") {
		t.Error("Missing bug fixes section")
	}

	// Check commit details
	if !strings.Contains(content, "add new feature") {
		t.Error("Missing unreleased feature")
	}

	if !strings.Contains(content, "**api:** add authentication") {
		t.Error("Missing feature with scope")
	}

	if !strings.Contains(content, "correct validation bug") {
		t.Error("Missing bug fix")
	}
}

func TestWriterGenerateBreakingChanges(t *testing.T) {
	writer := NewWriter()

	changelog := &Changelog{
		Releases: []Release{
			{
				Version: "2.0.0",
				Date:    mustParseTime("2024-02-01T10:00:00Z"),
				Tag:     "v2.0.0",
				Commits: []Commit{
					{
						Type:     TypeFeat,
						Subject:  "remove legacy API",
						Breaking: true,
						Footer:   "BREAKING CHANGE: Legacy API endpoints have been removed",
						Hash:     "abc1234",
					},
					{
						Type:    TypeFeat,
						Subject: "add new endpoints",
						Hash:    "def5678",
					},
				},
			},
		},
	}

	content := writer.Generate(changelog)

	// Check breaking changes section
	if !strings.Contains(content, "### ⚠ BREAKING CHANGES") {
		t.Error("Missing breaking changes section")
	}

	// Breaking changes should come before features
	breakingIdx := strings.Index(content, "### ⚠ BREAKING CHANGES")
	featuresIdx := strings.Index(content, "### Features")

	if breakingIdx > featuresIdx {
		t.Error("Breaking changes should appear before features")
	}
}

func TestWriterWithHashes(t *testing.T) {
	writer := NewWriter().WithHashes(true)

	changelog := &Changelog{
		Releases: []Release{
			{
				Version: "1.0.0",
				Date:    mustParseTime("2024-01-15T10:00:00Z"),
				Commits: []Commit{
					{
						Type:    TypeFeat,
						Subject: "add feature",
						Hash:    "abc123456789",
					},
				},
			},
		},
	}

	content := writer.Generate(changelog)

	// Should include short hash
	if !strings.Contains(content, "([abc1234])") {
		t.Error("Missing short hash in output")
	}
}

func TestWriterWithAuthors(t *testing.T) {
	writer := NewWriter().WithAuthors(true)

	changelog := &Changelog{
		Releases: []Release{
			{
				Version: "1.0.0",
				Date:    mustParseTime("2024-01-15T10:00:00Z"),
				Commits: []Commit{
					{
						Type:    TypeFeat,
						Subject: "add feature",
						Author:  "John Doe",
						Hash:    "abc1234",
					},
				},
			},
		},
	}

	content := writer.Generate(changelog)

	// Should include author
	if !strings.Contains(content, "- John Doe") {
		t.Error("Missing author in output")
	}
}

func TestWriterWrite(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "docs", "CHANGELOG.md")

	writer := NewWriter()
	changelog := &Changelog{
		Releases: []Release{
			{
				Version: "1.0.0",
				Date:    mustParseTime("2024-01-15T10:00:00Z"),
				Commits: []Commit{
					{Type: TypeFeat, Subject: "test feature"},
				},
			},
		},
	}

	err := writer.Write(changelog, outputPath)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// Check file exists
	if _, err := os.Stat(outputPath); os.IsNotExist(err) {
		t.Error("Output file was not created")
	}

	// Read and verify content
	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("Failed to read output file: %v", err)
	}

	if !strings.Contains(string(content), "# Changelog") {
		t.Error("Output file missing changelog header")
	}

	if !strings.Contains(string(content), "## [1.0.0]") {
		t.Error("Output file missing release version")
	}
}

func TestWriterAllCommitTypes(t *testing.T) {
	writer := NewWriter()

	changelog := &Changelog{
		Releases: []Release{
			{
				Version: "1.0.0",
				Date:    mustParseTime("2024-01-15T10:00:00Z"),
				Commits: []Commit{
					{Type: TypeFeat, Subject: "feature"},
					{Type: TypeFix, Subject: "fix"},
					{Type: TypeDocs, Subject: "docs"},
					{Type: TypeStyle, Subject: "style"},
					{Type: TypeRefactor, Subject: "refactor"},
					{Type: TypePerf, Subject: "perf"},
					{Type: TypeTest, Subject: "test"},
					{Type: TypeBuild, Subject: "build"},
					{Type: TypeCI, Subject: "ci"},
					{Type: TypeChore, Subject: "chore"},
					{Type: TypeRevert, Subject: "revert"},
				},
			},
		},
	}

	content := writer.Generate(changelog)

	expectedSections := []string{
		"### Features",
		"### Bug Fixes",
		"### Documentation",
		"### Code Refactoring",
		"### Performance Improvements",
		"### Tests",
		"### Build System",
		"### Continuous Integration",
		"### Chores",
		"### Reverts",
	}

	for _, section := range expectedSections {
		if !strings.Contains(content, section) {
			t.Errorf("Missing section: %s", section)
		}
	}
}

func TestWriterIdempotency(t *testing.T) {
	writer := NewWriter()

	changelog := &Changelog{
		Releases: []Release{
			{
				Version: "1.0.0",
				Date:    mustParseTime("2024-01-15T10:00:00Z"),
				Commits: []Commit{
					{Type: TypeFeat, Subject: "test feature"},
				},
			},
		},
	}

	// Generate twice
	content1 := writer.Generate(changelog)
	content2 := writer.Generate(changelog)

	if content1 != content2 {
		t.Error("Writer is not idempotent - outputs differ")
	}
}

func TestWriterEmptyChangelog(t *testing.T) {
	writer := NewWriter()

	changelog := &Changelog{
		Releases: []Release{},
	}

	content := writer.Generate(changelog)

	// Should still have header
	if !strings.Contains(content, "# Changelog") {
		t.Error("Missing header in empty changelog")
	}

	// Should not have any release sections
	if strings.Contains(content, "##") && !strings.Contains(content, "# Changelog") {
		t.Error("Empty changelog should not have release sections")
	}
}
