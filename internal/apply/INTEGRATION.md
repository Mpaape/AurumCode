# Integrate `internal/apply`

1. Import `internal/apply`; feed it the parsed `[]types.ReviewSuggestion` from a review result.
2. CLI: add an `aurumcode fix` subcommand that calls `apply.BuildPatch(suggestions, os.DirFS(dir))` (the hunks carry three lines of real file context read from `dir`), writes the returned patch to stdout or a file, and (optionally) pipes it to `git apply --check`.
3. GitHub: for each `FileEdit` in `apply.BuildPlan(suggestions, fsys)`, post a review comment on `File` anchored at the edit's start line, with the body rendered from the `Hunk` text.
4. Only render suggestions that survive `BuildPlan`; skipped entries are unsafe and must not be published.
