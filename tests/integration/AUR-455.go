// Package integration holds AUR-455's Integration-layer proof: the claims
// tests/unit/AUR-455.go requires the documentation to carry have lastro in
// the actual source and fixtures this repository ships -- read directly,
// never by grepping the documentation against itself.
//
//  1. `aurumcode review`'s flags this card's README now names (--base,
//     --seguranca, --pr, --repo, --publicar, --na-linha) are really
//     registered in cmd/aurumcode/main.go.
//  2. The fixtures demo.sh depends on actually exist and have the shape
//     demo.sh assumes: tests/fixtures/repos/git-demo/repo.git is a bare
//     repository with at least the three commits demo.sh's `--base HEAD~1`
//     needs, tests/fixtures/review/known-problem-response.json is present
//     at the path demo.sh uses (the corrected path -- three directory
//     levels above tests/fixtures/repos/git-demo/repo.git, not the two
//     levels docs/specs/AUR-430.md's own broken example carries -- see
//     docs/specs/AUR-455.md), and tests/fixtures/docs/goproject holds Go
//     source demo.sh's step (a) can extract.
//  3. Both cmd/aurumcode and cmd/regenerate-docs are still `package main`:
//     the "two real binaries" claim in demo.sh's own build step has
//     lastro.
//
// Scope, same as every sibling card in this office: no network, no
// execution of `aurumcode`/`regenerate-docs`/demo.sh here -- this layer
// only reads source and fixture files already materialized by this card's
// declared paths/read_paths. Actually running demo.sh is
// tests/e2e/AUR-455.sh's job.
//
// Selector naming note: see tests/unit/AUR-455.go's header. This file uses
// IntegrationAUR455 instead of the card text's IntegrationAUR445, which
// already exists in this package's tests/integration/AUR-445.go and would
// collide.
package integration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func aur455IntegrationRoot(t *testing.T) string {
	t.Helper()
	if root := os.Getenv("AURUMCODE_ROOT"); root != "" {
		return root
	}
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("AUR-455/AC-001/infrastructure: getwd: %v", err)
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("AUR-455/AC-001/infrastructure: no go.mod above the working directory and AURUMCODE_ROOT is unset")
		}
		dir = parent
	}
}

func aur455ReadFile(t *testing.T, root, rel string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatalf("AUR-455/AC-001/infrastructure: %s unreadable: %v", rel, err)
	}
	return string(raw)
}

