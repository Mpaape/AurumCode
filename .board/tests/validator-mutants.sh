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
  # The dispatcher is not decoration. A second reader proves that a declared
  # shell selector selects *something* by handing the program a selector it
  # cannot know and requiring exit 64; a program that ignores its argument
  # answers the same way to every name, and the declared layer name then proves
  # nothing. The `sr_no_assertion` mutant below reverts exactly this dispatcher.
  cat >"$root/tests/acceptance/AUR-001.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
selector="${1:-AC-001}"
case "$selector" in
  AC-001|IntegrationAUR001) ;;
  *) printf 'AUR-001/AC-001/unknown-selector\n' >&2; exit 64 ;;
esac
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
  # The gate is no longer opt-in: a card entering `review`/`done` must hand the
  # second reader something it can execute or be named in the exemption
  # registry. The base card therefore carries the concrete Integration citation
  # from the start, and the mutants that need its ABSENCE revert it explicitly.
  cite_integration "$root"
  # The image lock the `=== image:` frame is recomputed against. The fixture's
  # second reader is the shell engine, so only that lock is needed; the digest
  # is the same one `write_second_reader_log` frames.
  mkdir -p "$root/.board/locks/oci"
  cat >"$root/.board/locks/oci/second-reader-shell-v1.lock.json" <<'EOF'
{
"schema": "aurum.oci-image-lock",
"version": 1,
"profile": "second-reader-shell-v1",
"image": "bash@sha256:ae4668c2560999e65e89532cd2ad1b6688bb23298189f0bd229ef80fa4bd0831"
}
EOF
  # `exec` is the default mode and a layer that is not re-executed is now
  # INCONCLUSIVE rather than a silent pass. A hermetic stub executor plus a stub
  # engine on PATH is what keeps every fixture below deterministic on a host
  # with an OCI engine and on one without: the harness proves the validator's
  # reaction to its executor, never the host's inventory.
  mkdir -p "$root/.board/bin" "$root/stub-path"
  printf '#!/usr/bin/env bash\nexit 0\n' >"$root/.board/bin/second-reader"
  chmod 0755 "$root/.board/bin/second-reader"
  printf '#!/usr/bin/env bash\nexit 0\n' >"$root/stub-path/docker"
  chmod 0755 "$root/stub-path/docker"
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

# --- Second reader ------------------------------------------------------------
# `.board/bin/oci-run` executes ONE program inside a toolchain-free image with
# only the card's own paths materialized, so a concrete `Integration:` citation
# was never run by any gate. The fixtures below build a `done` bundle that
# carries a real second-reader record, and each mutant breaks exactly one of the
# invariants that make that record mean something.
#
# The fixture's second reader is the SHELL engine on purpose: it needs no Go
# toolchain and no container, so the harness stays hermetic and deterministic on
# any machine, while exercising the same recomputation path the Go engine uses.

# Writes the raw log exactly as `.board/bin/second-reader` frames it. Every
# captured line carries a `| ` prefix, which is what makes the frame lines
# unforgeable by the program under test.
write_second_reader_log() {
  local file="$1" root="$2" exit_code="$3" control_exit="$4" body="$5"
  local script="$root/tests/acceptance/AUR-001.sh"
  mkdir -p "$(dirname "$file")"
  {
    printf '=== second-reader-log v1\n'
    printf '=== card: AUR-001\n'
    printf '=== layer: Integration\n'
    printf '=== engine: shell\n'
    printf '=== test-path: tests/acceptance/AUR-001.sh\n'
    printf '=== selector: IntegrationAUR001\n'
    printf '=== test-file-sha256: sha256:%s\n' "$(sha256sum "$script" | awk '{print $1}')"
    printf '=== image: bash@sha256:ae4668c2560999e65e89532cd2ad1b6688bb23298189f0bd229ef80fa4bd0831\n'
    printf '=== command: bash tests/acceptance/AUR-001.sh IntegrationAUR001\n'
    printf '=== output-begin\n'
    printf '%s\n' "$body"
    printf '=== output-end\n'
    printf '=== exit: %s\n' "$exit_code"
    printf '=== control-command: bash tests/acceptance/AUR-001.sh AURUM-SECOND-READER-CONTROL\n'
    printf '=== control-output-begin\n'
    printf '| AUR-001/AC-001/unknown-selector\n'
    printf '=== control-output-end\n'
    printf '=== control-exit: %s\n' "$control_exit"
  } >"$file"
}

write_second_reader_observation() {
  local file="$1" root="$2" identity="$3" context="$4" backend="$5" matched="$6" exit_code="$7"
  local script="$root/tests/acceptance/AUR-001.sh"
  local log="$root/.board/evidence/AUR-001/second-reader/Integration.raw.txt"
  local declaration='- Integration: `tests/acceptance/AUR-001.sh::IntegrationAUR001`.'
  local command='bash tests/acceptance/AUR-001.sh IntegrationAUR001'
  cat >"$file" <<EOF
{"schema":"aurum.second-reader-observation","version":1,"card_id":"AUR-001","candidate_identity_digest":"$identity","role":"second-reader","sealed":true,"role_nonce":"nonce-second-reader-1234567890","context_digest":"$context","backend_family_digest":"$backend","observation_trusted":false,"layer":"Integration","engine":"shell","test_path":"tests/acceptance/AUR-001.sh","selector":"IntegrationAUR001","declaration_digest":"$(digest "$declaration")","test_file_digest":"sha256:$(sha256sum "$script" | awk '{print $1}')","command_digest":"$(digest "$command")","raw_output_path":".board/evidence/AUR-001/second-reader/Integration.raw.txt","raw_output_sha256":"sha256:$(sha256sum "$log" | awk '{print $1}')","matched_tests":$matched,"exit_code":$exit_code,"control_exit_code":64,"verdict":"pass"}
EOF
}

