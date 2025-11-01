package changelog

import (
	"strings"
	"testing"
	"time"
)

func TestParseCommit(t *testing.T) {
	parser := NewParser()
	date := "2024-01-15T10:30:00Z"

	tests := []struct {
		name     string
		message  string
		expected Commit
	}{
		{
			name:    "simple feature",
			message: "feat: add user authentication",
			expected: Commit{
				Type:     TypeFeat,
				Subject:  "add user authentication",
				Breaking: false,
			},
		},
		{
			name:    "feature with scope",
			message: "feat(api): add user authentication endpoint",
			expected: Commit{
				Type:     TypeFeat,
				Scope:    "api",
				Subject:  "add user authentication endpoint",
				Breaking: false,
			},
		},
		{
			name:    "breaking change with !",
			message: "feat!: remove legacy API",
			expected: Commit{
				Type:     TypeFeat,
				Subject:  "remove legacy API",
				Breaking: true,
			},
		},
		{
			name:    "breaking change with scope",
			message: "feat(api)!: remove v1 endpoints",
			expected: Commit{
				Type:     TypeFeat,
				Scope:    "api",
				Subject:  "remove v1 endpoints",
				Breaking: true,
			},
		},
		{
			name: "breaking change in footer",
			message: `feat: update authentication

BREAKING CHANGE: JWT tokens now expire in 1 hour`,
			expected: Commit{
				Type:     TypeFeat,
				Subject:  "update authentication",
				Body:     "",
				Footer:   "BREAKING CHANGE: JWT tokens now expire in 1 hour",
				Breaking: true,
			},
		},
		{
			name:    "bug fix",
			message: "fix: correct validation error",
			expected: Commit{
				Type:     TypeFix,
				Subject:  "correct validation error",
				Breaking: false,
			},
		},
		{
			name:    "documentation",
			message: "docs: update installation guide",
			expected: Commit{
				Type:     TypeDocs,
				Subject:  "update installation guide",
				Breaking: false,
			},
		},
		{
			name:    "refactor",
			message: "refactor(core): simplify error handling",
			expected: Commit{
				Type:     TypeRefactor,
				Scope:    "core",
				Subject:  "simplify error handling",
				Breaking: false,
			},
		},
		{
			name:    "performance",
			message: "perf: optimize database queries",
			expected: Commit{
				Type:     TypePerf,
				Subject:  "optimize database queries",
				Breaking: false,
			},
		},
		{
			name:    "test",
			message: "test: add unit tests for parser",
			expected: Commit{
				Type:     TypeTest,
				Subject:  "add unit tests for parser",
				Breaking: false,
			},
		},
		{
			name:    "non-conventional commit",
			message: "Update README",
			expected: Commit{
				Type:     TypeChore,
				Subject:  "Update README",
				Breaking: false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			commit, err := parser.ParseCommit("abc1234", "John Doe", date, tt.message)
			if err != nil {
				t.Fatalf("ParseCommit failed: %v", err)
			}

			if commit.Type != tt.expected.Type {
				t.Errorf("Type = %v, want %v", commit.Type, tt.expected.Type)
			}

			if commit.Scope != tt.expected.Scope {
				t.Errorf("Scope = %v, want %v", commit.Scope, tt.expected.Scope)
			}

			if commit.Subject != tt.expected.Subject {
				t.Errorf("Subject = %v, want %v", commit.Subject, tt.expected.Subject)
			}

			if commit.Breaking != tt.expected.Breaking {
				t.Errorf("Breaking = %v, want %v", commit.Breaking, tt.expected.Breaking)
			}

			if tt.expected.Footer != "" && commit.Footer != tt.expected.Footer {
				t.Errorf("Footer = %v, want %v", commit.Footer, tt.expected.Footer)
			}
		})
	}
}

func TestParseLog(t *testing.T) {
	parser := NewParser()

	logOutput := `abc1234|John Doe|2024-01-15T10:30:00Z|feat: add authentication
def5678|Jane Smith|2024-01-14T09:20:00Z|fix: correct validation bug
ghi9012|Bob Johnson|2024-01-13T14:45:00Z|docs: update README`

	commits, err := parser.ParseLog(logOutput)
	if err != nil {
		t.Fatalf("ParseLog failed: %v", err)
	}

	if len(commits) != 3 {
		t.Fatalf("Expected 3 commits, got %d", len(commits))
	}

	// Check first commit
	if commits[0].Hash != "abc1234" {
		t.Errorf("Hash = %v, want abc1234", commits[0].Hash)
	}
	if commits[0].Type != TypeFeat {
		t.Errorf("Type = %v, want feat", commits[0].Type)
	}
	if commits[0].Author != "John Doe" {
		t.Errorf("Author = %v, want John Doe", commits[0].Author)
	}

	// Check second commit
	if commits[1].Type != TypeFix {
		t.Errorf("Type = %v, want fix", commits[1].Type)
	}

	// Check third commit
	if commits[2].Type != TypeDocs {
		t.Errorf("Type = %v, want docs", commits[2].Type)
	}
}

