// The --pr path's checkout identity: the local codebase context is sent to the
// provider only when the local checkout is verified to be the reviewed
// repository at the pull request's reviewed head commit, with no uncommitted
// or untracked content; otherwise the context is omitted and the omission is
// declared as a review limitation.
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Mpaape/AurumCode/internal/analyzer"
	"github.com/Mpaape/AurumCode/internal/git/githubclient"
	"github.com/Mpaape/AurumCode/internal/i18n"
)

// codebaseContextReasonUnverifiable is the fixed reason code returned
// whenever this checkout's identity, HEAD, or (AUR-536) clean-tree state
// simply could not be confirmed -- as opposed to "repository"/"head", where
// it WAS confirmed and found to mismatch, or "dirty" (aur536.go), where it
// was confirmed and found unclean. Hoisted into one constant so every
// "cannot tell" return agrees, and so a single, stable edit can flip them
// all for a skeptical mutation.
const codebaseContextReasonUnverifiable = "unverifiable"

// codebaseContextMismatch returns "" when the local checkout is verified as
// owner/repoName at the pull request's reviewed head commit; otherwise it
// returns a short, fixed reason code ("repository", "head" or
// codebaseContextReasonUnverifiable) naming why it could not be trusted.
// The reason is a constant from this function, never remote or local
// repository text, so it carries nothing that needs redaction.
func codebaseContextMismatch(ctx context.Context, client *githubclient.Client, owner, repoName string, prNumber int) string {
	dir, err := os.Getwd()
	if err != nil {
		return codebaseContextReasonUnverifiable
	}
	localOwner, localRepo, localHead, err := localCheckoutIdentity(dir)
	if err != nil {
		return codebaseContextReasonUnverifiable
	}
	// Local, free check first: only a repository that already claims to be
	// the right one ever reaches the GitHub API below.
	if !strings.EqualFold(localOwner, owner) || !strings.EqualFold(localRepo, repoName) {
		return "repository"
	}
	headSHA, err := resolvePullRequestHeadSHA(ctx, client, owner, repoName, prNumber)
	if err != nil || strings.TrimSpace(headSHA) == "" {
		return codebaseContextReasonUnverifiable
	}
	if !strings.EqualFold(localHead, strings.TrimSpace(headSHA)) {
		return "head"
	}
	return ""
}

// localCheckoutIdentity reads dir's git remote ("origin") and HEAD commit,
// entirely read-only and without requiring a git binary on PATH. Neither
// read writes the repository and neither fetches: both are local.
func localCheckoutIdentity(dir string) (owner, repo, headSHA string, err error) {
	remoteURL, cfgErr := originRemoteURL(dir)
	if cfgErr != nil || strings.TrimSpace(remoteURL) == "" {
		return "", "", "", fmt.Errorf("no readable origin remote: %w", cfgErr)
	}
	o, r, ok := ownerRepoFromRemoteURL(remoteURL)
	if !ok {
		return "", "", "", fmt.Errorf("origin remote does not name an owner/repo")
	}
	repoReader, openErr := analyzer.OpenRepo(dir)
	if openErr != nil {
		return "", "", "", fmt.Errorf("opening local checkout: %w", openErr)
	}
	head, headErr := repoReader.ResolveRef("HEAD")
	if headErr != nil || strings.TrimSpace(head) == "" {
		return "", "", "", fmt.Errorf("no resolvable HEAD: %w", headErr)
	}
	return o, r, strings.ToLower(strings.TrimSpace(head)), nil
}

// originRemoteURL returns the raw "remote.origin.url" value for the
// repository containing dir, found the same way git itself discovers it:
// walking up from dir for a ".git" entry (a real repository root is never
// more than a handful of levels up), following a linked worktree's
// "gitdir:" file to its gitdir and then that gitdir's own "commondir" file
// to the shared repository config, and parsing only the
// `[remote "origin"]` section's `url` line. A bare repository (dir itself
// holding HEAD/objects/refs) is also recognized.
func originRemoteURL(dir string) (string, error) {
	configPath, err := gitConfigPath(dir)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		return "", err
	}
	inOrigin := false
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "[") {
			inOrigin = strings.EqualFold(line, `[remote "origin"]`)
			continue
		}
		if !inOrigin {
			continue
		}
		if key, value, ok := strings.Cut(line, "="); ok && strings.TrimSpace(key) == "url" {
			return strings.TrimSpace(value), nil
		}
	}
	return "", fmt.Errorf("no remote.origin.url in %s", configPath)
}

// gitConfigPath resolves the config file that governs dir's repository,
// walking up at most maxGitConfigWalk levels for a ".git" entry.
func gitConfigPath(dir string) (string, error) {
	cur := dir
	for i := 0; i < maxGitConfigWalk; i++ {
		candidate := filepath.Join(cur, ".git")
		info, err := os.Stat(candidate)
		if err == nil {
			if info.IsDir() {
				return filepath.Join(candidate, "config"), nil
			}
			return worktreeConfigPath(candidate, cur)
		}
		if looksLikeBareGitDir(cur) {
			return filepath.Join(cur, "config"), nil
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			break
		}
		cur = parent
	}
	return "", fmt.Errorf("no .git found walking up from %s", dir)
}

