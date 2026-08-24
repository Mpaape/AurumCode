package unit

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Mpaape/AurumCode/internal/analyzer"
	"github.com/Mpaape/AurumCode/internal/prompt"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// TestAUR475 is this card's unit selector. It proves, at the public
// boundary of internal/prompt (PromptBuilder.BuildPrompt, the exact
// function internal/review/reviewer.go calls), that a single oversized
// file at the front of the priority order no longer starves every file
// behind it:
//
//  1. the files that DO fit are still reviewed, and the file that doesn't
//     fit is named in the coverage declaration (AC-001);
//  2. no file that appears in the reviewed content is ever partial --
//     it is either whole or entirely absent, never truncated (AC-002);
//  3. a file whose hunks are PARTLY covered classifies "partial" (not
//     "omitted"), and a file whose hunks are entirely skipped classifies
//     "omitted" (not "partial") -- the exact interaction the card calls
//     out: flipping break to continue lets more files become partial,
//     and that must not blur into omitted or vice versa (AC-003).
//
// This file is a plain .go program, not a _test.go file: the acceptance
// script bridges it (see tests/acceptance/AUR-475.sh), matching the
// convention tests/unit/AUR-467.go set.
func TestAUR475(t *testing.T) {
	t.Run("AC001_LaterSmallerFilesStillReviewed", testAUR475LaterSmallerFilesStillReviewed)
	t.Run("AC002_NoFileEverPartialContent", testAUR475NoFileEverPartialContent)
	t.Run("AC003_HunkLevelPartialVsOmittedClassification", testAUR475HunkLevelPartialVsOmitted)
}

