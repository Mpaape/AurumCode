package analyzer

import (
	"bytes"
	"compress/zlib"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
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
// repository directly.
//
// Reading a repository has two implementations, selected once in OpenRepo:
//
//   - When a `git` binary is on PATH, every read goes through it (`git
//     rev-parse`, `git cat-file`) with the reviewed repository's own
//     directory as cwd, exactly the os/exec approach this card's own text
//     calls out ("use os/exec para chamar git do host/imagem"). Letting git
//     itself discover the repository is what makes this correct on a linked
//     worktree (".git" is a *file* there, `gitdir: ...`, which the naive
//     bare/non-bare check below does not understand) and on any repository
//     whose objects have been packed by `git gc` -- both are the normal
//     shape of a real, lived-in checkout, this one included.
//   - When no `git` binary is present, a pure-Go reader inflates loose
//     objects directly from the object database (zlib + the standard
//     "type size\0content" header) and resolves refs by reading the loose
//     ref files. This is the path this card's own sealed acceptance profile
//     (bootstrap-readonly-v1: Go and bash, no git, no network) exercises,
//     and the only one tests/fixtures/repos/git-demo (a set of loose
//     objects, see build-fixture.sh) needs to prove.
//
// Both paths return the same information through the same Repo methods, so
// everything past OpenRepo (ResolveRef, Diff, the tree walk, the line diff
// in textdiff.go) is written once and is exercised by both: AUR-430's own
// test suite runs entirely on the pure-Go path (this repository's sandbox
// has no git), and gitrepo_gitbinary_test.go runs the git-binary path
// whenever a `git` binary is available to run it with (skipped, never
// silently passed, when one is not).

// Repo is a read-only handle onto a local git repository.
type Repo struct {
	// gitBinary is the resolved path to `git`, non-empty when useGitBinary
	// is true. workDir is the directory git commands run from; git's own
	// discovery (walking up for ".git", following a worktree's ".git" file,
	// or recognizing a bare repository) does the rest.
	useGitBinary bool
	gitBinary    string
	workDir      string

	// gitDir is the resolved git directory used by the pure-Go loose-object
	// reader only.
	gitDir string
}

// OpenRepo resolves root as a git repository. It prefers a `git` binary on
// PATH when one is available (see the package doc above for why); when none
// is available it falls back to treating root/.git as a normal working
// tree's git dir, or root itself as a bare repository (it has HEAD, objects
// and refs directly).
func OpenRepo(root string) (*Repo, error) {
	if gitBin, err := exec.LookPath("git"); err == nil {
		cmd := exec.Command(gitBin, "rev-parse", "--git-dir")
		cmd.Dir = root
		if cmd.Run() == nil {
			return &Repo{useGitBinary: true, gitBinary: gitBin, workDir: root}, nil
		}
		// git is present but root is not a repository it recognizes; fall
		// through to the pure-Go detection below rather than assume, since
		// the two checks agree on every case this reader actually supports.
	}

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
// suffix applied to any of the above. When a `git` binary is in use, the
// full range of git's own revision syntax is accepted instead, since `git
// rev-parse` does the resolving.
func (r *Repo) ResolveRef(ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", fmt.Errorf("empty ref")
	}

	if r.useGitBinary {
		out, err := r.git("rev-parse", "--verify", ref+"^{commit}")
		if err != nil {
			return "", fmt.Errorf("resolving ref %q: %w", ref, err)
		}
		sha := strings.ToLower(strings.TrimSpace(out))
		if !hexSHA.MatchString(sha) {
			return "", fmt.Errorf("resolving ref %q: unexpected rev-parse output %q", ref, out)
		}
		return sha, nil
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

// git runs `git <args...>` with workDir as its working directory and
// returns trimmed stdout. Used only on the git-binary path.
func (r *Repo) git(args ...string) (string, error) {
	cmd := exec.Command(r.gitBinary, args...)
	cmd.Dir = r.workDir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
	}
	return stdout.String(), nil
}

// readObject returns an object's git type ("commit", "tree" or "blob") and
// raw content (the inflated payload with the "type size\0" header already
// stripped), via whichever backend OpenRepo selected.
func (r *Repo) readObject(sha string) (string, []byte, error) {
	sha = strings.ToLower(sha)
	if !hexSHA.MatchString(sha) {
		return "", nil, fmt.Errorf("invalid object id %q", sha)
	}
	if r.useGitBinary {
		return r.readObjectViaGit(sha)
	}
	return r.readLooseObject(sha)
}

// readObjectViaGit reads an object of any kind -- loose or packed -- through
// `git cat-file`, which is exactly why the git-binary path has no packfile
// limitation: git itself resolves that, this reader never touches a .pack
// file's format.
func (r *Repo) readObjectViaGit(sha string) (string, []byte, error) {
	typeOut, err := r.git("cat-file", "-t", sha)
	if err != nil {
		return "", nil, fmt.Errorf("object %s: %w", sha, err)
	}
	objType := strings.TrimSpace(typeOut)

	cmd := exec.Command(r.gitBinary, "cat-file", objType, sha)
	cmd.Dir = r.workDir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", nil, fmt.Errorf("object %s: git cat-file %s: %s", sha, objType, msg)
	}
	return objType, stdout.Bytes(), nil
}

// readLooseObject reads and inflates a loose object by its 40-hex SHA,
// returning its git object type ("commit", "tree" or "blob") and content.
// Packed objects are out of scope for this path (see the package doc): a
// clear, typed error is returned rather than a silently empty diff.
func (r *Repo) readLooseObject(sha string) (string, []byte, error) {
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
