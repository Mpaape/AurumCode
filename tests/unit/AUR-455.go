// Package unit holds AUR-455's Unit-layer proof: the 2026-08-14 onboarding
// audit (.board/cards/ready/AUR-455.md, "Achados medidos") found the
// product's main feature invisible in the README, a documented build
// command that fails, a broken relative path in the only executable review
// example, and a naming collision in docs/getting-started.md. This layer
// checks, by static text, that the fix this card ships actually removed
// each false or missing statement and left true, honest replacement text in
// its place.
//
// What this layer does NOT do: it never runs `go build`, `go run` or
// demo.sh itself -- that is tests/e2e/AUR-455.go's job (and
// tests/acceptance/AUR-455.sh's AC-001, which wraps it). This layer only
// reads README.md, docs/getting-started.md and demo.sh as text, exactly
// like tests/unit/AUR-445.go reads its six root documentation files.
//
// Selector naming note: the card's own "TDD proof" section names the
// selectors TestAUR445 / IntegrationAUR445 / E2EAUR445 -- residue from the
// AUR-445 card this one was cloned from as a template. Those symbols
// already exist in tests/unit/AUR-445.go and tests/integration/AUR-445.go,
// in these same packages (`unit`, `integration`); redeclaring them here
// would collide and break `go build ./cmd/... ./internal/... ./pkg/...`
// and every `go test` invocation that touches these packages. This file
// uses TestAUR455 instead, matching this card's own ID and the naming
// pattern every other card in the office follows (see
// tests/unit/AUR-445.go's own identical note about AUR-428). Recorded in
// docs/specs/AUR-455.md.
package unit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// aur455RepoRoot resolves the repository root: the acceptance script
// exports AURUMCODE_ROOT pointing at its staged copy; a developer running
// this test directly gets a walk-up from the working directory to the
// nearest go.mod. Duplicated rather than shared with tests/unit/AUR-445.go
// because these test-support files are not wired into a shared internal
// package, matching every sibling card in this office.
func aur455RepoRoot(t *testing.T) string {
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

// aur455Normalize mirrors tests/unit/AUR-445.go's normalization: strip
// backticks, lowercase, collapse all whitespace (including newlines) to a
// single space, so a required or banned phrase cannot dodge the check by
// being re-wrapped across a markdown line break or re-spelled with
// different backtick placement.
func aur455Normalize(s string) string {
	s = strings.ReplaceAll(s, "`", "")
	s = strings.ToLower(s)
	fields := strings.Fields(s)
	return strings.Join(fields, " ")
}

type aur455Anchor struct {
	label      string
	normalized string
}

// aur455RequiredInReadme names the true statements this card's audit
// demanded README.md carry: the quickstart pointing at demo.sh, the three
// features each with their real command, the corrected build command and
// its reason, the workflows/examples link, and the two honesty
// requirements (the --seguranca subset count and the unpublished v1 tag).
var aur455RequiredInReadme = []aur455Anchor{
	{"quickstart-runs-demo", aur455Normalize("bash demo.sh")},
	{"review-feature-named", aur455Normalize("aurumcode review --base HEAD~1 --seguranca")},
	{"pr-review-feature-named", aur455Normalize("aurumcode review --pr 42 --repo owner/project --publicar --na-linha")},
	{"docs-feature-named", aur455Normalize("go run ./cmd/regenerate-docs")},
	{"corrected-build-command", aur455Normalize("go build ./cmd/... ./internal/... ./pkg/...")},
	{"build-failure-explained", aur455Normalize("package collision")},
	{"workflows-examples-linked", aur455Normalize(".github/workflows/examples/code-review.yml")},
	{"workflows-examples-linked-docs", aur455Normalize(".github/workflows/examples/documentation.yml")},
	{"seguranca-subset-honest", aur455Normalize("security pass applied 3 of 8 security rules")},
	{"v1-tag-not-published", aur455Normalize("is not published yet")},
}

// aur455RequiredInGettingStarted names the fix to docs/getting-started.md's
// naming collision (line 51 in the audited tree): the build output is
// renamed away from `aurumcode`, and the file explains why.
var aur455RequiredInGettingStarted = []aur455Anchor{
	{"renamed-output-binary", aur455Normalize("go build -o regenerate-docs ./cmd/regenerate-docs")},
	{"collision-explained", aur455Normalize("shadow that binary")},
}

// aur455BannedInGettingStarted is the exact broken line the audit measured:
// building the documentation generator under the same name as the real
// review CLI.
var aur455BannedInGettingStarted = []aur455Anchor{
	{"colliding-output-name", aur455Normalize("go build -o aurumcode ./cmd/regenerate-docs")},
}

func aur455ReadNormalized(t *testing.T, root, rel string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatalf("AUR-455/AC-001/behavior-missing: %s unreadable: %v", rel, err)
	}
	if len(raw) == 0 {
		t.Fatalf("AUR-455/AC-001/behavior-missing: %s is empty", rel)
	}
	return aur455Normalize(string(raw))
}

