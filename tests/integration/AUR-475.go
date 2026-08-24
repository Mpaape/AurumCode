package integration

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/analyzer"
	"github.com/Mpaape/AurumCode/internal/prompt"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// IntegrationAUR475 is this card's integration selector. Where
// tests/unit/AUR-475.go proves the AC-001/002/003 mechanics through
// PromptBuilder.BuildPrompt, this program exercises the exported budgeting
// primitives (NewTokenBudget, BuildContextSegments, TrimToFit) directly --
// the same boundary AUR-467's measurement reconstruction used -- with
// assertions distinct from the unit selector: order-independence across
// several interleaved sizes, that an included segment's token count is
// never altered (proof against silent truncation at the primitive level),
// and Meta arithmetic consistency across a spread of budgets.
func IntegrationAUR475(t *testing.T) {
	t.Run("OrderIndependentSkipping", integrationAUR475OrderIndependentSkipping)
	t.Run("IncludedSegmentsNeverAltered", integrationAUR475IncludedSegmentsNeverAltered)
	t.Run("ArithmeticConsistencyAcrossBudgets", integrationAUR475ArithmeticConsistency)
}

func integrationAUR475File(path string, lines ...string) types.DiffFile {
	return types.DiffFile{Path: path, Hunks: []types.DiffHunk{{Lines: lines}}}
}

// integrationAUR475OrderIndependentSkipping proves the fix at the raw
// primitive level, with oversized segments interleaved in THREE different
// positions (front, middle, back) among small ones -- not just the
// front-loaded shape the unit selector and the 2026-08-14 measurement
// both used. Every small segment must survive regardless of where the
// oversized ones land in sort order, and every oversized one must be
// skipped whole (never present with reduced Tokens).
func integrationAUR475OrderIndependentSkipping(t *testing.T) {
	est := prompt.NewHeuristicEstimator()
	detector := analyzer.NewLanguageDetector()

	files := []types.DiffFile{
		integrationAUR475File("src/a_big.go", "+"+strings.Repeat("x", 40000)),   // front
		integrationAUR475File("src/m0_small.go", "+const M0 = true"),
		integrationAUR475File("src/m1_small.go", "+const M1 = true"),
		integrationAUR475File("src/n_big.go", "+"+strings.Repeat("x", 40000)),   // middle
		integrationAUR475File("src/m2_small.go", "+const M2 = true"),
		integrationAUR475File("src/z_big.go", "+"+strings.Repeat("x", 40000)),   // back
	}
	diff := &types.Diff{Files: files}

	budget := prompt.NewTokenBudget(est, 6000, 500)
	segments := budget.BuildContextSegments(diff, detector)
	trimmed := budget.TrimToFit(segments, 0)

	got := make(map[string]int, len(trimmed))
	for _, s := range trimmed {
		got[s.FilePath] = s.Tokens
	}

	for _, small := range []string{"src/m0_small.go", "src/m1_small.go", "src/m2_small.go"} {
		if _, ok := got[small]; !ok {
			t.Fatalf("%s was skipped even though it fits: oversized segments elsewhere in the order must not starve it", small)
		}
	}
	for _, big := range []string{"src/a_big.go", "src/n_big.go", "src/z_big.go"} {
		if _, ok := got[big]; ok {
			t.Fatalf("%s (oversized) survived TrimToFit -- it should have been skipped whole, not admitted", big)
		}
	}
}

