package unit

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/analyzer"
	"github.com/Mpaape/AurumCode/internal/prompt"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// aur467TightBudget finds, empirically, the smallest MaxTokens at which
// builder admits at least one of diff's code hunks while still omitting at
// least one -- the exact "tight but not empty" shape AC-003 exercises. It
// starts just above the builder's own measured fixed overhead
// (prompt.PromptBuilder.FixedOverheadTokens, AUR-539) and grows in small
// steps, so the budget always tracks the CURRENT fixed prompt content
// (instructions, rule catalog, schema) instead of a literal that rots when
// that content grows, as AUR-467's original hardcoded 1700 did. ceiling
// bounds the search so a product defect that never admits anything fails
// the test instead of hanging it.
func aur467TightBudget(t *testing.T, builder *prompt.PromptBuilder, diff *types.Diff, metrics *analyzer.DiffMetrics, reserveReply, step, ceiling int) (prompt.PromptParts, int) {
	t.Helper()
	opts0 := prompt.BuildOptions{SchemaKind: "review", Role: "reviewer"}
	fixedOverhead, err := builder.FixedOverheadTokens(diff, metrics, opts0)
	if err != nil {
		t.Fatalf("FixedOverheadTokens failed: %v", err)
	}
	return aur467SearchBudget(t, builder, diff, metrics, fixedOverhead, reserveReply, step, ceiling, func(parts prompt.PromptParts) bool {
		complete, _ := strconv.Atoi(parts.Meta["code_files_complete"])
		partial, _ := strconv.Atoi(parts.Meta["code_files_partial"])
		omitted, _ := strconv.Atoi(parts.Meta["code_files_omitted"])
		return complete+partial >= 1 && omitted >= 1
	})
}

// aur467SearchBudget is the shared search aur467TightBudget and the
// partial-hunk fixture use: grow MaxTokens from fixedOverhead in steps of
// step, up to fixedOverhead+reserveReply+ceiling, and return the first
// assembled prompt for which want reports true. Deriving the budget this
// way, instead of a literal, keeps the fixture valid as the measured fixed
// prompt content (and therefore fixedOverhead) changes.
func aur467SearchBudget(t *testing.T, builder *prompt.PromptBuilder, diff *types.Diff, metrics *analyzer.DiffMetrics, fixedOverhead, reserveReply, step, ceiling int, want func(prompt.PromptParts) bool) (prompt.PromptParts, int) {
	t.Helper()
	for extra := 0; extra <= ceiling; extra += step {
		maxTokens := fixedOverhead + reserveReply + extra
		parts, err := builder.BuildPrompt(diff, metrics, prompt.BuildOptions{
			MaxTokens: maxTokens, SchemaKind: "review", Role: "reviewer", ReserveReply: reserveReply,
		})
		if err != nil {
			continue // still refusing: not enough room yet
		}
		if want(parts) {
			return parts, maxTokens
		}
	}
	t.Fatalf("could not find a budget (fixedOverhead=%d) satisfying the fixture's predicate within the search ceiling", fixedOverhead)
	return prompt.PromptParts{}, 0
}

// TestAUR467 is this card's unit selector. It proves, at the public
// boundary of internal/prompt, that a review prompt built from a diff
// mixing code and prose files:
//
//  1. never carries a documentation file's hunk content into the section
//     the model is told to apply the code rule catalog to (AC-001);
//  2. still carries every code file's hunk content when the budget has
//     room, exactly as before this card (AC-002);
//  3. declares, in both the assembled prompt and PromptParts.Meta, how
//     many code files a tight budget left out, by name (AC-003).
//
// A fourth subtest is this card's required measurement: it reconstructs,
// against the exported budgeting primitives (NewTokenBudget,
// BuildContextSegments, TrimToFit), why the 2026-08-14 gateway measurement
// found all seven findings on AGENTS.md and none of the fifteen `.mjs`
// files got a single comment. The original user diff was never captured
// -- this is a reconstruction shaped like the measured commit (one
// uppercase-named prose file plus many lowercase code files), not a
// replay of it.
//
// This file is a plain .go program, not a _test.go file: the acceptance
// script bridges it (see tests/acceptance/AUR-467.sh), matching the
// convention tests/unit/AUR-461.go set.
func TestAUR467(t *testing.T) {
	t.Run("AC001_NoCodeRuleContentOnProseFiles", testAUR467NoProseContentInReview)
	t.Run("AC002_CodeFilesStillCovered", testAUR467CodeFilesStillCovered)
	t.Run("AC003_PartialCoverageDeclared", testAUR467PartialCoverageDeclared)
	t.Run("MeasuredCauseOfMJSOmission", testAUR467MeasuredOrderingStarvation)
	t.Run("Blocker1_CoverageDeclarationNeverOverflowsBudget", testAUR467CoverageDeclarationNeverOverflowsBudget)
	t.Run("Blocker2_PartialHunkNeverSilentlyComplete", testAUR467PartialHunkNeverSilentlyComplete)
}

