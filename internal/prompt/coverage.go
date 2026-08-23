package prompt

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Mpaape/AurumCode/internal/analyzer"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// AUR-467 AC-003: "quando a review nao cobre todos os arquivos de codigo
// do diff ... a saida diz quantos ficaram de fora." This package owns the
// assembled prompt (PromptParts.System/User/Meta) -- it does not own
// cmd/aurumcode's stdout, so the declaration below is what the reviewer
// (and the LLM) reads inside the assembled prompt, plus the same counts
// in PromptParts.Meta for a caller (or a test) to assert on directly.
// Surfacing this on the CLI's own stdout, alongside the finding list,
// needs a follow-up card that owns cmd/aurumcode (AUR-476); this card
// does not.
//
// Adversarial review (post-ff64e18) found two measured defects in the
// first cut of this file, both fixed here:
//
//  1. The declaration text was appended to userContent AFTER TrimToFit
//     had already closed the token budget, and Meta["estimated_tokens"]
//     summed baseTokens + the trimmed segments only -- never the
//     declaration itself. Measured: an 8-omitted-file diff at
//     MaxTokens=1500 assembled to ~1546 estimated tokens (3% over) while
//     Meta reported 1419, an ~127-token undercount. The defect composes
//     with severity: the MORE files a tight budget omits, the LONGER the
//     declaration naming them, so the worst overshoot lands exactly when
//     the budget was already tightest. Fixed by reserving the
//     declaration's WORST-CASE size (maxCoverageDeclarationTokens, below)
//     out of the content budget BEFORE TrimToFit runs, and by computing
//     Meta["estimated_tokens"] from the actual final System+User text
//     builder.go assembles, not a partial sum.
//
//  2. coveredFiles (removed) marked a file "covered" if ANY hunk of it
//     survived TrimToFit -- so a 2-hunk file whose second hunk was
//     dropped by TrimToFit's break read as code_files_omitted=0 and
//     nothing was ever declared. That is silence on partial coverage,
//     which this card's own Outcome forbids. Coverage is now tracked per
//     HUNK (coveredHunkCounts) and every code file is classified into one
//     of three states -- complete (every hunk present), partial (some
//     hunks present, some cut), omitted (zero hunks present) -- and
//     partial files are named in the declaration with their hunk
//     fraction, never folded silently into "reviewed".

// splitFilesByProse returns the distinct file paths of diff, partitioned
// by classifyFile (filetype.go) into code and prose (documentation), each
// sorted for deterministic output.
func splitFilesByProse(diff *types.Diff, detector *analyzer.LanguageDetector) (codePaths, prosePaths []string) {
	seen := make(map[string]bool, len(diff.Files))
	for _, f := range diff.Files {
		if seen[f.Path] {
			continue
		}
		seen[f.Path] = true
		if _, isProse := classifyFile(f.Path, detector); isProse {
			prosePaths = append(prosePaths, f.Path)
		} else {
			codePaths = append(codePaths, f.Path)
		}
	}
	sort.Strings(codePaths)
	sort.Strings(prosePaths)
	return codePaths, prosePaths
}

// hunkTotals returns, for every file path in diff, how many hunks it has
// in the diff (before any budget trimming).
func hunkTotals(diff *types.Diff) map[string]int {
	totals := make(map[string]int, len(diff.Files))
	for _, f := range diff.Files {
		totals[f.Path] += len(f.Hunks)
	}
	return totals
}

// coveredHunkCounts returns, per FilePath, how many of its hunks are
// present in segments -- i.e. survived TrimToFit's budget cut. Hunk
// granular, not file granular: see the blocker-2 note above.
func coveredHunkCounts(segments []ContextSegment) map[string]int {
	counts := make(map[string]int, len(segments))
	for _, s := range segments {
		if s.FilePath != "" {
			counts[s.FilePath]++
		}
	}
	return counts
}

// fileCoverage is one code file's hunk coverage.
type fileCoverage struct {
	path    string
	covered int
	total   int
}

