package changelog

import "time"

// CommitType represents the conventional commit type
type CommitType string

const (
	TypeFeat     CommitType = "feat"
	TypeFix      CommitType = "fix"
	TypeDocs     CommitType = "docs"
	TypeStyle    CommitType = "style"
	TypeRefactor CommitType = "refactor"
	TypePerf     CommitType = "perf"
	TypeTest     CommitType = "test"
	TypeBuild    CommitType = "build"
	TypeCI       CommitType = "ci"
	TypeChore    CommitType = "chore"
	TypeRevert   CommitType = "revert"
)

// Commit represents a parsed conventional commit
type Commit struct {
	Hash      string     `json:"hash"`
	Type      CommitType `json:"type"`
	Scope     string     `json:"scope,omitempty"`
	Subject   string     `json:"subject"`
	Body      string     `json:"body,omitempty"`
	Footer    string     `json:"footer,omitempty"`
	Breaking  bool       `json:"breaking"`
	Date      time.Time  `json:"date"`
	Author    string     `json:"author"`
}

// Release represents a version release with associated commits
type Release struct {
	Version   string    `json:"version"`
	Date      time.Time `json:"date"`
	Tag       string    `json:"tag"`
	Commits   []Commit  `json:"commits"`
}

// Changelog represents the complete changelog
type Changelog struct {
	Releases   []Release `json:"releases"`
	Unreleased []Commit  `json:"unreleased,omitempty"`
}

// GroupedCommits groups commits by type
type GroupedCommits struct {
	Breaking []Commit
	Features []Commit
	Fixes    []Commit
	Perf     []Commit
	Refactor []Commit
	Docs     []Commit
	Style    []Commit
	Test     []Commit
	Build    []Commit
	CI       []Commit
	Chore    []Commit
	Revert   []Commit
	Other    []Commit
}

// GroupByType groups commits by their type
func (g *GroupedCommits) Add(c Commit) {
	if c.Breaking {
		g.Breaking = append(g.Breaking, c)
		return
	}

	switch c.Type {
	case TypeFeat:
		g.Features = append(g.Features, c)
	case TypeFix:
		g.Fixes = append(g.Fixes, c)
	case TypePerf:
		g.Perf = append(g.Perf, c)
	case TypeRefactor:
		g.Refactor = append(g.Refactor, c)
	case TypeDocs:
		g.Docs = append(g.Docs, c)
	case TypeStyle:
		g.Style = append(g.Style, c)
	case TypeTest:
		g.Test = append(g.Test, c)
	case TypeBuild:
		g.Build = append(g.Build, c)
	case TypeCI:
		g.CI = append(g.CI, c)
	case TypeChore:
		g.Chore = append(g.Chore, c)
	case TypeRevert:
		g.Revert = append(g.Revert, c)
	default:
		g.Other = append(g.Other, c)
	}
}