# A complete, valid `done` bundle: two sealed reviews, a sealed skeptic, the
# sealed acceptance observation, and the sealed second-reader record for the
# card's one concrete TDD citation. Every mutant below is one deviation from it.
make_valid_done() {
  local root="$1"
  local with_second_reader="${2:-yes}"
  local card_file="$root/.board/cards/done/AUR-001.md"
  local base_sha='1111111111111111111111111111111111111111'
  local spec_digest identity_input identity evidence_root
  local repository_digest base_tree_digest head_tree_digest change_digest
  local configuration_digest policy_digest prompt_digest skill_digest provider_digest
  local tool_digest lock_digest image_digest test_digest role_context_digest
  local context_a context_b context_s context_x context_r
  local session_a session_b session_s session_x backend profile_digest
  local report_a report_b skeptic acceptance observation log_file
  local accept_command

  mv "$root/.board/cards/ready/AUR-001.md" "$card_file"
  sed -i 's/^status: ready$/status: done/; s/^base_sha: lock-at-execution$/base_sha: 1111111111111111111111111111111111111111/; s/^spec_digest: lock-at-execution$/spec_digest: sha256:0000000000000000000000000000000000000000000000000000000000000000/' "$card_file"
  spec_digest="sha256:$(sed -E 's/^status: .+$/status: STATE/; s/^spec_digest: sha256:[0-9a-f]{64}$/spec_digest: sha256:SELF/' "$card_file" | sha256sum | awk '{print $1}')"
  sed -i "s/^spec_digest: sha256:[0-9a-f]\{64\}$/spec_digest: $spec_digest/" "$card_file"
  sed -i 's#cards/ready/AUR-001.md#cards/done/AUR-001.md#; s/| ready |/| done |/' "$root/.board/INDEX.md"

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

  context_a="$(digest context-a)"; context_b="$(digest context-b)"
  context_s="$(digest context-s)"; context_x="$(digest context-x)"
  context_r="$(digest context-r)"
  session_a="$(digest session-a)"; session_b="$(digest session-b)"
  session_s="$(digest session-s)"; session_x="$(digest session-x)"
  backend="$(digest backend)"
  profile_digest="$(digest profile)"
  accept_command='./.board/bin/oci-run --profile bootstrap-readonly-v1 --card AUR-001'

  evidence_root="$root/.board/evidence/AUR-001"
  mkdir -p "$evidence_root/acceptance" "$evidence_root/second-reader"
  report_a="$evidence_root/a.json"
  report_b="$evidence_root/b.json"
  skeptic="$evidence_root/skeptic.json"
  acceptance="$evidence_root/acceptance/AC-001.json"
  observation="$evidence_root/second-reader/Integration.json"
  log_file="$evidence_root/second-reader/Integration.raw.txt"
  write_review_report "$report_a" reviewer-a "$identity" "$context_a" "$backend"
  write_review_report "$report_b" reviewer-b "$identity" "$context_b" "$backend"
  cat >"$skeptic" <<EOF
{"schema":"aurum.skeptic-report","version":1,"card_id":"AUR-001","candidate_identity_digest":"$identity","role":"skeptic","sealed":true,"role_nonce":"nonce-skeptic-1234","context_digest":"$context_s","backend_family_digest":"$backend","verdict":"pass","challenge_plan":{"sealed_before_reviews":true,"digest":"$(digest challenge)","all_acceptance_criteria":true,"all_trust_boundaries":true},"isolation":{"reviews_visible_before_challenge_seal":false},"mutation":{"detected":true},"clean_replay":{"pass":true},"secret_canaries":{"pass":true}}
EOF
  cat >"$acceptance" <<EOF
{"schema":"aurum.acceptance-observation","version":1,"card_id":"AUR-001","candidate_identity_digest":"$identity","role":"acceptance","sealed":true,"role_nonce":"nonce-acceptance-1234567890","context_digest":"$context_x","backend_family_digest":"$backend","verdict":"pass","command_digest":"$(digest "$accept_command")","container_profile_digest":"$profile_digest","red":{"exit_code":1},"green":{"exit_code":0},"mutation":{"detected":true,"exit_code":1,"restored_exit_code":0,"mutation_id":"MUT-001"},"clean_replay":{"exit_code":0},"secret_canaries":{"pass":true,"leaked":0},"scenarios":[{"id":"AC-001","exit_code":0}]}
EOF
  if [[ "$with_second_reader" == yes ]]; then
    write_second_reader_log "$log_file" "$root" 0 64 '| {"schema":"aurum.acceptance-observation","version":1,"card_id":"AUR-001","scenario_id":"AC-001"}'
    write_second_reader_observation "$observation" "$root" "$identity" "$context_r" "$backend" 1 0
  else
    rm -rf "$evidence_root/second-reader"
  fi

  reseal_done_evidence "$root" "$identity" "$base_sha" "$spec_digest" \
    "$repository_digest" "$base_tree_digest" "$head_tree_digest" "$change_digest" \
    "$configuration_digest" "$policy_digest" "$prompt_digest" "$skill_digest" \
    "$provider_digest" "$tool_digest" "$lock_digest" "$image_digest" \
    "$test_digest" "$role_context_digest" \
    "$context_a" "$context_b" "$context_s" "$context_x" \
    "$session_a" "$session_b" "$session_s" "$session_x" \
    "$backend" "$profile_digest" "$accept_command"
}

