package unit

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/analyzer"
	"github.com/Mpaape/AurumCode/internal/prompt"
	"github.com/Mpaape/AurumCode/pkg/types"
)

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

	// A budget deliberately too small to fit every file's ~100-token hunk.
	// The base review prompt (~1305 tokens) plus this diff's worst-case
	// coverage-declaration reservation (~186 tokens for 8 files) already
	// consume most of a 1700-token budget, leaving room for exactly one
	// file's hunk.
	parts := aur467BuildPrompt(t, diff, 1700, 40)

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
	if full > 1700 {
		t.Fatalf("assembled prompt is %d estimated tokens, over the 1700-token budget (blocker 1 regression)", full)
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

	const maxTokens = 1700
	parts := aur467BuildPrompt(t, diff, maxTokens, 40)

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

	// Sized so hunk0 fits and hunk1 does not: base prompt (~1305) plus
	// this single-file diff's coverage reservation (~130) leaves room for
	// exactly one ~30-token hunk out of two.
	parts := aur467BuildPrompt(t, diff, 1470, 20)

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
