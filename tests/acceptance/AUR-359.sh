#!/usr/bin/env bash
set -euo pipefail

export LC_ALL=C
umask 077

readonly card='AUR-359'
selector="${1:-AC-001}"
case "$selector" in
  AC-001|ContractAUR381|IntegrationAUR381|E2EAUR381) ;;
  *) printf '%s/AC-001/unknown-selector\n' "$card" >&2; exit 64 ;;
esac

repo_root="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd -P)"
source_file="$repo_root/.board/bootstrap/locks/trust-root.yml"

fail() {
  printf '%s/AC-001/%s\n' "$card" "$1" >&2
  exit 1
}

[[ -f "$source_file" && ! -L "$source_file" ]] || fail trust_root_mismatch
grep -Fqx 'schema: bootstrap-lock-v1' "$source_file" || fail invalid_lock_or_manifest
grep -Eq 'sha256:[0-9a-f]{64}' "$source_file" || fail invalid_lock_or_manifest
if grep -Eiq '(^|[^a-z])latest([^a-z]|$)|@[[:space:]]*(main|master|head)([^a-z]|$)' "$source_file"; then
  fail mutable_reference
fi
runner="$repo_root/.board/bin/oci-run"
[[ -f "$runner" && ! -L "$runner" ]] || fail runner_missing
runner_sha="$(sha256sum "$runner" | awk '{print $1}')"
grep -Fq "$runner_sha" "$source_file" || fail runner_digest_mismatch
grep -Fq 'bash@sha256:ae4668c2560999e65e89532cd2ad1b6688bb23298189f0bd229ef80fa4bd0831' "$source_file" || fail image_digest_mismatch

printf '{"card":"%s","scenario":"AC-001","selector":"%s","result":"pass"}\n' "$card" "$selector"

