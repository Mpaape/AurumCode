// AUR-438: the PR review path for `aurumcode review`.
//
//	aurumcode review --pr <numero> --repo <dono>/<projeto> --publicar --na-linha
//
// reads a pull request's changes through the restored GitHub client
// (AUR-437, internal/git/githubclient), runs them through the exact same
// review engine and provider selection the --base path already uses
// (internal/review.Reviewer, selectProvider), and publishes every finding
// as a pull request comment: at the file's exact changed line when the
// diff actually added that line, or as a general pull request comment when
// the finding sits outside the changed lines -- so a real finding is never
// silently dropped just because it cannot be anchored to a line the diff
// touched (see MUT-001), and one finding's failure to post never costs
// another finding its own comment (the publish loop aggregates failures
// instead of aborting on the first one). Publishing refuses, before any
// comment is posted, when the token lacks write permission on the
// repository -- the same fail-closed guarantee AUR-437 already proved for
// the client itself (internal/git/githubclient.HasWritePermission) -- and
// refuses just as fail-closed when an inline comment would need a commit
// SHA that is not available (GITHUB_SHA unset): never a POST built with an
// empty commit_id.
//
// This file owns only the wiring: flag handling, the diff-shape conversion
// from the client's package-local types to pkg/types (the client
// deliberately cannot import pkg/types itself -- see
// internal/git/githubclient/diff.go), the changed/unchanged-line
// classification, and the publish loop. It reuses the client and the
// engine exactly as they already exist; see docs/specs/AUR-438.md.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/Mpaape/AurumCode/internal/git/githubclient"
	"github.com/Mpaape/AurumCode/internal/llm"
	"github.com/Mpaape/AurumCode/internal/prompt"
	"github.com/Mpaape/AurumCode/internal/review"
	"github.com/Mpaape/AurumCode/pkg/types"
)

