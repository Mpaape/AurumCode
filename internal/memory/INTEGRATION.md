# Memory integration

1. `store, err := memory.New(cfg.Mode, cfg.Dir)` at startup; empty mode defaults to `off` (no-op).
2. Before a review: `notes, err := store.Load()` and filter by `RuleID` / `PathPattern`.
3. Fold `require` notes into the prompt as hard requirements, `suppress` as exclusions, `note` as context.
4. Never let a remembered note override the composition root's policy; notes are observations only.
5. After publication: `store.Save(append(existing, newFeedback...))` to persist new observations.
