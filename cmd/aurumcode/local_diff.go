// The local diff of --base: the git diff against a ref, the commits of the
// range and the redacted notices about files the diff could not carry.
package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/Mpaape/AurumCode/internal/analyzer"
	"github.com/Mpaape/AurumCode/internal/changelog"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// computeDiff reads the diff between base and HEAD directly from the git
// object database at repoRoot. See internal/analyzer/gitrepo.go for why
// this reads git's on-disk format in pure Go instead of shelling out to a
// `git` binary.
// diffPaths returns the changed file paths of diff, in file order, for
// config.WrapProvider and PathInstructionsProvider's applyTo matching. A
// nil diff (never actually produced by computeDiff, but kept defensive
// for callers) yields nil.
func diffPaths(diff *types.Diff) []string {
	if diff == nil {
		return nil
	}
	paths := make([]string, 0, len(diff.Files))
	for _, f := range diff.Files {
		paths = append(paths, f.Path)
	}
	return paths
}

func computeDiff(repoRoot, base string) (*types.Diff, []analyzer.DiffNotice, error) {
	repo, err := analyzer.OpenRepo(repoRoot)
	if err != nil {
		// analyzer.OpenRepo has exactly one error case (gitrepo.go), and its
		// own message already says "not a git repository (...)"; wrapping
		// it with another "is not a git repository" prefix used to print a
		// literally duplicated phrase ("... is not a git repository: not a
		// git repository (no .git dir ...)"). That message is fully known
		// (see the package doc above), so this replaces it outright with
		// one clean, actionable sentence instead of restating the same
		// fact twice.
		return nil, nil, fmt.Errorf("%s is not a git repository (no .git directory here, and this path is not itself a bare repository)", repoRoot)
	}
	diff, notices, err := repo.Diff(base, "HEAD")
	if err != nil {
		return nil, nil, cleanRefError(repoRoot, base, err)
	}
	return diff, notices, nil
}

// localRangeCommits reads the commit messages of base..HEAD from the local
// repository through the read-only analyzer (gitrepo.go's Commits). The
// returned text is raw untrusted repository content; buildChangelogSection
// redacts it before rendering.
func localRangeCommits(repoRoot, base string) ([]changelog.Commit, error) {
	repo, err := analyzer.OpenRepo(repoRoot)
	if err != nil {
		return nil, err
	}
	infos, err := repo.Commits(base, "HEAD")
	if err != nil {
		return nil, err
	}
	commits := make([]changelog.Commit, 0, len(infos))
	for _, info := range infos {
		commits = append(commits, changelog.Commit{Subject: info.Subject, Body: info.Body, Hash: info.Hash})
	}
	return commits, nil
}

