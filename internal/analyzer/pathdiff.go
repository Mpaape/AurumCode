package analyzer

import (
	"fmt"
	"strings"

	"github.com/Mpaape/AurumCode/pkg/types"
)

// NoticeReasonNoContentChange is the Reason of a changed path whose content
// is the same on both sides (a pure rename, a mode change) or empty: there
// is no line to review. It is declared, never counted as reviewed.
const NoticeReasonNoContentChange = "no content change"

// PathDiffsFromObjects recomputes, from the object database, the diff of
// the given paths over baseRef...headRef (from their merge base, as a forge
// diffs a pull request). It is how --pr reviews a file the API diff carried
// without any line. Each path ends up in exactly one of the results, or in
// neither when the range does not touch it:
//
//   - files: text whose content changed, with windowed hunks;
//   - notices: binary or generated content (ClassifyBlob), or no content
//     change (NoticeReasonNoContentChange: same blob as in the base, under
//     the same or another path, or empty on both sides).
func (r *Repo) PathDiffsFromObjects(baseRef, headRef string, paths []string) ([]types.DiffFile, []DiffNotice, error) {
	baseSHA, err := r.ResolveRef(baseRef)
	if err != nil {
		return nil, nil, fmt.Errorf("resolving base ref %q: %w", baseRef, err)
	}
	headSHA, err := r.ResolveRef(headRef)
	if err != nil {
		return nil, nil, fmt.Errorf("resolving head ref %q: %w", headRef, err)
	}
	mergeBase, err := r.mergeBaseFromObjects(strings.ToLower(baseSHA), strings.ToLower(headSHA))
	if err != nil {
		return nil, nil, err
	}
	before, after, err := r.commitFiles(mergeBase, headSHA)
	if err != nil {
		return nil, nil, err
	}
	baseBlobs := make(map[string]bool, len(before))
	for _, b := range before {
		baseBlobs[b.sha] = true
	}
	var files []types.DiffFile
	var notices []DiffNotice
	for _, p := range paths {
		oldBlob, hadOld := before[p]
		newBlob, hasNew := after[p]
		if !hadOld && !hasNew {
			continue
		}
		if hasNew && baseBlobs[newBlob.sha] && (!hadOld || oldBlob.sha == newBlob.sha) {
			notices = append(notices, noContentChange(p))
			continue
		}
		oldContent, newContent, err := r.sides(oldBlob, hadOld, newBlob, hasNew)
		if err != nil {
			return nil, nil, fmt.Errorf("reading %s: %w", p, err)
		}
		if n := classifyPair(p, oldContent, newContent); n != nil {
			notices = append(notices, *n)
			continue
		}
		file := BuildWindowedDiffFile(p, string(oldContent), string(newContent))
		if len(file.Hunks) == 0 {
			notices = append(notices, noContentChange(p))
			continue
		}
		files = append(files, file)
	}
	return files, notices, nil
}

// classifyPair runs ClassifyBlob over both sides, the new one first: a file
// that is or was binary or generated is not line-diffed.
func classifyPair(path string, oldContent, newContent []byte) *DiffNotice {
	if n := ClassifyBlob(path, newContent); n != nil {
		return n
	}
	return ClassifyBlob(path, oldContent)
}

// noContentChange is the notice of a path with no line to review.
func noContentChange(path string) DiffNotice {
	return DiffNotice{
		Path:    path,
		Message: "no content change (rename, mode or empty file), nothing to review: " + path,
		Reason:  NoticeReasonNoContentChange,
	}
}
