// AUR-515: the --pr path's codebase-context pass (resolveCodebaseContext,
// passes.go) reads whatever checkout the process happens to be running
// from. On --base that checkout IS the thing being reviewed, by
// construction. On --pr it is not: a direct CLI invocation, a monorepo
// runner, or a stale checkout can reach `aurumcode review --pr` from a
// directory that is a different repository entirely, or the right
// repository at a different commit. Sending that checkout's files to the
// provider as "codebase context" would leak unrelated or stale code into
// the prompt under the reviewed pull request's name.
//
// codebaseContextMismatch verifies, read-only and before any context is
// resolved, that the local checkout IS the exact repository named by
// --repo, sitting at the exact commit GitHub reports as the pull request's
// head (GetPullRequestMetadata.HeadSHA, via the already-existing
// resolvePullRequestHeadSHA). The repository check runs first and is
// entirely local (no network): only once it passes does this call out to
// the GitHub API at all, so an unrelated checkout never causes an extra
// remote call. Any mismatch -- a different repository, a divergent HEAD,
// or simply being unable to tell -- fails closed: the caller omits the
// codebase context and records why as a published review limitation,
// never a fatal error. The remote diff review itself is entirely
// unaffected: it never depended on the local checkout at all.
//
// The official reusable workflow (.github/workflows/review.yml) checks out
// the pull request's own head SHA into the exact directory it then mounts
// as the container's working directory, so this check passes there
// unchanged and codebase context keeps flowing exactly as before.
//
// HEAD is read through internal/analyzer.OpenRepo/ResolveRef -- the same
// dual-path reader (git binary when present, a pure-Go loose-object/ref
// reader otherwise) the rest of this codebase already relies on for
// sealed, network-denied environments with no git binary at all. The
// origin remote is read the same way git itself would locate it: by
// walking up from the working directory for a ".git" entry, following a
// linked worktree's "gitdir:"/"commondir" indirection to the shared
// config, and parsing only the "[remote \"origin\"]" section's url --
// never requiring a git binary either.
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Mpaape/AurumCode/internal/analyzer"
	"github.com/Mpaape/AurumCode/internal/git/githubclient"
)

// codebaseContextMismatch returns "" when the local checkout is verified as
// owner/repoName at the pull request's reviewed head commit; otherwise it
// returns a short, fixed reason code ("repository", "head" or
// "unverifiable") naming why it could not be trusted. The reason is a
// constant from this function, never remote or local repository text, so it
// carries nothing that needs redaction.
func codebaseContextMismatch(ctx context.Context, client *githubclient.Client, owner, repoName string, prNumber int) string {
	dir, err := os.Getwd()
	if err != nil {
		return "unverifiable"
	}
	localOwner, localRepo, localHead, err := localCheckoutIdentity(dir)
	if err != nil {
		return "unverifiable"
	}
	// Local, free check first: only a repository that already claims to be
	// the right one ever reaches the GitHub API below.
	if !strings.EqualFold(localOwner, owner) || !strings.EqualFold(localRepo, repoName) {
		return "repository"
	}
	headSHA, err := resolvePullRequestHeadSHA(ctx, client, owner, repoName, prNumber)
	if err != nil || strings.TrimSpace(headSHA) == "" {
		return "unverifiable"
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
	ptBR := language == "pt-BR" || language == "pt"
	var why string
	switch reason {
	case "repository":
		if ptBR {
			why = "o checkout local não é o repositório revisado"
		} else {
			why = "the local checkout is not the reviewed repository"
		}
	case "head":
		if ptBR {
			why = "o HEAD do checkout local não corresponde ao commit revisado do pull request"
		} else {
			why = "the local checkout's HEAD does not match the pull request's reviewed commit"
		}
	default:
		if ptBR {
			why = "não foi possível confirmar a identidade do checkout local"
		} else {
			why = "the local checkout's identity could not be confirmed"
		}
	}
	if ptBR {
		return fmt.Sprintf("Contexto do repositório omitido: %s; esta revisão não envia o conteúdo do checkout local ao provedor e considera apenas o diff remoto.", why)
	}
	return fmt.Sprintf("Repository context omitted: %s; this review does not send the local checkout's content to the provider and considers only the remote diff.", why)
}