# Recomputes every digest the `done` manifest binds, from whatever the evidence
# files currently contain. A mutant edits an artifact and calls this, so the
# bundle stays internally consistent and the mutant proves the mutated invariant
# rather than a broken re-seal.
reseal_done_evidence() {
  local root="$1" identity="$2" base_sha="$3" spec_digest="$4"
  local repository_digest="$5" base_tree_digest="$6" head_tree_digest="$7" change_digest="$8"
  local configuration_digest="$9" policy_digest="${10}" prompt_digest="${11}" skill_digest="${12}"
  local provider_digest="${13}" tool_digest="${14}" lock_digest="${15}" image_digest="${16}"
  local test_digest="${17}" role_context_digest="${18}"
  local context_a="${19}" context_b="${20}" context_s="${21}" context_x="${22}"
  local session_a="${23}" session_b="${24}" session_s="${25}" session_x="${26}"
  local backend="${27}" profile_digest="${28}" accept_command="${29}"
  local evidence_root="$root/.board/evidence/AUR-001"
  local file relative sha chain_input='' chain_digest hashes=''
  local -a artifacts=()

  while IFS= read -r -d '' file; do
    [[ "$file" != "$evidence_root/manifest.json" ]] || continue
    artifacts+=("$file")
  done < <(find "$evidence_root" -type f -print0 | LC_ALL=C sort -z)

  for file in "${artifacts[@]}"; do
    relative=".board/evidence/AUR-001/${file#"$evidence_root/"}"
    sha="sha256:$(sha256sum "$file" | awk '{print $1}')"
    printf -v chain_input '%sartifact.path=%s\nartifact.sha256=%s\n' "$chain_input" "$relative" "$sha"
    [[ -z "$hashes" ]] || hashes+=','
    hashes+="{\"path\":\"$relative\",\"sha256\":\"$sha\"}"
  done
  chain_digest="sha256:$(printf 'candidate_identity_digest=%s\n%s' "$identity" "$chain_input" | sha256sum | awk '{print $1}')"

  local sha_a sha_b sha_s sha_x
  sha_a="sha256:$(sha256sum "$evidence_root/a.json" | awk '{print $1}')"
  sha_b="sha256:$(sha256sum "$evidence_root/b.json" | awk '{print $1}')"
  sha_s="sha256:$(sha256sum "$evidence_root/skeptic.json" | awk '{print $1}')"
  sha_x="sha256:$(sha256sum "$evidence_root/acceptance/AC-001.json" | awk '{print $1}')"

  cat >"$evidence_root/manifest.json" <<EOF
{"schema":"aurum.evidence-manifest","version":1,"card_id":"AUR-001","clean_tree":true,"candidate_identity_digest":"$identity","candidate_identity":{"schema":"CandidateIdentityV1","version":1,"repository_identity":"$repository_digest","base_tree_digest":"$base_tree_digest","head_tree_digest":"$head_tree_digest","change_digest":"$change_digest","task_spec_digest":"$spec_digest","configuration_digest":"$configuration_digest","policy_digest":"$policy_digest","prompt_and_rubric_digest":"$prompt_digest","skill_set_digest":"$skill_digest","provider_model_backend_identity_digest":"$provider_digest","toolchain_and_tool_set_digest":"$tool_digest","dependency_lock_digest":"$lock_digest","container_image_set_digest":"$image_digest","test_manifest_digest":"$test_digest","role_context_manifest_digest":"$role_context_digest"},"provenance":{"base_sha":"$base_sha","head_sha":"2222222222222222222222222222222222222222"},"gates":{"status":"pass","fail_closed":true,"secret_canary":"pass","supply_chain":"pass","coverage_manifest_digest":"$(digest gate-coverage)","container_profile_digest":"$profile_digest","test_manifest_digest":"$test_digest"},"tdd":{"acceptance_tests_locked":true,"red":{"behavioral_failure_verified":true},"green":{"status":"pass"},"refactor":{"status":"pass"}},"reviews":{"barrier_sealed":true,"sealed_before_reconciliation":true,"independence_level":"I1","a":{"path":".board/evidence/AUR-001/a.json","sha256":"$sha_a","candidate_identity_digest":"$identity","sealed":true,"context_digest":"$context_a","session_digest":"$session_a","backend_family_digest":"$backend"},"b":{"path":".board/evidence/AUR-001/b.json","sha256":"$sha_b","candidate_identity_digest":"$identity","sealed":true,"context_digest":"$context_b","session_digest":"$session_b","backend_family_digest":"$backend"}},"skeptic":{"path":".board/evidence/AUR-001/skeptic.json","sha256":"$sha_s","candidate_identity_digest":"$identity","sealed":true,"context_digest":"$context_s","session_digest":"$session_s","backend_family_digest":"$backend","challenge_presealed":true},"acceptance":{"path":".board/evidence/AUR-001/acceptance/AC-001.json","sha256":"$sha_x","candidate_identity_digest":"$identity","sealed":true,"command_digest":"$(digest "$accept_command")","context_digest":"$context_x","session_digest":"$session_x","backend_family_digest":"$backend"},"approval":{"unresolved_blockers":0,"verdict":"candidate_approved"},"evidence_hashes_complete":true,"evidence_chain_digest":"$chain_digest","evidence_hashes":[$hashes]}
EOF
}

# Re-seals a `done` bundle after a mutant edited one of its artifacts, reusing
# the digests the fixture generator fixed. Kept as a thin wrapper so a mutant
# reads as one edit plus one re-seal.
reseal_done() {
  local root="$1"
  reseal_done_evidence "$root" \
    "$(digest_from_manifest "$root" candidate_identity_digest)" \
    '1111111111111111111111111111111111111111' \
    "$(sed -n 's/^spec_digest: //p' "$root/.board/cards/done/AUR-001.md")" \
    "$(digest repository)" "$(digest base-tree)" "$(digest head-tree)" "$(digest change)" \
    "$(digest configuration)" "$(digest policy)" "$(digest prompt)" "$(digest skills)" \
    "$(digest provider)" "$(digest tools)" "$(digest locks)" "$(digest images)" \
    "$(digest tests)" "$(digest role-contexts)" \
    "$(digest context-a)" "$(digest context-b)" "$(digest context-s)" "$(digest context-x)" \
    "$(digest session-a)" "$(digest session-b)" "$(digest session-s)" "$(digest session-x)" \
    "$(digest backend)" "$(digest profile)" \
    './.board/bin/oci-run --profile bootstrap-readonly-v1 --card AUR-001'
}

digest_from_manifest() {
  grep -o "\"$2\":\"sha256:[0-9a-f]\{64\}\"" "$1/.board/evidence/AUR-001/manifest.json" |
    head -n 1 | sed 's/^.*:"//; s/"$//'
}

# Turns the base card's Integration layer into a concrete, executable citation.
# Must run BEFORE the card is sealed, because it changes the spec digest.
cite_integration() {
  local root="$1"
  local target="${2:-tests/acceptance/AUR-001.sh::IntegrationAUR001}"
  sed -i "s#^- Integration: .*#- Integration: \`$target\`.#" "$root/.board/cards/ready/AUR-001.md"
}

# Removes one named rule block from a copy of the validator. A mutant that still
# dies with its rule removed was never proving that rule; the `*_rule_off`
# pass-cases below make that claim testable instead of asserted.
disable_rule() {
  local validator="$1" rule="$2"
  python3 - "$validator" "$rule" <<'PYEOF'
import sys
path, rule = sys.argv[1], sys.argv[2]
begin, end = "# RULE:%s" % rule, "# /RULE:%s" % rule
out, inside, found, removed = [], False, False, 0
for line in open(path).read().split("\n"):
    stripped = line.strip()
    if stripped == begin:
        inside, found = True, True
        continue
    if inside and stripped == end:
        inside = False
        continue
    if inside:
        removed += 1
        continue
    out.append(line)
assert found, "rule marker not found: %s" % rule
assert not inside, "unterminated rule block: %s" % rule
assert removed > 0, "rule block is empty: %s" % rule
open(path, "w").write("\n".join(out))
PYEOF
}

# Builds `<name>_rule_off`: the same broken tree, judged by a validator with the
# named rule(s) surgically removed. More than one name is only ever used where a
# single tree is refused by two rules that guard the same registry line and the
# first one `continue`s past the second; the pair still proves the named rule,
# because each of those rules also owns a mutant where it is the only one armed.
rule_off_copy() {
  local name="$1"
  shift
  local rule
  cp -R "$probe_root/$name" "$probe_root/${name}_rule_off"
  for rule in "$@"; do
    disable_rule "$probe_root/${name}_rule_off/.board/validate.sh" "$rule"
  done
  chmod 0755 "$probe_root/${name}_rule_off/.board/validate.sh"
}

# Writes a legacy registry with the canonical `# count:` header the validator
# derives and re-checks. The counts are passed explicitly so a fixture states
# its own size the same way the real registry does.
write_legacy_registry() {
  local root="$1" entries="$2" cards="$3" distinct="$4"
  shift 4
  {
    printf '# Second-reader legacy registry fixture.\n'
    printf '# count: %s entry(ies) across %s card(s), %s distinct captured program(s)\n' \
      "$entries" "$cards" "$distinct"
    printf '%s\n' "$@"
  } >"$root/.board/second-reader-legacy.tsv"
}

