package analyzer

import (
	"bytes"
	"compress/zlib"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/Mpaape/AurumCode/pkg/types"
)

// This file is new. AurumCode's original engine only ever received an
// already-computed diff from a GitHub pull request payload (see the
// deleted internal/git/githubclient tree); it never needed to read a git
// repository directly. cmd/aurumcode does need that, and the sandbox
// profile this card's acceptance runs under (bootstrap-readonly-v1) carries
// Go but not a `git` binary, so shelling out with os/exec is not viable
// inside that profile even though it is the more obvious design. Reading
// git's on-disk object format directly is a deliberate, documented choice
// (see docs/specs/AUR-430.md): it keeps the reviewed-repository boundary a
// plain filesystem read, needs no subprocess and no network, and matches
// the tests/fixtures/repos/git-demo fixture exactly, since that fixture is
// itself a set of loose objects (see build-fixture.sh).
//
// Scope: this reader supports loose objects only (the format every fresh
// commit and every un-gc'd repository -- including the fixture -- uses). A
// repository whose objects have been packed by `git gc` is out of scope and
// returns a clear error rather than silently reporting an empty diff;
// packfile support is future work, not a silent degradation.

// Repo is a read-only handle onto a local git repository's object database,
// resolved to either a bare repository root or a working tree's ".git" dir.
type Repo struct {
	gitDir string
}

// OpenRepo resolves root as a git repository: root/.git if it exists (a
// normal working tree), otherwise root itself if it looks like a bare
// repository (it has HEAD, objects and refs directly).
func OpenRepo(root string) (*Repo, error) {
	candidate := filepath.Join(root, ".git")
	if info, err := os.Stat(candidate); err == nil && info.IsDir() {
		return &Repo{gitDir: candidate}, nil
	}

	if looksLikeGitDir(root) {
		return &Repo{gitDir: root}, nil
	}

	return nil, fmt.Errorf("not a git repository (no .git dir and %s is not bare): %s", root, root)
}

func looksLikeGitDir(dir string) bool {
	for _, must := range []string{"HEAD", "objects", "refs"} {
		if _, err := os.Stat(filepath.Join(dir, must)); err != nil {
			return false
		}
	}
	return true
}

var hexSHA = regexp.MustCompile(`^[0-9a-fA-F]{40}$`)
var refWithParent = regexp.MustCompile(`^(.*?)~([0-9]*)$`)

// ResolveRef resolves a ref expression to a commit SHA. It understands: a
// raw 40-character hex SHA, "HEAD" (following one level of symbolic
// indirection to refs/heads/<branch>), a bare branch name under
// refs/heads/, and a "<ref>~N" (or "<ref>~", meaning N=1) parent-walk
// suffix applied to any of the above.
func (r *Repo) ResolveRef(ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", fmt.Errorf("empty ref")
	}

	if m := refWithParent.FindStringSubmatch(ref); m != nil {
		base := m[1]
		n := 1
		if m[2] != "" {
			parsed, err := strconv.Atoi(m[2])
			if err != nil {
				return "", fmt.Errorf("invalid parent count in ref %q: %w", ref, err)
			}
			n = parsed
		}
		sha, err := r.ResolveRef(base)
		if err != nil {
			return "", err
		}
		for i := 0; i < n; i++ {
			_, parents, err := r.readCommit(sha)
			if err != nil {
				return "", fmt.Errorf("resolving %s: %w", ref, err)
			}
			if len(parents) == 0 {
				return "", fmt.Errorf("resolving %s: commit %s has no parent", ref, sha)
			}
			sha = parents[0]
		}
		return sha, nil
	}

	if hexSHA.MatchString(ref) {
		return strings.ToLower(ref), nil
	}

	if ref == "HEAD" {
		return r.resolveSymbolic("HEAD", 0)
	}

	// Bare branch name.
	return r.resolveSymbolic("refs/heads/"+ref, 0)
}

