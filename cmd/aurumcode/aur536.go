// AUR-536 closes out AUR-515's own review follow-up: once the --pr path has
// verified that the local checkout IS the reviewed repository at the
// reviewed pull request's exact head commit (codebaseContextMismatch,
// aur515.go), the codebase-context pass (resolveCodebaseContext, passes.go)
// still reads whatever sits in that checkout's working tree via a plain
// filepath.WalkDir (internal/context/resolver.go). A verified repository
// and HEAD do not imply a clean tree: an uncommitted edit, an untracked
// file, or files belonging to a nested clone copied into the tree all sit
// on disk and would reach the model as "codebase context" even though none
// of them is part of the commit the pull request actually names.
//
// verifiedCleanCheckoutReason closes that gap: it proves every file under
// the checkout (outside ".git") is exactly the content git already has
// recorded for a committed blob at HEAD, and fails closed -- "dirty" or
// "unverifiable" -- on anything it cannot prove. This runs only on the --pr
// path (pr.go, right after codebaseContextMismatch); --base is deliberately
// left untouched, since there the checkout IS the change under review by
// construction (see aur515.go's own package doc) and demanding a clean tree
// there would break reviewing a local, uncommitted diff.
//
// The proof itself is a single comparison against internal/analyzer's own
// exported Repo.TrackedFiles(ref): analyzer.OpenRepo already resolves, once,
// which backend reads this repository -- the git binary (loose or packed
// objects, via `git cat-file`) when one is on PATH, or a pure-Go
// loose-object reader when it is not -- and TrackedFiles returns HEAD's
// entire tracked tree, as a path -> {blob id, mode} map, through whichever
// one it picked. There is exactly one map-building codepath, not two: the
// git-binary and git-less backends feed the identical comparison
// (cleanAgainstTracked), so they can only ever disagree on what a loose
// object lookup finds, never on what "clean" means. A git-less checkout
// whose objects have been packed is TrackedFiles' hard error case (a loose
// object the pure-Go reader cannot find): that is reported
// "unverifiable", never "dirty" -- an inability to prove the tree is dirty
// is not evidence that it is. It never happens in production, where the
// shipped Dockerfile always installs git; it is exactly the sealed
// acceptance profile's own git-less, loose-object-only fixtures that
// exercise this path for real.
//
// Once the tree verifies clean, the exact file set TrackedFiles named is
// handed, by construction, to internal/context's resolver
// (resolveVerifiedCodebaseContext, ResolveWithFiles) as the ONLY files its
// own repo-wide reference scan may read -- never a second, independent
// filesystem walk that this proof and the resolver could, in principle,
// disagree about.
package main

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/Mpaape/AurumCode/internal/analyzer"
	codebasectx "github.com/Mpaape/AurumCode/internal/context"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// errCheckoutTooLarge is walkVerified's sentinel error for exceeding its
// limit, distinct from an ordinary read failure: cleanAgainstTracked treats
// it as proof of an extra, untracked file (codebaseContextReasonDirty)
// rather than as codebaseContextReasonUnverifiable.
var errCheckoutTooLarge = errors.New("checkout file count exceeds limit")

// codebaseContextReasonDirty is returned by verifiedCleanCheckoutReason when
// the checkout's working tree diverges from its committed content: an
// uncommitted edit, a staged-but-uncommitted change, an untracked file
// (including an untracked or retargeted symlink), a deleted tracked file,
// or an extra file (for instance a nested clone) with no committed
// counterpart. It is a fixed reason code, never remote or local repository
// text, so it needs no redaction.
const codebaseContextReasonDirty = "dirty"

// verifiedCleanCheckoutReason returns ("", files) when every file under dir
// (outside ".git") is exactly the content already committed at HEAD, where
// files is the sorted set of root-relative paths proven clean -- the exact
// set resolveVerifiedCodebaseContext goes on to read, and no more.
// Otherwise it returns (codebaseContextReasonDirty, nil) or
// (codebaseContextReasonUnverifiable, nil) (aur515.go). dir must already be
// verified, by the caller, as the reviewed repository at the reviewed pull
// request's head commit -- this function only adds the clean-tree half of
// that proof.
func verifiedCleanCheckoutReason(dir string) (reason string, files []string) {
	repo, err := analyzer.OpenRepo(dir)
	if err != nil {
		return codebaseContextReasonUnverifiable, nil
	}
	tracked, err := repo.TrackedFiles("HEAD")
	if err != nil {
		return codebaseContextReasonUnverifiable, nil
	}
	return cleanAgainstTracked(dir, tracked)
}

