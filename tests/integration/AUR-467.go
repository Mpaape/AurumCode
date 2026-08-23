package integration

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/analyzer"
	"github.com/Mpaape/AurumCode/internal/prompt"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// IntegrationAUR467 is this card's integration selector. Where
// tests/unit/AUR-467.go proves the AC-001/002/003 mechanics and
// reconstructs the measured cause on a minimal diff, this program exercises
// the same PromptBuilder boundary against a diff shaped closer to the
// 2026-08-14 measurement's scale and mix -- several prose file types, a
// wider spread of code languages, and a no-prose control diff -- with
// assertions distinct from the unit selector.
func IntegrationAUR467(t *testing.T) {
	t.Run("MixedProseTypesAllExcluded", integrationAUR467MixedProseTypes)
	t.Run("ArithmeticConsistency", integrationAUR467ArithmeticConsistency)
	t.Run("NoProseDiffUnaffected", integrationAUR467NoProseDiffUnaffected)
	t.Run("RuleCatalogStillPresentForCode", integrationAUR467RuleCatalogStillPresent)
}

func integrationAUR467Build(t *testing.T, diff *types.Diff, maxTokens, reserve int) prompt.PromptParts {
	t.Helper()
	metrics := analyzer.NewDiffAnalyzer().AnalyzeDiff(diff)
	parts, err := prompt.NewPromptBuilder().BuildPrompt(diff, metrics, prompt.BuildOptions{
		MaxTokens: maxTokens, SchemaKind: "review", Role: "reviewer", ReserveReply: reserve,
	})
	if err != nil {
		t.Fatalf("BuildPrompt failed: %v", err)
	}
	return parts
}

func integrationAUR467File(path string, lines ...string) types.DiffFile {
	return types.DiffFile{Path: path, Hunks: []types.DiffHunk{{Lines: lines}}}
}

// integrationAUR467MixedProseTypes covers both prose extensions this
// package's classifier recognizes (markdown and restructuredtext), plus
// code across several languages, in one diff -- unlike the unit selector's
// single-markdown-file case.
func integrationAUR467MixedProseTypes(t *testing.T) {
	diff := &types.Diff{Files: []types.DiffFile{
		integrationAUR467File("AGENTS.md", "+PROSE_MARKER_MD_115", "+regra redundante"),
		integrationAUR467File("docs/design.rst", "+PROSE_MARKER_RST_1", "+design notes"),
		integrationAUR467File("cmd/server/main.go", `+func main() { println("PROSE_MARKER_MUST_NOT_APPEAR") }`),
		integrationAUR467File("web/app.mjs", `+export const CODE_MARKER_MJS = 1;`),
		integrationAUR467File("scripts/deploy.py", `+CODE_MARKER_PY = True`),
	}}

	parts := integrationAUR467Build(t, diff, 8000, 1000)

	for _, marker := range []string{"PROSE_MARKER_MD_115", "PROSE_MARKER_RST_1"} {
		if strings.Contains(parts.User, marker) {
			t.Fatalf("prose content (%s) leaked into the reviewed content:\n%s", marker, parts.User)
		}
	}
	for _, path := range []string{"AGENTS.md", "docs/design.rst"} {
		if !strings.Contains(parts.User, path) {
			t.Fatalf("excluded prose file %s is not declared anywhere in the output (silent drop)", path)
		}
	}
	for _, marker := range []string{"CODE_MARKER_MJS", "CODE_MARKER_PY"} {
		if !strings.Contains(parts.User, marker) {
			t.Fatalf("code content (%s) was dropped alongside prose:\n%s", marker, parts.User)
		}
	}
	if got := parts.Meta["prose_files_excluded"]; got != "2" {
		t.Fatalf("Meta[prose_files_excluded] = %q, want \"2\" (AGENTS.md + docs/design.rst)", got)
	}
	if got := parts.Meta["code_files_total"]; got != "3" {
		t.Fatalf("Meta[code_files_total] = %q, want \"3\"", got)
	}
}