func (r *Repo) resolveSymbolic(refPath string, depth int) (string, error) {
	if depth > 10 {
		return "", fmt.Errorf("ref indirection too deep resolving %s", refPath)
	}

	raw, err := os.ReadFile(filepath.Join(r.gitDir, filepath.FromSlash(refPath)))
	if err != nil {
		// Fall back to packed-refs for a ref that has no loose file.
		if sha, ok := r.lookupPackedRef(refPath); ok {
			return sha, nil
		}
		return "", fmt.Errorf("resolving ref %s: %w", refPath, err)
	}

	content := strings.TrimSpace(string(raw))
	if strings.HasPrefix(content, "ref: ") {
		return r.resolveSymbolic(strings.TrimSpace(strings.TrimPrefix(content, "ref: ")), depth+1)
	}
	if !hexSHA.MatchString(content) {
		return "", fmt.Errorf("ref %s does not contain a commit SHA: %q", refPath, content)
	}
	return strings.ToLower(content), nil
}

func (r *Repo) lookupPackedRef(refPath string) (string, bool) {
	raw, err := os.ReadFile(filepath.Join(r.gitDir, "packed-refs"))
	if err != nil {
		return "", false
	}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "^") {
			continue
		}
		fields := strings.SplitN(line, " ", 2)
		if len(fields) == 2 && fields[1] == refPath && hexSHA.MatchString(fields[0]) {
			return strings.ToLower(fields[0]), true
		}
	}
	return "", false
}

// readObject reads and inflates a loose object by its 40-hex SHA, returning
// its git object type ("commit", "tree" or "blob") and content.
func (r *Repo) readObject(sha string) (string, []byte, error) {
	sha = strings.ToLower(sha)
	if !hexSHA.MatchString(sha) {
		return "", nil, fmt.Errorf("invalid object id %q", sha)
	}
	path := filepath.Join(r.gitDir, "objects", sha[:2], sha[2:])
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", nil, fmt.Errorf("object %s not found as a loose object (packed objects are not supported by this reader): %w", sha, err)
	}

	zr, err := zlib.NewReader(bytes.NewReader(raw))
	if err != nil {
		return "", nil, fmt.Errorf("object %s is not valid zlib data: %w", sha, err)
	}
	defer zr.Close()

	inflated, err := io.ReadAll(zr)
	if err != nil {
		return "", nil, fmt.Errorf("object %s: inflate failed: %w", sha, err)
	}

	nul := bytes.IndexByte(inflated, 0)
	if nul < 0 {
		return "", nil, fmt.Errorf("object %s: missing header terminator", sha)
	}
	header := string(inflated[:nul])
	content := inflated[nul+1:]

	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 {
		return "", nil, fmt.Errorf("object %s: malformed header %q", sha, header)
	}
	objType := parts[0]
	declaredLen, err := strconv.Atoi(parts[1])
	if err != nil || declaredLen != len(content) {
		return "", nil, fmt.Errorf("object %s: size mismatch in header %q", sha, header)
	}

	return objType, content, nil
}

func (r *Repo) readCommit(sha string) (tree string, parents []string, err error) {
	objType, content, err := r.readObject(sha)
	if err != nil {
		return "", nil, err
	}
	if objType != "commit" {
		return "", nil, fmt.Errorf("object %s is a %s, not a commit", sha, objType)
	}

	for _, line := range strings.Split(string(content), "\n") {
		if line == "" {
			break // blank line separates headers from the commit message
		}
		switch {
		case strings.HasPrefix(line, "tree "):
			tree = strings.TrimSpace(strings.TrimPrefix(line, "tree "))
		case strings.HasPrefix(line, "parent "):
			parents = append(parents, strings.TrimSpace(strings.TrimPrefix(line, "parent ")))
		}
	}
	if tree == "" {
		return "", nil, fmt.Errorf("commit %s has no tree header", sha)
	}
	return tree, parents, nil
}

type treeEntry struct {
	mode string
	name string
	sha  string
}