// integrationAUR475IncludedSegmentsNeverAltered is AC-002 checked at the
// primitive level: every segment TrimToFit returns must have EXACTLY the
// Tokens (and Content) it was given -- proof that inclusion never goes
// through truncateSegment, which the old code called on the very first
// non-fitting segment.
func integrationAUR475IncludedSegmentsNeverAltered(t *testing.T) {
	est := prompt.NewHeuristicEstimator()
	detector := analyzer.NewLanguageDetector()

	diff := &types.Diff{Files: []types.DiffFile{
		integrationAUR475File("src/big.go", "+"+strings.Repeat("x", 20000)),
		integrationAUR475File("src/small.go", "+const SMALL_MARKER = true"),
	}}

	budget := prompt.NewTokenBudget(est, 6000, 500)
	original := budget.BuildContextSegments(diff, detector)
	originalTokens := make(map[string]int, len(original))
	originalContent := make(map[string]string, len(original))
	for _, s := range original {
		originalTokens[s.FilePath] = s.Tokens
		originalContent[s.FilePath] = s.Content
	}

	trimmed := budget.TrimToFit(original, 0)
	if len(trimmed) == 0 {
		t.Fatalf("test setup invalid: nothing survived TrimToFit")
	}
	for _, s := range trimmed {
		if s.Tokens != originalTokens[s.FilePath] {
			t.Fatalf("%s: Tokens changed from %d to %d -- an included segment must never be altered from what BuildContextSegments produced", s.FilePath, originalTokens[s.FilePath], s.Tokens)
		}
		if s.Content != originalContent[s.FilePath] {
			t.Fatalf("%s: Content changed -- an included segment must never be truncated", s.FilePath)
		}
	}
}

// integrationAUR475ArithmeticConsistency checks, across a spread of
// budgets on the same diff, that Meta's own numbers agree (total ==
// complete + partial + omitted) and that the declared omitted count in
// the assembled prompt matches Meta -- the same property AUR-467's
// integration selector checks for prose exclusion, asserted here for the
// break-to-continue fix instead.
func integrationAUR475ArithmeticConsistency(t *testing.T) {
	var files []types.DiffFile
	files = append(files, integrationAUR475File("src/aaa_big.go", "+"+strings.Repeat("x", 6000)))
	for i := 0; i < 10; i++ {
		files = append(files, integrationAUR475File(fmt.Sprintf("src/mod%02d.go", i), "+const V = true"))
	}
	diff := &types.Diff{Files: files}

	for _, mt := range []int{1800, 3000, 6000} {
		metrics := analyzer.NewDiffAnalyzer().AnalyzeDiff(diff)
		parts, err := prompt.NewPromptBuilder().BuildPrompt(diff, metrics, prompt.BuildOptions{
			MaxTokens: mt, SchemaKind: "review", Role: "reviewer", ReserveReply: 40,
		})
		if err != nil {
			t.Fatalf("MaxTokens=%d: BuildPrompt failed: %v", mt, err)
		}
		var total, complete, partial, omitted int
		if _, err := fmt.Sscanf(parts.Meta["code_files_total"], "%d", &total); err != nil {
			t.Fatalf("MaxTokens=%d: code_files_total not numeric: %v", mt, err)
		}
		if _, err := fmt.Sscanf(parts.Meta["code_files_complete"], "%d", &complete); err != nil {
			t.Fatalf("MaxTokens=%d: code_files_complete not numeric: %v", mt, err)
		}
		if _, err := fmt.Sscanf(parts.Meta["code_files_partial"], "%d", &partial); err != nil {
			t.Fatalf("MaxTokens=%d: code_files_partial not numeric: %v", mt, err)
		}
		if _, err := fmt.Sscanf(parts.Meta["code_files_omitted"], "%d", &omitted); err != nil {
			t.Fatalf("MaxTokens=%d: code_files_omitted not numeric: %v", mt, err)
		}
		if total != 11 {
			t.Fatalf("MaxTokens=%d: code_files_total = %d, want 11", mt, total)
		}
		if complete+partial+omitted != total {
			t.Fatalf("MaxTokens=%d: complete(%d)+partial(%d)+omitted(%d) != total(%d)", mt, complete, partial, omitted, total)
		}
		if omitted > 0 && !strings.Contains(parts.User, fmt.Sprintf("NOT reviewed by this review (token budget): %d", omitted)) {
			t.Fatalf("MaxTokens=%d: declared omitted count does not match Meta (omitted=%d):\n%s", mt, omitted, parts.User)
		}
		if strings.Contains(parts.User, "(truncated)") {
			t.Fatalf("MaxTokens=%d: truncation marker present -- AC-002 violated", mt)
		}
	}
}
