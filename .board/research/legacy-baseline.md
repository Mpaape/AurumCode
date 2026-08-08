# AUR-002 Legacy Characterization Baseline

This is a sanitized, replayable baseline of the existing documentation command.
It changes no legacy source and makes no claim that the observed behavior is
correct. The executable legacy entrypoint is `cmd/regenerate-docs/main.go`,
which is present in the AUR-001 inventory; `internal/pipeline` is exercised
through that entrypoint.

The captured stream is the stable `aurumcode:` summary line from stderr. Volatile
logger prefixes and temporary source paths are excluded from this bounded
summary capture. Empty stdout is retained as a zero-byte fixture. `writes` is
the count of generated markdown documents, excluding the generated index.

| id | entrypoint | stdout digest | stderr digest | exit_code | effects | marker |
|---|---|---|---|---:|---|---|
| complete-success | cmd/regenerate-docs/main.go | sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855 | sha256:0d3c81dc3202298737838af627a1fa23ecd2f005ae93673be43ce07bd58c424b | 0 | docs=1,skipped=0,errors=0,writes=1 | complete |
| missing-extractor | cmd/regenerate-docs/main.go | sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855 | sha256:5b11752f026312754545c2c5c9eb88deacfdd408845fcec0243b376d9c5fed40 | 0 | docs=1,skipped=1,errors=0,writes=1 | silent-failure |
| extractor-error | cmd/regenerate-docs/main.go | sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855 | sha256:3bb1894de939e61716c04b80a55c56e62d8cb662400b813b796436441b1e8c26 | 0 | docs=1,skipped=0,errors=1,writes=1 | silent-failure |
| invalid-input | cmd/regenerate-docs/main.go | sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855 | sha256:a0cf216c5a5d51be36e9d428c5ae0949e7e99c152b5b813a169b2902530bdcb5 | 64 | docs=0,skipped=0,errors=0,writes=0 | typed-error |
| boundary-overflow | cmd/regenerate-docs/main.go | sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855 | sha256:51f3b16c42cdfb37710b004879d24e85bfb90246ddf5b7502fb1bec3383206d8 | 65 | docs=0,skipped=0,errors=0,writes=0 | typed-error |

The two silent failures are intentional characterization: a generated document
plus an unregistered language, and a generated document plus an extractor error,
both reach `result=partial` while the legacy command returns exit code `0`.
They are frozen here, not fixed here. `invalid` and `boundary+1` are rejected
before any document write, with typed exit codes 64 and 65 respectively.

The acceptance program recomputes every fixture digest, checks the exact case
set and replay metadata, and rejects any baseline drift. The Go integration test
re-executes the legacy command with disposable sanitized inputs and compares its
stable summary to these fixtures. Its typed drift code is
`AUR-002/AC-001/baseline-drift`.