// integrationAUR467ArithmeticConsistency checks that Meta's own numbers
// agree with each other (total == reviewed + omitted) across a diff large
// enough that a modest budget must omit some code files -- a property the
// unit selector checks for one fixed shape, asserted here across three
// different budgets on the same diff.
func integrationAUR467ArithmeticConsistency(t *testing.T) {
	var files []types.DiffFile
	files = append(files, integrationAUR467File("README.md", "+prose"))
	for i := 0; i < 12; i++ {
		files = append(files, integrationAUR467File(fmt.Sprintf("pkg/mod%02d.mjs", i),
			"+"+strings.Repeat("y", 300)))
	}
	diff := &types.Diff{Files: files}

	for _, mt := range []int{1500, 2000, 8000} {
		parts := integrationAUR467Build(t, diff, mt, 40)
		total, reviewed, omitted := parts.Meta["code_files_total"], parts.Meta["code_files_reviewed"], parts.Meta["code_files_omitted"]
		var t1, r1, o1 int
		if _, err := fmt.Sscanf(total, "%d", &t1); err != nil {
			t.Fatalf("MaxTokens=%d: code_files_total %q not numeric: %v", mt, total, err)
		}
		if _, err := fmt.Sscanf(reviewed, "%d", &r1); err != nil {
			t.Fatalf("MaxTokens=%d: code_files_reviewed %q not numeric: %v", mt, reviewed, err)
		}
		if _, err := fmt.Sscanf(omitted, "%d", &o1); err != nil {
			t.Fatalf("MaxTokens=%d: code_files_omitted %q not numeric: %v", mt, omitted, err)
		}
		if t1 != 12 {
			t.Fatalf("MaxTokens=%d: code_files_total = %d, want 12 (README.md must not count as code)", mt, t1)
		}
		if r1+o1 != t1 {
			t.Fatalf("MaxTokens=%d: reviewed(%d) + omitted(%d) != total(%d)", mt, r1, o1, t1)
		}
		if o1 > 0 && !strings.Contains(parts.User, fmt.Sprintf("NOT covered by this review (token budget): %d", o1)) {
			t.Fatalf("MaxTokens=%d: declared omitted count does not match Meta (o1=%d):\n%s", mt, o1, parts.User)
		}
	}
}

// integrationAUR467NoProseDiffUnaffected is a regression control: a diff
// with zero prose files must behave exactly as before this card -- every
// code file covered, prose_files_excluded is "0" (stated, not absent).
func integrationAUR467NoProseDiffUnaffected(t *testing.T) {
	diff := &types.Diff{Files: []types.DiffFile{
		integrationAUR467File("a.go", "+CODE_A"),
		integrationAUR467File("b.go", "+CODE_B"),
		integrationAUR467File("c.go", "+CODE_C"),
	}}
	parts := integrationAUR467Build(t, diff, 8000, 1000)

	for _, marker := range []string{"CODE_A", "CODE_B", "CODE_C"} {
		if !strings.Contains(parts.User, marker) {
			t.Fatalf("no-prose diff regressed: %s missing from review", marker)
		}
	}
	if got := parts.Meta["prose_files_excluded"]; got != "0" {
		t.Fatalf("Meta[prose_files_excluded] = %q, want \"0\" (stated, not omitted)", got)
	}
	if got := parts.Meta["code_files_omitted"]; got != "0" {
		t.Fatalf("Meta[code_files_omitted] = %q, want \"0\"", got)
	}
}

// integrationAUR467RuleCatalogStillPresent guards against a fix that
// removes the AUR-461 rule catalog while chasing this card's defect: the
// System half of the prompt must still teach the closed rule_id list for
// code files.
func integrationAUR467RuleCatalogStillPresent(t *testing.T) {
	diff := &types.Diff{Files: []types.DiffFile{
		integrationAUR467File("AGENTS.md", "+prose"),
		integrationAUR467File("x.go", "+code"),
	}}
	parts := integrationAUR467Build(t, diff, 8000, 1000)

	for _, id := range prompt.DefaultRuleCatalog {
		if !strings.Contains(parts.System, id) {
			t.Fatalf("rule catalog id %s missing from the System prompt: AUR-461's catalog must survive this card's change", id)
		}
	}
}
