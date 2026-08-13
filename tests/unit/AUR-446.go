// AUR-446 unit selector: proves the anchor checker this card's acceptance
// relies on is correct, using in-memory fixtures -- not the real files on
// disk (IntegrationAUR446 in tests/integration/AUR-446.go exercises those).
//
// AC-001's "grep de verificacao estatica sobre os specs entregues" is, at
// its core, a substring check: a corrected spec must contain the anchor that
// states the delivered behaviour and must not contain the anchor that
// states the stale, pre-correction claim. specAnchors() below is the exact
// table tests/acceptance/AUR-446.sh and tests/integration/AUR-446.go also
// use (duplicated per file on purpose: each selector is staged alone into
// its own sealed container, per tests/acceptance/EXIT_CODE_CONVENTION.md's
// documented idiom, so a shared sourced library would not resolve there).
//
// Anchors are compared after whitespace normalization (every run of
// whitespace, including newlines, collapsed to one space) so a check is
// immune to where a paragraph happens to wrap -- verified empirically
// against the real pre-fix files in docs/specs/AUR-446.md's "O comando"
// table before this file was written.
//
// This file is a plain (non-_test) source in package unit, per the board's
// selector convention; the acceptance stages it next to a generated bridge
// _test file that calls TestAUR446.
package unit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/git/githubclient"
)

// specAnchor is one card's worth of literal, whitespace-normalized checks
// against one spec file's full content.
type specAnchor struct {
	label          string // e.g. "docs/specs/AUR-425.md"
	mustContain    []string
	mustNotContain []string
}

