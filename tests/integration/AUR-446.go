// AUR-446 integration selector: the anchor checker from
// tests/unit/AUR-446.go proved correct on synthetic fixtures; this layer
// runs the SAME logic against the REAL corrected files on disk, and
// additionally corroborates the underlying facts those corrections state
// against the real sources this card's read_paths materializes
// (cmd/aurumcode, .github/workflows/examples/code-review.yml) -- not just
// that the prose says something, but that the thing it says is grounded.
//
// docs/specs/AUR-428.md is checked like every other corrected spec below.
// It was originally out of scope (the card's paths duplicated
// docs/specs/AUR-446.md instead of listing AUR-428.md); the coordinator
// amended the frontmatter (see docs/specs/AUR-446.md, "Sexto achado"), so it
// is now a declared deliverable of this card like the other five.
//
// Not named "_test.go" on purpose (same technique as every sibling card):
// tests/acceptance/AUR-446.sh stages a private copy and bridges this
// function into `go test`.
package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type specAnchor struct {
	label          string
	mustContain    []string
	mustNotContain []string
}

func specAnchors() []specAnchor {
	return []specAnchor{
		{
			label:       "docs/specs/AUR-424.md",
			mustContain: []string{"O accept selado sai **0**"},
			mustNotContain: []string{
				"Dentro do sandbox ela sai 69",
			},
		},
		{
			label:       "docs/specs/AUR-425.md",
			mustContain: []string{"delivered by AUR-426 (`cmd/aurumcode/docs.go`"},
			mustNotContain: []string{
				"No `aurumcode docs` subcommand exists:",
			},
		},
		{
			label:       "docs/specs/AUR-428.md",
			mustContain: []string{"code-review.yml` **não** está mais nesse grupo: o AUR-440 o repinou para a tag `v1`"},
			mustNotContain: []string{
				"Os demais exemplos em `.github/workflows/examples/` (code-review, qa-testing, all-pipelines) continuam pinados por SHA",
			},
		},
		{
			label:       "docs/specs/AUR-429.md",
			mustContain: []string{"docs` subcommand from AUR-426 (`cmd/aurumcode/docs.go`)"},
			mustNotContain: []string{
				"No `aurumcode docs` subcommand exists:",
			},
		},
		{
			label:       "docs/specs/AUR-437.md",
			mustContain: []string{"internal/git/githubclient/client.go:795-802"},
		},
		{
			label:       "docs/specs/AUR-440.md",
			mustContain: []string{"AUR-438 esta done: `cmd/aurumcode` ja publica"},
			mustNotContain: []string{
				"restaurado pelo AUR-437 e ainda nao foi executado",
			},
		},
	}
}

func normalizeWS(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func checkAnchor(content string, a specAnchor) []string {
	normalized := normalizeWS(content)
	var violations []string
	for _, want := range a.mustContain {
		if !strings.Contains(normalized, normalizeWS(want)) {
			violations = append(violations, "AUR-446/AC-001/behavior-missing:"+a.label+":missing:"+want)
		}
	}
	for _, unwanted := range a.mustNotContain {
		if strings.Contains(normalized, normalizeWS(unwanted)) {
			violations = append(violations, "AUR-446/AC-001/MUT-001:"+a.label+":stale-claim-present:"+unwanted)
		}
	}
	return violations
}

func aur446RepoRoot(t *testing.T) string {
	t.Helper()
	if root := os.Getenv("AURUMCODE_ROOT"); root != "" {
		return root
	}
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("AUR-446/AC-001/infrastructure: getwd: %v", err)
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("AUR-446/AC-001/infrastructure: no go.mod above the working directory and AURUMCODE_ROOT is unset")
		}
		dir = parent
	}
}