# Same, for the exemption registry.
write_exempt_registry() {
  local root="$1" entries="$2"
  shift 2
  {
    printf '# Second-reader exemption registry fixture.\n'
    printf '# count: %s entry(ies) across %s card(s)\n' "$entries" "$entries"
    printf '%s\n' "$@"
  } >"$root/.board/second-reader-exempt.tsv"
}

# Reverts the base card's Integration citation, putting the card back in the
# shape the gate used to walk straight past: four `not-applicable` layers and
# nothing any second-reader engine can execute.
uncite_all_layers() {
  local root="$1"
  sed -i 's#^- Integration: .*#- Integration: not-applicable: no adapter boundary exists in this fixture.#' \
    "$root/.board/cards/ready/AUR-001.md"
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
  if PATH="$probe_root/$name/stub-path:$PATH" \
    bash "$probe_root/$name/.board/validate.sh" >"$output" 2>&1; then
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
  if ! PATH="$probe_root/$name/stub-path:$PATH" \
    bash "$probe_root/$name/.board/validate.sh" >"$output" 2>&1; then
    printf 'validator rejected valid fixture: %s\n' "$name" >&2
    sed -n '1,80p' "$output" >&2
    return 1
  fi
  printf 'validator accepted valid fixture: %s\n' "$name"
}

