#!/usr/bin/env bash
set -euo pipefail

export LC_ALL=C
umask 077

readonly card='AUR-359'
selector="${1:-AC-001}"
case "$selector" in
  AC-001|ContractAUR359|IntegrationAUR359|E2EAUR359) ;;
  *) printf '%s/AC-001/unknown-selector\n' "$card" >&2; exit 64 ;;
esac

# Exit codes follow tests/acceptance/EXIT_CODE_CONVENTION.md:
#   0  the promised behavior was observed
#   1  behavioral RED — a deliverable this card OWNS is missing or wrong
#   64 unknown selector
#   79 inconclusive / infrastructure — a tool, a bound, or this program's own
#      setup failed. Never valid red evidence, never a pass.
# The convention forbids a shared sourced library (the acceptance program is
# materialized alone into the sealed container), so the two verdict helpers
# are declared per-file, on purpose. A same-file helper function (not a
# sourced external file) is not a violation of that rule — see AUR-233.sh for
# the identical pattern in a sibling card of the same office.

# fail() is reserved for artifacts listed in this card's own `paths:`
# (.board/bootstrap/locks/trust-root.yml, .board/bin/oci-run,
# .board/oci/profiles/trust-root-docker-v1.json,
# .board/locks/oci/trust-root-docker-v1.lock.json). Their absence or
# corruption IS the behavior under test failing to exist: that is evidence.
fail() {
  printf '%s/AC-001/%s\n' "$card" "$1" >&2
  [[ -z "${2:-}" ]] || printf '%s/AC-001/detail expected-artifact=%s\n' "$card" "$2" >&2
  exit 1
}

# infra() is reserved for everything this card does NOT own as a behavioral
# claim: a missing tool, a bound exceeded, this program's own fixture
# machinery failing to reproduce. An environment gap must never be laundered
# into behavioral red.
infra() {
  printf '%s/AC-001/inconclusive/%s\n' "$card" "$1" >&2
  exit 79
}

for tool in grep sha256sum wc; do
  command -v "$tool" >/dev/null 2>&1 || infra "missing_tool:$tool"
done