func (r *Repo) readTree(sha string) ([]treeEntry, error) {
	objType, content, err := r.readObject(sha)
	if err != nil {
		return nil, err
	}
	if objType != "tree" {
		return nil, fmt.Errorf("object %s is a %s, not a tree", sha, objType)
	}

	var entries []treeEntry
	for len(content) > 0 {
		sp := bytes.IndexByte(content, ' ')
		if sp < 0 {
			return nil, fmt.Errorf("tree %s: malformed entry (no space)", sha)
		}
		mode := string(content[:sp])
		rest := content[sp+1:]

		nul := bytes.IndexByte(rest, 0)
		if nul < 0 {
			return nil, fmt.Errorf("tree %s: malformed entry (no NUL)", sha)
		}
		name := string(rest[:nul])

		if len(rest) < nul+1+20 {
			return nil, fmt.Errorf("tree %s: truncated entry id", sha)
		}
		entrySHA := hex.EncodeToString(rest[nul+1 : nul+1+20])

		entries = append(entries, treeEntry{mode: mode, name: name, sha: entrySHA})
		content = rest[nul+1+20:]
	}
	return entries, nil
}

// blobAt is a flattened path -> blob mapping for one commit's tree.
type blobAt struct {
	sha  string
	mode string
}

// flattenTree walks tree sha recursively, collecting every blob under it
// keyed by its full slash-separated path relative to the tree root.
// Submodule entries (mode "160000", gitlink) are skipped: their "sha" names
// a commit in a different repository, not an object this reader can read.
func (r *Repo) flattenTree(sha, prefix string, out map[string]blobAt) error {
	entries, err := r.readTree(sha)
	if err != nil {
		return err
	}
	for _, e := range entries {
		path := e.name
		if prefix != "" {
			path = prefix + "/" + e.name
		}
		switch e.mode {
		case "40000": // directory
			if err := r.flattenTree(e.sha, path, out); err != nil {
				return err
			}
		case "160000": // submodule / gitlink: not readable from this object DB.
			continue
		default: // "100644", "100755", "120000" (symlink target stored as blob content)
			out[path] = blobAt{sha: e.sha, mode: e.mode}
		}
	}
	return nil
}

func (r *Repo) blobContent(sha string) (string, error) {
	objType, content, err := r.readObject(sha)
	if err != nil {
		return "", err
	}
	if objType != "blob" {
		return "", fmt.Errorf("object %s is a %s, not a blob", sha, objType)
	}
	return string(content), nil
}

// Diff computes the changed-file diff between baseRef and headRef as a
// types.Diff, in the same shape AnalyzeDiff and the prompt builder already
// consume. Unchanged files (identical blob SHA on both sides) are omitted,
// matching how `git diff` itself only reports files that changed. Files are
// returned sorted by path so the same two commits always produce
// byte-identical output.
func (r *Repo) Diff(baseRef, headRef string) (*types.Diff, error) {
	baseSHA, err := r.ResolveRef(baseRef)
	if err != nil {
		return nil, fmt.Errorf("resolving base ref %q: %w", baseRef, err)
	}
	headSHA, err := r.ResolveRef(headRef)
	if err != nil {
		return nil, fmt.Errorf("resolving head ref %q: %w", headRef, err)
	}

	baseTree, _, err := r.readCommit(baseSHA)
	if err != nil {
		return nil, err
	}
	headTree, _, err := r.readCommit(headSHA)
	if err != nil {
		return nil, err
	}

	before := map[string]blobAt{}
	after := map[string]blobAt{}
	if err := r.flattenTree(baseTree, "", before); err != nil {
		return nil, fmt.Errorf("reading base tree: %w", err)
	}
	if err := r.flattenTree(headTree, "", after); err != nil {
		return nil, fmt.Errorf("reading head tree: %w", err)
	}

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
			continue // unchanged
		}

		var oldContent, newContent string
		if hadOld {
			oldContent, err = r.blobContent(oldBlob.sha)
			if err != nil {
				return nil, fmt.Errorf("reading old content of %s: %w", p, err)
			}
		}
		if hasNew {
			newContent, err = r.blobContent(newBlob.sha)
			if err != nil {
				return nil, fmt.Errorf("reading new content of %s: %w", p, err)
			}
		}

		diff.Files = append(diff.Files, BuildDiffFile(p, oldContent, newContent))
	}

	return diff, nil
}