// cleanAgainstTracked walks dir and reports codebaseContextReasonDirty
// unless every file (outside ".git") matches tracked's entry for its own
// relative path -- blob id for an ordinary file, or blob id of the link
// target's bytes for a symlink (tracked's own git mode says which this
// path must be) -- and every tracked path is present on disk. The walk is
// bounded at len(tracked)+1, not a fixed constant: git itself has already
// named exactly how many paths this commit tracks, so seeing one more
// filesystem entry than that already proves an untracked file exists
// (dirty) without needing to find it by name, and a real repository's own
// size (this one included, at over 2000 files) never makes the check
// itself unverifiable.
func cleanAgainstTracked(dir string, tracked map[string]analyzer.TrackedEntry) (string, []string) {
	matched := 0
	dirty := false
	err := walkVerified(dir, len(tracked)+1, func(rel string, isSymlink bool, data []byte) {
		entry, ok := tracked[rel]
		if !ok || isSymlink != (entry.Mode == "120000") || entry.SHA != blobSHA1(data) {
			dirty = true
			return
		}
		matched++
	})
	if err != nil {
		if errors.Is(err, errCheckoutTooLarge) {
			return codebaseContextReasonDirty, nil
		}
		return codebaseContextReasonUnverifiable, nil
	}
	if dirty || matched != len(tracked) {
		return codebaseContextReasonDirty, nil
	}
	files := make([]string, 0, len(tracked))
	for rel := range tracked {
		files = append(files, rel)
	}
	return "", files
}

// walkVerified walks every file under dir, skipping ".git" entirely --
// whether it is this checkout's own git directory or, exactly the same
// way, a linked worktree's or a submodule's "gitdir: ..." indirection FILE
// (neither is tracked content) -- and calls visit with each file's
// root-relative slash path, whether it is a symlink, and its comparable
// content: a regular file's own bytes, or a symlink's link-target bytes
// (os.Readlink) -- never a symlink's target's CONTENT, so a symlink this
// walk follows is never itself dereferenced and read. It is bounded by
// limit so a pathological tree cannot make verification itself unbounded;
// exceeding the bound returns errCheckoutTooLarge, not a visit, so the
// caller decides what that means for its own backend.
func walkVerified(dir string, limit int, visit func(rel string, isSymlink bool, data []byte)) error {
	count := 0
	return filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Name() == ".git" {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil // a ".git" FILE: worktree/submodule indirection, not tracked content.
		}
		if d.IsDir() {
			return nil
		}
		isSymlink := d.Type()&fs.ModeSymlink != 0
		if !isSymlink && !d.Type().IsRegular() {
			return nil // device, socket, etc.: never tracked content either.
		}
		count++
		if count > limit {
			return errCheckoutTooLarge
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		var data []byte
		if isSymlink {
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			data = []byte(target)
		} else {
			data, err = os.ReadFile(path)
			if err != nil {
				return err
			}
		}
		visit(filepath.ToSlash(rel), isSymlink, data)
		return nil
	})
}

// resolveVerifiedCodebaseContext is --pr's own codebase-context pass
// (AUR-536): identical in output shape to resolveCodebaseContext
// (passes.go, shared with --base), except the resolver's own file
// discovery is replaced, by construction, with exactly files --
// verifiedCleanCheckoutReason's own proven-clean set -- so the set this
// check verified and the set the resolver actually reads can never
// diverge, not even by a future, independent bug in either side's walk.
func resolveVerifiedCodebaseContext(diff *types.Diff, dir string, files []string) string {
	pack, err := codebasectx.NewResolver().ResolveWithFiles(dir, diffPaths(diff), files)
	if err != nil || pack == nil {
		return ""
	}
	data, err := json.Marshal(pack)
	if err != nil {
		return ""
	}
	return string(data)
}

// blobSHA1 returns the hex git blob object id of data -- the same id
// `git hash-object` and a tree entry's own recorded id use. For a symlink,
// data is the link target's own bytes (os.Readlink), matching how git
// itself stores a symlink's blob content.
func blobSHA1(data []byte) string {
	h := sha1.New()
	fmt.Fprintf(h, "blob %d\x00", len(data))
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}
