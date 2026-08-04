# Skeptical Approval Protocol v1

This policy uses the key words **MUST**, **MUST NOT**, **SHOULD**, and **MAY**
as defined by [BCP 14](https://www.rfc-editor.org/info/bcp14). It adapts
independent verification ideas to agent review; it does not claim formal
ISO, NIST, or IV&V conformity.

## Roles and independence

| Role | Purpose | Must not receive |
|---|---|---|
| Specifier | Lock one falsifiable outcome and its boundaries | Candidate implementation |
| Test designer | Produce acceptance proof from the locked spec | Implementer discussion |
| Builder | Turn a verified red test green | Future reviews or verdicts |
| Reviewer A | Full ten-dimension review, prioritizing correctness/design | Builder trace or Reviewer B report |
| Reviewer B | Full ten-dimension review, prioritizing security/resilience | Builder trace or Reviewer A report |
| Skeptic | Seal and execute a falsification plan | Peer identities or author defense |
| Adjudicator | Decide one evidence-based appeal | Participant identities or status |
| Human gate owner | Authorize merge, publication, or risk acceptance | This authority is never delegated |
| Coordinator | Validate evidence and derive state | Authority to rewrite reports or self-approve |

Every role MUST use a fresh session and role nonce. Reviewers receive the same
immutable factual snapshot and remain blind until both reports are sealed. The
skeptic seals its challenge plan before seeing either review. Shared
conversation or scratchpad memory is forbidden; a snapshot-bound entity index
is allowed as untrusted factual input.

Independence MUST be reported honestly:

- `I0`: same session or trace; invalid.
- `I1`: isolated contexts, nonces, and prompts.
- `I2`: `I1` plus different model families or providers.
- `I3`: withdrawn. It required an organizationally independent human approval,
  and this board has no human gate: every card is proved by agent-driven
  validation plus a skeptical mutation that must make the acceptance proof
  fail. `I2` is the attainable ceiling and no report may claim `I3`.

Offline fake agents prove orchestration at `I1`, not semantic review quality.
If distinct providers are unavailable, the report says `context-independent`;
it MUST NOT claim model independence. Different model families behind one
provider reach `I2`, and the manifest MUST record the correlation explicitly
rather than presenting it as provider independence.

The adversarial role is constituted as a named agent with its own definition,
system prompt and model — `.claude/agents/adversarial-reviewer.md` — so the
reviewer is not a persona of the author. Two personas inside one request remain
`I0` and are invalid.

Both reviewers cover all ten reviewer-contract dimensions and every
human-authored hunk. Their named lenses are priorities for adversarial depth,
not a partition of responsibility. The independence manifest MUST record and
test fresh provider conversation/thread IDs, process identity, cache and global
state isolation, peer-memory absence, prompt/rubric digests, provider/model
aliases, backend-family identity, and the achieved `I0`/`I1`/`I2`/`I3` level.
Two aliases that resolve to the same backend family do not establish `I2`.

## Immutable task and evidence identity

A locked task identifies its objective, non-goals, allowed and forbidden paths,
base SHA, preconditions, postconditions, Given/When/Then scenarios, public
contract changes, risk, data class, trust boundaries, tests, docs,
compatibility, migration, rollback, dependencies, and spec digest.

Every verdict, challenge, publication, appeal, and human approval binds to the
same `CandidateIdentityV1` defined in
[`requirements/REQUIREMENTS.md`](requirements/REQUIREMENTS.md):

```text
repository_identity + base_tree_digest + head_tree_digest + change_digest
+ task_spec_digest + configuration_digest + policy_digest
+ prompt_and_rubric_digest + skill_set_digest
+ provider_model_backend_identity_digest
+ toolchain_and_tool_set_digest + dependency_lock_digest
+ container_image_set_digest + test_manifest_digest
+ role_context_manifest_digest
```

No subsystem may define a shorter approval identity. Changing any bound input
invalidates dependent evidence. A code change always
invalidates deterministic gates, both reviews, skeptical approval, and human
approval. A new head never inherits an approval.

## TDD evidence

1. The test designer writes acceptance/contract tests from the locked spec.
2. The coordinator executes them on the base SHA.
3. A feature or fix MUST be `RED` for the expected behavioral reason; a compile,
   dependency, or infrastructure error is not a valid red.
4. Acceptance tests are locked before the builder receives the task.
5. The builder produces minimum `GREEN`, unit tests, and behavior documentation.
6. A refactor uses characterization `GREEN`, a targeted mutation `RED`, then
   refactor `GREEN`; it MUST NOT fabricate a feature-style red.
7. Both reviewers inspect test quality.
8. The skeptic mutates or injects faults and proves the required test fails.

Changing an acceptance test creates a new spec version and restarts the cycle.

## Derived state machine

```text
Draft -> SpecLocked -> AcceptanceTestsRedVerified -> Implemented
      -> DeterministicGatesPassed
      -> ReviewASealed + ReviewBSealed + ChallengePlanSealed
      -> AdversarialGatesPassed -> CandidateApproved
      -> HumanApprovalPending -> Accepted
```

Lateral states are `Rework`, `Appealed`, `Rejected`, `Blocked`, `Cancelled`, and
`Superseded`. `CandidateApproved` does not mean merged. `Accepted` requires an
authenticated human/SCM event. The coordinator cannot write `done`; it derives
it from a complete valid evidence bundle.

## Reviewer contract

Both reviewers cover every human-authored hunk and explicitly report:

1. contract and functionality;
2. design, SOLID boundaries, and code health;
3. compatibility, migration, and rollback;
4. tests and regression sensitivity;
5. security, privacy, and supply chain;
6. concurrency, cancellation, retries, and idempotency;
7. errors, observability, and operations;
8. documentation;
9. scope, simplicity, and dead code;
10. hunk coverage.

Each dimension is `covered`, justified `not_applicable`, or `abstain`. An
uncovered hunk invalidates the report. Generated code is reviewed through its
source, generator, digest, and resulting diff.

Reviewer verdicts are `pass`, `pass_with_nits`, `request_changes`, or `abstain`.
One valid blocker vetoes the candidate. An uncodified aesthetic preference
never blocks. A blocking finding contains a stable fingerprint, rule/category,
severity, disposition, validated location or justified global scope,
falsifiable claim, evidence, impact, reproduction or verifiable reasoning,
objective resolution criterion, confidence, and evidence references.

Approval is the pure conjunction:

```text
deterministic gates pass
AND reviewer A in {pass, pass_with_nits}
AND reviewer B in {pass, pass_with_nits}
AND skeptic passes
AND no unresolved blocker
AND evidence integrity passes
```

`abstain` triggers replacement within the bounded retry policy. Agent consensus
never overrides deterministic evidence.

## Skeptical falsification

The skeptic MUST challenge each acceptance criterion and trust boundary. It
looks for false success, missing mutation sensitivity, hostile-input escape,
secret leakage, retry/cancel/restart duplication, cache or network dependence,
and documentation that disagrees with behavior.

For each card it:

1. creates at least one failure hypothesis per acceptance criterion/boundary;
2. seals the challenge plan;
3. starts a cold checkout with empty caches and denied external network;
4. runs the decisive acceptance command;
5. applies the card's reversible mutation plus risk-relevant fault injection;
6. proves the same acceptance command fails for the intended reason;
7. restores the candidate and reruns acceptance;
8. checks provenance hashes and secret canaries;
9. reconciles sealed reviews only after testing; and
10. emits `pass`, `veto`, or `inconclusive` (`inconclusive` blocks).

Default per-card bounds are 12 hypotheses, 20 executions, 20 minutes, 2 CPUs,
4 GiB RAM, 256 PIDs, 10 MiB combined output, and one retry exclusively for a
classified transient infrastructure failure.

## Appeals and bounded loops

- A valid veto moves the card to `Rework`.
- A malformed or unevidenced report is `review_error`; replace the reviewer.
- A builder may appeal once per `finding_fingerprint + head_sha`, with technical
  counter-evidence only.
- A fresh adjudicator emits `uphold`, `dismiss`, or `remand`.
- Critical/high security risk acceptance requires a separately authenticated
  human owner, justification, scope, and expiry.
- No participant may approve its own work, veto reversal, or appeal.
- Three unchanged implement/review cycles are the limit. Repeated patch and
  blocker digests produce `no_progress` and require decomposition or a human
  decision.
- Restart resumes append-only events idempotently and never repeats publication.

## Evidence bundle and sensitive information

`.board/evidence/<card-id>/` is content-addressed and contains sanitized specs,
digests, base/head/patch identity, clean-tree proof, pinned image/toolchain
identity, argv and exit codes, timing, JUnit, coverage, race/fuzz seeds,
mutation results, scanners, SBOM, policy, coverage maps, sealed verdicts,
challenge result, appeals, adjudication, authenticated human event, and hash
chain.

For a card in `review`, a schema-valid bundle includes the candidate identity,
locked acceptance manifest, red/green/refactor outputs, deterministic gates,
clean-tree proof, both sealed full-review reports, sealed challenge plan and
skeptic result. For `done`, it additionally includes a separately authenticated
human integration event bound to the exact identity. An empty or partial
manifest is invalid; file existence is never approval evidence.

It MUST NOT contain API keys, tokens, environment values, raw prompts or model
responses, chain-of-thought, remote headers/bodies, secret values or reusable
secret hashes, or an unauthorized private checkout. Store only provider/model
aliases, versions, rubric/prompt digests, bounded usage, and validated outputs.
Credentials are injected by a gateway and never exposed to agent containers.

## Container approval environment

The E2E topology contains an orchestrator, scripted OpenAI-compatible fake
providers for each agent role, fake SCM, and a distinct test worker with no
network. Images are digest-pinned, rootless, read-only, capability-dropped,
resource-limited, and have no Docker socket. Clock, IDs, seeds, and ordering are
deterministic. Fakes validate forbidden context and simulate pass, blocker,
abstain, timeout, malformed output, forbidden tool calls, budget overflow, and
exfiltration attempts.

Before this harness exists, a minimal bootstrap verifier statically rejects
absolute/root paths, privileged/root containers, host or engine-socket mounts,
devices, added capabilities, writable checkout mounts, unpinned images/actions,
unlocked toolchains/dependencies/grammars, and external egress. The verifier is
executed by a pinned non-root image with a read-only root filesystem, no
capabilities, no-new-privileges, no network, bounded resources, and only the
board mounted read-only. The future harness MUST pass the same conformance
suite through both Docker and Podman; engine-specific behavior is an adapter.

The protocol suite MUST include happy path, invalid red, acceptance-test
tampering, role reuse, pre-seal leakage, one-reviewer veto, blind approval,
mutation survival, appeal, stale base/head, replay/tamper, prompt injection,
malicious skill/memory, secret canaries, denied privilege/egress, bounded
timeouts/loops, flakiness, missing docs, false success, restart idempotency,
deterministic replay, forged human approval, and correlated blind spots.

The suite passes only with 100% detection of planted critical failures and zero
accepted candidate without an authenticated human event.