# Same contract as run_case, for the one invariant that is about how the gate
# reacts to its executor: the mode is chosen by AURUM_SECOND_READER and the
# executor's availability by PATH. Both are passed as separate values, never as
# a word-split string -- the ambient PATH contains spaces on some hosts and
# splitting it silently turned this case into a different experiment once.
run_case_exec() {
  local name="$1"
  local expected="$2"
  local mode="$3"
  local path_prefix="$4"
  local output="$probe_root/$name.output"
  if AURUM_SECOND_READER="$mode" PATH="$path_prefix:$PATH" \
    bash "$probe_root/$name/.board/validate.sh" >"$output" 2>&1; then
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

run_pass_case_exec() {
  local name="$1"
  local mode="$2"
  local path_prefix="$3"
  local output="$probe_root/$name.output"
  if ! AURUM_SECOND_READER="$mode" PATH="$path_prefix:$PATH" \
    bash "$probe_root/$name/.board/validate.sh" >"$output" 2>&1; then
    printf 'validator rejected valid fixture: %s\n' "$name" >&2
    sed -n '1,80p' "$output" >&2
    return 1
  fi
  printf 'validator accepted valid fixture: %s\n' "$name"
}

write_base_fixture
if ! PATH="$probe_root/base/stub-path:$PATH" \
  bash "$probe_root/base/.board/validate.sh" >"$probe_root/base.output" 2>&1; then
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

# --- Second reader: structural citation rules --------------------------------
# These bind at `review`, the first state in which a cited layer test is a
# delivered artifact rather than the builder's pending output.

# Positive control: a `review` card citing a concrete, existing, owned,
# executable Integration test must stay green. Every structural mutant below is
# one deviation from this tree.
prepare sr_present_valid
cite_integration "$probe_root/sr_present_valid"
make_valid_review "$probe_root/sr_present_valid"

# (1) The citation names a test that does not exist. `paths` owns the directory,
# so ownership is satisfied and only the existence rule can fire.
prepare sr_missing_file
sed -i 's#^paths: .*#paths: [tests/acceptance, docs/specs/AUR-001.md]#' "$probe_root/sr_missing_file/.board/cards/ready/AUR-001.md"
cite_integration "$probe_root/sr_missing_file" 'tests/acceptance/AUR-001-missing.sh::IntegrationAUR001'
make_valid_review "$probe_root/sr_missing_file"
rule_off_copy sr_missing_file second-reader-file-exists

# (2) The citation names a test that EXISTS but belongs to no path this card
# owns. Existence is satisfied, so only the ownership rule can fire.
prepare sr_outside_paths
cite_integration "$probe_root/sr_outside_paths" 'tests/acceptance/AUR-002.sh::IntegrationAUR002'
cp "$probe_root/sr_outside_paths/tests/acceptance/AUR-001.sh" \
  "$probe_root/sr_outside_paths/tests/acceptance/AUR-002.sh"
# The copy dispatches the selector it is cited by, so the only deviation from
# the positive control stays ownership. Without this the selector-definition
# rule would fire too and the `_rule_off` half of the pair could never survive.
sed -i 's/IntegrationAUR001/IntegrationAUR002/' "$probe_root/sr_outside_paths/tests/acceptance/AUR-002.sh"
make_valid_review "$probe_root/sr_outside_paths"
rule_off_copy sr_outside_paths second-reader-test-owned

# (3) The citation names an existing, owned artifact that no second-reader
# engine can execute: it is neither a Go test file nor an acceptance program.
prepare sr_unrunnable_kind
sed -i 's#^paths: .*#paths: [tests/acceptance/AUR-001.sh, tests/specs/AUR-001, docs/specs/AUR-001.md]#' "$probe_root/sr_unrunnable_kind/.board/cards/ready/AUR-001.md"
mkdir -p "$probe_root/sr_unrunnable_kind/tests/specs/AUR-001"
printf '{"case":"vector"}\n' >"$probe_root/sr_unrunnable_kind/tests/specs/AUR-001/vectors.json"
cite_integration "$probe_root/sr_unrunnable_kind" 'tests/specs/AUR-001/vectors.json::IntegrationAUR001'
make_valid_review "$probe_root/sr_unrunnable_kind"
rule_off_copy sr_unrunnable_kind second-reader-runnable-kind

# --- Second reader: the `done` execution gate --------------------------------

# Positive control for every execution mutant: a complete `done` bundle whose
# concrete Integration citation carries a real second-reader record.
prepare sr_done_valid
cite_integration "$probe_root/sr_done_valid"
make_valid_done "$probe_root/sr_done_valid"

# (4) A `done` card that cites a concrete test and carries no second-reader
# record at all, and is not recorded in the legacy registry. This is the state
# every card in `done` was in before this gate existed.
prepare sr_unlisted_done
cite_integration "$probe_root/sr_unlisted_done"
make_valid_done "$probe_root/sr_unlisted_done" no
rule_off_copy sr_unlisted_done second-reader-done-requires-execution

# (5) The selector matched nothing. `go test -run` that matches no test exits
# ZERO and says so in one line; the shell engine's equivalent is recorded the
# same way. The observation still claims a pass, which is the point.
prepare sr_selector_no_match
cite_integration "$probe_root/sr_selector_no_match"
make_valid_done "$probe_root/sr_selector_no_match"
write_second_reader_log "$probe_root/sr_selector_no_match/.board/evidence/AUR-001/second-reader/Integration.raw.txt" \
  "$probe_root/sr_selector_no_match" 0 64 '| testing: warning: no tests to run'
python3 - "$probe_root/sr_selector_no_match" <<'PYEOF'
import hashlib, re, sys
root = sys.argv[1]
log = root + "/.board/evidence/AUR-001/second-reader/Integration.raw.txt"
obs = root + "/.board/evidence/AUR-001/second-reader/Integration.json"
sha = "sha256:" + hashlib.sha256(open(log, "rb").read()).hexdigest()
body = open(obs).read()
body = re.sub(r'"raw_output_sha256":"sha256:[0-9a-f]{64}"', '"raw_output_sha256":"%s"' % sha, body)
open(obs, "w").write(body)
PYEOF
reseal_done "$probe_root/sr_selector_no_match"
rule_off_copy sr_selector_no_match second-reader-selector-matches

# (6) A test that passes without asserting anything about its own selector: the
# acceptance program stops dispatching on its argument, so it answers 0 to a
# selector nobody declared and the cited layer name selects nothing. The script
# still MENTIONS the selector, which keeps the selector-definition rule (15)
# satisfied and the deviation single: naming a selector and dispatching on it
# are two different properties and this pair is what separates them.
prepare sr_no_assertion
cite_integration "$probe_root/sr_no_assertion"
cat >"$probe_root/sr_no_assertion/tests/acceptance/AUR-001.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
# Selectors handled here: AC-001, IntegrationAUR001.
printf '{"schema":"aurum.acceptance-observation","version":1,"card_id":"AUR-001","scenario_id":"AC-001"}\n'
EOF
chmod 0755 "$probe_root/sr_no_assertion/tests/acceptance/AUR-001.sh"
make_valid_done "$probe_root/sr_no_assertion"
write_second_reader_log "$probe_root/sr_no_assertion/.board/evidence/AUR-001/second-reader/Integration.raw.txt" \
  "$probe_root/sr_no_assertion" 0 0 '| {"schema":"aurum.acceptance-observation","version":1,"card_id":"AUR-001","scenario_id":"AC-001"}'
write_second_reader_observation \
  "$probe_root/sr_no_assertion/.board/evidence/AUR-001/second-reader/Integration.json" \
  "$probe_root/sr_no_assertion" "$(digest_from_manifest "$probe_root/sr_no_assertion" candidate_identity_digest)" \
  "$(digest context-r)" "$(digest backend)" 1 0
reseal_done "$probe_root/sr_no_assertion"
rule_off_copy sr_no_assertion second-reader-selector-matches

# (7) Forged execution evidence, form one: the observation claims a pass while
# the raw bytes it points at record a non-zero exit. Every digest is forged back
# into agreement, so only the recomputation from the log can catch it.
prepare sr_forged_result
cite_integration "$probe_root/sr_forged_result"
make_valid_done "$probe_root/sr_forged_result"
write_second_reader_log "$probe_root/sr_forged_result/.board/evidence/AUR-001/second-reader/Integration.raw.txt" \
  "$probe_root/sr_forged_result" 1 64 '| AUR-001/AC-001/behavior-missing'
write_second_reader_observation \
  "$probe_root/sr_forged_result/.board/evidence/AUR-001/second-reader/Integration.json" \
  "$probe_root/sr_forged_result" "$(digest_from_manifest "$probe_root/sr_forged_result" candidate_identity_digest)" \
  "$(digest context-r)" "$(digest backend)" 1 0
reseal_done "$probe_root/sr_forged_result"
rule_off_copy sr_forged_result second-reader-run-passed

# (8) Forged execution evidence, form two: the test file changed after the run,
# and the observation's summary was updated to the new bytes while the raw log
# still describes the old ones. This is a pasted run covering code it never saw.
prepare sr_stale_test_digest
cite_integration "$probe_root/sr_stale_test_digest"
make_valid_done "$probe_root/sr_stale_test_digest"
printf '# a behavioral change the recorded run never saw\n' \
  >>"$probe_root/sr_stale_test_digest/tests/acceptance/AUR-001.sh"
write_second_reader_observation \
  "$probe_root/sr_stale_test_digest/.board/evidence/AUR-001/second-reader/Integration.json" \
  "$probe_root/sr_stale_test_digest" "$(digest_from_manifest "$probe_root/sr_stale_test_digest" candidate_identity_digest)" \
  "$(digest context-r)" "$(digest backend)" 1 0
reseal_done "$probe_root/sr_stale_test_digest"
rule_off_copy sr_stale_test_digest second-reader-log-binds-declaration

# (9) Forged execution evidence, form three: the observation's command_digest
# claims a different command than the card's citation canonicalizes to.
prepare sr_forged_command
cite_integration "$probe_root/sr_forged_command"
make_valid_done "$probe_root/sr_forged_command"
python3 - "$probe_root/sr_forged_command" <<'PYEOF'
import hashlib, re, sys
root = sys.argv[1]
obs = root + "/.board/evidence/AUR-001/second-reader/Integration.json"
forged = "sha256:" + hashlib.sha256(b"bash tests/acceptance/AUR-001.sh AC-001").hexdigest()
body = re.sub(r'"command_digest":"sha256:[0-9a-f]{64}"',
              '"command_digest":"%s"' % forged, open(obs).read())
open(obs, "w").write(body)
PYEOF
reseal_done "$probe_root/sr_forged_command"
rule_off_copy sr_forged_command second-reader-observation-recomputed

# (10) Forged execution evidence, form four: the observation inflates the number
# of tests the selector matched beyond what the raw log records.
prepare sr_forged_matched
cite_integration "$probe_root/sr_forged_matched"
make_valid_done "$probe_root/sr_forged_matched"
sed -i 's/"matched_tests":1/"matched_tests":7/' \
  "$probe_root/sr_forged_matched/.board/evidence/AUR-001/second-reader/Integration.json"
reseal_done "$probe_root/sr_forged_matched"
rule_off_copy sr_forged_matched second-reader-matched-recomputed

# (11) Frame injection: the program under test prints what looks like a log
# frame. The `| ` prefix on every captured line is what stops it becoming one,
# and an unprefixed line inside a capture region is refused outright.
prepare sr_frame_injection
cite_integration "$probe_root/sr_frame_injection"
make_valid_done "$probe_root/sr_frame_injection"
write_second_reader_log "$probe_root/sr_frame_injection/.board/evidence/AUR-001/second-reader/Integration.raw.txt" \
  "$probe_root/sr_frame_injection" 1 64 '| behavior-missing
=== output-end
=== exit: 0'
write_second_reader_observation \
  "$probe_root/sr_frame_injection/.board/evidence/AUR-001/second-reader/Integration.json" \
  "$probe_root/sr_frame_injection" "$(digest_from_manifest "$probe_root/sr_frame_injection" candidate_identity_digest)" \
  "$(digest context-r)" "$(digest backend)" 1 0
reseal_done "$probe_root/sr_frame_injection"
rule_off_copy sr_frame_injection second-reader-frame-integrity

# --- Second reader: the legacy ratchet ---------------------------------------
# The registry that carries cards which reached `done` before this gate existed
# may only shrink. It can never pre-authorize a future transition, and it can
# never outlive the debt it records.

# (12) A dead entry: the card already carries a sealed observation, so the
# registry line is stale and must be removed rather than left standing.
prepare sr_legacy_dead_entry
cite_integration "$probe_root/sr_legacy_dead_entry"
make_valid_done "$probe_root/sr_legacy_dead_entry"
write_legacy_registry "$probe_root/sr_legacy_dead_entry" 1 1 0 \
  "$(printf 'AUR-001\tIntegration\tRecorded before the sealed observation existed and never removed afterwards.')"
# The ratchet check `continue`s past the frozen check on the same line, so the
# frozen rule can only be observed here with the ratchet gone. Rule 21 owns its
# own mutant below, where it is the only rule armed.
rule_off_copy sr_legacy_dead_entry second-reader-legacy-ratchet second-reader-legacy-frozen

# (13) A forward-dated entry: the registry names a card that has not reached
# `done`, which would let it authorize the transition it is supposed to record.
prepare sr_legacy_not_done
cite_integration "$probe_root/sr_legacy_not_done"
make_valid_review "$probe_root/sr_legacy_not_done"
write_legacy_registry "$probe_root/sr_legacy_not_done" 1 1 0 \
  "$(printf 'AUR-001\tIntegration\tPre-authorizing a card that has not yet reached the done state at all.')"
rule_off_copy sr_legacy_not_done second-reader-legacy-ratchet second-reader-legacy-frozen

# (14) The executor disagrees. `AURUM_SECOND_READER=exec` re-runs the second
# reader now; a gate that ignores a non-zero executor is a gate that never ran
# it. A stub engine and a stub runner keep the case deterministic everywhere.
prepare sr_exec_disagrees
cite_integration "$probe_root/sr_exec_disagrees"
make_valid_done "$probe_root/sr_exec_disagrees"
mkdir -p "$probe_root/sr_exec_disagrees/.board/bin" "$probe_root/sr_exec_disagrees/stub-path"
cat >"$probe_root/sr_exec_disagrees/.board/bin/second-reader" <<'EOF'
#!/usr/bin/env bash
printf 'second-reader: re-execution did not reproduce the recorded verdict\n' >&2
exit 3
EOF
chmod 0755 "$probe_root/sr_exec_disagrees/.board/bin/second-reader"
printf '#!/usr/bin/env bash\nexit 0\n' >"$probe_root/sr_exec_disagrees/stub-path/docker"
chmod 0755 "$probe_root/sr_exec_disagrees/stub-path/docker"
rule_off_copy sr_exec_disagrees second-reader-reexecute

# --- Second reader: the second round --------------------------------------
# Eight rules answering the eight ways this gate was shown to be satisfiable
# without proving anything. Same contract as above: mutant dies, identical tree
# with the one rule removed survives.

# (15) The cited file does not DEFINE the cited selector. Ownership, existence,
# non-emptiness and the sha256 of that file are all satisfied; the dispatcher
# simply no longer carries the name the card promised. The Go form of this hole
# is the sharper one -- `go test ./<dir>/... -run ^SEL$` is package-scoped, so an
# owned `_test.go` with no test at all can cite a neighbouring card's selector
# and produce a green log -- and it is proved on the real tree, not here, since
# this harness is deliberately toolchain-free.
prepare sr_selector_undefined
cat >"$probe_root/sr_selector_undefined/tests/acceptance/AUR-001.sh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
selector="${1:-AC-001}"
case "$selector" in
  AC-001) ;;
  *) printf 'AUR-001/AC-001/unknown-selector\n' >&2; exit 64 ;;