// IntegrationAUR446 checks every corrected spec (this card's own declared
// deliverables, so their absence is behavior-missing, never infrastructure)
// and corroborates the underlying facts against real read_paths sources.
func IntegrationAUR446(t *testing.T) {
	root := aur446RepoRoot(t)

	for _, a := range specAnchors() {
		a := a
		t.Run(a.label, func(t *testing.T) {
			full := filepath.Join(root, filepath.FromSlash(a.label))
			raw, err := os.ReadFile(full)
			if err != nil {
				t.Fatalf("AUR-446/AC-001/behavior-missing: %s unreadable: %v", a.label, err)
			}
			if v := checkAnchor(string(raw), a); len(v) != 0 {
				t.Fatalf("%s", strings.Join(v, "\n"))
			}
		})
	}

	// Corroboration: the fact AUR-425.md/AUR-429.md now state (AUR-426
	// delivered an `aurumcode docs` subcommand) is grounded in the real
	// source this card's read_paths materializes, not asserted on faith.
	t.Run("cmd/aurumcode really has a docs subcommand (corroborates AUR-425/AUR-429)", func(t *testing.T) {
		mainRaw, err := os.ReadFile(filepath.Join(root, "cmd", "aurumcode", "main.go"))
		if err != nil {
			t.Fatalf("AUR-446/AC-001/infrastructure: cmd/aurumcode/main.go unreadable: %v", err)
		}
		if !strings.Contains(string(mainRaw), `case "docs":`) {
			t.Fatalf("AUR-446/AC-001/behavior-missing: cmd/aurumcode/main.go has no case \"docs\": " +
				"dispatch; the AUR-425/AUR-429 correction would be unsupported by the real source")
		}
		if _, err := os.Stat(filepath.Join(root, "cmd", "aurumcode", "docs.go")); err != nil {
			t.Fatalf("AUR-446/AC-001/behavior-missing: cmd/aurumcode/docs.go absent: %v", err)
		}
	})

	// Corroboration: the fact AUR-440.md now states (AUR-438 delivered
	// --pr/--repo/--publicar/--na-linha to cmd/aurumcode) is grounded in the
	// real source.
	t.Run("cmd/aurumcode really has --publicar/--pr/--repo (corroborates AUR-440)", func(t *testing.T) {
		var found []string
		for _, f := range []string{"main.go", "pr.go"} {
			raw, err := os.ReadFile(filepath.Join(root, "cmd", "aurumcode", f))
			if err != nil {
				t.Fatalf("AUR-446/AC-001/infrastructure: cmd/aurumcode/%s unreadable: %v", f, err)
			}
			for _, flag := range []string{`"pr"`, `"repo"`, `"publicar"`, `"na-linha"`} {
				if strings.Contains(string(raw), flag) {
					found = append(found, flag)
				}
			}
		}
		for _, want := range []string{`"pr"`, `"repo"`, `"publicar"`, `"na-linha"`} {
			ok := false
			for _, f := range found {
				if f == want {
					ok = true
				}
			}
			if !ok {
				t.Fatalf("AUR-446/AC-001/behavior-missing: flag %s not found registered in cmd/aurumcode; "+
					"the AUR-440 correction would be unsupported by the real source", want)
			}
		}
	})

	// Corroboration: docs/specs/AUR-437.md's residual-case anchor names an
	// exact file:line that must exist and still contain the LastIndex
	// heuristic it describes -- tests/unit/AUR-446.go proves the resulting
	// BEHAVIOUR; this proves the anchor's file:line reference is not stale.
	t.Run("internal/git/githubclient/client.go:795-802 still holds extractFilePath (corroborates AUR-437 anchor)", func(t *testing.T) {
		raw, err := os.ReadFile(filepath.Join(root, "internal", "git", "githubclient", "client.go"))
		if err != nil {
			t.Fatalf("AUR-446/AC-001/infrastructure: internal/git/githubclient/client.go unreadable: %v", err)
		}
		lines := strings.Split(string(raw), "\n")
		if len(lines) < 802 {
			t.Fatalf("AUR-446/AC-001/behavior-missing: client.go has fewer than 802 lines (%d); AUR-437.md's line reference is stale", len(lines))
		}
		window := strings.Join(lines[794:802], "\n") // 1-indexed 795..802
		if !strings.Contains(window, "func extractFilePath") || !strings.Contains(window, "LastIndex") {
			t.Fatalf("AUR-446/AC-001/behavior-missing: client.go:795-802 no longer matches extractFilePath's LastIndex heuristic; AUR-437.md's line reference is stale:\n%s", window)
		}
	})

	// Corroboration: the fact AUR-428.md now states (AUR-440 repinned
	// code-review.yml to the v1 tag) is grounded in the real workflow file
	// this card's read_paths materializes, not asserted on faith.
	t.Run("code-review.yml really uses ref: v1, not a SHA (corroborates AUR-428)", func(t *testing.T) {
		raw, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "examples", "code-review.yml"))
		if err != nil {
			t.Fatalf("AUR-446/AC-001/infrastructure: .github/workflows/examples/code-review.yml unreadable: %v", err)
		}
		if !strings.Contains(string(raw), "ref: v1") {
			t.Fatalf("AUR-446/AC-001/behavior-missing: code-review.yml has no `ref: v1`; " +
				"the AUR-428 correction would be unsupported by the real workflow file")
		}
	})
}
