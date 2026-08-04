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
# materialized alone into the sealed container), so the two verdict helpers are
# declared per-file, on purpose.

# fail() is reserved for artifacts listed in this card's own `paths:`
# (.board/bootstrap/locks/trust-root.yml and .board/bin/oci-run). Their absence
# or corruption IS the behavior under test failing to exist: that is evidence.
fail() {
  printf '%s/AC-001/%s\n' "$card" "$1" >&2
  printf '%s/AC-001/detail expected-artifact=%s\n' "$card" "${2:-.board/bootstrap/locks/trust-root.yml}" >&2
  exit 1
}

# infra() is reserved for everything this card does NOT own: a missing tool, an
# unreadable input, a bound exceeded, a path this program could not resolve.
# An environment gap must never be laundered into behavioral red.
infra() {
  printf '%s/AC-001/inconclusive/%s\n' "$card" "$1" >&2
  exit 79
}

# Tool preflight. `command -v` is a bash builtin, so this check cannot itself be
# defeated by the missing-tool condition it is testing for. Without it a missing
# `grep` exits 1 with a card-specific error code — a fabricated behavioral red.
for tool in grep sha256sum wc; do
  command -v "$tool" >/dev/null 2>&1 || infra "missing_tool:$tool"
done

# Resolve the repository root without `dirname`: one less external dependency,
# and a missing `dirname` previously degraded silently into repo_root="/" and a
# false trust_root_mismatch.
script_path="$0"
case "$script_path" in
  */*) script_dir="${script_path%/*}" ;;
  *)   script_dir='.' ;;
esac
repo_root="$(CDPATH='' cd -- "$script_dir/../.." >/dev/null 2>&1 && pwd -P)" || infra repo_root_unresolved
[[ -n "$repo_root" && -d "$repo_root" ]] || infra repo_root_unresolved

# Only the two artifacts this card declares in `paths:` are ever read. No
# forbidden path (.git, .env, credentials, secrets, .board/cards) is touched,
# and nothing outside `paths:` is required for this program to reach a verdict.
readonly source_rel='.board/bootstrap/locks/trust-root.yml'
readonly runner_rel='.board/bin/oci-run'
source_file="$repo_root/$source_rel"
runner="$repo_root/$runner_rel"

# 4 MiB, the bound the card declares for its sealed input.
readonly max_source_bytes=4194304

# --- this card's own deliverable: absence and tamper are behavioral RED -------
[[ -e "$source_file" ]] || fail trust_root_mismatch "$source_rel"
[[ ! -L "$source_file" ]] || fail source_integrity_error "$source_rel"
[[ -f "$source_file" ]] || fail trust_root_mismatch "$source_rel"

# ...but "present and unreadable" or "present and oversized" is an environment
# condition, not a verdict about the lock's content.
[[ -r "$source_file" ]] || infra source_unreadable
source_bytes="$(wc -c <"$source_file")" || infra source_unreadable
[[ "$source_bytes" =~ ^[0-9]+$ ]] || infra source_size_unreadable
(( source_bytes <= max_source_bytes )) || infra input_limit_exceeded

grep -Fqx 'schema: bootstrap-lock-v1' "$source_file" || fail invalid_lock_or_manifest "$source_rel"
grep -Eq 'sha256:[0-9a-f]{64}' "$source_file" || fail invalid_lock_or_manifest "$source_rel"
if grep -Eiq '(^|[^a-z])latest([^a-z]|$)|@[[:space:]]*(main|master|head)([^a-z]|$)' "$source_file"; then
  fail mutable_reference "$source_rel"
fi

[[ -e "$runner" ]] || fail runner_missing "$runner_rel"
[[ ! -L "$runner" ]] || fail source_integrity_error "$runner_rel"
[[ -f "$runner" ]] || fail runner_missing "$runner_rel"
[[ -r "$runner" ]] || infra runner_unreadable

# Drop `awk`: the digest is sliced with bash parameter expansion, so the hash
# step depends on `sha256sum` alone. A malformed digest is an infra fault in
# this program's own measurement, not evidence that the lock disagrees.
runner_digest_line="$(sha256sum -- "$runner")" || infra runner_digest_unreadable
runner_sha="${runner_digest_line%% *}"
[[ "$runner_sha" =~ ^[0-9a-f]{64}$ ]] || infra runner_digest_unreadable

grep -Fq "$runner_sha" "$source_file" || fail runner_digest_mismatch "$runner_rel"
grep -Fq 'bash@sha256:ae4668c2560999e65e89532cd2ad1b6688bb23298189f0bd229ef80fa4bd0831' "$source_file" || fail image_digest_mismatch "$source_rel"

printf '{"card":"%s","scenario":"AC-001","selector":"%s","result":"pass"}\n' "$card" "$selector"