esac
printf '{"schema":"aurum.acceptance-observation","version":1,"card_id":"AUR-001","scenario_id":"AC-001"}\n'
EOF
chmod 0755 "$probe_root/sr_selector_undefined/tests/acceptance/AUR-001.sh"
make_valid_review "$probe_root/sr_selector_undefined"
rule_off_copy sr_selector_undefined second-reader-selector-defined

# (16) The recorded run names an image the lock does not pin. `=== image:` is
# written by the runner, so a record produced in ANY image satisfied every other
# frame; recomputing the digest from the lock is what binds it.
prepare sr_forged_image
cite_integration "$probe_root/sr_forged_image"
make_valid_done "$probe_root/sr_forged_image"
sed -i 's#^=== image: .*#=== image: bash@sha256:0000000000000000000000000000000000000000000000000000000000000000#' \
  "$probe_root/sr_forged_image/.board/evidence/AUR-001/second-reader/Integration.raw.txt"
write_second_reader_observation \
  "$probe_root/sr_forged_image/.board/evidence/AUR-001/second-reader/Integration.json" \
  "$probe_root/sr_forged_image" "$(digest_from_manifest "$probe_root/sr_forged_image" candidate_identity_digest)" \
  "$(digest context-r)" "$(digest backend)" 1 0
reseal_done "$probe_root/sr_forged_image"
rule_off_copy sr_forged_image second-reader-image-pinned

# (17) A `done` card with all four TDD layers `not-applicable` and no exemption
# entry. This is the shape that walked past the gate entirely: no citation, no
# observation, no registry line, no note -- two real cards sat in `done` like
# this while the board reported them proved by two engines.
prepare sr_no_citation_done
uncite_all_layers "$probe_root/sr_no_citation_done"
make_valid_done "$probe_root/sr_no_citation_done" no
rule_off_copy sr_no_citation_done second-reader-coverage-required

# (18) A card that joins the exemption registry on its own authority. The
# frozen set lives in the validator, so the second of the two paths a card
# without an executable citation may take is a REVIEWED code change, never a
# data-only edit made in the same commit that moves the card to `done`. The
# `_rule_off` half of this pair is that second path taken legitimately: the same
# `done` card, its absence named in the registry, green -- and printed on every
# run as `board note:` rather than skipped in silence.
prepare sr_exempt_frozen
uncite_all_layers "$probe_root/sr_exempt_frozen"
make_valid_done "$probe_root/sr_exempt_frozen" no
write_exempt_registry "$probe_root/sr_exempt_frozen" 1 \
  "$(printf 'AUR-001\tAll four TDD layers are not-applicable, so no artifact exists for either second-reader engine to execute at all.')"
rule_off_copy sr_exempt_frozen second-reader-exempt-frozen

# (19) The exemption registry carries the legacy ratchet, both ways. As with the
# legacy pair, the ratchet `continue`s past the frozen check on the same line,
# so the `_rule_off` copies drop both and rule 18 above keeps its own mutant.
# (19a) forward-dated: an exemption for a card that has not reached `done`,
# which would pre-authorize the very transition it claims to record.
prepare sr_exempt_not_done
uncite_all_layers "$probe_root/sr_exempt_not_done"
make_valid_review "$probe_root/sr_exempt_not_done"
write_exempt_registry "$probe_root/sr_exempt_not_done" 1 \
  "$(printf 'AUR-001\tPre-authorizing a card that has not yet reached the done state at all, which the ratchet refuses.')"