func aur467BuildPrompt(t *testing.T, diff *types.Diff, maxTokens, reserve int) prompt.PromptParts {
	t.Helper()
	metrics := analyzer.NewDiffAnalyzer().AnalyzeDiff(diff)
	parts, err := prompt.NewPromptBuilder().BuildPrompt(diff, metrics, prompt.BuildOptions{
		MaxTokens:    maxTokens,
		SchemaKind:   "review",
		Role:         "reviewer",
		ReserveReply: reserve,
	})
	if err != nil {
		t.Fatalf("BuildPrompt failed: %v", err)
	}
	return parts
}

// mustFixedOverhead is aur467TightBudget's single-use sibling for fixtures
// that need the measured fixed overhead directly, to drive their own
// search predicate rather than aur467TightBudget's "some reviewed, some
// omitted" one.
func mustFixedOverhead(t *testing.T, diff *types.Diff, metrics *analyzer.DiffMetrics) int {
	t.Helper()
	fixedOverhead, err := prompt.NewPromptBuilder().FixedOverheadTokens(diff, metrics, prompt.BuildOptions{SchemaKind: "review", Role: "reviewer"})
	if err != nil {
		t.Fatalf("FixedOverheadTokens failed: %v", err)
	}
	return fixedOverhead
}

func aur467CodeFile(path string, lines ...string) types.DiffFile {
	return types.DiffFile{
		Path:  path,
		Hunks: []types.DiffHunk{{Lines: lines}},
	}
}

// testAUR467NoProseContentInReview is AC-001: the 2026-08-14 measurement's
// exact shape -- one markdown file (AGENTS.md) alongside code files -- must
// not leak the prose hunk into the reviewed content, and must declare the
// exclusion rather than go silent about it (Non-goals #2 and #3).
func testAUR467NoProseContentInReview(t *testing.T) {
	const marker = "AGENTS_MD_PROSE_MARKER_LINE_58"
	diff := &types.Diff{Files: []types.DiffFile{
		aur467CodeFile("AGENTS.md",
			"+## O que NAO fazer",
			"+"+marker,
			"+esta secao descreve invariantes em portugues",
		),
		aur467CodeFile("src/app.mjs",
			`+export function run(x) { return x + 1; }`,
		),
	}}

	parts := aur467BuildPrompt(t, diff, 8000, 1000)

	if strings.Contains(parts.User, marker) {
		t.Fatalf("prose content from AGENTS.md reached the code-review section:\n%s", parts.User)
	}
	if !strings.Contains(parts.User, "AGENTS.md") {
		t.Fatalf("AGENTS.md's exclusion was not declared anywhere in the assembled prompt (silent drop):\n%s", parts.User)
	}
	if !strings.Contains(parts.User, "src/app.mjs") {
		t.Fatalf("the code file was dropped along with the prose file:\n%s", parts.User)
	}
	if got := parts.Meta["prose_files_excluded"]; got != "1" {
		t.Fatalf("Meta[prose_files_excluded] = %q, want \"1\"", got)
	}
}

