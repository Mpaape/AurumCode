# ADR-0002: Product delivery ran outside the card process

**Status:** accepted

## Context

An adversarial reviewer measured that 16 files were modified and 7 directories created with
`cards/doing/` and `cards/review/` both empty, and that no reviewer ran `validate.sh` before
the work landed. The finding is correct and it is about how this phase was run, not about
the code.

The board's operating model says a card moves to `doing` only after its acceptance test
fails for the intended behavioral reason, that one builder then owns it and may change only
its declared paths, and that two blind reviews plus a skeptical approver decide.

None of that governed the product delivery in this phase. The owner directed that the
product be delivered and that the human gate be removed, and the work proceeded on that
directive: fixing an Action whose every GitHub expression was escaped, a pipeline that
returned `nil` while every extractor failed, and a Dockerfile that built three commands
which do not exist.

Those defects sit in paths owned by cards that are still in `backlog`, behind dependency
chains that the same phase was busy unblocking. Waiting for the card process would have
meant waiting for the board to become executable before fixing the product that the board
exists to reconstruct.

## Decision

Product delivery in this phase runs outside the card process, by owner directive, and the
deviation is recorded here rather than presented as compliance.

What replaced the card gates is not nothing:

- every change carries a test that fails before and passes after, executed in a pinned
  container;
- a constituted adversarial reviewer (`.claude/agents/adversarial-reviewer.md`) re-executes
  each proof from scratch and re-applies the mutation, and its `REQUEST_CHANGES` blocks the
  work;
- `validate.sh` and `validator-mutants.sh` are run by the coordinator before integration.

What is genuinely lost, and must not be claimed later: these changes have no card, no
`.board/evidence/<card-id>/` bundle, no `CandidateIdentityV1` binding, and no two
independent sealed verdicts. They are delivered software, not accepted cards.

## Consequences

The cards whose paths this work touched cannot later be closed by pointing at it. When
those cards execute, their acceptance programs must still fail first for a behavioral
reason and then pass — the existing code is the baseline they characterize, not their
evidence. `AUR-001` and `AUR-014` already demonstrated the failure mode this guards
against: an acceptance program that passes before its card exists proves nothing.

The board's audit trail stays honest because the gap is written down. A future reviewer who
finds product code with no corresponding evidence bundle will find this record explaining
why, instead of concluding the evidence was lost or forged.

Rejected alternative: retrofitting cards and evidence bundles onto work already done. That
would manufacture a red-then-green history that never happened, which is precisely the
self-declared authority the board refuses everywhere else.
