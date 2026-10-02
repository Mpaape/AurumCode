// The verified identity (owner/repo) of the local checkout, read from the
// "origin" remote. Exceptions and verdict reuse are keyed on it; the matching
// itself lives in internal/gate.
package main

import "strings"

// localRepoIdentity returns the --base path's own "owner/repo" identity,
// derived the exact same read-only way aur515.go's codebaseContextMismatch
// already derives it for --pr's local-checkout verification: the
// repository's configured "origin" remote, parsed for the owner/repo it
// names, never anything the reviewed diff or a provider reply could
// influence. ok is false when no origin remote could be read or parsed
// at all -- a repo with no configured remote, a detached object store, or
// any other reason the identity cannot be confirmed -- and every caller
// must then pass "" as the exception match's repoIdentity, which
// matchException already treats as never matching a configured
// exception's required, non-empty Repo field (fail closed, AC-003's own
// "repo that cannot be determined" case).
func localRepoIdentity(dir string) (identity string, ok bool) {
	remoteURL, err := originRemoteURL(dir)
	if err != nil || strings.TrimSpace(remoteURL) == "" {
		return "", false
	}
	owner, repo, matched := ownerRepoFromRemoteURL(remoteURL)
	if !matched {
		return "", false
	}
	return owner + "/" + repo, true
}