// testAUR467CodeFilesStillCovered is AC-002: "um candidato que zera o
// AC-001 revisando menos codigo e rejeitado." At a budget with plenty of
// room, every code file's hunk content must still reach the assembled
// prompt, unchanged from before this card, alongside one prose file that
// must not.
func testAUR467CodeFilesStillCovered(t *testing.T) {
	files := []types.DiffFile{aur467CodeFile("AGENTS.md", "+prose that must not gate code coverage")}
	const codeFileCount = 5
	var wantMarkers []string
	for i := 0; i < codeFileCount; i++ {
		marker := fmt.Sprintf("CODE_MARKER_%02d", i)
		files = append(files, aur467CodeFile(fmt.Sprintf("src/mod%02d.mjs", i),
			fmt.Sprintf("+export const v%02d = %q;", i, marker),
		))
		wantMarkers = append(wantMarkers, marker)
	}
	diff := &types.Diff{Files: files}

	parts := aur467BuildPrompt(t, diff, 8000, 1000)

	for i, marker := range wantMarkers {
		if !strings.Contains(parts.User, marker) {
			t.Fatalf("code file src/mod%02d.mjs's content is missing from the review at a budget with room:\n%s", i, parts.User)
		}
	}
	if got := parts.Meta["code_files_total"]; got != fmt.Sprintf("%d", codeFileCount) {
		t.Fatalf("Meta[code_files_total] = %q, want %d", got, codeFileCount)
	}
	if got := parts.Meta["code_files_complete"]; got != fmt.Sprintf("%d", codeFileCount) {
		t.Fatalf("Meta[code_files_complete] = %q, want %d (AC-002: coverage must not drop)", got, codeFileCount)
	}
	if got := parts.Meta["code_files_partial"]; got != "0" {
		t.Fatalf("Meta[code_files_partial] = %q, want \"0\" at a budget with room", got)
	}
	if got := parts.Meta["code_files_omitted"]; got != "0" {
		t.Fatalf("Meta[code_files_omitted] = %q, want \"0\" at a budget with room", got)
	}
}

// testAUR467PartialCoverageDeclared is AC-003: when the budget cannot fit
// every code file, the assembled prompt and Meta must say how many were
// left out. Silence on this is the mutation MUT-002 targets.
func testAUR467PartialCoverageDeclared(t *testing.T) {
	files := []types.DiffFile{}
	const codeFileCount = 8
	bigLine := "+" + strings.Repeat("x", 400) // ~100 tokens/hunk at 4 chars/token
	for i := 0; i < codeFileCount; i++ {
		files = append(files, aur467CodeFile(fmt.Sprintf("src/big%02d.mjs", i), bigLine))
	}
	diff := &types.Diff{Files: files}
	metrics := analyzer.NewDiffAnalyzer().AnalyzeDiff(diff)

	// A budget deliberately too small to fit every file's ~100-token hunk,
	// derived from the builder's own measured fixed overhead (AUR-539)
	// rather than a literal that rots as the fixed prompt content grows:
	// start just above the fixed cost and grow until some files are
	// reviewed and some are omitted.
	parts, maxTokens := aur467TightBudget(t, prompt.NewPromptBuilder(), diff, metrics, 40, 20, 4000)

	total := parts.Meta["code_files_total"]
	complete := parts.Meta["code_files_complete"]
	partial := parts.Meta["code_files_partial"]
	omitted := parts.Meta["code_files_omitted"]
	if total != fmt.Sprintf("%d", codeFileCount) {
		t.Fatalf("Meta[code_files_total] = %q, want %d", total, codeFileCount)
	}
	if omitted == "0" {
		t.Fatalf("test setup did not actually exceed the budget: code_files_omitted = 0 (complete=%s partial=%s)", complete, partial)
	}
	if !strings.Contains(parts.User, "Code files NOT reviewed by this review (token budget)") {
		t.Fatalf("assembled prompt does not declare partial coverage at all:\n%s", parts.User)
	}
	if !strings.Contains(parts.User, "- Code files NOT reviewed by this review (token budget): "+omitted) {
		t.Fatalf("declared omitted count in the prompt does not match Meta[code_files_omitted]=%s:\n%s", omitted, parts.User)
	}

	// The full assembled prompt -- including the coverage declaration
	// itself -- must never exceed MaxTokens, and Meta[estimated_tokens]
	// must equal what was actually estimated for it (blocker 1: the
	// declaration used to be appended after the budget was already
	// closed, and Meta undercounted it).
	full := prompt.NewHeuristicEstimator().Estimate(parts.System + parts.User)
	if full > maxTokens {
		t.Fatalf("assembled prompt is %d estimated tokens, over the %d-token budget (blocker 1 regression)", full, maxTokens)
	}
	if got := parts.Meta["estimated_tokens"]; got != fmt.Sprintf("%d", full) {
		t.Fatalf("Meta[estimated_tokens] = %q, want %d (must count the full assembled prompt, not a partial sum)", got, full)
	}
}

