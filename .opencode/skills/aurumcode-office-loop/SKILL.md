---
name: aurumcode-office-loop
description: Use for the AurumCode board office loop, sprint cycles, goal-to-zero-backlog execution, and bounded Ralph continuation.
---

# AurumCode Office Loop

This is an operational loop for the reconstruction board. It never invents
work, bypasses the board, or weakens evidence. The canonical lightweight flow is
in `.board/README.md` and `.board/AGENT_PLAYBOOK.md`; the frozen
`.board/validate.sh`, `.board/REVIEW_PROTOCOL.md`, `.board/bin/second-reader`,
and the Reviewer A/B / test-designer ceremony are legacy evidence for old `done`
cards only and are not part of the current session gate.

Hard rule: **git runs on the host, never inside the container.** The shared
`aurum-go` container (`./.board/bin/go-shared`) exists only to compile and run
code and tests; branches, worktrees, commits, diffs, and `git notes` stay on the
host. Review is done on the host git diff of the immutable SHA.

## Start and 20-minute review

1. Read `AGENTS.md`, `.board/README.md`, `.board/AGENT_PLAYBOOK.md`, and the
   target card.
2. Run `bash .board/office-cycle.sh --status` and
   `bash .board/pipeline.sh` before dispatch.
3. Run `bash .board/office-cycle.sh --start` once for a new series of reviews.
4. At each real 20-minute boundary, run
   `bash .board/office-cycle.sh --review`. It records `done_delta` and exits
   `75` after two reviews without progress. Stop and change approach on `75`.
5. Do not use `.board/validate.sh`, its exit code, or an agent's prose as a
   replacement for these gates.

## Dispatch gate

- Only `.board/cards/ready/` authorizes a builder.
- Every builder uses a clean worktree and only declared `paths`.
- Before any builder, reviewer, or validator run
  `PREFLIGHT_RUN=1 bash .board/card-preflight.sh AUR-NNN /clean/worktree`.
- The preflight checks dependencies, tracked inputs, executable acceptance,
  mutation signal, exact profile-bound accept command, digest lock, local image,
  and a real image smoke test for `bash` plus `go` when the acceptance uses Go.
- `oci-run` repeats the image runtime probe immediately before materialization.
  A Bash-only image cannot execute Go acceptance, and a Go image without Bash
  cannot execute this runner.
- After changing a gate, runner, or office skill, run
  `bash .board/tests/office-process-regression.sh` and `git diff --check`.
- Do not create a feature-local OCI profile to bypass AUR-402/AUR-403 ownership.

## Delivery gates

1. The builder proves baseline/mutation RED for the promised behavior before
   GREEN. Infrastructure failure is never RED.
2. The builder returns a patch plus raw output, never approval or a card move.
   The card stays `ready` during review.
3. Exactly one independent reviewer checks the immutable SHA on the host git
   diff: every hunk, contract, schema/parser agreement, acceptance, paths, exits,
   and security boundaries, plus `go test` over every package in `paths`. The
   verdict is recorded with `git notes --ref=reviews`. Do not dispatch Reviewer
   A/B, a second reviewer, or a skeptic.
4. Only after approval does the coordinator integrate that exact SHA into
   `main`, add the Delivery record (`- commit: <sha in main>`), and move the card
   to `validating`.
5. Validator runs the acceptance and declared layers on the same clean SHA.
   Exit `0` is evidence, `69/79` is inconclusive, and exit `1` is behavioral RED
   only after the program actually started.
6. The coordinator writes sanitized evidence, updates the Delivery record, runs
   `bash .board/pipeline.sh`, and only then moves the card to `done`.

## Safety stops

- Never run the legacy `.board/bin/second-reader` or `.board/validate.sh` for an
  active card; they are frozen and write evidence.
- Never run `git` inside the container; git is host-only.
- Missing engine, image, runtime, module cache, queued workflow, or timeout is
  an infrastructure blocker, not approval.
- Never duplicate an active builder or continue a card with unmet dependencies.
- Never trust `MERGEABLE`, `CLEAN`, `--watch` exit status, JSON verdict fields,
  or a successful command that selected no test.
- If the coordinator worktree is dirty, keep it out of build and validation.

## Completion

Output `<promise>DONE</promise>` only when the board has no backlog, ready,
doing, review, validating, or unresolved infrastructure/specification blocker,
and all current delivery records, reviews, validations, and real publication
checks are present. Until then, report the measured blocker or continue only
authorized work.
