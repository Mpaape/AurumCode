// The --pr files the API diff carried without any line: their diff is
// recomputed from the verified checkout, so a text file the forge did not
// render (git marked it binary, or the API gave no patch) is reviewed with
// hunks instead of passing as reviewed with nothing shown to the model.
package main

import (
	"github.com/Mpaape/AurumCode/internal/analyzer"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// patchSource recomputes the diff of the given paths over the pull
// request's range. It is a field so a test can drive the merge without a
// GitHub API.
type patchSource func(paths []string) ([]types.DiffFile, []analyzer.DiffNotice, error)

// recoverMissingPatches replaces each line-less file of diff whose diff the
// source can recompute: text that changed comes back with hunks; content
// that did not change (a pure rename, a mode change, an empty file) leaves
// the diff and is declared (NoticeReasonNoContentChange), never counted as
// reviewed. Binary or generated content and any failure leave the file as
// it was, so uninspectedPRNotices keeps it unreviewed (fail closed).
func recoverMissingPatches(diff *types.Diff, source patchSource) (*types.Diff, []analyzer.DiffNotice) {
	if diff == nil || source == nil {
		return diff, nil
	}
	var missing []string
	for _, f := range diff.Files {
		if len(f.Hunks) == 0 {
			missing = append(missing, f.Path)
		}
	}
	if len(missing) == 0 {
		return diff, nil
	}
	files, notices, err := source(missing)
	if err != nil {
		return diff, nil
	}
	recovered := make(map[string]types.DiffFile, len(files))
	for _, f := range files {
		recovered[f.Path] = f
	}
	unchanged := map[string]bool{}
	var declared []analyzer.DiffNotice
	for _, n := range notices {
		if n.Reason == analyzer.NoticeReasonNoContentChange {
			unchanged[n.Path] = true
			declared = append(declared, n)
		}
	}
	out := *diff
	out.Files = make([]types.DiffFile, 0, len(diff.Files))
	for _, f := range diff.Files {
		switch {
		case len(f.Hunks) != 0:
			out.Files = append(out.Files, f)
		case unchanged[f.Path]:
		default:
			if r, ok := recovered[f.Path]; ok {
				r.Lang = f.Lang
				f = r
			}
			out.Files = append(out.Files, f)
		}
	}
	return &out, declared
}

// checkoutPatchSource reads the line-less files' diff from the verified
// checkout over the same range the local-diff fallback uses. Without a
// verified checkout there is no source, and nothing is recovered.
func (p *prReview) checkoutPatchSource() patchSource {
	if p.verifiedDir == "" {
		return nil
	}
	return func(paths []string) ([]types.DiffFile, []analyzer.DiffNotice, error) {
		checkout, err := analyzer.OpenRepo(p.verifiedDir)
		if err != nil {
			return nil, nil, err
		}
		rng, err := pullRequestRangeIn(p.ctx, p.client, p.owner, p.repoName, p.prNumber, p.env().baseSHA, checkout)
		if err != nil {
			return nil, nil, err
		}
		return checkout.PathDiffsFromObjects(rng.base, rng.head, paths)
	}
}
