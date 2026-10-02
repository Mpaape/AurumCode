package main

import (
	"github.com/Mpaape/AurumCode/internal/grammar"
	"github.com/Mpaape/AurumCode/internal/prompt"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// noStructureDiffPaths returns the diff's files for which the grammar provider
// has no grammar, so the review declares that their structural context was not
// produced. The answer is the provider's; no extension or language is named
// here.
func noStructureDiffPaths(p grammar.Provider, diff *types.Diff) []string {
	if diff == nil {
		return nil
	}
	var out []string
	seen := make(map[string]struct{})
	for _, f := range diff.Files {
		if _, ok := seen[f.Path]; ok {
			continue
		}
		seen[f.Path] = struct{}{}
		if p.Detect(f.Path, nil) == "" {
			out = append(out, f.Path)
		}
	}
	return out
}

// applyStructuralCoverage is the one place both review paths (--base and --pr)
// record what the review could not cover: files without a grammar are
// declared (never a failure by themselves), and any file declared out of
// reach (binary, generated, too large) withholds approval with the same
// engine-owned marker the gate uses, which the model can neither set nor
// erase (AUR-522: an unreviewed file never counts as approved).
func applyStructuralCoverage(p grammar.Provider, diff *types.Diff, c *reviewCoverageBreakdown, result *types.ReviewResult) {
	c.NoStructure = noStructureDiffPaths(p, diff)
	if len(c.FilteredPaths) == 0 || result == nil {
		return
	}
	if result.Metadata == nil {
		result.Metadata = make(map[string]string)
	}
	result.Metadata[prompt.PolicyGateWithheldKey] = "true"
}
