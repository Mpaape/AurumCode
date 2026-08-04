# Acceptance program exit-code convention

Every acceptance program under `tests/acceptance/` is materialized alone into
an ephemeral, read-only container (`.board/README.md`): only the card's own
`paths`/`read_paths` and the single acceptance script travel with it. That
rules out a shared, sourced shell library as the mechanism for the
environment-vs-behavior distinction below — a `source
../lib/verdict.sh` would simply not resolve inside the sealed container. The
convention is therefore a documented, per-file idiom: every acceptance
program that can fail for an environment reason declares its own tiny
`infra()`/`fail()` (and, where useful, `inconclusive()`) functions with the
exit codes fixed here, so the *shape* is shared and reviewable even though
the bytes are duplicated.

## Exit codes

| Code | Meaning                                                                 |
|------|--------------------------------------------------------------------------|
| `0`  | The promised behavior was observed. A real pass.                         |
| `1`  | Behavioral RED: the property under test does not hold. This is evidence. |
| `64` | Unknown selector argument. Not a verdict about the card.                 |
| `79` | Inconclusive / infrastructure: a dependency the card does not own was   |
|      | never materialized, a required tool is missing, a bound was exceeded, or |
|      | the acceptance program's own setup is unsound (e.g. a literal list it    |
|      | relies on came back empty). **Never valid red evidence, never a pass.**  |

`79` is the code already used across the more heavily audited gates in this
tree (`AUR-001`, `AUR-203`, `AUR-337`, `AUR-339`, `AUR-423`) for exactly this
"inconclusive" meaning, so new and fixed programs standardize on it rather
than inventing another number. Some older programs still use `69` for the
same concept. That set is not recited from memory here: it is recomputed from
the tree by `scripts/tests/exit-code-convention.test.sh`, case `EC-6`, which
fails whenever the delimited list below stops matching
`grep -lE '(exit|=)[[:space:]]*69([^0-9]|$)' tests/acceptance/AUR-*.sh`.

<!-- 69-USERS:BEGIN -->
`AUR-010`, `AUR-233`, `AUR-313`, `AUR-355`
<!-- 69-USERS:END -->

Those are left as-is rather than renumbered in the dark. `AUR-001`
also keeps a pre-existing `infra()` at `3`, reserved strictly for the
harness/tool-availability preflight at its own top of file (missing
`awk`/`sha256sum`/`wc`/`find`/`sort`/`readlink`) — that call predates this
note and is left untouched. Any exit-code decision this pass *adds or edits*
converges on `79` (`inconclusive()`), including inside `AUR-001.sh`: the
anti-shortcut list-corruption guard added for this pass calls
`inconclusive()`, not the file's pre-existing `infra()`, so the one new
decision point and the documented convention agree.

## The rule this encodes

A check must never let "the input I depend on is missing" and "the thing I
was asked to verify is wrong" collapse into the same exit code. Concretely:

- If the program reads a file that is **this card's own declared
  deliverable** (something `paths` says the card produces), that file's
  absence is the behavior under test failing to exist: exit `1`.
- If the program reads a file that is **someone else's artifact** — a
  dependency this card merely characterizes or reads, not one it owns or
  that is guaranteed to be materialized into the container — that file's
  absence is an environment/materialization gap, not a verdict: exit `79`.
- A bound being exceeded (oversized input, a timeout, a tool missing) is
  always `79`, never `1`.
- A literal list embedded in the program to defeat a stub/mock (an
  "anti-shortcut" list) must assert its own non-emptiness before it is used.
  A `for x in "${list[@]}"` over a silently-emptied list reports success
  without checking anything — the single most dangerous shape of false
  green — so an empty or truncated list is an infrastructure defect in the
  acceptance program itself: exit `79` (or the file's existing dedicated
  `infra()` code), never a silent pass.

## Applied in this pass

- `AUR-001.sh`: the embedded `legacy_surfaces` anti-shortcut list is now
  asserted to have exactly the declared 10 entries before the loop that
  consumes it runs; a corrupted (including emptied) list calls the file's
  `inconclusive()` (exit `79`) instead of silently completing with nothing
  checked.
