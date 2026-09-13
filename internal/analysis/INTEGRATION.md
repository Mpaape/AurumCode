# Integration: deterministic analysis pass

1. `r := analysis.NewRunner()` once; it is zero-config and concurrency-safe.
2. Merge `r.Analyze(diff)` with LLM findings: map each `Finding` to
   `types.ReviewIssue{File: Path, Line, Side, Severity, RuleID, Message}`,
   then dedupe against model issues by `(File, Line, Side, RuleID)`.
3. For `r.Vet(ctx, dir, run)`, pass a sandboxed `commandRunner`, e.g. one
   wrapping `exec.CommandContext(ctx, "go", "vet", "./...")` in the container;
   merge its `go-vet` findings the same way.
4. Sort the merged list by `(File, Line, Side, RuleID)` for stable output.
