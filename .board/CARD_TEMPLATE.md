# Atomic TaskSpec card template

Copy this file into exactly one state directory and replace every placeholder.
A card owns one independently observable and reversible outcome. If two
acceptance scenarios can fail independently, split the card or make this card
a pure evidence aggregator with no production implementation.

```markdown
---
id: AUR-000
version: 1
title: Imperative artifact-specific outcome
status: backlog
office: O00-governance
depends_on: []
requirements: [PR-XXX-001]
controls: [CR-GATE-001]
paths: [path/owned/by/card, tests/integration/AUR-000.go, tests/acceptance/AUR-000.sh]
read_paths: [go.mod, go.sum, path/read/by/acceptance]
forbidden_paths: [.git, .env, secrets]
base_sha: lock-at-execution
spec_digest: lock-at-execution
risk: low|medium|high|critical
data_class: public|internal|confidential|restricted
trust_boundaries: [repository]
---

## Outcome

One externally observable outcome with no independently deployable sibling.

## Non-goals

- Explicit behavior, adapter, language, migration, or optimization excluded.

## Preconditions

- Exact dependency artifact or base behavior needed before the red test.

## Postconditions

- Exact state/artifact observed after success.
- Exact state that remains unchanged.

## Acceptance scenarios

### AC-001: Behavior-specific name

- Given: exact initial state and bounded fixture.
- When: exact public operation.
- Then: exact value, artifact, state transition, or typed failure.
- Covers requirements: [PR-XXX-001]
- Covers controls: [CR-GATE-001]

## Public contract

- Added/changed/unchanged interface, command, config, schema, or artifact.

## TDD proof

- Test: `tests/acceptance/AUR-000.sh::AC-001`.
- Red: on the locked base, the test reaches the promised behavior and exits
  non-zero with `AUR-000/AC-001: <behavior absent>`; compile, dependency,
  environment, or assertion-loader failure is invalid red evidence.
- Green: the minimum implementation makes AC-001 pass and emits its named
  artifact; no excluded behavior is added.
- Refactor: rerun AC-001 plus the named unit/contract profile and preserve the
  public contract and deterministic artifact digest.
- Unit: `path/owned/by/card/file_test.go::TestExactName`.
- Contract: not-applicable: concrete reason of at least sixteen characters.
- Integration: `tests/integration/AUR-000.go::TestExactBoundary`.
- E2E: not-applicable: concrete reason of at least sixteen characters.

## Acceptance

container_profile: `bootstrap-readonly-v1` or a versioned runtime profile.

accept: `./.board/bin/oci-run --profile bootstrap-readonly-v1 --card AUR-000`

Expected artifact: the coordinator, never the test process, derives
`.board/evidence/AUR-000/acceptance/AC-001.json` from bounded execution output,
validates it against the acceptance-result schema, and binds
`CandidateIdentityV1`.

## Skeptical mutations

### MUT-001: AC-001 / repository boundary

- Change: exact reversible source, config, fixture, or fault alteration.
- Expected: the unchanged `accept` command exits non-zero and the result names
  `AUR-000/AC-001/MUT-001`; restore and replay MUST pass.

Add at least one mutation/fault hypothesis per acceptance scenario and trust
boundary. `noop`, deleting the test, breaking compilation, or infrastructure
failure is not a skeptical mutation.

## Security and privacy

- Data flow: bounded source -> transformation -> sanitized sink.
- Secrets: exact canary/assertion or `not-applicable` with reason.
- Least privilege: exact capability and denied capabilities.

## Documentation

- Required behavior/contract/operations page, or `not-applicable` with reason.

## Compatibility, migration, rollback

- Compatibility: exact preserved/broken contract and consumer.
- Migration: exact one-way/dry-run step, or `not-applicable` with reason.
- Rollback: exact safe reversal and persistent-state consequence.

## Review

- Reviewer A: full ten-dimension and every-hunk review, prioritizing
  correctness, SOLID boundaries, compatibility, and test sensitivity.
- Reviewer B: full ten-dimension and every-hunk review, prioritizing hostile
  input, secrets, permissions, supply chain, resilience, and fail-closed paths.
- Independence: sealed same `CandidateIdentityV1`; fresh process/session and
  provider conversation; isolated caches/memory; manifest reports I0/I1/I2/I3
  honestly and treats backend aliases as one family.
- Skeptical approver: pre-seal hypotheses, run every mutation in a clean OCI
  worker, restore, replay, and veto on failure or inconclusive evidence.

## Evidence

Only the coordinator writes sanitized artifacts under
`.board/evidence/AUR-000/`; tests and builders emit untrusted observations and
never approval. The manifest
MUST bind the canonical candidate identity, locked spec, red/green/refactor,
deterministic gates, clean tree, full Review A and B coverage maps, pre-sealed
challenge, mutations, restored replay, hashes, and verdicts. `done` additionally
requires an authenticated human integration event. Never store secrets, raw
prompts/responses, credentials, environment values, or chain-of-thought.
```

Every concrete TDD test reference uses the closed `` `path::selector` `` form,
must resolve directionally inside `paths`, and may not offer an alternative
`not-applicable`. `read_paths` is a read-only input allowlist; it grants no
write ownership and may not overlap `forbidden_paths`.

`evidence_chain_digest` is not declarative. The validator recomputes it as the
SHA-256 of `candidate_identity_digest=<digest>\n` followed by strictly
path-sorted pairs `artifact.path=<path>\nartifact.sha256=<digest>\n`. Until a
separate trusted verifier authenticates a human SCM/service event, `done`
remains fail-closed; a JSON field named `authenticated` grants no authority.

`base_sha` and `spec_digest` are deliberately `lock-at-execution` while a card
is backlog/ready. Moving to `doing` replaces them in the immutable evidence
TaskSpec, not by mutating the board after reviewers have started.