func TestGroupCommits(t *testing.T) {
	parser := NewParser()

	commits := []Commit{
		{
			Hash:    "commit1",
			Type:    TypeFeat,
			Subject: "new feature",
			Date:    mustParseTime("2024-01-20T10:00:00Z"),
		},
		{
			Hash:    "commit2",
			Type:    TypeFix,
			Subject: "bug fix",
			Date:    mustParseTime("2024-01-15T10:00:00Z"),
		},
		{
			Hash:    "commit3",
			Type:    TypeDocs,
			Subject: "update docs",
			Date:    mustParseTime("2024-01-10T10:00:00Z"),
		},
	}

	tags := map[string]time.Time{
		"v1.0.0": mustParseTime("2024-01-12T10:00:00Z"),
	}

	changelog := parser.GroupCommits(commits, tags)

	// Check unreleased commits
	if len(changelog.Unreleased) != 2 {
		t.Errorf("Expected 2 unreleased commits, got %d", len(changelog.Unreleased))
	}

	// Check releases
	if len(changelog.Releases) != 1 {
		t.Fatalf("Expected 1 release, got %d", len(changelog.Releases))
	}

	release := changelog.Releases[0]
	if release.Version != "1.0.0" {
		t.Errorf("Version = %v, want 1.0.0", release.Version)
	}

	if len(release.Commits) != 1 {
		t.Errorf("Expected 1 commit in release, got %d", len(release.Commits))
	}
}

func TestGroupCommitsNoTags(t *testing.T) {
	parser := NewParser()

	commits := []Commit{
		{Type: TypeFeat, Subject: "feature 1"},
		{Type: TypeFix, Subject: "fix 1"},
	}

	changelog := parser.GroupCommits(commits, nil)

	if len(changelog.Unreleased) != 2 {
		t.Errorf("Expected 2 unreleased commits, got %d", len(changelog.Unreleased))
	}

	if len(changelog.Releases) != 0 {
		t.Errorf("Expected 0 releases, got %d", len(changelog.Releases))
	}
}

func TestValidateCommit(t *testing.T) {
	tests := []struct {
		name    string
		message string
		wantErr bool
	}{
		{"valid feat", "feat: add feature", false},
		{"valid fix", "fix(api): correct bug", false},
		{"valid breaking", "feat!: breaking change", false},
		{"invalid format", "Update README", true},
		{"invalid format 2", "Added new feature", true},
		{"empty", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCommit(tt.message)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateCommit() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestExtractVersion(t *testing.T) {
	tests := []struct {
		tag  string
		want string
	}{
		{"v1.0.0", "1.0.0"},
		{"v2.3.4", "2.3.4"},
		{"1.0.0", "1.0.0"},
		{"release-1.0.0", "release-1.0.0"},
	}

	for _, tt := range tests {
		t.Run(tt.tag, func(t *testing.T) {
			got := extractVersion(tt.tag)
			if got != tt.want {
				t.Errorf("extractVersion(%v) = %v, want %v", tt.tag, got, tt.want)
			}
		})
	}
}

func TestGroupedCommitsAdd(t *testing.T) {
	grouped := &GroupedCommits{}

	commits := []Commit{
		{Type: TypeFeat, Subject: "feature", Breaking: false},
		{Type: TypeFix, Subject: "fix", Breaking: false},
		{Type: TypeDocs, Subject: "docs", Breaking: false},
		{Type: TypeFeat, Subject: "breaking feature", Breaking: true},
	}

	for _, c := range commits {
		grouped.Add(c)
	}

	if len(grouped.Features) != 1 {
		t.Errorf("Expected 1 feature, got %d", len(grouped.Features))
	}

	if len(grouped.Fixes) != 1 {
		t.Errorf("Expected 1 fix, got %d", len(grouped.Fixes))
	}

	if len(grouped.Docs) != 1 {
		t.Errorf("Expected 1 doc, got %d", len(grouped.Docs))
	}

	if len(grouped.Breaking) != 1 {
		t.Errorf("Expected 1 breaking change, got %d", len(grouped.Breaking))
	}
}

// Helper function
func mustParseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}
