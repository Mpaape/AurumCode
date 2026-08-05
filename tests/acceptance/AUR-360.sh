#!/usr/bin/env bash
set -euo pipefail

export LC_ALL=C
umask 077

readonly card='AUR-360'
selector="${1:-AC-001}"
case "$selector" in
  AC-001|ContractAUR360|IntegrationAUR360|E2EAUR360) ;;
  *) printf '%s/AC-001/unknown-selector\n' "$card" >&2; exit 64 ;;
esac

# Exit codes follow tests/acceptance/EXIT_CODE_CONVENTION.md:
#   0  the promised behavior was observed
#   1  behavioral RED — a deliverable this card OWNS is missing or wrong
#   64 unknown selector
#   79 inconclusive / infrastructure — a tool, a bound, this program's own
#      setup, or a read_path this card does not own failed. Never valid red
#      evidence, never a pass.
# No sourced shared library (the acceptance program is materialized alone
# into the sealed container): the two verdict helpers below are declared
# per-file on purpose, mirroring tests/acceptance/AUR-359.sh.

# fail() is reserved for this card's own deliverable
# (.board/bootstrap/locks/go.yml) failing to prove what the Outcome and
# AC-001 Then promise: absence, drift from go.mod/go.sum, structural
# corruption, a mutable reference, or an over-limit field IS the behavior
# under test failing — that is evidence, not infrastructure.
fail() {
  printf '%s/AC-001/%s\n' "$card" "$1" >&2
  [[ -z "${2:-}" ]] || printf '%s/AC-001/detail %s\n' "$card" "$2" >&2
  exit 1
}

# infra() is reserved for everything this card does NOT own as a behavioral
# claim: a missing tool, this program's own fixture machinery failing to
# reproduce, or one of the read_paths (go.mod, go.sum) this card only reads
# being absent/unreadable in the materialized tree. An environment gap must
# never be laundered into behavioral red.
infra() {
  printf '%s/AC-001/inconclusive/%s\n' "$card" "$1" >&2
  exit 79
}

for tool in grep sha256sum wc; do
  command -v "$tool" >/dev/null 2>&1 || infra "missing_tool:$tool"
done

