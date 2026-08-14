// Package unit holds AUR-456's Unit-layer proof: the 2026-08-14 audit
// (.board/cards/ready/AUR-456.md, "Achados medidos") found a closed set of
// files that either had no importer anywhere in the repository or had
// diverged from the version the binary actually embeds. This layer is a
// pure filesystem check -- existence, not content -- over the exact paths
// this card owns:
//
//   - .aurumcode/rules/ (whole directory): dead since the first commit; the
//     live catalog is internal/review/rules/*.yml, embedded via go:embed
//     (internal/review/rules.go) and diverged from the dead copy on
//     2026-08-13.
//   - .aurumcode/prompts/*.md (six files, everything except
//     documentation/welcome-page.md): dead, because internal/prompt embeds
//     its own templates/*.md at build time (internal/prompt/builder.go).
//   - configs/ (whole directory): zero importer in Go or docs.
//   - index.md and _config.yml at the REPOSITORY ROOT (not .aurumcode/):
//     never read by any generator -- cmd/aurumcode/docs.go and
//     cmd/regenerate-docs/main.go both default their output directory to
//     ".aurumcode", never ".".
//   - _api/ (whole directory): documents pipelines
//     (review_pipeline.go/docs_pipeline.go/qa_pipeline.go) that do not
//     exist under internal/pipeline/.
//   - .github/actions/aurumcode-docs/ (whole directory, including
//     README.md): action.yml is a second, incompatible Action definition no
//     workflow references; its sibling README.md exists only to document
//     that action.yml, so keeping the README after removing action.yml
//     would create a NEW false claim (a reusable Action at a path with no
//     action.yml) rather than remove one -- see docs/specs/AUR-456.md.
//   - pages-fix.md, test-jekyll.sh: stray files with no importer.
//
// The one exception this card must prove stays alive:
// .aurumcode/prompts/documentation/welcome-page.md, read as an optional
// override by internal/documentation/welcome/generator.go's
// defaultPromptPath (cross-checked against the source directly in
// tests/integration/AUR-456.go, not re-derived here).
//
// Not named "_test.go" on purpose, mirroring every sibling card in this
// office (AUR-402..AUR-411, AUR-422, AUR-424, AUR-428, AUR-440, AUR-445):
// tests/acceptance/AUR-456.sh stages a private writable copy of the module
// and writes a tiny bridge "_test.go" file that calls TestAUR456, so these
// assertions run inside the sandboxed acceptance instead of being swept into
// an unrelated top-level `go test ./...`.
//
// Selector naming note: the card's own "TDD proof" section names the
// selectors TestAUR445 / IntegrationAUR445 / E2EAUR445 -- residue from the
// AUR-445 template this card was cloned from. Those symbols already exist in
// tests/unit/AUR-445.go and tests/integration/AUR-445.go, in these same
// packages (unit, integration); redeclaring them here would collide and
// break `go build ./...` / `go vet ./...` over this package, exactly the
// problem AUR-445 itself hit and documented against its own AUR-428
// template residue. This file uses TestAUR456 instead, matching this card's
// own ID and the naming pattern every other card in the office follows.
// Recorded in docs/specs/AUR-456.md.
package unit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// aur456RepoRoot resolves the repository root: the acceptance script exports
// AURUMCODE_ROOT pointing at its staged copy; a developer running the bridge
// manually gets a walk-up from the working directory to the nearest go.mod.
func aur456RepoRoot(t *testing.T) string {
	t.Helper()
	if root := os.Getenv("AURUMCODE_ROOT"); root != "" {
		return root
	}
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("AUR-456/AC-001/infrastructure: getwd: %v", err)
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("AUR-456/AC-001/infrastructure: no go.mod above the working directory and AURUMCODE_ROOT is unset")
		}
		dir = parent
	}
}

// aur456DeadPaths is the exact, closed set of paths this card removes. Only
// the .aurumcode/rules/security.yml entry carries a mutation label, because
// MUT-001 (the card's own skeptical mutation) is specifically "reintroduzir
// uma copia morta de regra" -- the other dead paths have no mutation case.
type aur456DeadPath struct {
	rel       string
	mutatable bool
}

