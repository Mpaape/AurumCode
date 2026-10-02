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
// verifiedCleanCheckoutReason closes that gap: it proves every regular file
// under the checkout (outside ".git") is exactly the content git already
// has recorded for a committed blob, and fails closed -- "dirty" or
// "unverifiable" -- on anything it cannot prove. This runs only on the --pr
// path (pr.go, right after codebaseContextMismatch); --base is deliberately
// left untouched, since there the checkout IS the change under review by
// construction (see aur515.go's own package doc) and demanding a clean tree
// there would break reviewing a local, uncommitted diff.
//
// Two backends, chosen once per call, mirror internal/analyzer.OpenRepo's
// own git-binary/pure-Go split:
//
//   - git on PATH: `git ls-tree -r -z HEAD` lists every path git tracks at
//     HEAD together with its blob id, resolved by git itself -- so this
//     works whether the repository's objects are loose or packed. Each
//     working-tree file's own git blob id (sha1("blob "+len+"\0"+content"),
//     computed here, not shelled out per file) is compared against that
//     map. A file absent from the map, a hash mismatch, or a tracked path
//     missing from disk is "dirty". This is the path the shipped Docker
//     image (Dockerfile installs git) and any ordinary workstation use.
//   - no git on PATH: there is no public API in internal/analyzer to list a
//     commit's tree (its tree/commit readers are deliberately unexported;
//     this card's paths do not include internal/analyzer), so the fallback
//     instead asks whether each working-tree file's git blob id names an
//     object that already exists, loose, in .git/objects. Existence of a
//     correctly-named loose object is strong evidence the exact content was
//     committed at some point: git's own loose-object format is
//     content-addressed, so nothing can occupy that path except that exact
//     content. This is weaker than the git-binary path in two documented
//     ways -- it does not confirm the blob is recorded at HEAD for that
//     exact path (only that it exists somewhere in this repository's
//     history), and it cannot see packed objects at all, so a repository
//     that has been packed (any non-trivial real clone, past roughly 100
//     objects) reads every file as unverifiable and omits context -- which
//     is a fail-closed outcome, not a wrong one. It is exactly good enough
//     for the sealed, git-less acceptance profile this card's own tests run
//     in, built from small, loose-object-only fixtures; production always
//     has git (the shipped Dockerfile), so it never exercises this
//     fallback at all.
package main

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	codebasectx "github.com/Mpaape/AurumCode/internal/context"
)

// codebaseContextReasonDirty is returned by verifiedCleanCheckoutReason when
// the checkout's working tree diverges from its committed content: an
// uncommitted edit, an untracked file, or an extra file (for instance a
// nested clone) with no committed counterpart. It is a fixed reason code,
// never remote or local repository text, so it needs no redaction.
const codebaseContextReasonDirty = "dirty"

// maxVerifiedCheckoutFiles bounds the working-tree walk verifiedCleanCheckoutReason
// performs, matching internal/context's own enumeration bound (DefaultMaxFiles)
// so this verification pass and the resolver it gates stay consistent: a
// repository too large for the resolver to fully enumerate is, by the same
// bound, too large for this check to fully verify, and is reported
// "unverifiable" rather than silently checking only a prefix.
const maxVerifiedCheckoutFiles = codebasectx.DefaultMaxFiles

// verifiedCleanCheckoutReason returns "" when every regular file under dir
// (outside ".git") is exactly the content already committed at HEAD;
// otherwise it returns codebaseContextReasonDirty or
// codebaseContextReasonUnverifiable (aur515.go). dir must already be
// verified, by the caller, as the reviewed repository at the reviewed pull
// request's head commit -- this function only adds the clean-tree half of
// that proof.
func verifiedCleanCheckoutReason(dir string) string {
	if tracked, ok := gitTrackedBlobs(dir); ok {
		return cleanAgainstTracked(dir, tracked)
	}
	objectsDir, err := gitObjectsDir(dir)
	if err != nil {
		return codebaseContextReasonUnverifiable
	}
	return cleanAgainstLooseObjects(dir, objectsDir)
}