// TestAUR455 is AUR-455's Unit-layer selector.
func TestAUR455(t *testing.T) {
	root := aur455RepoRoot(t)

	readme := aur455ReadNormalized(t, root, "README.md")
	for _, req := range aur455RequiredInReadme {
		if !strings.Contains(readme, req.normalized) {
			t.Fatalf("AUR-455/AC-001/behavior-missing: README.md carries no %q anchor (%s)", req.normalized, req.label)
		}
	}

	gettingStarted := aur455ReadNormalized(t, root, filepath.Join("docs", "getting-started.md"))
	for _, req := range aur455RequiredInGettingStarted {
		if !strings.Contains(gettingStarted, req.normalized) {
			t.Fatalf("AUR-455/AC-001/behavior-missing: docs/getting-started.md carries no %q anchor (%s)", req.normalized, req.label)
		}
	}
	for _, banned := range aur455BannedInGettingStarted {
		if strings.Contains(gettingStarted, banned.normalized) {
			t.Fatalf("AUR-455/AC-001/behavior-missing: docs/getting-started.md still contains the banned %q line (%s)", banned.normalized, banned.label)
		}
	}

	// demo.sh: exists, is a regular executable file, and its text names
	// every one of the four labeled steps the card's "Achados medidos"
	// demanded. This does NOT run the script -- see tests/e2e/AUR-455.sh
	// for execution.
	demoPath := filepath.Join(root, "demo.sh")
	info, err := os.Stat(demoPath)
	if err != nil {
		t.Fatalf("AUR-455/AC-001/behavior-missing: demo.sh absent: %v", err)
	}
	if info.Mode()&0o111 == 0 {
		t.Fatalf("AUR-455/AC-001/behavior-missing: demo.sh is not executable (mode %v)", info.Mode())
	}
	demoRaw, err := os.ReadFile(demoPath)
	if err != nil {
		t.Fatalf("AUR-455/AC-001/behavior-missing: demo.sh unreadable: %v", err)
	}
	demo := string(demoRaw)
	demoRequired := []aur455Anchor{
		{"builds-regenerate-docs", "./cmd/regenerate-docs"},
		{"builds-aurumcode", "./cmd/aurumcode"},
		{"step-a-result-marker", "result=ok"},
		{"step-b-seguranca-flag", "--seguranca"},
		{"step-c-llm-fixture-env", "AURUMCODE_LLM_FIXTURE"},
		{"uses-git-demo-fixture", "tests/fixtures/repos/git-demo/repo.git"},
		{"uses-review-fixture-file", "tests/fixtures/review/known-problem-response.json"},
		{"step-d-jekyll-marker", "Build Jekyll site with:"},
	}
	for _, req := range demoRequired {
		if !strings.Contains(demo, req.normalized) {
			t.Fatalf("AUR-455/AC-001/behavior-missing: demo.sh never mentions %q (%s)", req.normalized, req.label)
		}
	}

	t.Logf("AUR-455/AC-001/unit pass readme_anchors=%d getting_started_anchors=%d demo_anchors=%d",
		len(aur455RequiredInReadme), len(aur455RequiredInGettingStarted)+len(aur455BannedInGettingStarted), len(demoRequired))
}