// testAUR467CoverageDeclarationNeverOverflowsBudget is the regression for
// adversarial-review blocker 1: the declaration renderCoverageDeclaration
// appends is itself prompt content, and its size grows with the number of
// omitted files -- exactly the case where the budget was already
// tightest. Before the fix, an 8-omitted-file diff assembled to an
// estimated size over MaxTokens while Meta[estimated_tokens] silently
// undercounted it by omitting the declaration from the sum.
func testAUR467CoverageDeclarationNeverOverflowsBudget(t *testing.T) {
	files := []types.DiffFile{aur467CodeFile("README.md", "+prose")}
	for i := 0; i < 8; i++ {
		files = append(files, aur467CodeFile(fmt.Sprintf("src/big%02d.mjs", i), "+"+strings.Repeat("x", 400)))
	}
	diff := &types.Diff{Files: files}
	metrics := analyzer.NewDiffAnalyzer().AnalyzeDiff(diff)

	// Derived (AUR-539), not a literal: a budget tight enough to omit at
	// least one of the 8 big files, which is exactly when the coverage
	// declaration's own size (it grows with the omitted count) is most
	// likely to push the assembled prompt over MaxTokens if blocker 1 ever
	// regresses.
	parts, maxTokens := aur467TightBudget(t, prompt.NewPromptBuilder(), diff, metrics, 40, 20, 4000)

	full := prompt.NewHeuristicEstimator().Estimate(parts.System + parts.User)
	if full > maxTokens {
		t.Fatalf("assembled prompt is %d estimated tokens, over the %d-token budget", full, maxTokens)
	}
	if got := parts.Meta["estimated_tokens"]; got != fmt.Sprintf("%d", full) {
		t.Fatalf("Meta[estimated_tokens] = %q, want %d (the actual assembled prompt size)", got, full)
	}
}

// testAUR467PartialHunkNeverSilentlyComplete is the regression for
// adversarial-review blocker 2: a code file whose hunks only PARTLY
// survive TrimToFit's budget cut must be classified "partial", carrying
// its hunk fraction, never folded into complete or omitted.
func testAUR467PartialHunkNeverSilentlyComplete(t *testing.T) {
	diff := &types.Diff{Files: []types.DiffFile{{
		Path: "src/two.mjs",
		Hunks: []types.DiffHunk{
			{Lines: []string{"+hunk0 " + strings.Repeat("a", 100)}},
			{Lines: []string{"+hunk1 " + strings.Repeat("b", 100)}},
		},
	}}}

	// AUR-539: derived from the builder's measured fixed overhead instead
	// of the literal MaxTokens=1510 AUR-475 hardcoded, which the fixed
	// prompt content later outgrew. Search from fixedOverhead upward (fine
	// 4-token steps, matching the estimator's ~4-chars-per-token
	// granularity) for the first budget where hunk0 survives and hunk1
	// does not -- the exact partial-file shape this test exercises,
	// wherever the boundary between one hunk and two now falls.
	metrics := analyzer.NewDiffAnalyzer().AnalyzeDiff(diff)
	parts, _ := aur467SearchBudget(t, prompt.NewPromptBuilder(), diff, metrics,
		mustFixedOverhead(t, diff, metrics), 20, 4, 400,
		func(p prompt.PromptParts) bool {
			return strings.Contains(p.User, "hunk0") && !strings.Contains(p.User, "hunk1")
		})

	if !strings.Contains(parts.User, "hunk0") {
		t.Fatalf("test setup invalid: hunk0 did not survive the budget at all:\n%s", parts.User)
	}
	if strings.Contains(parts.User, "hunk1") {
		t.Fatalf("test setup invalid: both hunks survived; nothing to classify as partial")
	}
	if got := parts.Meta["code_files_omitted"]; got != "0" {
		t.Fatalf("Meta[code_files_omitted] = %q, want \"0\": the file is not fully omitted, it has one hunk present", got)
	}
	if got := parts.Meta["code_files_complete"]; got != "0" {
		t.Fatalf("Meta[code_files_complete] = %q, want \"0\": a file missing a hunk is not complete (blocker 2 regression)", got)
	}
	if got := parts.Meta["code_files_partial"]; got != "1" {
		t.Fatalf("Meta[code_files_partial] = %q, want \"1\"", got)
	}
	if !strings.Contains(parts.User, "src/two.mjs (1/2 hunks)") {
		t.Fatalf("assembled prompt does not name the partial file with its hunk fraction:\n%s", parts.User)
	}
}