// gitTrackedBlobs returns dir's HEAD tree as a path -> git blob id map via
// `git ls-tree`, when a git binary is available; ok is false (map nil)
// whenever git cannot answer, so the caller falls back to the loose-object
// check instead of trusting an empty tree.
func gitTrackedBlobs(dir string) (tracked map[string]string, ok bool) {
	gitBin, err := exec.LookPath("git")
	if err != nil {
		return nil, false
	}
	cmd := exec.Command(gitBin, "-c", "safe.directory="+dir, "-C", dir, "ls-tree", "-r", "-z", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return nil, false
	}
	tracked = make(map[string]string)
	for _, record := range strings.Split(string(out), "\x00") {
		if record == "" {
			continue
		}
		meta, path, cut := strings.Cut(record, "\t")
		if !cut || path == "" {
			continue
		}
		fields := strings.Fields(meta)
		if len(fields) != 3 {
			continue
		}
		tracked[filepath.ToSlash(path)] = fields[2]
	}
	return tracked, true
}

// gitObjectsDir resolves dir's shared git object database directory,
// reusing gitConfigPath (aur515.go) so a linked worktree's objects --
// always stored in the main checkout's gitdir, never the worktree's own --
// resolve the same way AUR-515's own origin/HEAD reads already do.
func gitObjectsDir(dir string) (string, error) {
	configPath, err := gitConfigPath(dir)
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(configPath), "objects"), nil
}

// cleanAgainstTracked walks dir and reports codebaseContextReasonDirty
// unless every regular file (outside ".git") matches tracked's blob id for
// its own relative path and every tracked path is present on disk.
func cleanAgainstTracked(dir string, tracked map[string]string) string {
	matched := 0
	dirty := false
	err := walkVerified(dir, func(rel string, data []byte) {
		want, ok := tracked[rel]
		if !ok || want != blobSHA1(data) {
			dirty = true
			return
		}
		matched++
	})
	if err != nil {
		return codebaseContextReasonUnverifiable
	}
	if dirty || matched != len(tracked) {
		return codebaseContextReasonDirty
	}
	return ""
}

// cleanAgainstLooseObjects walks dir and reports codebaseContextReasonDirty
// unless every regular file's (outside ".git") git blob id names a loose
// object that already exists under objectsDir -- see the package doc for
// exactly what that does and does not prove.
func cleanAgainstLooseObjects(dir, objectsDir string) string {
	dirty := false
	err := walkVerified(dir, func(rel string, data []byte) {
		sha := blobSHA1(data)
		if _, statErr := os.Stat(filepath.Join(objectsDir, sha[:2], sha[2:])); statErr != nil {
			dirty = true
		}
	})
	if err != nil {
		return codebaseContextReasonUnverifiable
	}
	if dirty {
		return codebaseContextReasonDirty
	}
	return ""
}

// walkVerified walks every regular file under dir, skipping ".git" entirely
// and skipping symlinks (the resolver itself never reads their content
// either, so they carry no leak risk here), and calls visit with each
// file's root-relative slash path and full content. It is bounded by
// maxVerifiedCheckoutFiles so a pathological tree cannot make verification
// itself unbounded; exceeding the bound is reported as a walk error so the
// caller fails closed.
func walkVerified(dir string, visit func(rel string, data []byte)) error {
	count := 0
	return filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return fs.SkipDir
			}
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 || !d.Type().IsRegular() {
			return nil
		}
		count++
		if count > maxVerifiedCheckoutFiles {
			return fmt.Errorf("checkout has more than %d files", maxVerifiedCheckoutFiles)
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		visit(filepath.ToSlash(rel), data)
		return nil
	})
}

// blobSHA1 returns the hex git blob object id of data -- the same id
// `git hash-object` and a tree entry's own recorded id use.
func blobSHA1(data []byte) string {
	h := sha1.New()
	fmt.Fprintf(h, "blob %d\x00", len(data))
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}