var aur456Dead = []aur456DeadPath{
	{rel: filepath.Join(".aurumcode", "rules"), mutatable: true},
	{rel: filepath.Join(".aurumcode", "prompts", "changelog-generation.md")},
	{rel: filepath.Join(".aurumcode", "prompts", "review.md")},
	{rel: filepath.Join(".aurumcode", "prompts", "documentation.md")},
	{rel: filepath.Join(".aurumcode", "prompts", "test.md")},
	{rel: filepath.Join(".aurumcode", "prompts", "summary.md")},
	{rel: filepath.Join(".aurumcode", "prompts", "documentation-generation.md")},
	{rel: "configs"},
	{rel: "index.md"},
	{rel: "_config.yml"},
	{rel: "_api"},
	{rel: filepath.Join(".github", "actions", "aurumcode-docs")},
	{rel: "pages-fix.md"},
	{rel: "test-jekyll.sh"},
}

// aur456CheckAbsent fails Unit-layer AC-001 unless rel is absent from root.
// mutatable paths distinguish this card's own pre-fix RED (dead copy still
// present, no marker) from the deliberate MUT-001 run (dead copy
// reintroduced into a scratch copy, AUR456_MUTATION=MUT-001 exported)
// exactly as tests/unit/AUR-445.go's LICENSE check does: the reintroduced
// bytes are identical to the pre-fix state, so only the run itself, never
// the content, can tell them apart.
func aur456CheckAbsent(t *testing.T, root string, d aur456DeadPath) {
	t.Helper()
	path := filepath.Join(root, d.rel)
	if _, err := os.Lstat(path); err == nil {
		if d.mutatable && os.Getenv("AUR456_MUTATION") == "MUT-001" {
			t.Fatalf("AUR-456/AC-001/MUT-001: %s was reintroduced", d.rel)
		}
		t.Fatalf("AUR-456/AC-001/behavior-missing: dead copy still present: %s", d.rel)
	} else if !os.IsNotExist(err) {
		t.Fatalf("AUR-456/AC-001/infrastructure: stat %s: %v", d.rel, err)
	}
}

// TestAUR456 is AUR-456's Unit-layer selector.
//
// Before the fix (or on the untouched pre-fix tree) this fails with
// AUR-456/AC-001/behavior-missing for the first dead path still present. A
// MUT-001 run that reintroduces .aurumcode/rules/security.yml into a
// mutated scratch copy fails here with the AUR-456/AC-001/MUT-001 label
// instead.
func TestAUR456(t *testing.T) {
	root := aur456RepoRoot(t)

	for _, d := range aur456Dead {
		aur456CheckAbsent(t, root, d)
	}

	// The one live exception must survive and be non-empty: a repository
	// that installs the Action still needs generator.go's optional override
	// path to resolve to real content.
	welcomeOverride := filepath.Join(root, ".aurumcode", "prompts", "documentation", "welcome-page.md")
	raw, err := os.ReadFile(welcomeOverride)
	if err != nil {
		t.Fatalf("AUR-456/AC-001/behavior-missing: live override .aurumcode/prompts/documentation/welcome-page.md unreadable: %v", err)
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		t.Fatalf("AUR-456/AC-001/behavior-missing: live override .aurumcode/prompts/documentation/welcome-page.md is empty")
	}

	// The ENTRYPOINT script this card must not disturb (read_paths carries
	// "scripts", not the Dockerfile itself -- the Dockerfile cross-check
	// lives in docs/specs/AUR-456.md as host-run evidence; see that file's
	// "Fora do container" section for why it cannot run inside
	// bootstrap-readonly-v1).
	entrypoint := filepath.Join(root, "scripts", "action-entrypoint.sh")
	info, err := os.Stat(entrypoint)
	if err != nil {
		t.Fatalf("AUR-456/AC-001/behavior-missing: scripts/action-entrypoint.sh unreadable: %v", err)
	}
	if info.IsDir() {
		t.Fatalf("AUR-456/AC-001/behavior-missing: scripts/action-entrypoint.sh is a directory")
	}

	t.Logf("AUR-456/AC-001/unit pass dead_paths=%d live_kept=2", len(aur456Dead))
}
