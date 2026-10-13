// The permission a publication to GitHub needs, named when GitHub denies
// it: the workflow token's scope is the one thing the operator can change,
// so a refused POST says which permission to grant and where.
package main

import (
	"github.com/Mpaape/AurumCode/internal/git/githubclient"
	"github.com/Mpaape/AurumCode/internal/i18n"
)

// publishAction is what a publication does, by the workflow permission it
// needs. The product writes comments, reviews and commit statuses to
// GitHub; nothing else is posted.
type publishAction int

const (
	// publishComment posts or edits the parecer, an inline comment or a
	// formal review: pull-requests: write.
	publishComment publishAction = iota
	// publishStatus posts a commit status: statuses: write.
	publishStatus
)

// permission is the `permissions:` entry the action needs.
func (a publishAction) permission() string {
	if a == publishStatus {
		return "statuses: write"
	}
	return "pull-requests: write"
}

// publishError is the diagnosis of a failed publication. A permission
// denial (401 or 403 from GitHub) names the permission the token lacks and
// where to grant it, keeping GitHub's own answer; any other error is
// reported as it is, so nothing else changes.
func publishError(language string, action publishAction, err error) string {
	if !githubclient.IsPermissionDenied(err) {
		return err.Error()
	}
	return i18n.Format(language, "terminal.publish_denied", action.permission(), err)
}
