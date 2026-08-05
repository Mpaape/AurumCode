#!/usr/bin/env bash
set -euo pipefail

repo_root="$(pwd -P)"
[[ -d "$repo_root/.board/cards" ]] || {
  printf 'validator mutant harness must run at repository root\n' >&2
  exit 64
}
probe_root="$(mktemp -d)"
trap 'rm -rf -- "$probe_root"' EXIT

digest() {
  printf 'sha256:%s' "$(printf '%s' "$1" | sha256sum | awk '{print $1}')"
}

write_base_fixture() {
  local root="$probe_root/base"
  local state
  mkdir -p "$root/.board/cards" "$root/.board/evidence" "$root/.board/requirements" \
    "$root/.board/research" "$root/tests/acceptance" "$root/docs/specs"
  for state in backlog ready doing review done blocked-on-owner cancelled; do
    mkdir -p "$root/.board/cards/$state"
  done
  cp "$repo_root/.board/validate.sh" "$root/.board/validate.sh"
  chmod 0755 "$root/.board/validate.sh"
  cat >"$root/.board/requirements/REQUIREMENTS.md" <<'EOF'
# Requirements

| ID | Requirement |
|---|---|
| PR-TST-001 | The focused fixture proves one bounded behavior. |
EOF
  cat >"$root/.board/research/code-review-standards.md" <<'EOF'
# Standards

`CR-GATE-001` The deterministic acceptance gate fails closed.
EOF
  cat >"$root/.board/cards/ready/AUR-001.md" <<'EOF'
---
id: AUR-001
version: 1
title: Validate one bounded parser result
status: ready
office: O00-governance
depends_on: []
requirements: [PR-TST-001]
controls: [CR-GATE-001]
paths: [tests/acceptance/AUR-001.sh, docs/specs/AUR-001.md]
forbidden_paths: [.git, .env, secrets]
base_sha: lock-at-execution
spec_digest: lock-at-execution
risk: low
data_class: internal
trust_boundaries: [repository]
---

## Outcome

The fixture emits one deterministic parser result with a decisive exit code.

## Non-goals

- It does not publish, mutate external state, or add another parser behavior.

## Preconditions

- The sealed fixture and pinned bootstrap execution profile are available.

## Postconditions

- The parser result has the expected value and unrelated bytes remain unchanged.

## Acceptance scenarios

### AC-001: Return the bounded parser result

- Given: one deterministic local fixture with no credential or external input.
- When: the acceptance parser reads that fixture exactly once.
- Then: the parser returns the expected value and a decisive zero exit code.

## Public contract

- Only the fixture result and documented exit behavior are observable.

## TDD proof

- Test: `tests/acceptance/AUR-001.sh::AC-001`.
- Red: the locked base reaches the parser and reports the missing expected value.
- Green: the parser returns the exact expected value without another behavior.
- Refactor: the same fixture preserves output ordering, exit code, and digest.
- Unit: not-applicable: the acceptance parser is the smallest focused unit.
- Contract: not-applicable: no port or wire protocol exists in this fixture.
- Integration: not-applicable: no adapter boundary exists in this fixture.
- E2E: not-applicable: this validator fixture has no consumer workflow.

## Acceptance

container_profile: `bootstrap-readonly-v1`

accept: `./.board/bin/oci-run --profile bootstrap-readonly-v1 --card AUR-001`

Expected artifact: `.board/evidence/AUR-001/acceptance/AC-001.json` is coordinator-derived.

## Skeptical mutations

### MUT-001: AC-001 / repository

- Change: replace the exact expected value with one different deterministic byte.
- Expected: the unchanged acceptance exits non-zero for MUT-001 and restored replay passes.

## Security and privacy

- Bounded fixture bytes flow to a sanitized local observation with no credential input.

## Documentation

- The exact fixture, result, exit behavior, and failure diagnosis are documented.

## Compatibility, migration, rollback

- Existing behavior remains unchanged and rollback removes only this bounded fixture.

## Review

- Reviewer A: every-hunk review across ten dimensions with correctness priority.
- Reviewer B: every-hunk review across ten dimensions with adversarial priority.
- Independence: isolated sessions, contexts, caches, memory, and sealed reports.
- Skeptical approver: pre-sealed challenge, mutation, restore, and clean replay.

## Evidence

Only `.board/evidence/AUR-001/` may contain sanitized coordinator-produced artifacts.
EOF
  cat >"$root/tests/acceptance/AUR-001.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
printf '{"schema":"aurum.acceptance-observation","version":1,"card_id":"AUR-001","scenario_id":"AC-001"}\n'
EOF
  chmod 0755 "$root/tests/acceptance/AUR-001.sh"
  printf 'fixture documentation with a deterministic bounded result\n' >"$root/docs/specs/AUR-001.md"
  cat >"$root/.board/INDEX.md" <<'EOF'
# Index

| ID | State | Office | Risk | Dependencies | Title |
|---|---|---|---|---|---|
| [AUR-001](cards/ready/AUR-001.md) | ready | O00-governance | low | `[]` | Validate one bounded parser result |
EOF
  # The declared-path materialization gate separates "this artifact is the work
  # still to be done" from "the tree lost an artifact a card owns" by asking Git
  # what the repository tracks, so the fixture carries a real repository. Both
  # records the gate consults must exist here: the index, and one commit, because
  # `git rm` erases a path from the index and the working tree together and only
  # the committed tree still remembers it. This repository lives inside the
  # `mktemp -d` probe root, is deleted by the harness EXIT trap, and is never a
  # remote, a branch, or a publication of this project; the commit identity is
  # passed per invocation with `-c` and is never written to any configuration.
  git -C "$root" -c init.defaultBranch=main init -q
  git -C "$root" add --force --all -- .
  git -C "$root" \
    -c user.name='board validator fixture' \
    -c user.email='board-validator-fixture@invalid' \
    -c commit.gpgsign=false \
    commit -q --no-verify -m 'validator fixture baseline'
}

prepare() {
  local name="$1"
  cp -R "$probe_root/base" "$probe_root/$name"
}

# --- Cancelled lane fixture ---------------------------------------------------
# The owner's rule ("you don't delete a card, you send it to cancelled") is
# only real if a validator gate enforces it. Three cards prove the graph-level
# consequence of cancellation: AUR-001 is cancelled and declares AUR-002 as
# its accepted successor; AUR-002 is the successor itself; AUR-003 is an
# ordinary card that depends on the cancelled AUR-001 *and* on its declared
# successor AUR-002, which is the only way a dependent may cite a cancelled
# card at all. Each mutant below breaks exactly one of the five requirements
# and nothing else, mirroring the single-variable mutation discipline used
# throughout this file.
write_cancel_skeleton() {
  local root="$1"
  local state
  mkdir -p "$root/.board/cards" "$root/.board/evidence" "$root/.board/requirements" \
    "$root/.board/research" "$root/tests/acceptance" "$root/docs/specs"
  for state in backlog ready doing review done blocked-on-owner cancelled; do
    mkdir -p "$root/.board/cards/$state"
  done
  cp "$repo_root/.board/validate.sh" "$root/.board/validate.sh"
  chmod 0755 "$root/.board/validate.sh"
  cat >"$root/.board/requirements/REQUIREMENTS.md" <<'EOF'
# Requirements

| ID | Requirement |
|---|---|
| PR-TST-001 | The focused fixture proves one bounded behavior. |
EOF
  cat >"$root/.board/research/code-review-standards.md" <<'EOF'
# Standards

`CR-GATE-001` The deterministic acceptance gate fails closed.
EOF
  cat >"$root/.board/INDEX.md" <<'EOF'
# Index

| ID | State | Office | Risk | Dependencies | Title |
|---|---|---|---|---|---|
EOF
}