rule_off_copy sr_exempt_not_done second-reader-exempt-ratchet second-reader-exempt-frozen
# (19b) stale: the card now cites a concrete, executable layer test, so the
# exemption is dead and must be removed instead of left standing.
prepare sr_exempt_stale
cite_integration "$probe_root/sr_exempt_stale"
make_valid_done "$probe_root/sr_exempt_stale"
write_exempt_registry "$probe_root/sr_exempt_stale" 1 \
  "$(printf 'AUR-001\tExempted before the card started citing a concrete layer test, and never removed afterwards.')"
rule_off_copy sr_exempt_stale second-reader-exempt-ratchet second-reader-exempt-frozen

# (20) The registry states a size its own contents contradict. Three texts once
# stated three different numbers for one list -- the header said sixteen, the
# README said fifteen, the file carried fifteen -- so the number is derived from
# the file here and both texts must carry the derived line.
# (20a) the header disagrees with the entries.
prepare sr_count_disagrees
cite_integration "$probe_root/sr_count_disagrees"
make_valid_done "$probe_root/sr_count_disagrees"
write_legacy_registry "$probe_root/sr_count_disagrees" 9 4 2
rule_off_copy sr_count_disagrees second-reader-registry-counts
# (20b) the header is right and the README says something else. Same rule, other
# half: a number stated twice must be stated identically twice.
prepare sr_count_readme_disagrees
cite_integration "$probe_root/sr_count_readme_disagrees"
make_valid_done "$probe_root/sr_count_readme_disagrees"
write_legacy_registry "$probe_root/sr_count_readme_disagrees" 0 0 0
cat >"$probe_root/sr_count_readme_disagrees/.board/README.md" <<'EOF'
# Board

- `.board/second-reader-legacy.tsv` records 9 entry(ies) across 4 card(s), 2 distinct captured program(s).
EOF
rule_off_copy sr_count_readme_disagrees second-reader-registry-counts

# (21) An orphan record: evidence-shaped text under the legacy directory that no
# accepted entry names. Nothing recomputes it and nothing prints it, so it
# outlives the debt it once documented.
prepare sr_legacy_orphan
cite_integration "$probe_root/sr_legacy_orphan"
make_valid_done "$probe_root/sr_legacy_orphan"
write_second_reader_log \
  "$probe_root/sr_legacy_orphan/.board/tests/second-reader-legacy/AUR-001.Integration.raw.txt" \
  "$probe_root/sr_legacy_orphan" 0 64 '| {"schema":"aurum.acceptance-observation","version":1,"card_id":"AUR-001","scenario_id":"AC-001"}'
rule_off_copy sr_legacy_orphan second-reader-legacy-orphan

# (22) A key that was never part of the cutover. Every other ratchet rule
# refuses an entry that went STALE; this one refuses an entry that was never
# owed, which is the only thing a card could otherwise do in the same commit
# that moves it to `done`. The frozen set lives in the validator, so growing
# either registry is a reviewed code change and never a data edit.
prepare sr_legacy_unfrozen
cite_integration "$probe_root/sr_legacy_unfrozen"
make_valid_done "$probe_root/sr_legacy_unfrozen" no
write_legacy_registry "$probe_root/sr_legacy_unfrozen" 1 1 1 \
  "$(printf 'AUR-001\tIntegration\tAdded to the legacy registry in the same commit that moved this card into done.')"
write_second_reader_log \
  "$probe_root/sr_legacy_unfrozen/.board/tests/second-reader-legacy/AUR-001.Integration.raw.txt" \
  "$probe_root/sr_legacy_unfrozen" 0 64 '| {"schema":"aurum.acceptance-observation","version":1,"card_id":"AUR-001","scenario_id":"AC-001"}'
rule_off_copy sr_legacy_unfrozen second-reader-legacy-frozen

# (23) The invisible downgrade. With no executor the gate used to skip the
# re-execution in silence and print stderr byte-identical to a fully executed
# run, so an operator on a host without an OCI engine could not tell a proof
# from a shape check. It is now its own terminal verdict with its own exit
# status: not `board valid`, not `board invalid`.
prepare sr_inconclusive
cite_integration "$probe_root/sr_inconclusive"
make_valid_done "$probe_root/sr_inconclusive"
rm -f "$probe_root/sr_inconclusive/.board/bin/second-reader"
rule_off_copy sr_inconclusive second-reader-inconclusive

card='\.board/cards/[a-z-]+/AUR-001\.md'
manifest='\.board/evidence/AUR-001/manifest\.json'
report='\.board/evidence/AUR-001'
second_reader='\.board/evidence/AUR-001/second-reader'
legacy_registry='\.board/second-reader-legacy\.tsv'
exempt_registry='\.board/second-reader-exempt\.tsv'

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

# --- Second reader ------------------------------------------------------------
# Every case below comes in a pair: the mutant must DIE with the rule in place,
# and the identical tree must SURVIVE with that one rule surgically removed. A
# mutant that dies either way is decoration -- it proves some other check, not
# the rule it is named for.
run_pass_case sr_present_valid & pids+=("$!")
run_case sr_missing_file \
  "^board error: .*/$card: review card cites Integration test tests/acceptance/AUR-001-missing\.sh, which does not exist\$" & pids+=("$!")
run_pass_case sr_missing_file_rule_off & pids+=("$!")
run_case sr_outside_paths \
  "^board error: .*/$card: Integration TDD test is outside owned paths: tests/acceptance/AUR-002\.sh\$" & pids+=("$!")
run_pass_case sr_outside_paths_rule_off & pids+=("$!")
run_case sr_unrunnable_kind \
  "^board error: .*/$card: Integration cites an artifact no second-reader engine can execute \(expected \*_test\.go or \*\.sh\): tests/specs/AUR-001/vectors\.json\$" & pids+=("$!")
run_pass_case sr_unrunnable_kind_rule_off & pids+=("$!")

run_pass_case sr_done_valid & pids+=("$!")
run_case sr_unlisted_done \
  "^board error: .*/$card: done card cites Integration test .tests/acceptance/AUR-001\.sh::IntegrationAUR001. but no second reader ever executed it: \.board/evidence/AUR-001/second-reader/Integration\.json is absent and AUR-001/Integration is not recorded in \.board/second-reader-legacy\.tsv\$" & pids+=("$!")
run_pass_case sr_unlisted_done_rule_off & pids+=("$!")
run_case sr_selector_no_match \
  "^board error: .*/$second_reader/Integration\.raw\.txt: the declared selector IntegrationAUR001 matched no test; a run that did no work is not a pass\$" & pids+=("$!")