// state classifies a file into exactly one of three states. omitted takes
// priority over partial when total is 0 (defensive; every real diff file
// has at least one hunk).
func (c fileCoverage) state() string {
	switch {
	case c.covered <= 0:
		return "omitted"
	case c.covered < c.total:
		return "partial"
	default:
		return "complete"
	}
}

// classifyCodeCoverage builds one fileCoverage per code path from its
// total vs. covered hunk counts.
func classifyCodeCoverage(codePaths []string, totals, covered map[string]int) []fileCoverage {
	out := make([]fileCoverage, 0, len(codePaths))
	for _, p := range codePaths {
		out = append(out, fileCoverage{path: p, covered: covered[p], total: totals[p]})
	}
	return out
}

// renderCoverageDeclaration is the single call site that states review
// coverage in three explicit states -- complete, partial (named, with its
// hunk fraction: this is the line AC-003 requires and Non-goal #3 forbids
// going silent on), and omitted -- plus which documentation files were
// excluded from the code rule catalog and why. Emitted unconditionally,
// even when every count is zero, so MUT-002 has one unique anchor and
// AC-003 cannot be satisfied by an empty-string special case.
func renderCoverageDeclaration(coverages []fileCoverage, prosePaths []string) string {
	var complete, partial, omitted []fileCoverage
	for _, c := range coverages {
		switch c.state() {
		case "complete":
			complete = append(complete, c)
		case "partial":
			partial = append(partial, c)
		default:
			omitted = append(omitted, c)
		}
	}

	var sb strings.Builder
	sb.WriteString("## Review Coverage\n")
	sb.WriteString(fmt.Sprintf("- Code files in this diff: %d\n", len(coverages)))
	sb.WriteString(fmt.Sprintf("- Code files fully reviewed (every hunk included): %d\n", len(complete)))
	sb.WriteString(fmt.Sprintf("- Code files PARTIALLY reviewed (some hunks omitted by the token budget -- findings may miss the omitted hunks): %d\n", len(partial)))
	for _, c := range partial {
		sb.WriteString(fmt.Sprintf("  - %s (%d/%d hunks)\n", c.path, c.covered, c.total))
	}
	sb.WriteString(fmt.Sprintf("- Code files NOT reviewed by this review (token budget): %d\n", len(omitted)))
	for _, c := range omitted {
		sb.WriteString(fmt.Sprintf("  - %s (0/%d hunks)\n", c.path, c.total))
	}
	sb.WriteString(fmt.Sprintf("- Documentation files excluded from the code rule catalog (no prose rule catalog exists yet -- see AUR-467 Non-goals): %d\n", len(prosePaths)))
	for _, p := range prosePaths {
		sb.WriteString("  - " + p + "\n")
	}
	return sb.String()
}

// maxCoverageDeclarationTokens estimates the WORST CASE rendered size of
// renderCoverageDeclaration for this diff's file set: every code file
// classified "omitted" (the state that puts every single code path on its
// own bullet line -- the maximum bullet count the renderer can ever emit,
// since a "complete" file contributes zero bullet lines in any real run,
// and "partial"/"omitted" bullet lines differ only by a digit or two).
// AUR-467 blocker 1: this is computed and reserved out of the content
// budget BEFORE TrimToFit runs, so the ACTUAL declaration built afterward
// -- built from whatever mix of complete/partial/omitted TrimToFit
// actually produced -- can never be longer than what was reserved for it.
// A small fixed margin absorbs the digit-count variance between "0/N" and
// "k/N" bullets.
func maxCoverageDeclarationTokens(codePaths, prosePaths []string, totals map[string]int, est TokenEstimator) int {
	worst := make([]fileCoverage, 0, len(codePaths))
	for _, p := range codePaths {
		worst = append(worst, fileCoverage{path: p, covered: 0, total: totals[p]})
	}
	rendered := renderCoverageDeclaration(worst, prosePaths)
	return est.Estimate(rendered) + 24
}