# $1 root $2 id $3 state $4 depends_on (canonical bracket list) $5 tag. The
# tag makes every checked spec field (Given/When/Then/Green/Non-goal/And)
# unique per card so this fresh probe board -- which carries no spec-collision
# baseline file -- never trips that unrelated ratchet.
write_cancel_card() {
  local root="$1" id="$2" state="$3" depends_on="$4" tag="$5"
  cat >"$root/.board/cards/$state/$id.md" <<EOF
---
id: $id
version: 1
title: Validate one bounded parser result ($tag)
status: $state
office: O00-governance
depends_on: $depends_on
requirements: [PR-TST-001]
controls: [CR-GATE-001]
paths: [tests/acceptance/$id.sh, docs/specs/$id.md]
forbidden_paths: [.git, .env, secrets]
base_sha: lock-at-execution
spec_digest: lock-at-execution
risk: low
data_class: internal
trust_boundaries: [repository]
---

## Outcome

The fixture emits one deterministic parser result unique to $tag with a decisive exit code.

## Non-goals

- It does not publish, mutate external state, or add another parser behavior for $tag.

## Preconditions

- The sealed fixture and pinned bootstrap execution profile are available for $tag.

## Postconditions

- The parser result for $tag has the expected value and unrelated bytes remain unchanged.

## Acceptance scenarios

### AC-001: Return the bounded parser result for $tag

- Given: \`tests/specs/$id/cases.yaml\` is a deterministic local fixture unique to $tag with no credential or external input.
- When: the acceptance parser for $tag reads that fixture exactly once.
- Then: the parser for $tag returns the expected value and a decisive zero exit code.
- And: the same replay is idempotent and reproduces the digest for $tag.

## Public contract

- Only the fixture result and documented exit behavior for $tag are observable.

## TDD proof

- Test: \`tests/acceptance/$id.sh::AC-001\`.
- Red: the locked base for $tag reaches the parser and reports the missing expected value.
- Green: the parser for $tag returns the exact expected value without another behavior.
- Refactor: the same fixture for $tag preserves output ordering, exit code, and digest.
- Unit: not-applicable: the acceptance parser is the smallest focused unit.
- Contract: not-applicable: no port or wire protocol exists in this fixture.
- Integration: not-applicable: no adapter boundary exists in this fixture.
- E2E: not-applicable: this validator fixture has no consumer workflow.

## Acceptance

container_profile: \`bootstrap-readonly-v1\`

accept: \`./.board/bin/oci-run --profile bootstrap-readonly-v1 --card $id\`

Expected artifact: \`.board/evidence/$id/acceptance/AC-001.json\` is coordinator-derived.

## Skeptical mutations

### MUT-001: AC-001 / repository

- Change: replace the exact expected value with one different deterministic byte for $tag.
- Expected: the unchanged acceptance exits non-zero for MUT-001 and restored replay passes.

## Security and privacy

- Bounded fixture bytes for $tag flow to a sanitized local observation with no credential input.

## Documentation

- The exact fixture, result, exit behavior, and failure diagnosis for $tag are documented.

## Compatibility, migration, rollback

- Existing behavior remains unchanged and rollback removes only this bounded fixture ($tag).

## Review

- Reviewer A: every-hunk review across ten dimensions with correctness priority.
- Reviewer B: every-hunk review across ten dimensions with adversarial priority.
- Independence: isolated sessions, contexts, caches, memory, and sealed reports.
- Skeptical approver: pre-sealed challenge, mutation, restore, and clean replay.

## Evidence

Only \`.board/evidence/$id/\` may contain sanitized coordinator-produced artifacts.
EOF
  cat >"$root/tests/acceptance/$id.sh" <<EOF
#!/usr/bin/env bash
set -euo pipefail
printf '{"schema":"aurum.acceptance-observation","version":1,"card_id":"$id","scenario_id":"AC-001"}\n'
EOF
  chmod 0755 "$root/tests/acceptance/$id.sh"
  printf 'fixture documentation with a deterministic bounded result (%s)\n' "$tag" >"$root/docs/specs/$id.md"
}

# Locks base_sha/spec_digest exactly the way doing/review/done cards are
# locked (cancelled is now the same kind of frozen, non-backlog state) and
# echoes the canonical self-referential digest on stdout, so the caller can
# also bind it into cancellation.json's card_digest.
lock_cancel_card() {
  local card_file="$1"
  local base_sha="$2"
  local digest
  sed -i "s/^base_sha: lock-at-execution\$/base_sha: $base_sha/; s/^spec_digest: lock-at-execution\$/spec_digest: sha256:0000000000000000000000000000000000000000000000000000000000000000/" "$card_file"
  digest="sha256:$(sed -E 's/^status: .+$/status: STATE/; s/^spec_digest: sha256:[0-9a-f]{64}$/spec_digest: sha256:SELF/' "$card_file" | sha256sum | awk '{print $1}')"
  sed -i "s/^spec_digest: sha256:[0-9a-f]\{64\}\$/spec_digest: $digest/" "$card_file"
  printf '%s' "$digest"
}

write_cancellation_base() {
  local root="$1"
  local cancelled_digest
  write_cancel_skeleton "$root"
  write_cancel_card "$root" AUR-001 cancelled '[]' cancelled
  write_cancel_card "$root" AUR-002 ready '[]' successor
  write_cancel_card "$root" AUR-003 backlog '[AUR-001, AUR-002]' dependent

  cancelled_digest="$(lock_cancel_card "$root/.board/cards/cancelled/AUR-001.md" 1111111111111111111111111111111111111111)"

  mkdir -p "$root/.board/evidence/AUR-001"
  cat >"$root/.board/evidence/AUR-001/cancellation.json" <<EOF
{"schema":"aurum.cancellation","version":1,"card_id":"AUR-001","approved_by_role":"manager","reason":"The manager decided this capability duplicates AUR-002 and is no longer part of the reconstruction plan.","superseded_by":"AUR-002","card_digest":"$cancelled_digest"}
EOF

  cat >>"$root/.board/INDEX.md" <<'EOF'
| [AUR-001](cards/cancelled/AUR-001.md) | cancelled | O00-governance | low | `[]` | Validate one bounded parser result (cancelled) |
| [AUR-002](cards/ready/AUR-002.md) | ready | O00-governance | low | `[]` | Validate one bounded parser result (successor) |
| [AUR-003](cards/backlog/AUR-003.md) | backlog | O00-governance | low | `[AUR-001, AUR-002]` | Validate one bounded parser result (dependent) |
EOF
}

write_cancellation_base "$probe_root/cancel_valid"
if ! bash "$probe_root/cancel_valid/.board/validate.sh" >"$probe_root/cancel_valid.baseline.output" 2>&1; then
  printf 'cancellation fixture baseline is invalid\n' >&2
  sed -n '1,120p' "$probe_root/cancel_valid.baseline.output" >&2
  exit 1
fi

# Requirement 1: cancelled is locked like doing/review/done, not left
# unlocked like backlog/ready. Reverting it to lock-at-execution must be
# rejected the same way an unlocked backlog/ready card would be if it tried
# to claim a locked-only state.
cp -R "$probe_root/cancel_valid" "$probe_root/cancel_unlocked"
sed -i "s/^base_sha: 1111111111111111111111111111111111111111\$/base_sha: lock-at-execution/; s/^spec_digest: sha256:[0-9a-f]\{64\}\$/spec_digest: lock-at-execution/" \
  "$probe_root/cancel_unlocked/.board/cards/cancelled/AUR-001.md"

# Requirement 2: cancellation.json must carry a specific, non-generic,
# manager-approved reason -- not a bare filler phrase, regardless of who
# approved it structurally.
cp -R "$probe_root/cancel_valid" "$probe_root/cancel_generic_reason"
sed -i 's/"reason":"[^"]*"/"reason":"obsolete"/' \
  "$probe_root/cancel_generic_reason/.board/evidence/AUR-001/cancellation.json"

# Requirement 3: AUR-003 depends on the cancelled AUR-001 but, in this
# mutant, no longer also depends on AUR-001's declared successor AUR-002 --
# exactly the silent-orphaning the dependency graph must never allow.
cp -R "$probe_root/cancel_valid" "$probe_root/cancel_no_override"
sed -i 's/^depends_on: \[AUR-001, AUR-002\]$/depends_on: [AUR-001]/' \
  "$probe_root/cancel_no_override/.board/cards/backlog/AUR-003.md"

# Requirement 4: INDEX.md must carry the canonical row for the cancelled
# card in exactly the same format every other lane uses.
cp -R "$probe_root/cancel_valid" "$probe_root/cancel_index_missing"
sed -i '/^| \[AUR-001\](cards\/cancelled\/AUR-001\.md)/d' \
  "$probe_root/cancel_index_missing/.board/INDEX.md"

# Requirement 5: cancellation is not completion. A cancelled card must never
# carry the `done` evidence bundle (manifest.json), even an otherwise-empty
# one.
cp -R "$probe_root/cancel_valid" "$probe_root/cancel_has_manifest"
printf '{}' >"$probe_root/cancel_has_manifest/.board/evidence/AUR-001/manifest.json"

# --- Spec-collision fixture -------------------------------------------------
# A second, independent skeleton (no AUR-001 card) that spec-collision cases
# populate with 2-3 cloned-but-distinguished cards. Every checked field
# (Given/When/Then/Green/Non-goal/And) is made unique per card via
# `unique_tag` EXCEPT the one field a given case deliberately collides, so a
# case's assertion is never contaminated by an accidental collision in an
# unrelated field.
write_ratchet_skeleton() {
  local root="$1"
  local state
  mkdir -p "$root/.board/cards" "$root/.board/evidence" "$root/.board/requirements" \
    "$root/.board/research" "$root/.board/tests" "$root/tests/acceptance" "$root/docs/specs"
  for state in backlog ready doing review done blocked-on-owner cancelled; do
    mkdir -p "$root/.board/cards/$state"
  done
  cp "$repo_root/.board/validate.sh" "$root/.board/validate.sh"
  chmod 0755 "$root/.board/validate.sh"
  cat >"$root/.board/requirements/REQUIREMENTS.md" <<'EOF'
# Requirements

| ID | Requirement |
|---|---|
| PR-TST-001 | The focused fixture proves one bounded behavior. |
EOF
  cat >"$root/.board/research/code-review-standards.md" <<'EOF'
# Standards

`CR-GATE-001` The deterministic acceptance gate fails closed.
EOF
  cat >"$root/.board/INDEX.md" <<'EOF'
# Index

| ID | State | Office | Risk | Dependencies | Title |
|---|---|---|---|---|---|
EOF
}

# $1 root, $2 id (AUR-NNN), $3 unique_tag (distinguishes every field except
# And), $4 and_text (the AC-001 And bullet -- the deliberately shared field).
write_ratchet_card() {
  local root="$1" id="$2" tag="$3" and_text="$4"
  cat >"$root/.board/cards/ready/$id.md" <<EOF
---
id: $id
version: 1
title: Validate one bounded parser result ($tag)
status: ready
office: O00-governance
depends_on: []
requirements: [PR-TST-001]
controls: [CR-GATE-001]
paths: [tests/acceptance/$id.sh, docs/specs/$id.md]
forbidden_paths: [.git, .env, secrets]
base_sha: lock-at-execution
spec_digest: lock-at-execution
risk: low
data_class: internal
trust_boundaries: [repository]
---

## Outcome

The fixture emits one deterministic parser result unique to $tag with a decisive exit code.

## Non-goals

- It does not publish, mutate external state, or add another parser behavior for $tag.

## Preconditions

- The sealed fixture and pinned bootstrap execution profile are available for $tag.

## Postconditions

- The parser result for $tag has the expected value and unrelated bytes remain unchanged.

## Acceptance scenarios

### AC-001: Return the bounded parser result for $tag

- Given: \`tests/specs/$id/cases.yaml\` is a deterministic local fixture unique to $tag with no credential or external input.
- When: the acceptance parser for $tag reads that fixture exactly once.
- Then: the parser for $tag returns the expected value and a decisive zero exit code.
- And: $and_text

## Public contract

- Only the fixture result and documented exit behavior for $tag are observable.

## TDD proof

- Test: \`tests/acceptance/$id.sh::AC-001\`.
- Red: the locked base for $tag reaches the parser and reports the missing expected value.
- Green: the parser for $tag returns the exact expected value without another behavior.
- Refactor: the same fixture for $tag preserves output ordering, exit code, and digest.
- Unit: not-applicable: the acceptance parser is the smallest focused unit.
- Contract: not-applicable: no port or wire protocol exists in this fixture.
- Integration: not-applicable: no adapter boundary exists in this fixture.
- E2E: not-applicable: this validator fixture has no consumer workflow.

## Acceptance

container_profile: \`bootstrap-readonly-v1\`

accept: \`./.board/bin/oci-run --profile bootstrap-readonly-v1 --card $id\`

Expected artifact: \`.board/evidence/$id/acceptance/AC-001.json\` is coordinator-derived.

## Skeptical mutations

### MUT-001: AC-001 / repository

- Change: replace the exact expected value with one different deterministic byte for $tag.
- Expected: the unchanged acceptance exits non-zero for MUT-001 and restored replay passes.

## Security and privacy

- Bounded fixture bytes for $tag flow to a sanitized local observation with no credential input.

## Documentation

- The exact fixture, result, exit behavior, and failure diagnosis for $tag are documented.

## Compatibility, migration, rollback

- Existing behavior remains unchanged and rollback removes only this bounded fixture ($tag).

## Review

- Reviewer A: every-hunk review across ten dimensions with correctness priority.
- Reviewer B: every-hunk review across ten dimensions with adversarial priority.
- Independence: isolated sessions, contexts, caches, memory, and sealed reports.
- Skeptical approver: pre-sealed challenge, mutation, restore, and clean replay.

## Evidence

Only \`.board/evidence/$id/\` may contain sanitized coordinator-produced artifacts.
EOF
  cat >"$root/tests/acceptance/$id.sh" <<EOF
#!/usr/bin/env bash
set -euo pipefail
printf '{"schema":"aurum.acceptance-observation","version":1,"card_id":"$id","scenario_id":"AC-001"}\n'
EOF
  chmod 0755 "$root/tests/acceptance/$id.sh"
  printf 'fixture documentation with a deterministic bounded result (%s)\n' "$tag" >"$root/docs/specs/$id.md"
  printf '| [%s](cards/ready/%s.md) | ready | O00-governance | low | `[]` | Validate one bounded parser result (%s) |\n' \
    "$id" "$id" "$tag" >>"$root/.board/INDEX.md"
}

# Same shape as write_ratchet_card, but the Given cites ONLY the card's own
# id -- no per-card tag -- so two such cards' Given collides purely through
# the AUR-NNN -> CARDREF self-id substitution and through nothing else. Every
# OTHER checked field (When/Then/Green/Non-goal/And) still carries a distinct
# per-card tag, so this fixture cannot pass or fail for an unrelated reason:
# the only thing that can possibly collide is Given, and only because of the
# self-id substitution under test.
write_bug1_card() {
  local root="$1" id="$2" tag="$3"
  write_ratchet_card "$root" "$id" "$tag" "the same replay is idempotent and reproduces the digest for $tag."
  sed -i "s#is a deterministic local fixture unique to $tag with#is a deterministic local fixture with#" \
    "$root/.board/cards/ready/$id.md"
}

write_review_report() {
  local file="$1"
  local role="$2"
  local identity="$3"
  local context="$4"
  local backend="$5"
  local dimensions=''
  local dimension
  for dimension in contract design compatibility tests security concurrency errors-operations documentation scope-simplicity hunks; do
    [[ -z "$dimensions" ]] || dimensions+=','
    dimensions+="{\"id\":\"$dimension\",\"status\":\"covered\",\"evidence_digest\":\"$(digest "dimension-$dimension")\"}"
  done
  cat >"$file" <<EOF
{"schema":"aurum.review-report","version":1,"card_id":"AUR-001","candidate_identity_digest":"$identity","role":"$role","sealed":true,"role_nonce":"nonce-$role-1234567890","context_digest":"$context","backend_family_digest":"$backend","verdict":"pass","coverage":{"all_hunks":true,"all_dimensions":true,"manifest_digest":"$(digest coverage)","uncovered_hunks":0,"dimensions":[$dimensions]},"independence_level":"I1","isolation":{"peer_report_visible_before_seal":false,"builder_trace_received":false,"shared_memory":false}}
EOF
}

make_valid_review() {
  local root="$1"
  local card_file="$root/.board/cards/review/AUR-001.md"
  local base_sha='1111111111111111111111111111111111111111'
  local spec_digest identity_input identity
  local repository_digest base_tree_digest head_tree_digest change_digest
  local configuration_digest policy_digest prompt_digest skill_digest provider_digest
  local tool_digest lock_digest image_digest test_digest role_context_digest
  local context_a context_b context_s session_a session_b session_s backend
  local report_a report_b skeptic sha_a sha_b sha_s chain_input chain_digest

  mv "$root/.board/cards/ready/AUR-001.md" "$card_file"
  sed -i 's/^status: ready$/status: review/; s/^base_sha: lock-at-execution$/base_sha: 1111111111111111111111111111111111111111/; s/^spec_digest: lock-at-execution$/spec_digest: sha256:0000000000000000000000000000000000000000000000000000000000000000/' "$card_file"
  spec_digest="sha256:$(sed -E 's/^status: .+$/status: STATE/; s/^spec_digest: sha256:[0-9a-f]{64}$/spec_digest: sha256:SELF/' "$card_file" | sha256sum | awk '{print $1}')"
  sed -i "s/^spec_digest: sha256:[0-9a-f]\{64\}$/spec_digest: $spec_digest/" "$card_file"
  sed -i 's#cards/ready/AUR-001.md#cards/review/AUR-001.md#; s/| ready |/| review |/' "$root/.board/INDEX.md"

  repository_digest="$(digest repository)"
  base_tree_digest="$(digest base-tree)"
  head_tree_digest="$(digest head-tree)"
  change_digest="$(digest change)"
  configuration_digest="$(digest configuration)"
  policy_digest="$(digest policy)"
  prompt_digest="$(digest prompt)"
  skill_digest="$(digest skills)"
  provider_digest="$(digest provider)"
  tool_digest="$(digest tools)"
  lock_digest="$(digest locks)"
  image_digest="$(digest images)"
  test_digest="$(digest tests)"
  role_context_digest="$(digest role-contexts)"
  identity_input="$(
    printf 'repository_identity=%s\n' "$repository_digest"
    printf 'base_tree_digest=%s\n' "$base_tree_digest"
    printf 'head_tree_digest=%s\n' "$head_tree_digest"
    printf 'change_digest=%s\n' "$change_digest"
    printf 'task_spec_digest=%s\n' "$spec_digest"
    printf 'configuration_digest=%s\n' "$configuration_digest"
    printf 'policy_digest=%s\n' "$policy_digest"
    printf 'prompt_and_rubric_digest=%s\n' "$prompt_digest"
    printf 'skill_set_digest=%s\n' "$skill_digest"
    printf 'provider_model_backend_identity_digest=%s\n' "$provider_digest"
    printf 'toolchain_and_tool_set_digest=%s\n' "$tool_digest"
    printf 'dependency_lock_digest=%s\n' "$lock_digest"
    printf 'container_image_set_digest=%s\n' "$image_digest"
    printf 'test_manifest_digest=%s\n' "$test_digest"
    printf 'role_context_manifest_digest=%s\n' "$role_context_digest"
  )"
  identity="sha256:$(printf '%s\n' "$identity_input" | sha256sum | awk '{print $1}')"

  context_a="$(digest context-a)"
  context_b="$(digest context-b)"
  context_s="$(digest context-s)"
  session_a="$(digest session-a)"
  session_b="$(digest session-b)"
  session_s="$(digest session-s)"
  backend="$(digest backend)"
  mkdir -p "$root/.board/evidence/AUR-001"
  report_a="$root/.board/evidence/AUR-001/a.json"
  report_b="$root/.board/evidence/AUR-001/b.json"
  skeptic="$root/.board/evidence/AUR-001/skeptic.json"
  write_review_report "$report_a" reviewer-a "$identity" "$context_a" "$backend"
  write_review_report "$report_b" reviewer-b "$identity" "$context_b" "$backend"
  cat >"$skeptic" <<EOF
{"schema":"aurum.skeptic-report","version":1,"card_id":"AUR-001","candidate_identity_digest":"$identity","role":"skeptic","sealed":true,"role_nonce":"nonce-skeptic-1234","context_digest":"$context_s","backend_family_digest":"$backend","verdict":"pass","challenge_plan":{"sealed_before_reviews":true,"digest":"$(digest challenge)","all_acceptance_criteria":true,"all_trust_boundaries":true},"isolation":{"reviews_visible_before_challenge_seal":false},"mutation":{"detected":true},"clean_replay":{"pass":true},"secret_canaries":{"pass":true}}
EOF
  sha_a="sha256:$(sha256sum "$report_a" | awk '{print $1}')"
  sha_b="sha256:$(sha256sum "$report_b" | awk '{print $1}')"
  sha_s="sha256:$(sha256sum "$skeptic" | awk '{print $1}')"
  chain_input="artifact.path=.board/evidence/AUR-001/a.json
artifact.sha256=$sha_a
artifact.path=.board/evidence/AUR-001/b.json
artifact.sha256=$sha_b
artifact.path=.board/evidence/AUR-001/skeptic.json
artifact.sha256=$sha_s
"
  chain_digest="sha256:$(printf 'candidate_identity_digest=%s\n%s' "$identity" "$chain_input" | sha256sum | awk '{print $1}')"
  cat >"$root/.board/evidence/AUR-001/manifest.json" <<EOF
{"schema":"aurum.evidence-manifest","version":1,"card_id":"AUR-001","clean_tree":true,"candidate_identity_digest":"$identity","candidate_identity":{"schema":"CandidateIdentityV1","version":1,"repository_identity":"$repository_digest","base_tree_digest":"$base_tree_digest","head_tree_digest":"$head_tree_digest","change_digest":"$change_digest","task_spec_digest":"$spec_digest","configuration_digest":"$configuration_digest","policy_digest":"$policy_digest","prompt_and_rubric_digest":"$prompt_digest","skill_set_digest":"$skill_digest","provider_model_backend_identity_digest":"$provider_digest","toolchain_and_tool_set_digest":"$tool_digest","dependency_lock_digest":"$lock_digest","container_image_set_digest":"$image_digest","test_manifest_digest":"$test_digest","role_context_manifest_digest":"$role_context_digest"},"provenance":{"base_sha":"$base_sha","head_sha":"2222222222222222222222222222222222222222"},"gates":{"status":"pass","fail_closed":true,"secret_canary":"pass","supply_chain":"pass","coverage_manifest_digest":"$(digest gate-coverage)","container_profile_digest":"$(digest profile)","test_manifest_digest":"$test_digest"},"tdd":{"acceptance_tests_locked":true,"red":{"behavioral_failure_verified":true},"green":{"status":"pass"},"refactor":{"status":"pass"}},"reviews":{"barrier_sealed":true,"sealed_before_reconciliation":true,"independence_level":"I1","a":{"path":".board/evidence/AUR-001/a.json","sha256":"$sha_a","candidate_identity_digest":"$identity","sealed":true,"context_digest":"$context_a","session_digest":"$session_a","backend_family_digest":"$backend"},"b":{"path":".board/evidence/AUR-001/b.json","sha256":"$sha_b","candidate_identity_digest":"$identity","sealed":true,"context_digest":"$context_b","session_digest":"$session_b","backend_family_digest":"$backend"}},"skeptic":{"path":".board/evidence/AUR-001/skeptic.json","sha256":"$sha_s","candidate_identity_digest":"$identity","sealed":true,"context_digest":"$context_s","session_digest":"$session_s","backend_family_digest":"$backend","challenge_presealed":true},"approval":{"unresolved_blockers":0,"verdict":"candidate_approved"},"evidence_hashes_complete":true,"evidence_chain_digest":"$chain_digest","evidence_hashes":[{"path":".board/evidence/AUR-001/a.json","sha256":"$sha_a"},{"path":".board/evidence/AUR-001/b.json","sha256":"$sha_b"},{"path":".board/evidence/AUR-001/skeptic.json","sha256":"$sha_s"}]}
EOF
}

# Re-seals the evidence bundle after a report artifact was mutated, so the
# recorded digests and the evidence chain stay internally consistent. Without
# this the harness would only prove that a digest changed, never that the
# validator asserts the mutated field itself.
reseal_evidence() {
  local root="$1"
  local evidence_root="$root/.board/evidence/AUR-001"
  local manifest="$evidence_root/manifest.json"
  local name sha identity chain_input='' chain_digest

  identity="$(grep -o '"candidate_identity_digest":"sha256:[0-9a-f]\{64\}"' "$manifest" | head -n 1 | sed 's/^.*:"//; s/"$//')"
  for name in a b skeptic; do
    sha="sha256:$(sha256sum "$evidence_root/$name.json" | awk '{print $1}')"
    sed -i "s#\(\"path\":\"\.board/evidence/AUR-001/$name\.json\",\"sha256\":\)\"sha256:[0-9a-f]\{64\}\"#\1\"$sha\"#g" "$manifest"
    printf -v chain_input '%sartifact.path=.board/evidence/AUR-001/%s.json\nartifact.sha256=%s\n' "$chain_input" "$name" "$sha"
  done
  chain_digest="sha256:$(printf 'candidate_identity_digest=%s\n%s' "$identity" "$chain_input" | sha256sum | awk '{print $1}')"
  sed -i "s#\"evidence_chain_digest\":\"sha256:[0-9a-f]\{64\}\"#\"evidence_chain_digest\":\"$chain_digest\"#" "$manifest"
}

# `expected` must be a whole-line, case-sensitive ERE that reproduces the exact
# diagnostic the validator emits for the mutated invariant. A loose substring
# (a bare word such as `Outcome` or an `a|b` alternation) lets any unrelated
# error mark the mutant as killed, so the suite could report every case green
# while the invariant under test was never enforced. Only the genuinely variable
# spans -- the temporary fixture root, a digest -- may stay wildcards.
run_case() {
  local name="$1"
  local expected="$2"
  local output="$probe_root/$name.output"
  if bash "$probe_root/$name/.board/validate.sh" >"$output" 2>&1; then
    printf 'validator mutant survived: %s\n' "$name" >&2
    return 1
  fi
  if ! grep -Eq "$expected" "$output"; then
    printf 'validator mutant failed for wrong reason: %s (expected %s)\n' "$name" "$expected" >&2
    sed -n '1,80p' "$output" >&2
    return 1
  fi
  printf 'validator mutant detected: %s\n' "$name"
}

run_pass_case() {
  local name="$1"
  local output="$probe_root/$name.output"
  if ! bash "$probe_root/$name/.board/validate.sh" >"$output" 2>&1; then
    printf 'validator rejected valid fixture: %s\n' "$name" >&2
    sed -n '1,80p' "$output" >&2
    return 1
  fi
  printf 'validator accepted valid fixture: %s\n' "$name"
}

write_base_fixture
if ! bash "$probe_root/base/.board/validate.sh" >"$probe_root/base.output" 2>&1; then
  printf 'validator fixture baseline is invalid\n' >&2
  sed -n '1,120p' "$probe_root/base.output" >&2
  exit 1
fi

prepare empty_outcome
empty_card="$probe_root/empty_outcome/.board/cards/ready/AUR-001.md"
awk '
  $0 == "## Outcome" { print; print ""; print ""; skipping=1; next }
  skipping && $0 == "## Non-goals" { skipping=0; print; next }
  !skipping { print }
' "$empty_card" >"$empty_card.new"
mv "$empty_card.new" "$empty_card"

prepare root_path
sed -i 's#^paths: .*#paths: [/]#' "$probe_root/root_path/.board/cards/ready/AUR-001.md"

# Single-variable privilege mutation: every other hardening flag the direct-OCI
# branch demands is present and correct, so `--privileged` is the only deviation
# and the case cannot be satisfied by an unrelated hardening diagnostic.
prepare privileged_acceptance
sed -i 's#^accept: .*#accept: `docker run --rm --privileged --network=none --read-only --cap-drop=ALL --security-opt=no-new-privileges --pids-limit=256 --memory=512m --cpus=1 --user=1000:1000 registry.invalid/acceptance@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef AUR-001`#' "$probe_root/privileged_acceptance/.board/cards/ready/AUR-001.md"

prepare tdd_outside
sed -i 's#^paths: .*#paths: [docs/specs/AUR-001.md]#' "$probe_root/tdd_outside/.board/cards/ready/AUR-001.md"

prepare tdd_ancestor
sed -i 's#^paths: .*#paths: [tests/acceptance/AUR-001.sh/subdir, docs/specs/AUR-001.md]#' "$probe_root/tdd_ancestor/.board/cards/ready/AUR-001.md"

prepare tdd_layer_outside
sed -i 's#^- Unit: .*#- Unit: `tests/unit/outside_test.go::TestOutside`.#' "$probe_root/tdd_layer_outside/.board/cards/ready/AUR-001.md"

prepare tdd_ambiguous
sed -i 's#^- Unit: .*#- Unit: `tests/unit/outside_test.go::TestOutside` or not-applicable: maybe later.#' "$probe_root/tdd_ambiguous/.board/cards/ready/AUR-001.md"

prepare tdd_traversal
sed -i 's#^- Test: .*#- Test: `tests/../secrets/x.sh::AC-001`.#' "$probe_root/tdd_traversal/.board/cards/ready/AUR-001.md"

prepare tdd_duplicate
sed -i '/^- Test:/a - Test: `tests/acceptance/AUR-001.sh::AC-001`.' "$probe_root/tdd_duplicate/.board/cards/ready/AUR-001.md"

prepare path_space_valid
sed -i 's#^paths: .*#paths: [tests/acceptance/AUR-001.sh, docs/specs/AUR-001.md, .env copy.example]#' "$probe_root/path_space_valid/.board/cards/ready/AUR-001.md"
printf 'safe placeholder without a credential value\n' >"$probe_root/path_space_valid/.env copy.example"

prepare path_leading_space
sed -i 's#^paths: .*#paths: [ tests/acceptance/AUR-001.sh, docs/specs/AUR-001.md]#' "$probe_root/path_leading_space/.board/cards/ready/AUR-001.md"

prepare path_trailing_space
sed -i 's#^paths: .*#paths: [tests/acceptance/AUR-001.sh , docs/specs/AUR-001.md]#' "$probe_root/path_trailing_space/.board/cards/ready/AUR-001.md"

prepare path_glob
sed -i 's#^paths: .*#paths: [tests/acceptance/AUR-001.sh, docs/specs/*.md]#' "$probe_root/path_glob/.board/cards/ready/AUR-001.md"

prepare path_double_slash
sed -i 's#^paths: .*#paths: [tests//acceptance/AUR-001.sh, docs/specs/AUR-001.md]#' "$probe_root/path_double_slash/.board/cards/ready/AUR-001.md"

# The tree loses a file a card owns: `docs/specs/AUR-001.md` is declared in the
# card's `paths`, the repository still tracks it, and the working tree no longer
# carries it. The card is in `ready`, so no state rule applies and only the
# deletion rule can catch it. This is the class the board's disjoint ownership
# depends on: one lane must not be able to delete another card's property while
# the gate reports success.
prepare path_deleted_from_tree
rm "$probe_root/path_deleted_from_tree/docs/specs/AUR-001.md"

# The same loss, committed the way a lane that produces a patch actually causes
# it: `git rm` drops the path from the index and from the working tree in one
# step, so the index no longer names it and an index-only gate sees planned work.
# Only the committed tree still remembers the file, which is why the tracked set
# is the union of both records.
prepare path_git_rm_from_index
git -C "$probe_root/path_git_rm_from_index" rm -q -- docs/specs/AUR-001.md

# The other half of the rule, isolated: a path that was never tracked and is
# absent, on a card in a state that requires the artifact to be materialized.
# Nothing was deleted, so the deletion rule cannot fire here.
prepare active_path_missing
sed -i 's#^paths: .*#paths: [tests/acceptance/AUR-001.sh, docs/specs/AUR-001.md, docs/specs/AUR-001-absent.md]#' "$probe_root/active_path_missing/.board/cards/ready/AUR-001.md"
make_valid_review "$probe_root/active_path_missing"

# Fail-closed control: without a readable index, planned work and a deleted
# artifact are indistinguishable, and the gate must say so rather than assume
# the friendly reading.
prepare path_index_unavailable
sed -i 's#^paths: .*#paths: [tests/acceptance/AUR-001.sh, docs/specs/AUR-001.md, docs/specs/AUR-001-absent.md]#' "$probe_root/path_index_unavailable/.board/cards/ready/AUR-001.md"
rm -rf "$probe_root/path_index_unavailable/.git"

# Positive control against a false red: a card that has not been executed yet
# may declare the artifact it is supposed to create. Absent plus untracked is
# exactly what planned work looks like, and it must stay green.
prepare path_future_valid
sed -i 's#^paths: .*#paths: [tests/acceptance/AUR-001.sh, docs/specs/AUR-001.md, docs/specs/AUR-001-future.md]#' "$probe_root/path_future_valid/.board/cards/ready/AUR-001.md"

prepare covers_partial
sed -i '/^- Then:/a - Covers requirements: [PR-TST-001]' "$probe_root/covers_partial/.board/cards/ready/AUR-001.md"

prepare orphan_requirement
printf '| PR-ORP-001 | This requirement has no implementing card. |\n' >>"$probe_root/orphan_requirement/.board/requirements/REQUIREMENTS.md"

prepare orphan_control
printf '\n`CR-ORP-001` This control has no implementing card.\n' >>"$probe_root/orphan_control/.board/research/code-review-standards.md"

prepare evidence_chain
make_valid_review "$probe_root/evidence_chain"
sed -i 's/"evidence_chain_digest":"sha256:[0-9a-f]\{64\}"/"evidence_chain_digest":"sha256:0000000000000000000000000000000000000000000000000000000000000000"/' "$probe_root/evidence_chain/.board/evidence/AUR-001/manifest.json"

prepare forged_human
make_valid_review "$probe_root/forged_human"
sed -i '/"evidence_hashes":/s/"evidence_hashes":/"human_approval":{"authenticated":true,"actor_type":"human"},"evidence_hashes":/' "$probe_root/forged_human/.board/evidence/AUR-001/manifest.json"

prepare unverified_done
make_valid_review "$probe_root/unverified_done"
mv "$probe_root/unverified_done/.board/cards/review/AUR-001.md" "$probe_root/unverified_done/.board/cards/done/AUR-001.md"
sed -i 's/^status: review$/status: done/' "$probe_root/unverified_done/.board/cards/done/AUR-001.md"
sed -i 's#cards/review/AUR-001.md#cards/done/AUR-001.md#; s/| review |/| done |/' "$probe_root/unverified_done/.board/INDEX.md"
# A human event is no longer a gate, so this mutant tests what its name claims:
# a card promoted to `done` whose acceptance was never sealed.
sed -i 's/"sealed":true/"sealed":false/' "$probe_root/unverified_done/.board/evidence/AUR-001/manifest.json"

# Positive control for every sealed-bundle mutant below: an untouched review
# bundle that is re-sealed by the same helper must still pass, so a mutant that
# fails proves the mutated invariant and not a broken re-seal.
prepare sealed_bundle_valid
make_valid_review "$probe_root/sealed_bundle_valid"
reseal_evidence "$probe_root/sealed_bundle_valid"

# Acceptance-test tampering: the sealed acceptance result is rewritten after the
# seal and every digest is forged back into agreement.
prepare acceptance_tampered
make_valid_review "$probe_root/acceptance_tampered"
sed -i 's/"verdict":"pass"/"verdict":"block"/' "$probe_root/acceptance_tampered/.board/evidence/AUR-001/a.json"
reseal_evidence "$probe_root/acceptance_tampered"

# Role reuse: the skeptic actor also signs as reviewer A, reusing that role and
# its nonce instead of holding an independent role.
prepare role_reuse
make_valid_review "$probe_root/role_reuse"
sed -i 's/"role":"skeptic"/"role":"reviewer-a"/; s/"role_nonce":"nonce-skeptic-1234"/"role_nonce":"nonce-reviewer-a-1234567890"/' "$probe_root/role_reuse/.board/evidence/AUR-001/skeptic.json"
reseal_evidence "$probe_root/role_reuse"

# Pre-seal leakage: reviewer B saw the peer report before the sealing barrier.
prepare preseal_leak
make_valid_review "$probe_root/preseal_leak"
sed -i 's/"peer_report_visible_before_seal":false/"peer_report_visible_before_seal":true/' "$probe_root/preseal_leak/.board/evidence/AUR-001/b.json"
reseal_evidence "$probe_root/preseal_leak"

# Stale identity: the sealed report binds a CandidateIdentityV1 digest that is
# not the current candidate's.
prepare stale_identity
make_valid_review "$probe_root/stale_identity"
sed -i "s/\"candidate_identity_digest\":\"sha256:[0-9a-f]\{64\}\"/\"candidate_identity_digest\":\"$(digest stale-candidate)\"/" "$probe_root/stale_identity/.board/evidence/AUR-001/a.json"
reseal_evidence "$probe_root/stale_identity"

# Mutation survival: the skeptical mutation was applied and acceptance still
# passed, so the acceptance proof is not falsifiable.
prepare mutation_survived
make_valid_review "$probe_root/mutation_survived"
sed -i 's/"mutation":{"detected":true}/"mutation":{"detected":false}/' "$probe_root/mutation_survived/.board/evidence/AUR-001/skeptic.json"
reseal_evidence "$probe_root/mutation_survived"

# Replay: re-running the acceptance in a clean clone does not reproduce.
prepare replay_failure
make_valid_review "$probe_root/replay_failure"
sed -i 's/"clean_replay":{"pass":true}/"clean_replay":{"pass":false}/' "$probe_root/replay_failure/.board/evidence/AUR-001/skeptic.json"
reseal_evidence "$probe_root/replay_failure"

# Secret canaries: a planted secret canary escaped the sanitization boundary.
prepare canary_leak
make_valid_review "$probe_root/canary_leak"
sed -i 's/"secret_canaries":{"pass":true}/"secret_canaries":{"pass":false}/' "$probe_root/canary_leak/.board/evidence/AUR-001/skeptic.json"
reseal_evidence "$probe_root/canary_leak"

# --- Spec-collision: BUG 1 (normalize_spec_text ordering) ------------------
# Two cards whose Given differs ONLY by each citing its own id must collide
# under the fixed validator and must NOT collide under the pre-fix one --
# proving the dead-code claim both ways, not just asserting the fix in
# isolation. `old_validate` is the exact pre-patch script (git HEAD); the copy
# under test everywhere else in this file is already the patched one.
old_validate="$probe_root/validate.sh.pre-patch"
# The "unfixed" validator is derived from the CURRENT file by re-breaking
# normalize_spec_text the way the original bug did: the self-id substitution
# is moved after the lowercasing pass, where its upper-case pattern can never
# match. Deriving it from HEAD via git show self-expires the moment the fix
# is committed -- HEAD then IS the fixed validator -- which is how this
# fixture broke once before.
cp "$repo_root/.board/validate.sh" "$old_validate"
python3 - "$old_validate" <<'PYEOF'
import re, sys
p = sys.argv[1]
s = open(p).read()
sub_block = """  if [[ -n \"$self_id\" ]]; then
    text=\"$(printf '%s' \"$text\" | sed -E \"s/${self_id}/CARDREF/g\")\"
  fi
"""
assert sub_block in s, "self-id substitution block not found; fixture needs updating"
s = s.replace(sub_block, "", 1)
lower_line = "    | tr '[:upper:]' '[:lower:]' \\\n"
assert lower_line in s, "lowercase pipeline line not found; fixture needs updating"
s = s.replace(lower_line, lower_line + "    | sed -E \"s/${self_id:-ZZNEVERMATCHZZ}/CARDREF/g\" \\\n", 1)
open(p, "w").write(s)
PYEOF
chmod 0755 "$old_validate"

write_ratchet_skeleton "$probe_root/bug1_fixed"
write_bug1_card "$probe_root/bug1_fixed" AUR-001 alpha
write_bug1_card "$probe_root/bug1_fixed" AUR-002 beta

cp -R "$probe_root/bug1_fixed" "$probe_root/bug1_unfixed"
cp "$old_validate" "$probe_root/bug1_unfixed/.board/validate.sh"
chmod 0755 "$probe_root/bug1_unfixed/.board/validate.sh"

# --- Spec-collision: BUG 2 ratchet mechanism --------------------------------
# Three cards, every field unique except AC-001's And, which AUR-alpha and
# AUR-beta share verbatim by construction (AUR-gamma's And is distinct).
write_ratchet_skeleton "$probe_root/ratchet_base"
write_ratchet_card "$probe_root/ratchet_base" AUR-001 alpha 'the same replay is idempotent and reproduces the digest.'
write_ratchet_card "$probe_root/ratchet_base" AUR-002 beta 'the same replay is idempotent and reproduces the digest.'
write_ratchet_card "$probe_root/ratchet_base" AUR-003 gamma 'a distinct third-card assertion that shares nothing with the others.'

# (c) baseline generated for exactly the current collision -> green.
cp -R "$probe_root/ratchet_base" "$probe_root/ratchet_pass"
cat >"$probe_root/ratchet_pass/.board/tests/spec-collision-baseline.txt" <<'EOF'
and AUR-001/AC-001 AUR-002/AC-001
EOF

# (a) a NEW collision (AUR-003's And is changed to match) is not in the
# baseline above -> reject. Same baseline as the green case; only the tree
# changes underneath it.
cp -R "$probe_root/ratchet_base" "$probe_root/ratchet_new_collision"
cp "$probe_root/ratchet_pass/.board/tests/spec-collision-baseline.txt" \
  "$probe_root/ratchet_new_collision/.board/tests/spec-collision-baseline.txt"
sed -i "s#a distinct third-card assertion that shares nothing with the others\.#the same replay is idempotent and reproduces the digest.#" \
  "$probe_root/ratchet_new_collision/.board/cards/ready/AUR-003.md"

# (b) a DEAD baseline entry: the baseline still claims AUR-001/AUR-002
# collide, but AUR-002's And was edited so it no longer does -> reject.
cp -R "$probe_root/ratchet_base" "$probe_root/ratchet_dead_entry"
cp "$probe_root/ratchet_pass/.board/tests/spec-collision-baseline.txt" \
  "$probe_root/ratchet_dead_entry/.board/tests/spec-collision-baseline.txt"
sed -i "s#the same replay is idempotent and reproduces the digest\.#a healed, no-longer-colliding assertion for beta.#" \
  "$probe_root/ratchet_dead_entry/.board/cards/ready/AUR-002.md"

card='\.board/cards/[a-z-]+/AUR-001\.md'
manifest='\.board/evidence/AUR-001/manifest\.json'
report='\.board/evidence/AUR-001'

pids=()
run_case empty_outcome \
  "^board error: .*/$card: ## Outcome must contain a specific, falsifiable statement\$" & pids+=("$!")
run_case root_path \
  "^board error: .*/$card: unsafe or non-repository-relative owned path: /\$" & pids+=("$!")
run_case privileged_acceptance \
  "^board error: .*/$card: acceptance contains shell composition, privilege escalation, host namespace/socket, or environment injection\$" & pids+=("$!")
run_case tdd_outside \
  "^board error: .*/$card: Acceptance TDD test is outside owned paths: tests/acceptance/AUR-001\.sh\$" & pids+=("$!")
run_case tdd_ancestor \
  "^board error: .*/$card: Acceptance TDD test is outside owned paths: tests/acceptance/AUR-001\.sh\$" & pids+=("$!")
run_case tdd_layer_outside \
  "^board error: .*/$card: Unit TDD test is outside owned paths: tests/unit/outside_test\.go\$" & pids+=("$!")
run_case tdd_ambiguous \
  "^board error: .*/$card: Unit must be exactly a backtick-delimited path::selector or not-applicable with a concrete reason\$" & pids+=("$!")
run_case tdd_traversal \
  "^board error: .*/$card: Acceptance test path is unsafe or not repository-relative: tests/\.\./secrets/x\.sh\$" & pids+=("$!")
run_case tdd_duplicate \
  "^board error: .*/$card: TDD proof must contain exactly one Test reference\$" & pids+=("$!")
run_pass_case path_space_valid & pids+=("$!")
run_case path_leading_space \
  "^board error: .*/$card: paths list is not canonical\$" & pids+=("$!")
run_case path_trailing_space \
  "^board error: .*/$card: paths list is not canonical\$" & pids+=("$!")
run_case path_glob \
  "^board error: .*/$card: unsafe or non-repository-relative owned path: docs/specs/\*\.md\$" & pids+=("$!")
run_case path_double_slash \
  "^board error: .*/$card: unsafe or non-repository-relative owned path: tests//acceptance/AUR-001\.sh\$" & pids+=("$!")
run_case path_deleted_from_tree \
  "^board error: .*/$card: declared path is tracked by the repository but missing from the tree: docs/specs/AUR-001\.md \(claimed by AUR-001\)\$" & pids+=("$!")
run_case path_git_rm_from_index \
  "^board error: .*/$card: declared path is tracked by the repository but missing from the tree: docs/specs/AUR-001\.md \(claimed by AUR-001\)\$" & pids+=("$!")
run_case active_path_missing \
  "^board error: .*/$card: review card declares owned path docs/specs/AUR-001-absent\.md, which is absent from the tree\$" & pids+=("$!")
run_case path_index_unavailable \
  "^board error: declared-path materialization is unverifiable: 1 declared path\(s\) are absent and .+ exposes no readable Git index to separate planned work from a deleted artifact\$" & pids+=("$!")
run_pass_case path_future_valid & pids+=("$!")
run_case covers_partial \
  "^board error: .*/$card: AC-001 must contain exactly one Covers controls list\$" & pids+=("$!")
run_case orphan_requirement \
  '^board error: requirements registry contains orphan product requirement PR-ORP-001$' & pids+=("$!")
run_case orphan_control \
  '^board error: code-review standards contain orphan control CR-ORP-001$' & pids+=("$!")
run_case evidence_chain \
  "^board error: .*/$manifest: evidence_chain_digest does not match the canonical CandidateIdentity/path/hash chain\$" & pids+=("$!")
run_case forged_human \
  "^board error: .*/$manifest: human_approval is not an accepted gate; prove the behavior and its skeptical mutation instead\$" & pids+=("$!")
run_case unverified_done \
  "^board error: .*/$manifest: acceptance.sealed must equal true\$" & pids+=("$!")
run_pass_case sealed_bundle_valid & pids+=("$!")
run_case acceptance_tampered \
  "^board error: .*/$report/a\.json: a review/done card requires a non-blocking sealed verdict\$" & pids+=("$!")
run_case role_reuse \
  "^board error: .*/$report/skeptic\.json: role must equal skeptic\$" & pids+=("$!")
run_case preseal_leak \
  "^board error: .*/$report/b\.json: isolation\.peer_report_visible_before_seal must equal false\$" & pids+=("$!")
run_case stale_identity \
  "^board error: .*/$report/a\.json: candidate_identity_digest must equal sha256:[0-9a-f]{64}\$" & pids+=("$!")
run_case mutation_survived \
  "^board error: .*/$report/skeptic\.json: mutation\.detected must equal true\$" & pids+=("$!")
run_case replay_failure \
  "^board error: .*/$report/skeptic\.json: clean_replay\.pass must equal true\$" & pids+=("$!")
run_case canary_leak \
  "^board error: .*/$report/skeptic\.json: secret_canaries\.pass must equal true\$" & pids+=("$!")

# BUG 1 (normalize_spec_text ordering): two cards whose Given differs ONLY by
# each citing its own id. The patched validator must detect it (dead-code
# claim disproven); the pre-patch validator (bug1_unfixed, running the exact
# git-HEAD script) must NOT (dead-code claim proven both ways).
run_case bug1_fixed \
  "^board error: spec-collision ratchet: new given collision not recorded in the committed baseline \(.*\): AUR-001/AC-001 AUR-002/AC-001\$" & pids+=("$!")
run_pass_case bug1_unfixed & pids+=("$!")

# BUG 2 ratchet mechanism, the three mandated mutants:
# (c) baseline generated for exactly the current tree's collisions -> green.
run_pass_case ratchet_pass & pids+=("$!")
# (a) a collision outside the baseline (AUR-003 now also collides) -> reject.
run_case ratchet_new_collision \
  "^board error: spec-collision ratchet: new and collision not recorded in the committed baseline \(.*\): AUR-001/AC-001 AUR-003/AC-001\$" & pids+=("$!")
# (b) a baseline entry that no longer collides (AUR-002's And healed) -> reject.
run_case ratchet_dead_entry \
  "^board error: spec-collision ratchet: baseline entry for and no longer collides; the ratchet only shrinks, remove the stale entry: AUR-001/AC-001 AUR-002/AC-001\$" & pids+=("$!")

# Cancelled lane: the valid three-card fixture (cancelled + successor +
# dependent-on-both) is the positive control every mutant below is a single
# deviation from.
run_pass_case cancel_valid & pids+=("$!")
# Requirement 1: cancelled must be locked like doing/review/done.
run_case cancel_unlocked \
  "^board error: .*/\.board/cards/cancelled/AUR-001\.md: locked base_sha must be an immutable Git object id\$" & pids+=("$!")
# Requirement 2: cancellation.json's reason must be specific, not generic.
run_case cancel_generic_reason \
  "^board error: .*/\.board/evidence/AUR-001/cancellation\.json: reason must be a specific, non-generic explanation of at least 40 characters\$" & pids+=("$!")
# Requirement 3: depending on a cancelled card without also depending on its
# declared successor must fail -- cancellation can never silently orphan the
# DAG.
run_case cancel_no_override \
  "^board error: .*/\.board/cards/backlog/AUR-003\.md: depends on cancelled AUR-001 but does not also depend on its declared successor AUR-002\$" & pids+=("$!")
# Requirement 4: INDEX.md must carry the canonical row for the cancelled card.
run_case cancel_index_missing \
  '^board error: INDEX\.md lacks the canonical row for AUR-001$' & pids+=("$!")
# Requirement 5: cancellation is not completion; a cancelled card must never
# carry a done evidence bundle.
run_case cancel_has_manifest \
  "^board error: .*/\.board/cards/cancelled/AUR-001\.md: cancelled card must not carry a done evidence bundle \(\.board/evidence/AUR-001/manifest\.json\); cancellation is not completion\$" & pids+=("$!")

failed=0
for pid in "${pids[@]}"; do
  wait "$pid" || failed=1
done
(( failed == 0 ))