// runPRReview is reached only when --pr was explicitly given (see the
// fs.Visit dispatch in runReview); every other flag's published behavior
// is therefore untouched by this function's existence.
//
// --repo, --publicar and --na-linha are all required here, mirroring how
// --base is already required on the base-diff path: this card builds and
// tests exactly the one command the card declares
// (`review --pr N --repo o/r --publicar --na-linha`), and Non-goals rules
// out inventing an alternate, unobserved behavior for what any of these
// three flags would mean if omitted.
func runPRReview(stdout, stderr io.Writer, prNumber int, repoFlag string, publicar, naLinha bool) int {
	if repoFlag == "" {
		fmt.Fprintln(stderr, "aurumcode review: --repo is required with --pr")
		return 2
	}
	if !naLinha {
		fmt.Fprintln(stderr, "aurumcode review: --na-linha is required with --pr")
		return 2
	}
	if !publicar {
		fmt.Fprintln(stderr, "aurumcode review: --publicar is required with --pr")
		return 2
	}
	if prNumber <= 0 {
		fmt.Fprintln(stderr, "aurumcode review: --pr must be a positive pull request number")
		return 2
	}
	owner, repoName, err := parseOwnerRepo(repoFlag)
	if err != nil {
		fmt.Fprintf(stderr, "aurumcode review: --repo: %v\n", err)
		return 2
	}

	ctx := context.Background()
	client := newGitHubClient()

	// Publishing gate, checked first -- before the diff fetch and before
	// any LLM call -- so a read-only token is refused without spending
	// either. Refuse fail-closed, exactly as
	// internal/git/githubclient.PostReviewComment and PostIssueComment
	// already refuse internally, but as a single clear message instead of
	// one failed POST per finding. Moved here (was: checked only after
	// GenerateReview) because a token that cannot publish must not still
	// pay for a model call it can never use the result of.
	ok, err := client.HasWritePermission(ctx, owner, repoName)
	if err != nil {
		fmt.Fprintf(stderr, "aurumcode review: checking write permission on %s/%s: %v\n", owner, repoName, err)
		return 1
	}
	if !ok {
		fmt.Fprintf(stderr, "aurumcode review: refusing to publish to %s/%s: %v\n", owner, repoName, githubclient.ErrNoWritePermission)
		return 1
	}

	ghDiff, err := client.GetPullRequestDiff(ctx, owner, repoName, prNumber)
	if err != nil {
		fmt.Fprintf(stderr, "aurumcode review: fetching pull request diff: %v\n", err)
		return 1
	}
	diff := convertDiff(ghDiff)

	// Provider selection is exactly the --base path's selectProvider: no
	// --modelo/--fail-on/--seguranca composition is declared for the PR
	// path by this card, so none is wired here (Non-goals).
	provider, err := selectProvider()
	if err != nil {
		fmt.Fprintf(stderr, "aurumcode review: %v\n", err)
		return 1
	}

	orchestrator := llm.NewOrchestrator(provider, nil, nil)
	reviewer := review.NewReviewer(orchestrator, review.DefaultConfig())

	result, err := reviewer.GenerateReview(ctx, diff)
	if err != nil {
		var parseErr *prompt.ParseError
		if errors.As(err, &parseErr) {
			fmt.Fprintf(stderr, "aurumcode review: could not understand the model's response (%s)\n", parseErr.Kind)
			return 1
		}
		fmt.Fprintf(stderr, "aurumcode review: %v\n", err)
		return 1
	}

	if len(result.Issues) == 0 {
		fmt.Fprintln(stdout, "No issues found.")
		return 0
	}

	// The engine already redacted every model-authored field on result
	// (internal/review.redactReviewResult, called inside GenerateReview
	// before it returns -- AUR-432). The comment bodies below are built
	// only from those already-redacted fields plus trusted literals (the
	// "[severity]" tag, "publicado..." suffixes), exactly like
	// printFindings' own comment explains for stdout, so nothing here
	// needs a second pass through the filter.
	issues := sortedIssues(result.Issues)

	// GITHUB_SHA is the standard GitHub Actions convention for "the commit
	// under review" (see docs/specs/AUR-438.md's Compatibility note): the
	// restored client's GetPullRequestDiff reads the unified diff format,
	// which carries no commit SHA, and this card does not extend the
	// client (internal/git/githubclient is a read_path here) to add an
	// endpoint that would fetch one.
	//
	// A real GitHub review-comment POST with an empty commit_id is
	// rejected (422): an inline comment cannot be anchored to no commit at
	// all. So when at least one finding needs an inline comment and no SHA
	// is available, this refuses before any comment -- inline or general
	// -- is posted, the same fail-closed shape as the permission check
	// above, never a POST built with an empty commit_id. A run whose
	// findings are all general (nothing anchors to a specific added line)
	// does not need a commit SHA at all -- PostIssueComment carries no
	// commit_id -- so it is not blocked by this gate.
	commitID := os.Getenv("GITHUB_SHA")
	needsCommitID := false
	for _, issue := range issues {
		if isInlineEligible(diff, issue) {
			needsCommitID = true
			break
		}
	}
	if needsCommitID && commitID == "" {
		fmt.Fprintln(stderr, "aurumcode review: refusing to publish: an inline comment requires a commit SHA and none is available; set GITHUB_SHA")
		return 1
	}

	// The publish loop never lets one finding's POST failure swallow the
	// rest: a failure is recorded and the loop continues, so an inline
	// comment that fails to post does not also cost the general comment
	// for a different, unrelated finding (or vice versa). Every recorded
	// failure is reported after the loop, and the command exits non-zero
	// if any occurred -- but every finding that COULD be published still
	// was.
	var failures []string
	inlineCount, generalCount := 0, 0
	for _, issue := range issues {
		line := fmt.Sprintf("%s:%d: [%s] %s", issue.File, issue.Line, issue.Severity, issue.Message)
		if isInlineEligible(diff, issue) {
			comment := githubclient.ReviewComment{
				Body:     fmt.Sprintf("[%s] %s", issue.Severity, issue.Message),
				CommitID: commitID,
				Path:     issue.File,
				Line:     issue.Line,
			}
			key := fmt.Sprintf("aurumcode/%d/%s/%d/%s", prNumber, issue.File, issue.Line, issue.RuleID)
			if err := client.PostReviewComment(ctx, owner, repoName, prNumber, comment, key); err != nil {
				fmt.Fprintf(stderr, "aurumcode review: publishing inline comment on %s:%d: %v\n", issue.File, issue.Line, err)
				failures = append(failures, fmt.Sprintf("%s:%d (na linha): %v", issue.File, issue.Line, err))
				continue
			}
			fmt.Fprintf(stdout, "%s -- publicado na linha\n", line)
			inlineCount++
			continue
		}

		// Outside the changed lines: still published, as a general pull
		// request comment, never dropped. MUT-001 mutates exactly this
		// branch.
		if err := client.PostIssueComment(ctx, owner, repoName, prNumber, line); err != nil {
			fmt.Fprintf(stderr, "aurumcode review: publishing general comment for %s:%d: %v\n", issue.File, issue.Line, err)
			failures = append(failures, fmt.Sprintf("%s:%d (geral): %v", issue.File, issue.Line, err))
			continue
		}
		fmt.Fprintf(stdout, "%s -- publicado como comentario geral (fora das linhas alteradas)\n", line)
		generalCount++
	}

	fmt.Fprintf(stdout, "%d comentario(s) publicado(s) no pull request #%d (%d na linha, %d geral).\n",
		inlineCount+generalCount, prNumber, inlineCount, generalCount)

	if len(failures) > 0 {
		fmt.Fprintf(stderr, "aurumcode review: %d comentario(s) falharam ao publicar:\n", len(failures))
		for _, f := range failures {
			fmt.Fprintf(stderr, "  %s\n", f)
		}
		return 1
	}
	return 0
}