// cleanRefError turns internal/analyzer's ref-resolution error into one
// clean, actionable line instead of the raw internal detail
// analyzer.Repo.Diff's error chain otherwise surfaces to the user:
//
//   - the pure-Go path (gitrepo.go's resolveSymbolic), the one this card's
//     own sealed acceptance profile exercises (bootstrap-readonly-v1: Go
//     and bash, no git binary), wraps a raw *fs.PathError, e.g. "open
//     <repo>/.git/refs/heads/<ref>: no such file or directory";
//   - the git-binary path (gitrepo.go's ResolveRef, when a `git` binary is
//     on PATH) wraps git's own "fatal: Needed a single revision" or
//     "fatal: ... unknown revision or path ..." stderr text.
//
// Both leak implementation detail (a filesystem path under .git, or a
// foreign tool's own wording) that means nothing to a user who has never
// read the source, and the two backends previously disagreed on what that
// detail even looked like. Both are recognized here and replaced by the
// identical clean sentence, naming the ref that failed and how a valid one
// is spelled, so a real repository behaves the same way regardless of
// which backend OpenRepo selected. Only the two prefixes
// analyzer.Repo.Diff itself always wraps with ("resolving base ref %q: " /
// "resolving head ref %q: ", gitrepo.go) are recognized, and only when the
// wrapped cause looks like "no such ref" specifically: any other Diff
// failure (a corrupted tree, an ambiguous parent count, and the like) is
// not concretely measured by this card's dogfooding and keeps its
// pre-existing wrapped message unchanged, so a genuinely new failure is
// never silently hidden behind a guessed diagnosis.
//
// A permission problem (the ref file exists but this process cannot read
// it -- exactly the shape a locked-down runtime like this card's own
// bootstrap-readonly-v1 profile, or any non-root uid, can hit) is
// deliberately NOT folded into "not found": refPathPermissionDenied below
// probes the concrete resource independently and is checked first, so a
// permission failure is reported as what it is instead of a confident,
// wrong "ref not found" -- a review found this card's first cut treated
// every *fs.PathError as "not found" regardless of cause (fixed: the
// pure-Go path now also requires errors.Is(err, fs.ErrNotExist)), and,
// separately, that `git rev-parse`'s own stderr text
// ("fatal: Needed a single revision") is byte-identical for ENOENT and
// EACCES on the git-binary path -- string-matching git's output alone can
// never tell the two apart, which is why the probe exists and is checked
// on both backends rather than only patching the pure-Go one.
//
// Once the switch above has matched one of the two prefixes, this failure
// is KNOWN to be about resolving `ref` specifically, and every branch from
// here on must return a clean, ref-scoped sentence -- never fall through
// to a raw %w wrap of `err` again. A second review found exactly one
// remaining fallthrough that did: `--base feature` against a repository
// whose only branch is `feature/sub` resolves refs/heads/feature to a
// DIRECTORY (a hierarchical branch namespace), so the pure-Go path's
// os.ReadFile fails with EISDIR -- neither fs.ErrNotExist nor
// fs.ErrPermission, so it fell through every existing check into the raw
// wrap, leaking the literal "refs/heads/" path this card's own acceptance
// already treats as a leak everywhere else. refPathIsDirectory closes it
// the same way refPathPermissionDenied closes EACCES: an independent
// probe of the concrete resource, checked on both backends, instead of
// inferring the cause from either backend's own text (the git-binary path
// already said a clean-but-imprecise "not found" for this exact case;
// probing first makes both backends agree on the more precise answer
// instead of only the pure-Go path being fixed).
func cleanRefError(repoRoot, base string, err error) error {
	msg := err.Error()
	ref := ""
	switch {
	case strings.HasPrefix(msg, "resolving base ref "):
		ref = base
	case strings.HasPrefix(msg, "resolving head ref "):
		ref = "HEAD"
	default:
		return fmt.Errorf("computing diff %s..HEAD: %w", base, err)
	}

	if refPathPermissionDenied(repoRoot, ref) {
		return fmt.Errorf("permission denied resolving ref %q in this repository: this process cannot read the ref's storage under .git (or the bare repository root) -- check its file permissions", ref)
	}
	if refPathIsDirectory(repoRoot, ref) {
		return fmt.Errorf("ref %q is not a branch: a directory of refs by that name exists (a hierarchical branch namespace, e.g. %q) -- name the exact branch, not a namespace prefix", ref, ref+"/...")
	}

	var pathErr *fs.PathError
	looksLikeNoSuchRef := (errors.As(err, &pathErr) && errors.Is(err, fs.ErrNotExist)) ||
		strings.Contains(msg, "Needed a single revision") ||
		strings.Contains(msg, "unknown revision or path")
	if looksLikeNoSuchRef {
		return fmt.Errorf("ref %q not found in this repository: expected a branch name, HEAD, a 40-character commit SHA, or a \"<ref>~N\" parent expression", ref)
	}

	// Some other, not concretely measured cause within ref resolution (a
	// corrupted ref file's content, an ambiguous parent count, indirection
	// too deep): still a clean, ref-scoped sentence, never the raw
	// internal chain -- see the note above this function for exactly why a
	// fallthrough here is the defect this fix closes.
	return fmt.Errorf("could not resolve ref %q in this repository", ref)
}

