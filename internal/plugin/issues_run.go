package plugin

import (
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/Mpaape/AurumCode/internal/i18n"
	"github.com/Mpaape/AurumCode/internal/security/redaction"
)

var repoPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

// IssuesOptions is one `aurumcode plugin issues`: the plugin, the SARIF its
// ingest wrote and the repository (OWNER/REPO) whose issues are synced.
type IssuesOptions struct {
	Name, SARIF, Repo string
	Filter            *redaction.Filter
}

// SyncIssues is the whole destination, in this order, so a refusal never
// writes:
//  1. the SARIF is read locally; an inconclusive scan (no SARIF, not a
//     successful run of this plugin) makes NO request at all;
//  2. the visibility is read from the API; a public repository, or one
//     whose visibility cannot be read, is refused before any write;
//  3. the plugin's issues are listed (every page) and the plan is applied.
func SyncIssues(ctx context.Context, api IssuesAPI, o IssuesOptions) (IssueCounts, error) {
	if !namePattern.MatchString(o.Name) || o.SARIF == "" || !repoPattern.MatchString(o.Repo) {
		return IssueCounts{}, &UsageError{Msg: "--name, --sarif e --repo OWNER/REPO são obrigatórios"}
	}
	findings, conclusive, err := LoadIssueFindings(o.SARIF, o.Name, o.Filter)
	if err != nil {
		return IssueCounts{}, ErrSARIFInconclusive
	}
	if !conclusive {
		return IssueCounts{}, ErrSARIFInconclusive
	}
	visibility, private, err := api.Visibility(ctx)
	if err != nil {
		return IssueCounts{}, ErrVisibilityUnreadable
	}
	if !IssuesAllowed(visibility, private) {
		return IssueCounts{}, ErrPublicIssues
	}
	label := IssueLabel(o.Name)
	if err := api.EnsureLabel(ctx, label); err != nil {
		return IssueCounts{}, err
	}
	issues, err := api.ListIssues(ctx, label)
	if err != nil {
		return IssueCounts{}, err
	}
	plan, counts := planIssues(o.Name, findings, issues)
	if err := applyIssues(ctx, api, o.Name, plan); err != nil {
		return IssueCounts{}, err
	}
	return counts, nil
}

// ErrVisibilityUnreadable refuses a repository whose visibility the API did
// not give: it is treated as public.
var ErrVisibilityUnreadable = errors.New("visibilidade do repositório ilegível: tratado como público, nenhuma issue publicada")

// IssuesAllowed is true only for a repository the API says is private or
// internal (and private): anything else may be readable by anyone.
func IssuesAllowed(visibility string, private bool) bool {
	return CheckDestination(visibility, DestinationIssues) == nil && private
}

// RunIssues prints ONLY counts or a fixed reason, never a title, path or
// description. Exit: 0 synced, 1 refused, inconclusive or API failure, 2
// usage.
func RunIssues(ctx context.Context, api IssuesAPI, o IssuesOptions, language string, stdout, stderr io.Writer) int {
	c, err := SyncIssues(ctx, api, o)
	var usage *UsageError
	switch {
	case err == nil:
		fmt.Fprintln(stdout, i18n.Format(language, "plugin.issues_summary", o.Name, c.Opened, c.Kept, c.Closed, c.Reopened))
		return 0
	case errors.As(err, &usage):
		fmt.Fprintf(stderr, "aurumcode plugin issues: %s\n", usage.Msg)
		return 2
	case errors.Is(err, ErrSARIFInconclusive):
		fmt.Fprintf(stderr, "aurumcode plugin issues: %s\n", i18n.Format(language, "plugin.issues_inconclusive", o.Name))
	case errors.Is(err, ErrPublicIssues), errors.Is(err, ErrVisibilityUnreadable):
		fmt.Fprintf(stderr, "aurumcode plugin issues: %s\n", i18n.Format(language, "plugin.issues_refused", o.Name))
	default:
		fmt.Fprintf(stderr, "aurumcode plugin issues: %s\n", i18n.Format(language, "plugin.issues_failed", o.Name, strings.TrimSpace(err.Error())))
	}
	return 1
}
