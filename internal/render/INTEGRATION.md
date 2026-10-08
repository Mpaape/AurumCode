# Render integration

`internal/render` holds the deterministic renderings a review publishes
beside the parecer:

1. `render.AuditRecord` and `render.SARIF*` write the audit record and the
   SARIF report of a run.
2. `render.FindingIdentityFor` / `render.FindingFingerprint` give a finding
   the identity rounds and caches recognize it by.

The parecer itself (the decision headline, what to fix, the observations,
the summary and the collapsed details) is rendered by `cmd/aurumcode`
(`pr_summary_format.go`); the terminal report of `--base` opens with the
same head.
