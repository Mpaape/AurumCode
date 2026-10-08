package main

import (
	"context"
	"encoding/json"

	"github.com/Mpaape/AurumCode/internal/config"
	"github.com/Mpaape/AurumCode/internal/git/githubclient"
	"github.com/Mpaape/AurumCode/internal/i18n"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
)

// pullRequestHistoryContext reads GitHub's existing conversation, not a second
// database or model-authored memory. It cannot resolve threads, suppress an
// issue, alter a rule or authorize publication. Even an APPROVED review is only
// an attributed observation to compare with the current patch. The redacted
// entries are returned too: the publication reads the earlier rounds'
// finding markers from them (pr_rounds.go) without a second request.
func pullRequestHistoryContext(ctx context.Context, client *githubclient.Client, owner, repo string, number int, head, base string, filter *redaction.Filter) (string, []githubclient.ReviewHistoryEntry, error) {
	ctx, cancel := context.WithTimeout(ctx, config.ProviderTimeout)
	defer cancel()
	entries, err := client.GetPullRequestHistory(ctx, owner, repo, number)
	if err != nil {
		return "", nil, err
	}
	if len(entries) == 0 {
		return "", entries, nil
	}
	for i := range entries {
		e := &entries[i]
		// Redact before JSON encoding, while multiline headers are still
		// actual lines rather than escaped JSON. Redact all textual fields,
		// including author/path metadata: remote data is not implicitly safe.
		e.Kind = filter.Redact(e.Kind)
		e.Body = filter.Redact(e.Body)
		e.Author = filter.Redact(e.Author)
		e.State = filter.Redact(e.State)
		e.CommitID = filter.Redact(e.CommitID)
		e.OriginalCommitID = filter.Redact(e.OriginalCommitID)
		e.Path = filter.Redact(e.Path)
		e.Side = filter.Redact(e.Side)
	}
	payload := struct {
		Repository string                            `json:"repository"`
		PR         int                               `json:"pull_request"`
		Head       string                            `json:"current_head"`
		Base       string                            `json:"current_base"`
		Entries    []githubclient.ReviewHistoryEntry `json:"observations"`
	}{filter.Redact(owner + "/" + repo), number, filter.Redact(head), filter.Redact(base), entries}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", nil, err
	}
	return string(data), entries, nil
}

func historyUnavailableNotice(language string) string {
	return i18n.Text(language, "notice.history_unavailable")
}
