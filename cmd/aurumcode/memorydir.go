package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/Mpaape/AurumCode/internal/memory"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// memoryDirFor resolves the on-disk directory internal/memory's "local" mode
// persists review notes under (AUR-489, AC-001). It never returns "": the
// old call site (memory.New(mode, "")) let an empty dir collapse onto
// os.UserCacheDir()/aurumcode/notes.json -- ONE file shared by every
// repository the command ever reviews, so a note saved while reviewing
// repo A was loaded, as an untrusted "observation", straight into repo B's
// prompt. The directory is chosen, in order of preference:
//
//  1. owner/repo, when both are known. The --pr path always has these
//     (--repo is required there), so this is the case every current
//     caller hits: readable on disk and deterministic across machines and
//     clones of the same repository.
//  2. a short hash of `git remote get-url origin`, when there is a git
//     checkout with that remote but no owner/repo (a caller other than
//     the --pr path, with no --repo flag).
//  3. a short hash of the checkout's own absolute working directory, when
//     neither of the above is available. Still per-repository and
//     deterministic on the same machine, never empty.
//
// owner and repoName are sanitized to plain filesystem-safe characters so a
// hostile value can never escape the cache root via a ".." or "/" segment.
func memoryDirFor(owner, repoName string) (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("resolve cache dir: %w", err)
	}
	root := filepath.Join(base, "aurumcode", "memory")

	owner = strings.TrimSpace(owner)
	repoName = strings.TrimSpace(repoName)
	if owner != "" && repoName != "" {
		return filepath.Join(root, "repo", sanitizeMemoryDirComponent(owner), sanitizeMemoryDirComponent(repoName)), nil
	}

	if remote, rerr := gitRemoteOriginURL(); rerr == nil && strings.TrimSpace(remote) != "" {
		return filepath.Join(root, "remote", shortMemoryHash(strings.TrimSpace(remote))), nil
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolve working directory: %w", err)
	}
	abs, err := filepath.Abs(cwd)
	if err != nil {
		return "", fmt.Errorf("resolve absolute working directory: %w", err)
	}
	return filepath.Join(root, "path", shortMemoryHash(abs)), nil
}

// gitRemoteOriginURL returns the checkout's "origin" remote URL, or an error
// when there is no git checkout, no such remote, or no git binary at all.
func gitRemoteOriginURL() (string, error) {
	out, err := exec.Command("git", "remote", "get-url", "origin").Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// shortMemoryHash renders a short, filesystem-safe, deterministic
// fingerprint of s so two runs against the same remote or path always agree
// on the same memory directory.
func shortMemoryHash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])[:16]
}

// sanitizeMemoryDirComponent reduces s to a single safe path segment: only
// ASCII letters, digits, '-', '_' and '.' survive, everything else (most
// importantly '/' and the ".." traversal segment) becomes '_'.
func sanitizeMemoryDirComponent(s string) string {
	mapped := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			return r
		default:
			return '_'
		}
	}, s)
	switch mapped {
	case "", ".", "..":
		return "_"
	default:
		return mapped
	}
}

// newRepoMemory is the ONLY way the review opens its memory store: the dir is
// derived from the repository here, so no caller can pass "" and collapse
// onto memory.go's process-wide fallback file. The wiring test opens two
// repositories through this same function and proves a note saved by one is
// invisible to the other.
func newRepoMemory(mode, owner, repoName string) (memory.Store, error) {
	dir, err := memoryDirFor(owner, repoName)
	if err != nil {
		return nil, err
	}
	return memory.New(mode, dir)
}
