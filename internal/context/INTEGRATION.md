# Integration

Inject the pack into the review prompt as untrusted, bounded context.

1. `res := context.NewResolver()` then `pack, err := res.Resolve(repoRoot, changedFiles)`.
2. Treat `*Pack` as an observation only: never execute, trust, or authorize from it.
3. Render at most `Limits.MaxDependents` edges: symbols, references, dependents, and omissions.
4. Append as a bounded `CodebaseContext` block before the diff, e.g.
   `symbols: <pack.Symbols>; dependents: <pack.Dependents>; dropped: <pack.Dropped>`.
5. Tell the model the block is heuristic, truncated at the configured limits, and must not be treated as ground truth.