# Resolve the repository root without `dirname`.
script_path="$0"
case "$script_path" in
  */*) script_dir="${script_path%/*}" ;;
  *)   script_dir='.' ;;
esac
repo_root="$(CDPATH='' cd -- "$script_dir/../.." >/dev/null 2>&1 && pwd -P)" || infra repo_root_unresolved
[[ -n "$repo_root" && -d "$repo_root" ]] || infra repo_root_unresolved

readonly source_rel='.board/bootstrap/locks/go.yml'
readonly gomod_rel='go.mod'
readonly gosum_rel='go.sum'
readonly cases_rel='tests/bootstrap/locks/AUR-360/cases.tsv'
readonly fixtures_rel='tests/bootstrap/locks/AUR-360/fixtures'

# 4 KiB: this is a small flat manifest (schema + go_version + module + two
# source digests + N module triples), not free-form input.
readonly max_source_bytes=4096
# The card names no separate numeric ceiling for module_count; this reuses
# the 256-row bound the card already declares for tests/bootstrap/locks/
# AUR-360/cases.tsv (a builder judgment call, documented in
# docs/specs/AUR-360.md) so the whole card family shares one limit rather
# than inventing an unrelated second number.
readonly max_module_count=256
readonly max_cases_bytes=4194304
readonly max_cases_rows=256

# ---------------------------------------------------------------------------
# verify_go_lock <lock_root> <gomod_file> <gosum_file>
#   Prints one of: pass | go_lock_mismatch | source_integrity_error |
#   invalid_lock_or_manifest | mutable_reference | input_limit_exceeded |
#   module-path-mismatch | module-checksum-missing
#   Returns: 0 pass; 1 behavioral (the printed word is the typed code);
#            2 infra (the printed word is the infra reason).
#
# <gomod_file>/<gosum_file> are always the real, materialized go.mod/go.sum
# (this card's read_paths) — fixtures under fixtures_rel supply only a
# candidate .board/bootstrap/locks/go.yml and are checked against that one
# real source pair, never a fixture-private copy, so Phase 2 proves lock
# *content* sensitivity against the one true source, never a diverging one.
# ---------------------------------------------------------------------------
verify_go_lock() {
  local lock_root="$1" gomod_file="$2" gosum_file="$3"
  local source_file="$lock_root/$source_rel"
  local bytes line key val req_key
  local gomod_sha_line gomod_sha gosum_sha_line gosum_sha
  local real_go_version real_module declared_paths_count
  local gs_path gs_ver gs_sum p v s i declared_count

  [[ -e "$source_file" ]] || { printf 'go_lock_mismatch\n'; return 1; }
  [[ ! -L "$source_file" ]] || { printf 'source_integrity_error\n'; return 1; }
  [[ -f "$source_file" ]] || { printf 'go_lock_mismatch\n'; return 1; }
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

  [[ -e "$gomod_file" && -f "$gomod_file" && ! -L "$gomod_file" && -r "$gomod_file" ]] || { printf 'gomod_unreadable\n'; return 2; }
  [[ -e "$gosum_file" && -f "$gosum_file" && ! -L "$gosum_file" && -r "$gosum_file" ]] || { printf 'gosum_unreadable\n'; return 2; }

  # -- parse the flat key: value lock into an associative array --
  local -A kv=()
  while IFS= read -r line || [[ -n "$line" ]]; do
    [[ -n "$line" ]] || continue
    case "$line" in
      '#'*) continue ;;
      *:*) ;;
      *) continue ;;
    esac
    key="${line%%:*}"
    case "$line" in
      *": "*) val="${line#*: }" ;;
      *) val='' ;;
    esac
    kv["$key"]="$val"
  done < "$source_file"

  for req_key in go_version module go_mod_sha256 go_sum_sha256 module_count; do
    [[ -n "${kv[$req_key]+set}" && -n "${kv[$req_key]}" ]] || { printf 'invalid_lock_or_manifest\n'; return 1; }
  done

  [[ "${kv[module_count]}" =~ ^[0-9]+$ ]] || { printf 'invalid_lock_or_manifest\n'; return 1; }
  declared_count="${kv[module_count]}"
  (( declared_count >= 1 )) || { printf 'invalid_lock_or_manifest\n'; return 1; }
  (( declared_count <= max_module_count )) || { printf 'input_limit_exceeded\n'; return 1; }

  # -- recompute go.mod / go.sum digests: a digest a file declares about
  #    itself is never trusted un-rechecked. --
  gomod_sha_line="$(sha256sum -- "$gomod_file")" || { printf 'gomod_unreadable\n'; return 2; }
  gomod_sha="${gomod_sha_line%% *}"
  [[ "$gomod_sha" =~ ^[0-9a-f]{64}$ ]] || { printf 'gomod_unreadable\n'; return 2; }
  gosum_sha_line="$(sha256sum -- "$gosum_file")" || { printf 'gosum_unreadable\n'; return 2; }
  gosum_sha="${gosum_sha_line%% *}"
  [[ "$gosum_sha" =~ ^[0-9a-f]{64}$ ]] || { printf 'gosum_unreadable\n'; return 2; }

  [[ "${kv[go_mod_sha256]}" == "sha256:$gomod_sha" ]] || { printf 'source_integrity_error\n'; return 1; }
  [[ "${kv[go_sum_sha256]}" == "sha256:$gosum_sha" ]] || { printf 'source_integrity_error\n'; return 1; }

  # -- go_version: go.mod's `go <version>` directive line --
  set -- $(grep -E '^go[[:space:]]+[0-9]' "$gomod_file" | head -n1)
  real_go_version="${2:-}"
  [[ -n "$real_go_version" ]] || { printf 'gomod_unreadable\n'; return 2; }
  [[ "${kv[go_version]}" == "$real_go_version" ]] || { printf 'go_lock_mismatch\n'; return 1; }

  # -- module path: go.mod's `module <path>` directive line --
  set -- $(grep -E '^module[[:space:]]' "$gomod_file" | head -n1)
  real_module="${2:-}"
  [[ -n "$real_module" ]] || { printf 'gomod_unreadable\n'; return 2; }
  [[ "${kv[module]}" == "$real_module" ]] || { printf 'module-path-mismatch\n'; return 1; }

  # -- module_count must equal the number of module_<n>_path lines the
  #    document actually carries (self-consistency, independent of go.sum). --
  declared_paths_count="$(grep -c -E '^module_[0-9]+_path:' "$source_file")" || declared_paths_count=0
  [[ "$declared_paths_count" == "$declared_count" ]] || { printf 'invalid_lock_or_manifest\n'; return 1; }

  # -- real module set from go.sum: content-hash rows only (version field
  #    without a trailing /go.mod), never the /go.mod-hash row. --
  local -A real_ver=() real_sum=()
  while IFS=' ' read -r gs_path gs_ver gs_sum || [[ -n "$gs_path" ]]; do
    [[ -n "$gs_path" ]] || continue
    case "$gs_ver" in
      */go.mod) continue ;;
    esac
    real_ver["$gs_path"]="$gs_ver"
    real_sum["$gs_path"]="$gs_sum"
  done < "$gosum_file"
  (( ${#real_ver[@]} > 0 )) || { printf 'gosum_unreadable\n'; return 2; }

  # -- lock module set: module_1..module_<declared_count> triples --
  local -A lock_ver=() lock_sum=()
  i=1
  while (( i <= declared_count )); do
    p="${kv[module_${i}_path]:-}"
    v="${kv[module_${i}_version]:-}"
    s="${kv[module_${i}_checksum]:-}"
    [[ -n "$p" && -n "$v" ]] || { printf 'invalid_lock_or_manifest\n'; return 1; }
    [[ -z "${lock_ver[$p]+set}" ]] || { printf 'invalid_lock_or_manifest\n'; return 1; }
    lock_ver["$p"]="$v"
    lock_sum["$p"]="$s"
    i=$(( i + 1 ))
  done

  # -- forward: every module go.sum records must be pinned, version-exact,
  #    checksum-present, checksum-exact. --
  for p in "${!real_ver[@]}"; do
    [[ -n "${lock_ver[$p]+set}" ]] || { printf 'go_lock_mismatch\n'; return 1; }
    [[ "${lock_ver[$p]}" == "${real_ver[$p]}" ]] || { printf 'go_lock_mismatch\n'; return 1; }
    [[ -n "${lock_sum[$p]}" ]] || { printf 'module-checksum-missing\n'; return 1; }
    [[ "${lock_sum[$p]}" == "${real_sum[$p]}" ]] || { printf 'go_lock_mismatch\n'; return 1; }
  done

  # -- reverse: no phantom pin for a module go.sum does not record. --
  for p in "${!lock_ver[@]}"; do
    [[ -n "${real_ver[$p]+set}" ]] || { printf 'go_lock_mismatch\n'; return 1; }
  done

  printf 'pass\n'
  return 0
}

# ---------------------------------------------------------------------------
# Phase 1 — this card's own deliverable, the real lock, checked against the
# real go.mod/go.sum FIRST and on its own: absence and drift are behavioral
# RED because .board/bootstrap/locks/go.yml is this card's own declared
# path, and that must be provable with nothing else in this file's fixture
# machinery built yet (RED-before-GREEN: tests/bootstrap/locks/AUR-360/
# cases.tsv and its fixtures do not gate this phase).
# ---------------------------------------------------------------------------
gomod_real="$repo_root/$gomod_rel"
gosum_real="$repo_root/$gosum_rel"

set +e
real_word="$(verify_go_lock "$repo_root" "$gomod_real" "$gosum_real")"
real_rc=$?
set -e
case "$real_rc" in
  0) ;;
  1) fail "$real_word" "source=$source_rel" ;;
  *) infra "$real_word" ;;
esac

# ---------------------------------------------------------------------------
# Phase 2 — only once the real lock verifies clean does this program trust
# itself to judge fixtures: it re-proves its own sensitivity against
# tests/bootstrap/locks/AUR-360/cases.tsv, which seals, per row: a case
# name, the fixture subdirectory under tests/bootstrap/locks/AUR-360/
# fixtures/, the SHA-256 of the fixture's own go.yml (or `-` when the case
# is deliberate absence), and the expected disposition word (`pass` or one
# of the typed codes above). Every fixture is checked against the same real
# go.mod/go.sum Phase 1 already trusted. A fixture whose recomputed digest
# diverges from what the row seals is this program's OWN setup being
# unsound: infra, never a verdict about the real tree Phase 1 accepted.
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
  case_word="$(verify_go_lock "$fixture_root" "$gomod_real" "$gosum_real")"
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