# Resolve the repository root without `dirname`: one less external
# dependency, and a missing `dirname` previously degraded silently into
# repo_root="/" and a false trust_root_mismatch.
script_path="$0"
case "$script_path" in
  */*) script_dir="${script_path%/*}" ;;
  *)   script_dir='.' ;;
esac
repo_root="$(CDPATH='' cd -- "$script_dir/../.." >/dev/null 2>&1 && pwd -P)" || infra repo_root_unresolved
[[ -n "$repo_root" && -d "$repo_root" ]] || infra repo_root_unresolved

readonly source_rel='.board/bootstrap/locks/trust-root.yml'
readonly runner_rel='.board/bin/oci-run'
readonly profile_rel='.board/oci/profiles/trust-root-docker-v1.json'
readonly ocilock_rel='.board/locks/oci/trust-root-docker-v1.lock.json'
readonly cases_rel='tests/bootstrap/locks/AUR-359/cases.tsv'
readonly fixtures_rel='tests/bootstrap/locks/AUR-359/fixtures'

readonly expected_image='bash@sha256:ae4668c2560999e65e89532cd2ad1b6688bb23298189f0bd229ef80fa4bd0831'
readonly digest_pinned_pattern='^"image": "[a-z0-9][a-z0-9._/-]*@sha256:[0-9a-f]{64}"$'

# 4 KiB: these are small flat manifests, not free-form input.
readonly max_source_bytes=4096
# 4 MiB / 256 rows: the bound the card declares for the sealed fixture table.
readonly max_cases_bytes=4194304
readonly max_cases_rows=256

# ---------------------------------------------------------------------------
# verify_trust_root_dir <root>
#   Prints one of: pass | trust_root_mismatch | source_integrity_error |
#   invalid_lock_or_manifest | runner_missing | runner-digest-mismatch |
#   image_digest_mismatch | mutable-image | egress-guard-missing |
#   input_limit_exceeded | mutable_reference
#   Returns: 0 pass; 1 behavioral (the printed word is the typed code);
#            2 infra (the printed word is the infra reason).
# Used both for the real tree (Phase 2) and for every sealed fixture
# (Phase 1), so the real verdict and the self-test run through one
# recomputation path, never two divergent ones.
# ---------------------------------------------------------------------------
verify_trust_root_dir() {
  local root="$1"
  local source_file="$root/$source_rel"
  local runner="$root/$runner_rel"
  local profile_file="$root/$profile_rel"
  local ocilock_file="$root/$ocilock_rel"
  local bytes runner_sha runner_digest_line ocilock_sha

  [[ -e "$source_file" ]] || { printf 'trust_root_mismatch\n'; return 1; }
  [[ ! -L "$source_file" ]] || { printf 'source_integrity_error\n'; return 1; }
  [[ -f "$source_file" ]] || { printf 'trust_root_mismatch\n'; return 1; }
  [[ -r "$source_file" ]] || { printf 'source_unreadable\n'; return 2; }
  bytes="$(wc -c <"$source_file")" || { printf 'source_unreadable\n'; return 2; }
  [[ "$bytes" =~ ^[0-9]+$ ]] || { printf 'source_size_unreadable\n'; return 2; }
  (( bytes > 0 )) || { printf 'invalid_lock_or_manifest\n'; return 1; }
  (( bytes <= max_source_bytes )) || { printf 'input_limit_exceeded\n'; return 1; }

  grep -Fqx 'schema: bootstrap-lock-v1' "$source_file" || { printf 'invalid_lock_or_manifest\n'; return 1; }
  grep -Fqx "card: $card" "$source_file" || { printf 'invalid_lock_or_manifest\n'; return 1; }
  grep -Eq 'sha256:[0-9a-f]{64}' "$source_file" || { printf 'invalid_lock_or_manifest\n'; return 1; }
  if grep -Eiq '(^|[^a-z])latest([^a-z]|$)|@[[:space:]]*(main|master|head)([^a-z]|$)' "$source_file"; then
    printf 'mutable_reference\n'; return 1
  fi
  grep -Fqx 'profile_network: none' "$source_file" || { printf 'egress-guard-missing\n'; return 1; }
  grep -Fqx 'profile_read_only_rootfs: true' "$source_file" || { printf 'trust_root_mismatch\n'; return 1; }
  grep -Fqx 'profile_cap_drop: ALL' "$source_file" || { printf 'trust_root_mismatch\n'; return 1; }
  grep -Fqx 'profile_no_new_privileges: true' "$source_file" || { printf 'trust_root_mismatch\n'; return 1; }
  grep -Fqx 'profile_user: 65532:65532' "$source_file" || { printf 'trust_root_mismatch\n'; return 1; }
  grep -Fqx 'engine_docker: observed' "$source_file" || { printf 'trust_root_mismatch\n'; return 1; }
  grep -Fqx 'engine_podman: not-observed' "$source_file" || { printf 'trust_root_mismatch\n'; return 1; }

  [[ -e "$runner" ]] || { printf 'runner_missing\n'; return 1; }
  [[ ! -L "$runner" ]] || { printf 'source_integrity_error\n'; return 1; }
  [[ -f "$runner" ]] || { printf 'runner_missing\n'; return 1; }
  [[ -r "$runner" ]] || { printf 'runner_unreadable\n'; return 2; }
  runner_digest_line="$(sha256sum -- "$runner")" || { printf 'runner_digest_unreadable\n'; return 2; }
  runner_sha="${runner_digest_line%% *}"
  [[ "$runner_sha" =~ ^[0-9a-f]{64}$ ]] || { printf 'runner_digest_unreadable\n'; return 2; }
  grep -Fq "$runner_sha" "$source_file" || { printf 'runner-digest-mismatch\n'; return 1; }
  grep -Fq "$expected_image" "$source_file" || { printf 'image_digest_mismatch\n'; return 1; }

  # -- profile document: same 24-key flat-JSON shape `.board/bin/oci-run`
  #    itself requires of a registered profile, checked here independently
  #    of that wrapper (the card's own accept never invokes it).
  [[ -e "$profile_file" ]] || { printf 'trust_root_mismatch\n'; return 1; }
  [[ ! -L "$profile_file" ]] || { printf 'source_integrity_error\n'; return 1; }
  [[ -f "$profile_file" ]] || { printf 'trust_root_mismatch\n'; return 1; }
  [[ -r "$profile_file" ]] || { printf 'profile_unreadable\n'; return 2; }
  bytes="$(wc -c <"$profile_file")" || { printf 'profile_unreadable\n'; return 2; }
  [[ "$bytes" =~ ^[0-9]+$ ]] || { printf 'profile_size_unreadable\n'; return 2; }
  (( bytes > 0 )) || { printf 'invalid_lock_or_manifest\n'; return 1; }
  (( bytes <= max_source_bytes )) || { printf 'input_limit_exceeded\n'; return 1; }
  grep -Fqx '"schema": "aurum.container-profile",' "$profile_file" || { printf 'invalid_lock_or_manifest\n'; return 1; }
  grep -Fqx '"profile": "trust-root-docker-v1",' "$profile_file" || { printf 'invalid_lock_or_manifest\n'; return 1; }
  grep -Fqx "\"lock\": \"$ocilock_rel\"," "$profile_file" || { printf 'invalid_lock_or_manifest\n'; return 1; }
  grep -Fqx '"network": "none",' "$profile_file" || { printf 'egress-guard-missing\n'; return 1; }
  grep -Fqx '"user": "65532:65532",' "$profile_file" || { printf 'trust_root_mismatch\n'; return 1; }
  grep -Fqx '"cap_drop": "ALL",' "$profile_file" || { printf 'trust_root_mismatch\n'; return 1; }
  grep -Fqx '"read_only_rootfs": true,' "$profile_file" || { printf 'egress-guard-missing\n'; return 1; }
  grep -Fqx '"no_new_privileges": true,' "$profile_file" || { printf 'trust_root_mismatch\n'; return 1; }
  grep -Fqx '"privileged": false,' "$profile_file" || { printf 'trust_root_mismatch\n'; return 1; }
  grep -Fqx '"memory_mb": 128,' "$profile_file" || { printf 'trust_root_mismatch\n'; return 1; }
  grep -Fqx '"cpu_millis": 500,' "$profile_file" || { printf 'trust_root_mismatch\n'; return 1; }
  grep -Fqx '"pids_limit": 64,' "$profile_file" || { printf 'trust_root_mismatch\n'; return 1; }

  # -- oci image lock: schema/version/profile/image, image digest-pinned.
  [[ -e "$ocilock_file" ]] || { printf 'trust_root_mismatch\n'; return 1; }
  [[ ! -L "$ocilock_file" ]] || { printf 'source_integrity_error\n'; return 1; }
  [[ -f "$ocilock_file" ]] || { printf 'trust_root_mismatch\n'; return 1; }
  [[ -r "$ocilock_file" ]] || { printf 'ocilock_unreadable\n'; return 2; }
  bytes="$(wc -c <"$ocilock_file")" || { printf 'ocilock_unreadable\n'; return 2; }
  [[ "$bytes" =~ ^[0-9]+$ ]] || { printf 'ocilock_size_unreadable\n'; return 2; }
  (( bytes > 0 )) || { printf 'invalid_lock_or_manifest\n'; return 1; }
  (( bytes <= max_source_bytes )) || { printf 'input_limit_exceeded\n'; return 1; }
  grep -Fqx '"schema": "aurum.oci-image-lock",' "$ocilock_file" || { printf 'invalid_lock_or_manifest\n'; return 1; }
  grep -Fqx '"profile": "trust-root-docker-v1",' "$ocilock_file" || { printf 'invalid_lock_or_manifest\n'; return 1; }
  grep -Eq "$digest_pinned_pattern" "$ocilock_file" || { printf 'mutable-image\n'; return 1; }
  grep -Fq "\"image\": \"$expected_image\"" "$ocilock_file" || { printf 'trust_root_mismatch\n'; return 1; }

  # -- binding: the profile's declared lock_digest must equal the
  #    recomputed content digest of the oci lock file it points at. A
  #    digest a file declares about itself is never trusted un-rechecked.
  ocilock_sha="$(sha256sum -- "$ocilock_file")" || { printf 'ocilock_unreadable\n'; return 2; }
  ocilock_sha="${ocilock_sha%% *}"
  [[ "$ocilock_sha" =~ ^[0-9a-f]{64}$ ]] || { printf 'ocilock_unreadable\n'; return 2; }
  grep -Fq "\"lock_digest\": \"sha256:$ocilock_sha\"" "$profile_file" || { printf 'invalid_lock_or_manifest\n'; return 1; }

  printf 'pass\n'
  return 0
}

# ---------------------------------------------------------------------------
# Phase 1 — this card's own deliverable, the real trust root, checked FIRST
# and on its own: absence and tamper are behavioral RED because every
# artifact checked below is this card's own declared path, and that must be
# provable with nothing else in this file's fixture machinery built yet
# (RED-before-GREEN: `tests/bootstrap/locks/AUR-359/cases.tsv` and its
# fixtures do not gate this phase).
# ---------------------------------------------------------------------------
set +e
real_word="$(verify_trust_root_dir "$repo_root")"
real_rc=$?
set -e
case "$real_rc" in
  0) ;;
  1) fail "$real_word" "$source_rel" ;;
  *) infra "$real_word" ;;
esac

# ---------------------------------------------------------------------------
# Phase 2 — only once the real trust root itself verifies clean does this
# program trust itself to judge fixtures: it re-proves its own sensitivity
# against `tests/bootstrap/locks/AUR-359/cases.tsv`, which seals, per row: a
# case name, the fixture subdirectory under
# tests/bootstrap/locks/AUR-359/fixtures/, the SHA-256 of the fixture's own
# trust-root.yml (or `-` when the case is deliberate absence), and the
# expected disposition word (`pass` or one of the typed codes above). A
# fixture whose recomputed digest diverges from what the row seals is this
# program's OWN setup being unsound: infra, never a verdict about the real
# tree that Phase 1 already accepted.
# ---------------------------------------------------------------------------
cases_file="$repo_root/$cases_rel"
[[ -e "$cases_file" ]] || infra 'cases_tsv_absent'
[[ -f "$cases_file" && ! -L "$cases_file" ]] || infra 'cases_tsv_unreadable'
[[ -r "$cases_file" ]] || infra 'cases_tsv_unreadable'
cases_bytes="$(wc -c <"$cases_file")" || infra 'cases_tsv_unreadable'
[[ "$cases_bytes" =~ ^[0-9]+$ ]] || infra 'cases_tsv_unreadable'
(( cases_bytes > 0 && cases_bytes <= max_cases_bytes )) || infra 'cases_tsv_out_of_bounds'

case_count=0
while IFS=$'\t' read -r c_name c_fixture c_input_sha c_expect || [[ -n "$c_name" ]]; do
  [[ -n "$c_name" ]] || continue
  [[ "${c_name:0:1}" != '#' ]] || continue
  case_count=$(( case_count + 1 ))
  (( case_count <= max_cases_rows )) || infra 'cases_tsv_exceeds_row_bound'

  fixture_root="$repo_root/$fixtures_rel/$c_fixture"
  [[ -d "$fixture_root" ]] || infra "fixture_directory_absent:$c_fixture"

  fixture_source="$fixture_root/$source_rel"
  if [[ "$c_input_sha" == '-' ]]; then
    [[ ! -e "$fixture_source" ]] || infra "fixture_expected_absent:$c_fixture"
  else
    [[ -e "$fixture_source" && -r "$fixture_source" ]] ||
      infra "fixture_source_unreadable:$c_fixture"
    fixture_digest_line="$(sha256sum -- "$fixture_source")" || infra "fixture_unhashable:$c_fixture"
    fixture_digest="${fixture_digest_line%% *}"
    [[ "$fixture_digest" == "$c_input_sha" ]] || infra "fixture_digest_diverges:$c_fixture"
  fi

  set +e
  case_word="$(verify_trust_root_dir "$fixture_root")"
  case_rc=$?
  set -e
  case "$case_rc" in
    0|1) ;;
    *) infra "fixture_case_infra:$c_fixture/$case_word" ;;
  esac
  [[ "$case_word" == "$c_expect" ]] ||
    fail 'fixture_sensitivity_error' "$c_name expected=$c_expect got=$case_word"
done < "$cases_file"
(( case_count > 0 )) || infra 'cases_tsv_empty'

printf '{"card":"%s","scenario":"AC-001","selector":"%s","fixture_cases":%d,"result":"pass"}\n' \
  "$card" "$selector" "$case_count"