// maxGitConfigWalk bounds the upward directory walk in gitConfigPath so a
// pathological or unexpected cwd cannot make this an unbounded scan.
const maxGitConfigWalk = 64

// looksLikeBareGitDir reports whether dir itself looks like a git
// directory (HEAD, objects and refs all present), the bare-repository
// shape internal/analyzer's own reader recognizes.
func looksLikeBareGitDir(dir string) bool {
	for _, must := range []string{"HEAD", "objects", "refs"} {
		if _, err := os.Stat(filepath.Join(dir, must)); err != nil {
			return false
		}
	}
	return true
}

// worktreeConfigPath follows a linked worktree's ".git" FILE (its content
// is "gitdir: <path to .git/worktrees/<name>>") to that worktree's own
// gitdir, then to the "commondir" file it carries, which names the shared
// repository directory -- the one whose config actually holds the
// configured remotes. A gitdir with no commondir file is treated as the
// config location directly (covers a submodule's own gitdir shape).
func worktreeConfigPath(gitFile, root string) (string, error) {
	data, err := os.ReadFile(gitFile)
	if err != nil {
		return "", err
	}
	line := strings.TrimSpace(string(data))
	const prefix = "gitdir:"
	if !strings.HasPrefix(line, prefix) {
		return "", fmt.Errorf("unrecognized .git file %s", gitFile)
	}
	gitDir := strings.TrimSpace(strings.TrimPrefix(line, prefix))
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(root, gitDir)
	}
	commonData, err := os.ReadFile(filepath.Join(gitDir, "commondir"))
	if err != nil {
		// No commondir file: treat gitDir itself as holding the config.
		return filepath.Join(gitDir, "config"), nil
	}
	commonRel := strings.TrimSpace(string(commonData))
	commonDir := commonRel
	if !filepath.IsAbs(commonRel) {
		commonDir = filepath.Clean(filepath.Join(gitDir, commonRel))
	}
	return filepath.Join(commonDir, "config"), nil
}

// ownerRepoFromRemoteURL extracts the "owner/repo" named by a git remote
// URL, accepting the https, ssh and scp-like ("git@host:owner/repo") forms
// GitHub and compatible hosts issue, with or without a trailing ".git" and
// with or without embedded userinfo (a token in the URL never reaches the
// returned owner/repo, and the raw URL itself is never returned or logged).
func ownerRepoFromRemoteURL(raw string) (owner, repo string, ok bool) {
	s := strings.TrimSpace(raw)
	s = strings.TrimSuffix(s, "/")
	s = strings.TrimSuffix(s, ".git")
	switch {
	case strings.Contains(s, "://"):
		s = s[strings.Index(s, "://")+3:]
		if at := strings.LastIndex(s, "@"); at >= 0 {
			s = s[at+1:]
		}
		slash := strings.Index(s, "/")
		if slash < 0 {
			return "", "", false
		}
		s = s[slash+1:]
	case strings.Contains(s, "@") && strings.Contains(s[strings.Index(s, "@"):], ":"):
		s = s[strings.Index(s, "@")+1:]
		s = s[strings.Index(s, ":")+1:]
	case strings.Contains(s, ":") && !strings.Contains(s[:strings.Index(s, ":")], "/"):
		s = s[strings.Index(s, ":")+1:]
	}
	s = strings.TrimPrefix(s, "/")
	parts := strings.Split(s, "/")
	if len(parts) < 2 {
		return "", "", false
	}
	owner, repo = parts[len(parts)-2], parts[len(parts)-1]
	if owner == "" || repo == "" {
		return "", "", false
	}
	return owner, repo, true
}

// codebaseContextOmittedNotice is the declared limitation published when
// the --pr path's codebase context was omitted because the local checkout
// could not be verified as the reviewed repository and commit (AC-001,
// AC-002). reason is one of codebaseContextMismatch's own fixed codes,
// never model- or remote-authored text, so this needs no redaction.
func codebaseContextOmittedNotice(language, reason string) string {
	return i18n.Format(language, "notice.context_omitted", contextOmittedReason(language, reason))
}

// contextOmittedReason is the catalog text of one codebaseContextMismatch code.
func contextOmittedReason(language, reason string) string {
	switch reason {
	case "repository":
		return i18n.Text(language, "notice.context_omitted.repository")
	case "head":
		return i18n.Text(language, "notice.context_omitted.head")
	case codebaseContextReasonDirty:
		return i18n.Text(language, "notice.context_omitted.dirty")
	}
	return i18n.Text(language, "notice.context_omitted.unconfirmed")
}