run_pass_case sr_selector_no_match_rule_off & pids+=("$!")
run_case sr_no_assertion \
  "^board error: .*/$second_reader/Integration\.raw\.txt: the acceptance program answered === control-exit: 0 to an unknown selector; IntegrationAUR001 selects nothing it does not already do\$" & pids+=("$!")
run_pass_case sr_no_assertion_rule_off & pids+=("$!")
run_case sr_forged_result \
  "^board error: .*/$second_reader/Integration\.raw\.txt: the recorded second-reader run did not exit zero \(=== exit: 1\)\$" & pids+=("$!")
run_pass_case sr_forged_result_rule_off & pids+=("$!")
run_case sr_stale_test_digest \
  "^board error: .*/$second_reader/Integration\.raw\.txt: the recorded run read a different tests/acceptance/AUR-001\.sh than the tree now carries\$" & pids+=("$!")
run_pass_case sr_stale_test_digest_rule_off & pids+=("$!")
run_case sr_forged_command \
  "^board error: .*/$second_reader/Integration\.json: command_digest must equal sha256:[0-9a-f]{64}\$" & pids+=("$!")
run_pass_case sr_forged_command_rule_off & pids+=("$!")
run_case sr_forged_matched \
  "^board error: .*/$second_reader/Integration\.json: matched_tests claims 7 but the raw log shows 1 passing selector match\(es\)\$" & pids+=("$!")
run_pass_case sr_forged_matched_rule_off & pids+=("$!")
run_case sr_frame_injection \
  "^board error: .*/$second_reader/Integration\.raw\.txt: the raw second-reader log does not contain exactly one '=== output-end' frame; the capture boundaries are forgeable\$" & pids+=("$!")
run_pass_case sr_frame_injection_rule_off & pids+=("$!")

run_case sr_legacy_dead_entry \
  "^board error: $legacy_registry:3: AUR-001/Integration already carries a sealed second-reader observation; the ratchet only shrinks, remove the stale entry\$" & pids+=("$!")
run_pass_case sr_legacy_dead_entry_rule_off & pids+=("$!")
run_case sr_legacy_not_done \
  "^board error: $legacy_registry:3: AUR-001 is not in done; the legacy registry can never pre-authorize a future transition\$" & pids+=("$!")
run_pass_case sr_legacy_not_done_rule_off & pids+=("$!")
run_case_exec sr_exec_disagrees \
  "^board error: .*/$second_reader/Integration\.json: re-executing the second reader disagreed with the record \(exit 3\): second-reader: re-execution did not reproduce the recorded verdict \$" \
  exec "$probe_root/sr_exec_disagrees/stub-path" & pids+=("$!")
run_pass_case_exec sr_exec_disagrees_rule_off \
  exec "$probe_root/sr_exec_disagrees_rule_off/stub-path" & pids+=("$!")

# --- Second reader, round two -------------------------------------------------
run_case sr_selector_undefined \
  "^board error: .*/$card: Integration cites tests/acceptance/AUR-001\.sh, which does not define the selector IntegrationAUR001; the log would bind a test this file never carried\$" & pids+=("$!")
run_pass_case sr_selector_undefined_rule_off & pids+=("$!")
run_case sr_forged_image \
  "^board error: .*/$second_reader/Integration\.raw\.txt: the recorded run does not name the locked shell image; missing frame: === image: bash@sha256:ae4668c2560999e65e89532cd2ad1b6688bb23298189f0bd229ef80fa4bd0831\$" & pids+=("$!")
run_pass_case sr_forged_image_rule_off & pids+=("$!")
run_case sr_no_citation_done \
  "^board error: .*/$card: done card hands the second reader nothing to execute -- all four TDD layers are not-applicable -- and it is not recorded in $exempt_registry\$" & pids+=("$!")
run_pass_case sr_no_citation_done_rule_off & pids+=("$!")
run_case sr_exempt_frozen \
  "^board error: $exempt_registry:3: AUR-001 is not in the frozen exemption set compiled into \.board/validate\.sh; a card cannot join the exemption registry in the same commit that moves it to done\$" & pids+=("$!")
# The same `done` card with its absence legitimately recorded: green, and named
# on stderr. This is the second of the two paths B2 leaves open.
run_pass_case sr_exempt_frozen_rule_off & pids+=("$!")
run_case sr_exempt_not_done \
  "^board error: $exempt_registry:3: AUR-001 is not in done; the exemption registry can never pre-authorize a future transition\$" & pids+=("$!")
run_pass_case sr_exempt_not_done_rule_off & pids+=("$!")
run_case sr_exempt_stale \
  "^board error: $exempt_registry:3: AUR-001 now cites a concrete TDD layer test; the ratchet only shrinks, remove the stale entry\$" & pids+=("$!")
run_pass_case sr_exempt_stale_rule_off & pids+=("$!")
run_case sr_count_disagrees \
  "^board error: $legacy_registry: the header must declare exactly '# count: 0 entry\(ies\) across 0 card\(s\), 0 distinct captured program\(s\)'; a registry whose stated size disagrees with its contents counts nothing\$" & pids+=("$!")
run_pass_case sr_count_disagrees_rule_off & pids+=("$!")
run_case sr_count_readme_disagrees \
  "^board error: \.board/README\.md: must carry exactly the line '- .$legacy_registry. records 0 entry\(ies\) across 0 card\(s\), 0 distinct captured program\(s\)\.'; the README and the registry may not state different sizes for the same list\$" & pids+=("$!")
run_pass_case sr_count_readme_disagrees_rule_off & pids+=("$!")
run_case sr_legacy_orphan \
  "^board error: \.board/tests/second-reader-legacy/AUR-001\.Integration\.raw\.txt: no accepted entry in $legacy_registry names AUR-001/Integration; an unreferenced record is evidence-shaped text nothing recomputes\$" & pids+=("$!")
run_pass_case sr_legacy_orphan_rule_off & pids+=("$!")
run_case sr_legacy_unfrozen \
  "^board error: $legacy_registry:3: AUR-001/Integration is not in the frozen cutover set compiled into \.board/validate\.sh; a card cannot join the legacy registry in the same commit that moves it to done\$" & pids+=("$!")
run_pass_case sr_legacy_unfrozen_rule_off & pids+=("$!")
# The inconclusive verdict is not an error line: it is a third terminal state
# with its own exit status, which is the whole point. The mutant must therefore
# be recognized by the summary it prints, not by a `board error:`.
run_case sr_inconclusive \
  "^board inconclusive: structure and raw-log recomputation passed on [0-9]+ atomic cards, but this run does NOT authorize a transition to done\$" & pids+=("$!")
run_pass_case sr_inconclusive_rule_off & pids+=("$!")

failed=0
for pid in "${pids[@]}"; do
  wait "$pid" || failed=1
done
(( failed == 0 ))
