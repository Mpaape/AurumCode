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
// needs a follow-up card that owns cmd/aurumcode; this card does not.

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

// coveredFiles returns the set of distinct FilePath values present in
// segments -- the code files that actually survived TrimToFit's budget
// cut and reached the assembled prompt.
func coveredFiles(segments []ContextSegment) map[string]bool {
	covered := make(map[string]bool, len(segments))
	for _, s := range segments {
		if s.FilePath != "" {
			covered[s.FilePath] = true
		}
	}
	return covered
}

// renderCoverageDeclaration is the single call site that states review
// coverage: how many of the diff's code files this assembled prompt
// actually carries versus the diff's total, and which documentation files
// were excluded from the code rule catalog and why. It is emitted
// unconditionally -- even "0 omitted" is a stated fact, not silence -- so
// AC-003 cannot be satisfied by an empty-string special case, and so
// MUT-002's mutation (deleting this call) has one unique anchor to target.
func renderCoverageDeclaration(codePaths, prosePaths []string, coveredCode map[string]bool) string {
	omitted := make([]string, 0, len(codePaths))
	for _, p := range codePaths {
		if !coveredCode[p] {
			omitted = append(omitted, p)
		}
	}

	var sb strings.Builder
	sb.WriteString("## Review Coverage\n")
	sb.WriteString(fmt.Sprintf("- Code files in this diff: %d\n", len(codePaths)))
	sb.WriteString(fmt.Sprintf("- Code files reviewed against the rule catalog: %d\n", len(codePaths)-len(omitted)))
	sb.WriteString(fmt.Sprintf("- Code files NOT covered by this review (token budget): %d\n", len(omitted)))
	for _, p := range omitted {
		sb.WriteString("  - " + p + "\n")
	}
	sb.WriteString(fmt.Sprintf("- Documentation files excluded from the code rule catalog (no prose rule catalog exists yet -- see AUR-467 Non-goals): %d\n", len(prosePaths)))
	for _, p := range prosePaths {
		sb.WriteString("  - " + p + "\n")
	}
	return sb.String()
}