// testAUR467MeasuredOrderingStarvation is this card's required measurement
// (Outcome, third defect): why the fifteen `.mjs` files received zero
// comments. It runs the exported budgeting primitives directly -- the same
// path BuildPrompt used before this card's fix, on the UNFILTERED segment
// list -- to reconstruct two independent mechanisms:
//
//  1. Ordering: determineFilePriority (budgeting.go) gives a documentation
//     file the same PriorityHigh tier as code, so ties break on SortKey,
//     which is the file path. "AGENTS.md" (leading byte 0x41) sorts before
//     any lowercase path, so its hunk is offered to TrimToFit first.
//  2. Starvation: TrimToFit stops (`break`) at the first segment that no
//     longer fits rather than skipping it and trying the next
//     (smaller) one, so once AGENTS.md's hunk fills the budget, every
//     later segment -- all fifteen reconstructed `.mjs` files -- is
//     dropped, not just truncated.
//
// It also proves this card's fix removes the reconstructed symptom: the
// SAME diff through the real BuildPrompt (which excludes prose before
// TrimToFit ever runs) now carries every `.mjs` file.
func testAUR467MeasuredOrderingStarvation(t *testing.T) {
	detector := analyzer.NewLanguageDetector()
	est := prompt.NewHeuristicEstimator()

	// A prose hunk sized to consume most, but not all, of a small budget
	// on its own -- exactly what a verbose AGENTS.md section would do.
	proseLine := "+" + strings.Repeat("p", 800) // ~200 tokens
	files := []types.DiffFile{aur467CodeFile("AGENTS.md", proseLine)}
	const mjsCount = 15
	for i := 0; i < mjsCount; i++ {
		files = append(files, aur467CodeFile(fmt.Sprintf("src/mod%02d.mjs", i), "+export const ok = true;"))
	}
	diff := &types.Diff{Files: files}

	// Mechanism 1 + 2, reconstructed against the raw primitives with NO
	// prose exclusion -- this is what BuildContextSegments/TrimToFit did
	// to this shape of diff before this card.
	budget := prompt.NewTokenBudget(est, 260, 40)
	rawSegments := budget.BuildContextSegments(diff, detector)
	rawTrimmed := budget.TrimToFit(rawSegments, 0)

	sawAgents, sawAnyMJS := false, false
	for _, seg := range rawTrimmed {
		if seg.FilePath == "AGENTS.md" {
			sawAgents = true
		}
		if strings.HasSuffix(seg.FilePath, ".mjs") {
			sawAnyMJS = true
		}
	}
	if !sawAgents {
		t.Fatalf("reconstruction invalid: AGENTS.md itself did not survive TrimToFit, so it cannot be the thing starving the .mjs files")
	}
	if sawAnyMJS {
		t.Fatalf("reconstruction did not reproduce the measured symptom: a .mjs segment survived alongside AGENTS.md under the unfiltered pre-fix path")
	}

	// The fix: through the real BuildPrompt, prose never enters this pool,
	// so the same diff now carries every .mjs file.
	parts := aur467BuildPrompt(t, diff, 8000, 1000)
	for i := 0; i < mjsCount; i++ {
		path := fmt.Sprintf("src/mod%02d.mjs", i)
		if !strings.Contains(parts.User, path) {
			t.Fatalf("post-fix BuildPrompt still omits %s: the measured cause is not actually fixed", path)
		}
	}
}
