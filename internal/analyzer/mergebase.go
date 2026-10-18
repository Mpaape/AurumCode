package analyzer

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/Mpaape/AurumCode/pkg/types"
)

// errAmbiguousMergeBase is a history with more than one best common
// ancestor (criss-cross merges): no single range is the pull request's.
var errAmbiguousMergeBase = errors.New("more than one best common ancestor (criss-cross history)")

// errNoCommonAncestor is two commits whose histories, as far as the
// repository holds them, never meet.
var errNoCommonAncestor = errors.New("no common ancestor")

// mergeBaseFromObjects returns the best common ancestor of a and b, read
// from the object database: the common ancestors that are not an ancestor
// of another common ancestor. Exactly one must exist; none or several is an
// error, never a guess. The walk (paintdown.go) reads only the commits that
// can decide the answer, never the whole history.
func (r *Repo) mergeBaseFromObjects(a, b string) (string, error) {
	read, err := r.commitReader()
	if err != nil {
		return "", err
	}
	return newPaintWalk(read).mergeBase(a, b)
}

// RangeDiffFromObjects is RangeDiff without a git binary: the diff of
// baseRef...headRef (from their merge base) read from the object database,
// with the same windowed hunks. A binary or generated file is listed without
// hunks, as a forge lists a file it does not render; size never is a reason.
func (r *Repo) RangeDiffFromObjects(baseRef, headRef string) (*types.Diff, error) {
	baseSHA, err := r.ResolveRef(baseRef)
	if err != nil {
		return nil, fmt.Errorf("resolving base ref %q: %w", baseRef, err)
	}
	headSHA, err := r.ResolveRef(headRef)
	if err != nil {
		return nil, fmt.Errorf("resolving head ref %q: %w", headRef, err)
	}
	mergeBase, err := r.mergeBaseFromObjects(strings.ToLower(baseSHA), strings.ToLower(headSHA))
	if err != nil {
		return nil, err
	}
	before, after, err := r.commitFiles(mergeBase, headSHA)
	if err != nil {
		return nil, err
	}
	return r.diffFileMaps(before, after)
}

// commitFiles flattens the trees of two commits.
func (r *Repo) commitFiles(oldSHA, newSHA string) (map[string]blobAt, map[string]blobAt, error) {
	before, after := map[string]blobAt{}, map[string]blobAt{}
	for _, side := range []struct {
		sha string
		out map[string]blobAt
	}{{oldSHA, before}, {newSHA, after}} {
		tree, _, err := r.readCommit(side.sha)
		if err != nil {
			return nil, nil, err
		}
		if err := r.flattenTree(tree, "", side.out); err != nil {
			return nil, nil, err
		}
	}
	return before, after, nil
}

// diffFileMaps diffs two flattened trees, paths in sorted order.
func (r *Repo) diffFileMaps(before, after map[string]blobAt) (*types.Diff, error) {
	paths := map[string]bool{}
	for p := range before {
		paths[p] = true
	}
	for p := range after {
		paths[p] = true
	}
	sorted := make([]string, 0, len(paths))
	for p := range paths {
		sorted = append(sorted, p)
	}
	sort.Strings(sorted)
	diff := &types.Diff{}
	for _, p := range sorted {
		oldBlob, hadOld := before[p]
		newBlob, hasNew := after[p]
		if hadOld && hasNew && oldBlob.sha == newBlob.sha {
			continue
		}
		oldContent, newContent, err := r.sides(oldBlob, hadOld, newBlob, hasNew)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", p, err)
		}
		if classifyBlob(p, newContent) != "" || classifyBlob(p, oldContent) != "" {
			diff.Files = append(diff.Files, types.DiffFile{Path: p})
			continue
		}
		diff.Files = append(diff.Files, BuildWindowedDiffFile(p, string(oldContent), string(newContent)))
	}
	return diff, nil
}

// sides reads the old and new content of one path; an absent side is empty.
func (r *Repo) sides(oldBlob blobAt, hadOld bool, newBlob blobAt, hasNew bool) ([]byte, []byte, error) {
	var oldContent, newContent []byte
	var err error
	if hadOld {
		if oldContent, err = r.blobBytes(oldBlob.sha); err != nil {
			return nil, nil, err
		}
	}
	if hasNew {
		if newContent, err = r.blobBytes(newBlob.sha); err != nil {
			return nil, nil, err
		}
	}
	return oldContent, newContent, nil
}

// forkPoint is the commit a --base diff starts from: the merge base of
// baseSHA and headRef, the newest one in a criss-cross history (as git
// picks). Only histories that truly never meet fall back to baseSHA itself
// (the tree comparison); a head that does not resolve also returns baseSHA, and
// the caller reports it. Any other failure is an error, never a silent
// tree comparison: in a shallow clone, "no common ancestor" may only mean
// the clone was cut before the fork point, so it is an error there too.
func (r *Repo) forkPoint(baseSHA, headRef string) (string, error) {
	headSHA, err := r.ResolveRef(headRef)
	if err != nil {
		return baseSHA, nil
	}
	mergeBase, err := r.mergeBaseFromObjects(strings.ToLower(baseSHA), strings.ToLower(headSHA))
	switch {
	case err == nil:
		return mergeBase, nil
	case errors.Is(err, errAmbiguousMergeBase):
		// A criss-cross history: start from the newest best common
		// ancestor, as `git diff base...head` does, never from the base's
		// own tree (which would show the base's gains as undone).
		var amb *ambiguousMergeBase
		if errors.As(err, &amb) && len(amb.candidates) > 0 {
			return amb.candidates[0], nil
		}
		return "", err
	case errors.Is(err, errNoCommonAncestor):
		shallow, serr := r.shallowCommits()
		if serr != nil {
			return "", serr
		}
		if len(shallow) > 0 {
			return "", fmt.Errorf("%w: the shallow clone does not reach the merge base of the range; fetch more history (git fetch --deepen=<n> origin <base> <head>, or git fetch --unshallow)", err)
		}
		return baseSHA, nil
	}
	return "", fmt.Errorf("finding the merge base of the range: %w", err)
}