// newGitHubClient builds the restored AUR-437 client.
// AURUMCODE_GITHUB_API_URL overrides the API base so this card's own tests
// can point it at a loopback httptest server (the sealed profile denies
// real network access); production use leaves it unset and gets
// githubclient.DefaultBaseURL. GITHUB_TOKEN follows the same convention
// GitHub Actions already exposes to a step (see docs/specs/AUR-440.md); an
// empty token still builds a working client -- reading a public repository
// needs no auth -- and every publish call still refuses fail-closed without
// write permission regardless of whether a token was supplied at all.
func newGitHubClient() *githubclient.Client {
	token := os.Getenv("GITHUB_TOKEN")
	if base := os.Getenv("AURUMCODE_GITHUB_API_URL"); base != "" {
		return githubclient.NewClientWithBaseURL(token, base)
	}
	return githubclient.NewClient(token)
}

// parseOwnerRepo splits the --repo flag's "owner/repo" form. Anything else
// -- no slash, an empty owner or repo segment, or more than one slash -- is
// a usage error.
func parseOwnerRepo(repo string) (owner, name string, err error) {
	idx := strings.IndexByte(repo, '/')
	if idx <= 0 || idx == len(repo)-1 {
		return "", "", fmt.Errorf("expected the form owner/repo, got %q", repo)
	}
	owner, name = repo[:idx], repo[idx+1:]
	if strings.IndexByte(name, '/') != -1 {
		return "", "", fmt.Errorf("expected the form owner/repo, got %q", repo)
	}
	return owner, name, nil
}

// convertDiff mirrors internal/git/githubclient's package-local diff shape
// into pkg/types.Diff, field by field. The client's Diff/DiffFile/DiffHunk
// are deliberately local to its own package -- AUR-437's sealed acceptance
// materializes only that package's declared paths/read_paths, so it cannot
// import pkg/types -- and the two shapes mirror each other exactly (same
// field names, same meaning) precisely so this conversion, owned by this
// card, is lossless and trivial at the one boundary where a caller holds
// both packages.
func convertDiff(d *githubclient.Diff) *types.Diff {
	out := &types.Diff{Files: make([]types.DiffFile, len(d.Files))}
	for i, f := range d.Files {
		hunks := make([]types.DiffHunk, len(f.Hunks))
		for j, h := range f.Hunks {
			lines := make([]string, len(h.Lines))
			copy(lines, h.Lines)
			hunks[j] = types.DiffHunk{
				OldStart: h.OldStart,
				OldLines: h.OldLines,
				NewStart: h.NewStart,
				NewLines: h.NewLines,
				Lines:    lines,
			}
		}
		out.Files[i] = types.DiffFile{Path: f.Path, Lang: f.Lang, Hunks: hunks}
	}
	return out
}

// addedLineNumbers returns the set of new-side line numbers hunk h adds.
// The restored client's parser (internal/git/githubclient/client.go,
// parseDiff) keeps every hunk content line's leading +/-/space marker: an
// added ('+') line is recorded at the current new-file counter and
// advances it; a context (' ') line only advances it; a removed ('-') line
// does neither, because it has no line on the new side to advance past or
// to anchor a comment to.
func addedLineNumbers(h types.DiffHunk) map[int]bool {
	added := make(map[int]bool)
	n := h.NewStart
	for _, l := range h.Lines {
		if l == "" {
			continue
		}
		switch l[0] {
		case '+':
			added[n] = true
			n++
		case ' ':
			n++
		}
	}
	return added
}

// isInlineEligible reports whether issue names a file and line this diff
// actually added. Only an added line can carry an exact-line pull request
// review comment (AC-001's "na linha exata"); a finding on any other line
// -- a file the diff never touched, a context/unchanged line, or a line
// number past every hunk -- is not dropped, it is published as a general
// comment instead (see runPRReview and MUT-001).
func isInlineEligible(diff *types.Diff, issue types.ReviewIssue) bool {
	for _, f := range diff.Files {
		if f.Path != issue.File {
			continue
		}
		for _, h := range f.Hunks {
			if addedLineNumbers(h)[issue.Line] {
				return true
			}
		}
	}
	return false
}

// sortedIssues returns a copy of issues ordered by (file, line) -- the same
// order printFindings already uses for the --base contract -- so the
// sequence of PostReviewComment/PostIssueComment calls this card makes is
// deterministic. AC-001 requires that repeating the same input produces the
// same output; for the PR path that output includes the publish
// transcript (what got posted, in what order), not only stdout.
func sortedIssues(issues []types.ReviewIssue) []types.ReviewIssue {
	out := make([]types.ReviewIssue, len(issues))
	copy(out, issues)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].File != out[j].File {
			return out[i].File < out[j].File
		}
		return out[i].Line < out[j].Line
	})
	return out
}