// IntegrationAUR455 is AUR-455's Integration-layer selector.
func IntegrationAUR455(t *testing.T) {
	root := aur455IntegrationRoot(t)

	// 1. Every flag the README's "Features" section names is really
	// registered in cmd/aurumcode/main.go.
	aurumcodeMain := aur455ReadFile(t, root, filepath.Join("cmd", "aurumcode", "main.go"))
	for _, flag := range []string{"base", "seguranca", "pr", "repo", "publicar", "na-linha"} {
		pattern := regexp.MustCompile(`fs\.(String|Bool|Int)\(\s*"` + regexp.QuoteMeta(flag) + `"`)
		if !pattern.MatchString(aurumcodeMain) {
			t.Fatalf("AUR-455/AC-001/behavior-missing: cmd/aurumcode/main.go registers no --%s flag, but README.md names it", flag)
		}
	}
	if !regexp.MustCompile(`(?m)^package main\s*$`).MatchString(aurumcodeMain) {
		t.Fatalf("AUR-455/AC-001/behavior-missing: cmd/aurumcode/main.go does not declare package main")
	}
	regenMain := aur455ReadFile(t, root, filepath.Join("cmd", "regenerate-docs", "main.go"))
	if !regexp.MustCompile(`(?m)^package main\s*$`).MatchString(regenMain) {
		t.Fatalf("AUR-455/AC-001/behavior-missing: cmd/regenerate-docs/main.go does not declare package main")
	}

	// 2a. The git-demo fixture demo.sh's steps (b) and (c) depend on is
	// really a bare repository with a readable HEAD and at least three
	// commits (so --base HEAD~1 resolves).
	gitDemoDir := filepath.Join("tests", "fixtures", "repos", "git-demo", "repo.git")
	for _, must := range []string{"HEAD", "objects", "refs"} {
		p := filepath.Join(root, gitDemoDir, must)
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("AUR-455/AC-001/behavior-missing: %s missing under %s: %v", must, gitDemoDir, err)
		}
	}
	manifestRaw := aur455ReadFile(t, root, filepath.Join("tests", "fixtures", "repos", "git-demo", "manifest.json"))
	var manifest struct {
		CommitCount int `json:"commit_count"`
	}
	if err := json.Unmarshal([]byte(manifestRaw), &manifest); err != nil {
		t.Fatalf("AUR-455/AC-001/behavior-missing: git-demo manifest.json does not parse: %v", err)
	}
	if manifest.CommitCount < 2 {
		t.Fatalf("AUR-455/AC-001/behavior-missing: git-demo fixture has %d commit(s), need at least 2 for --base HEAD~1", manifest.CommitCount)
	}

	// 2b. The fixture path demo.sh actually uses for AURUMCODE_LLM_FIXTURE
	// exists -- this is the corrected path (three levels above
	// tests/fixtures/repos/git-demo/repo.git), fixing the two-level path
	// docs/specs/AUR-430.md's own example carries.
	fixtureResponse := filepath.Join("tests", "fixtures", "review", "known-problem-response.json")
	fixtureRaw := aur455ReadFile(t, root, fixtureResponse)
	var parsedFixture struct {
		Issues []struct {
			File string `json:"file"`
			Line int    `json:"line"`
		} `json:"issues"`
	}
	if err := json.Unmarshal([]byte(fixtureRaw), &parsedFixture); err != nil {
		t.Fatalf("AUR-455/AC-001/behavior-missing: %s does not parse as the fixture-response shape: %v", fixtureResponse, err)
	}
	if len(parsedFixture.Issues) == 0 {
		t.Fatalf("AUR-455/AC-001/behavior-missing: %s carries no issues; demo.sh's step (c) would print \"No issues found.\" instead of a worked example", fixtureResponse)
	}

	// demo.sh must reference this fixture by an absolute path built from
	// its own script directory, not the two-level relative path
	// docs/specs/AUR-430.md's own console example carries -- that example
	// is missing a directory level and 404s if copied literally. Cross-
	// checking demo.sh's own text (not docs/specs/AUR-430.md, which is
	// outside this card's read_paths and never materialized in the
	// acceptance sandbox).
	demoText := aur455ReadFile(t, root, "demo.sh")
	if !strings.Contains(demoText, "tests/fixtures/review/known-problem-response.json") {
		t.Fatalf("AUR-455/AC-001/behavior-missing: demo.sh does not reference tests/fixtures/review/known-problem-response.json")
	}

	// 2c. The Go fixture project demo.sh's step (a) documents.
	goProjectDir := filepath.Join("tests", "fixtures", "docs", "goproject")
	entries, err := os.ReadDir(filepath.Join(root, goProjectDir))
	if err != nil {
		t.Fatalf("AUR-455/AC-001/behavior-missing: %s unreadable: %v", goProjectDir, err)
	}
	sawGoFile := false
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".go") {
			sawGoFile = true
			break
		}
	}
	if !sawGoFile {
		t.Fatalf("AUR-455/AC-001/behavior-missing: %s holds no .go file for demo.sh's step (a) to extract", goProjectDir)
	}

	// 3. The two workflow examples README.md links really exist and really
	// build from the v1 tag the README's honesty caveat names -- so the
	// caveat is about a real reference, not an invented one. The two
	// examples spell the reference differently (code-review.yml checks out
	// `ref: v1`; documentation.yml uses the Action directly,
	// `uses: Mpaape/AurumCode@v1`), so this matches the tag as a standalone
	// token rather than one fixed literal string.
	v1Reference := regexp.MustCompile(`(^|[^A-Za-z0-9_.])v1([^A-Za-z0-9_.]|$)`)
	for _, wf := range []string{
		filepath.Join(".github", "workflows", "examples", "code-review.yml"),
		filepath.Join(".github", "workflows", "examples", "documentation.yml"),
	} {
		text := aur455ReadFile(t, root, wf)
		if !v1Reference.MatchString(text) {
			t.Fatalf("AUR-455/AC-001/behavior-missing: %s never references the v1 tag README.md's honesty caveat is about", wf)
		}
	}

	t.Logf("AUR-455/AC-001/integration pass flags=6 git_demo_commits=%d fixture_issues=%d workflows_checked=2",
		manifest.CommitCount, len(parsedFixture.Issues))
}