// refPathCandidate computes the exact physical loose-ref location a ref
// name would resolve to under repoRoot -- refs/heads/<ref>, or HEAD for
// ref=="HEAD" -- the one location both analyzer.Repo backends ultimately
// consult for a bare branch name (see cleanRefError's doc above for why
// this card probes it directly instead of inferring from either backend's
// own error text). ok is false when there is nothing meaningful to probe:
// a raw 40-character commit SHA never touches a loose-ref file, and an
// empty ref cannot be resolved to a path.
func refPathCandidate(repoRoot, ref string) (path string, ok bool) {
	if ref == "" || looksLikeCommitSHA(ref) {
		return "", false
	}

	gitDir := filepath.Join(repoRoot, ".git")
	if info, err := os.Stat(gitDir); err != nil || !info.IsDir() {
		// No ".git" subdirectory: repoRoot is itself the bare repository
		// (see analyzer.OpenRepo's own fallback, gitrepo.go).
		gitDir = repoRoot
	}

	if ref == "HEAD" {
		return filepath.Join(gitDir, "HEAD"), true
	}
	return filepath.Join(gitDir, "refs", "heads", filepath.FromSlash(ref)), true
}

// refPathPermissionDenied independently probes whether ref's own loose-ref
// storage under repoRoot is unreadable for a permission reason (EACCES), as
// opposed to genuinely absent (ENOENT) -- the one distinction neither
// backend's own error text reliably carries (see cleanRefError's doc
// above). It reads the exact physical location either backend would have
// consulted, directly, from cmd/aurumcode, independent of which backend
// actually produced the original error: this is one of the things this
// card can do about the ambiguity without editing internal/analyzer (a
// read_path for this card).
//
// It is deliberately conservative: refPathCandidate returning ok==false
// (a raw SHA, an empty ref) leaves it alone, and any outcome other than a
// confirmed permission error (the file does not exist, it is a directory
// -- os.Open succeeds on a directory -- or the open succeeds) returns
// false and lets a later check answer instead -- a probe that cannot say
// anything useful must never manufacture a diagnosis of its own.
func refPathPermissionDenied(repoRoot, ref string) bool {
	candidate, ok := refPathCandidate(repoRoot, ref)
	if !ok {
		return false
	}

	f, err := os.Open(candidate)
	if err == nil {
		f.Close()
		return false
	}
	return errors.Is(err, fs.ErrPermission)
}

// refPathIsDirectory independently probes whether ref's own loose-ref
// location under repoRoot exists as a DIRECTORY rather than a ref file --
// the shape of a hierarchical branch namespace (refs/heads/feature/ holds
// refs/heads/feature/sub, so "feature" alone resolves to a directory, not
// a ref file). os.ReadFile on a directory fails with EISDIR, which is
// neither fs.ErrNotExist nor fs.ErrPermission, so without this check the
// failure fell through cleanRefError's classification entirely into the
// raw internal chain (see cleanRefError's doc above). Same conservative
// contract as refPathPermissionDenied: refPathCandidate returning
// ok==false leaves it alone, and anything other than a confirmed
// directory returns false.
func refPathIsDirectory(repoRoot, ref string) bool {
	candidate, ok := refPathCandidate(repoRoot, ref)
	if !ok {
		return false
	}

	info, err := os.Stat(candidate)
	return err == nil && info.IsDir()
}

// looksLikeCommitSHA reports whether ref is exactly 40 hex characters --
// the one ref shape that resolves without ever touching a loose-ref file,
// so refPathPermissionDenied has nothing to probe for it.
func looksLikeCommitSHA(ref string) bool {
	if len(ref) != 40 {
		return false
	}
	for _, c := range ref {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

// printNotices reports the changed files that were deliberately not
// reviewed -- binary blobs and files past the size limits documented in
// docs/specs/AUR-430.md. They print before the findings, on stdout, in the
// diff's own sorted path order, and they do not change the exit code: a
// skipped file is a normal, reportable outcome, not a failure. A run with
// nothing to skip prints nothing here, so ordinary output is unaffected.
// Notice text derives from diff paths -- repository-controlled input -- so
// it passes the redaction filter before reaching the sink (AUR-432); an
// ordinary path is filter-identity.
// The text is in the review's language (analyzer.DiffNotice.Text).
func printNotices(stdout io.Writer, filter *redaction.Filter, notices []analyzer.DiffNotice, language string) {
	for _, n := range notices {
		fmt.Fprintln(stdout, filter.Redact(n.Text(language)))
	}
}