func aur475Build(t *testing.T, diff *types.Diff, maxTokens, reserve int) prompt.PromptParts {
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

func aur475CodeFile(path string, lines ...string) types.DiffFile {
	return types.DiffFile{Path: path, Hunks: []types.DiffHunk{{Lines: lines}}}
}

// testAUR475LaterSmallerFilesStillReviewed is AC-001: the measured shape --
// one oversized code file sorting first, several small code files sorting
// after it -- must not lose the small files. "big.go" (leading byte 'b')
// sorts before "small0..4.go" alphabetically among same-priority code
// files, exactly like AGENTS.md sorted before the .mjs files in AUR-467's
// measurement, except this diff is ALL code: prose exclusion (AUR-467)
// cannot save it, only the break-to-continue fix can.
func testAUR475LaterSmallerFilesStillReviewed(t *testing.T) {
	files := []types.DiffFile{aur475CodeFile("src/big.go", "+"+strings.Repeat("z", 20000))}
	const smallCount = 5
	var wantMarkers []string
	for i := 0; i < smallCount; i++ {
		marker := fmt.Sprintf("SMALL_MARKER_%02d", i)
		files = append(files, aur475CodeFile(fmt.Sprintf("src/small%02d.go", i), "+const "+marker+" = true"))
		wantMarkers = append(wantMarkers, marker)
	}
	diff := &types.Diff{Files: files}

	parts := aur475Build(t, diff, 6000, 500)

	for i, marker := range wantMarkers {
		if !strings.Contains(parts.User, marker) {
			t.Fatalf("small file #%d (%s) was starved by the oversized file ahead of it in priority order:\n%s", i, marker, parts.User)
		}
	}
	if !strings.Contains(parts.User, "src/big.go") {
		t.Fatalf("the oversized file that did not fit is not named anywhere in the coverage declaration (silent drop):\n%s", parts.User)
	}
	if !strings.Contains(parts.User, "- Code files NOT reviewed by this review (token budget): 1") {
		t.Fatalf("expected exactly 1 omitted code file (the oversized one), got:\n%s", parts.User)
	}
	if got := parts.Meta["code_files_complete"]; got != "5" {
		t.Fatalf("Meta[code_files_complete] = %q, want \"5\" (all 5 small files, unaffected by the oversized one)", got)
	}
	if got := parts.Meta["code_files_omitted"]; got != "1" {
		t.Fatalf("Meta[code_files_omitted] = %q, want \"1\"", got)
	}
}

// testAUR475NoFileEverPartialContent is AC-002: skipping is honest,
// truncating is not. No segment content in the assembled prompt may ever
// carry the truncation marker budgeting.go's truncateSegment used to
// leave behind, and every marker that does appear must appear whole (not
// cut mid-line), across both a file that overflows the whole budget and a
// file that overflows only what remains after an earlier file.
func testAUR475NoFileEverPartialContent(t *testing.T) {
	full := "+const WHOLE_LINE_MARKER_" + strings.Repeat("A", 200) + "_END = true"
	diff := &types.Diff{Files: []types.DiffFile{
		aur475CodeFile("src/oversized.go", "+"+strings.Repeat("y", 20000)),
		aur475CodeFile("src/fits.go", full),
	}}

	parts := aur475Build(t, diff, 6000, 500)

	if strings.Contains(parts.User, "(truncated)") {
		t.Fatalf("truncation marker found in assembled prompt -- AC-002 forbids partial file content:\n%s", parts.User)
	}
	// Whatever content of src/fits.go appears must be the WHOLE line,
	// including its terminating marker, never a prefix of it.
	if strings.Contains(parts.User, "WHOLE_LINE_MARKER_") && !strings.Contains(parts.User, "_END = true") {
		t.Fatalf("src/fits.go's content reached the prompt cut mid-line: half a hunk produces a finding about code the reviewer never saw whole:\n%s", parts.User)
	}
	if !strings.Contains(parts.User, "_END = true") {
		t.Fatalf("test setup invalid: src/fits.go did not survive at all, nothing to check for partial content")
	}
}

// testAUR475HunkLevelPartialVsOmitted is AC-003 and the card's named
// interaction: flipping break to continue lets a file that used to be cut
// off entirely become "partial" instead, because a later, smaller hunk of
// the SAME file can now be reached. This must classify correctly in both
// directions: the file with one small surviving hunk out of two is
// "partial" (never folded into "omitted"), and a wholly-skipped
// single-hunk file stays "omitted" (never miscounted as "partial").
func testAUR475HunkLevelPartialVsOmitted(t *testing.T) {
	diff := &types.Diff{Files: []types.DiffFile{
		{
			Path: "src/two.go",
			Hunks: []types.DiffHunk{
				{Lines: []string{"+" + strings.Repeat("h", 20000)}}, // hunk0: too big, skipped
				{Lines: []string{"+const HUNK1_SURVIVES = true"}},   // hunk1: small, fits
			},
		},
		aur475CodeFile("src/wholly_skipped.go", "+"+strings.Repeat("w", 20000)), // one huge hunk: omitted
	}}

	parts := aur475Build(t, diff, 6000, 500)

	if !strings.Contains(parts.User, "HUNK1_SURVIVES") {
		t.Fatalf("test setup invalid: src/two.go's second (small) hunk did not survive, nothing to classify as partial:\n%s", parts.User)
	}
	if got := parts.Meta["code_files_partial"]; got != "1" {
		t.Fatalf("Meta[code_files_partial] = %q, want \"1\" (src/two.go: 1/2 hunks)", got)
	}
	if got := parts.Meta["code_files_omitted"]; got != "1" {
		t.Fatalf("Meta[code_files_omitted] = %q, want \"1\" (src/wholly_skipped.go: 0/1 hunks)", got)
	}
	if !strings.Contains(parts.User, "src/two.go (1/2 hunks)") {
		t.Fatalf("partial file not named with its hunk fraction:\n%s", parts.User)
	}
	if !strings.Contains(parts.User, "src/wholly_skipped.go (0/1 hunks)") {
		t.Fatalf("wholly-skipped file not named as omitted with its hunk fraction:\n%s", parts.User)
	}
}
