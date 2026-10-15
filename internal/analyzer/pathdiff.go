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
//   - notices: binary or generated content (ClassifyBlob); no content
//     change (NoticeReasonNoContentChange: the same blob under the same path,
//     a pure rename whose base path head no longer has, or empty on both
//     sides); or a byte change with no line to show (NoticeReasonNoPatch).
//     A copy (the base path is still in head) is a new file and is diffed.
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
	// renamedFrom holds the blobs of base paths that head no longer has: a
	// new path with one of them is a pure rename. A blob whose base path is
	// still in head is a copy, a new file to review.
	renamedFrom := make(map[string]bool, len(before))
	for path, b := range before {
		if _, kept := after[path]; !kept {
			renamedFrom[b.sha] = true
		}
	}
	var files []types.DiffFile
	var notices []DiffNotice
	for _, p := range paths {
		oldBlob, hadOld := before[p]
		newBlob, hasNew := after[p]
		if !hadOld && !hasNew {
			continue
		}
		sameBlob := hadOld && hasNew && oldBlob.sha == newBlob.sha
		if sameBlob || (!hadOld && hasNew && renamedFrom[newBlob.sha]) {
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
			notices = append(notices, noLineChange(p, oldContent, newContent))
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

// noLineChange is the notice of a changed path whose line diff is empty:
// nothing changed when both sides are equal (empty on both sides), but bytes
// that differ with no line to show (only the final newline) are a change the
// model cannot see, declared without a patch so the approval is withheld.
func noLineChange(path string, oldContent, newContent []byte) DiffNotice {
	if string(oldContent) == string(newContent) {
		return noContentChange(path)
	}
	return DiffNotice{
		Path:    path,
		Message: "changed with no line to show (final newline only), not reviewed: " + path,
		Reason:  NoticeReasonNoPatch,
	}
}

// noContentChange is the notice of a path with no line to review.
func noContentChange(path string) DiffNotice {
	return DiffNotice{
		Path:    path,
		Message: "no content change (rename, mode or empty file), nothing to review: " + path,
		Reason:  NoticeReasonNoContentChange,
	}
}