// specAnchors is the table this card's correction is judged against. Every
// entry mirrors a row of the "O comando" table in docs/specs/AUR-446.md.
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
			label:       "docs/specs/AUR-429.md",
			mustContain: []string{"docs` subcommand from AUR-426 (`cmd/aurumcode/docs.go`)"},
			mustNotContain: []string{
				"No `aurumcode docs` subcommand exists:",
			},
		},
		{
			label:       "docs/specs/AUR-437.md",
			mustContain: []string{"internal/git/githubclient/client.go:795-802"},
			// No mustNotContain here: the original defect-4 description was
			// incomplete, not false, for the case it covers. This card adds
			// disclosure of the residual case instead of retracting a claim.
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

// normalizeWS collapses every run of whitespace (space, tab, newline) to a
// single space so a substring check is immune to where Markdown happens to
// wrap a paragraph.
func normalizeWS(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// checkAnchor evaluates one specAnchor against one file's raw content and
// returns every violation label found (empty slice: the file passes).
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

// TestAUR446 proves the checker itself, both directions:
//   - fed the exact corrected excerpt, it reports zero violations (GREEN);
//   - fed the exact pre-correction excerpt (the card's RED, and the shape
//     MUT-001 reintroduces), it reports exactly the expected violation.
func TestAUR446(t *testing.T) {
	fixtures := map[string]struct {
		corrected string
		stale     string
	}{
		"docs/specs/AUR-424.md": {
			corrected: "A lane E2E executa o comando declarado de verdade. " +
				"O accept selado sai **0**, com a lane de regressao incluida.",
			stale: "A lane E2E, porem, nao aceita mais esse substituto: ela executa o comando\n" +
				"declarado de verdade. Dentro do sandbox ela sai 69\n" +
				"(`AUR-424/AC-001/infrastructure/regenerate_docs_deps_not_materialized`).",
		},
		"docs/specs/AUR-425.md": {
			corrected: "`cmd/aurumcode` later gained a flag-driven `docs` subcommand\n" +
				"delivered by AUR-426 (`cmd/aurumcode/docs.go`, wired at\n" +
				"`cmd/aurumcode/main.go`'s `case \"docs\":`).",
			stale: "how every shipped consumer invokes it (`action.yml`,\n" +
				"`scripts/action-entrypoint.sh`, `tests/e2e/smoke_test.go`). No `aurumcode\n" +
				"docs` subcommand exists: `cmd/aurumcode` publishes only `review` (AUR-430) and\n" +
				"is outside this card's `paths`.",
		},
		"docs/specs/AUR-429.md": {
			corrected: "`cmd/aurumcode` later gained a\n" +
				"`docs` subcommand from AUR-426 (`cmd/aurumcode/docs.go`), but it does not\n" +
				"publish a `verify` action.",
			stale: "`internal/qa/browserproof/docsverify` binary. No `aurumcode docs` subcommand\n" +
				"exists: `cmd/aurumcode` publishes `review` and `--fail-on` (AUR-430/431) and\n" +
				"is outside this card's `paths`.",
		},
		"docs/specs/AUR-440.md": {
			corrected: "restaurado pelo AUR-437. AUR-438 esta done: `cmd/aurumcode` ja publica\n" +
				"`--pr`, `--repo`, `--publicar` e `--na-linha`.",
			stale: "AUR-438, que depende do cliente GitHub restaurado pelo AUR-437 e ainda nao\n" +
				"foi executado.",
		},
	}

	for _, a := range specAnchors() {
		a := a
		if a.label == "docs/specs/AUR-437.md" {
			continue // no negative/positive round-trip fixture: see t.Run below.
		}
		fx, ok := fixtures[a.label]
		if !ok {
			t.Fatalf("AUR-446/AC-001/test-defect: no fixture registered for %s", a.label)
		}

		t.Run(a.label+"/corrected content passes", func(t *testing.T) {
			if v := checkAnchor(fx.corrected, a); len(v) != 0 {
				t.Fatalf("AUR-446/AC-001/behavior-missing\ncorrected fixture unexpectedly failed: %v", v)
			}
		})

		t.Run(a.label+"/stale content fails (RED and MUT-001 shape)", func(t *testing.T) {
			v := checkAnchor(fx.stale, a)
			if len(v) == 0 {
				t.Fatalf("AUR-446/AC-001/behavior-missing\nAUR-446/AC-001/MUT-001\n"+
					"stale fixture for %s passed the checker: the checker cannot catch reintroduction", a.label)
			}
			found := false
			for _, label := range v {
				if strings.HasPrefix(label, "AUR-446/AC-001/MUT-001:"+a.label+":stale-claim-present:") {
					found = true
				}
			}
			if !found {
				t.Fatalf("AUR-446/AC-001/behavior-missing: expected a MUT-001 stale-claim-present violation for %s, got %v", a.label, v)
			}
		})
	}

	t.Run("docs/specs/AUR-437.md/corrected content carries the residual-case anchor", func(t *testing.T) {
		a := specAnchors()[3]
		corrected := "Reproduzido em `internal/git/githubclient/client.go:795-802`: a linha\n" +
			"`diff --git a/foo b/bar.go b/foo b/bar.go` retorna `bar.go` em vez de\n" +
			"`foo b/bar.go`."
		if v := checkAnchor(corrected, a); len(v) != 0 {
			t.Fatalf("AUR-446/AC-001/behavior-missing\ncorrected AUR-437 fixture unexpectedly failed: %v", v)
		}
	})

	// The AUR-437 residual case is not just documented, it is real: prove it
	// through the package's public API rather than trusting the prose. A
	// textual check that "LastIndex" appears in the source would pass under
	// a behaviour change; this drives the actual code path.
	t.Run("AUR-437 residual ` b/`-in-new-path truncation is real code behaviour", func(t *testing.T) {
		diffContent := "diff --git a/foo b/bar.go b/foo b/bar.go\n" +
			"@@ -1,1 +1,1 @@\n" +
			"-old\n" +
			"+new\n"

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(diffContent))
		}))
		defer server.Close()

		client := githubclient.NewClientWithBaseURL("test-token", server.URL)
		diff, err := client.GetPullRequestDiff(context.Background(), "owner", "repo", 42)
		if err != nil {
			t.Fatalf("AUR-446/AC-001/behavior-missing: GetPullRequestDiff error: %v", err)
		}
		if len(diff.Files) != 1 {
			t.Fatalf("AUR-446/AC-001/behavior-missing: expected 1 file, got %d", len(diff.Files))
		}

		got := diff.Files[0].Path
		// The correct new-side path is "foo b/bar.go". The measured residual
		// defect truncates it to "bar.go" because the last " b/" separator
		// in the line falls INSIDE the new-side path itself. If this ever
		// stops reproducing, docs/specs/AUR-437.md's residual-case
		// disclosure has gone stale in the opposite direction (overclaiming
		// a bug that no longer exists) and must be corrected again.
		if got != "bar.go" {
			t.Fatalf("AUR-446/AC-001/behavior-missing: extractFilePath via GetPullRequestDiff "+
				"returned %q; expected the measured residual truncation %q "+
				"(want %q, the fix docs/specs/AUR-437.md now discloses is still open)",
				got, "bar.go", "foo b/bar.go")
		}
	})
}