- `AUR-365.sh` .. `AUR-391.sh` (27 "legacy characterization" acceptance
  programs, excluding the `AUR-359`..`AUR-364` bootstrap-lock family, whose
  `source_file` *is* the card's own deliverable): each now has its own
  `infra()` at exit `79`, used **only** for the byte-bound check
  (`(( bytes <= ... )) || infra input_limit_exceeded`) — an oversized input is
  always an environment/bound condition, never a verdict. The check that
  `source_file` exists is **not** converted: for every one of these 27 cards,
  `source_file` is itself listed in the card's own `paths:` (verified against
  `.board/cards/backlog/AUR-3NN.md` for all 27), so it is this card's own
  declared deliverable exactly like the manifest/claims file is — its absence
  stays `fail()` at exit `1`, same as the manifest/claims-absence and
  digest/content-mismatch checks below it. An earlier revision of this pass
  routed that existence check through `infra()` on the theory that
  `source_file` was an external dependency merely characterized by the card;
  that theory does not survive a check against `paths:` and has been
  reverted.

## Not yet converted

`AUR-015`..`AUR-021`, `AUR-334`..`AUR-336`, `AUR-338`, `AUR-340`,
`AUR-351`..`AUR-354`, `AUR-356`..`AUR-358` were surveyed and deliberately
left alone: each checks only a file that is the card's own declared
deliverable, using only baseline POSIX tools (`grep`, `find`) that the
`bootstrap-readonly-v1` profile guarantees, so there is no genuine
environment-failure path for them to mismodel as behavioral red today. If a
future revision adds a real external dependency or tool check to one of
them, it must follow the same convention.

## Scope: tests/acceptance/*.sh only

This convention binds `tests/acceptance/*.sh` only — programs materialized
alone into the ephemeral, read-only, per-card container described at the top
of this file. `scripts/action-entrypoint.sh`, `scripts/build-docs-site.sh`,
`scripts/generate-enhanced-docs.sh` and
`scripts/tests/documentation-mode.test.sh` are a different family: a
production GitHub Action entrypoint and the two helper scripts it dispatches
to, running inside the built image against a real consumer workspace rather
than inside a sealed per-card container.

Authority: each scripts/*.sh file documents its own exit codes inline; this file governs tests/acceptance/ only.

That single line is the entire scope claim this file makes about `scripts/`,
and it is checked verbatim, byte for byte, by
`scripts/tests/exit-code-convention.test.sh` (`EC-4`, `EC-5`) — edit the
wording and the guard fails; it does not parse prose for intent.

Two measured facts keep the families independently changeable, and neither
of them is about which convention came first:

1. The entrypoint reads its helpers' exit statuses **by literal value**.
   `scripts/action-entrypoint.sh` matches `"$EXIT_ENVIRONMENT"` (`3`) and
   `"$ENHANCE_NOOP"` (`20`) in the `case "$rc"` blocks that follow its
   `bash "$generate"` and `bash "$build"` calls, and those same numbers are
   defined independently inside `generate-enhanced-docs.sh` and
   `build-docs-site.sh`. Renumbering one file without the others silently
   reclassifies an environment failure as a behavioral one. Reproduce with
   `grep -n 'EXIT_ENVIRONMENT\|ENHANCE_NOOP\|EXIT_NOOP' scripts/*.sh`.
2. The two families never share a process. No program under
   `tests/acceptance/*.sh` invokes a file under `scripts/` — recomputed, not
   asserted, by `scripts/tests/exit-code-convention.test.sh`, case `EC-2`.
   No exit status of one family is ever read against the other family's
   table, so the same number is free to carry a different meaning in each
   scope.

The `scripts/` family keeps its own convention: `0` success, `1` behavioral
failure, `3` environment failure, and — only in `generate-enhanced-docs.sh`,
propagated through `action-entrypoint.sh` as `ENHANCE_NOOP` — `20` for a
documentation generator's own legitimate no-op (nothing changed to enhance),
a state this convention's four-code table has no slot for and is not the
place to add one. The Authority line above, not this paragraph, is what a
reader or a guard should cite for that split.

**Limit, stated plainly:** whether this paragraph's *opinion* about the split
is well-argued is not something `scripts/tests/exit-code-convention.test.sh`
can or does check — five rounds of adversarial paraphrase against earlier
revisions of `EC-5` established that free-form prose is always
paraphrasable around a lexical detector, so that case stopped trying. What
`EC-5` checks instead is a fact this paragraph cannot editorialize its way
around: the digits documented above next to `EXIT_ENVIRONMENT` and
`ENHANCE_NOOP`/`EXIT_NOOP` must equal what those names are actually assigned
to in `scripts/action-entrypoint.sh`, `scripts/build-docs-site.sh` and
`scripts/generate-enhanced-docs.sh`, in both directions. A real convergence
has to edit one of those `EXIT_*=` lines to have any effect, and that edit is
exactly what gets measured.
