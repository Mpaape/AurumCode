#!/usr/bin/env bash
set -euo pipefail

export LC_ALL=C
umask 077

readonly card='AUR-364'
selector="${1:-AC-001}"
case "$selector" in
  AC-001|ContractAUR386|IntegrationAUR386|E2EAUR386) ;;
  *) printf '%s/AC-001/unknown-selector\n' "$card" >&2; exit 64 ;;
esac

repo_root="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd -P)"
source_file="$repo_root/.board/bootstrap/locks/docs.yml"

fail() {
  printf '%s/AC-001/%s\n' "$card" "$1" >&2
  exit 1
}

[[ -f "$source_file" && ! -L "$source_file" ]] || fail docs_tool_lock_incomplete
grep -Fqx 'schema: bootstrap-lock-v1' "$source_file" || fail invalid_lock_or_manifest
grep -Eq 'sha256:[0-9a-f]{64}' "$source_file" || fail invalid_lock_or_manifest
if grep -Eiq '(^|[^a-z])latest([^a-z]|$)|@[[:space:]]*(main|master|head)([^a-z]|$)' "$source_file"; then
  fail mutable_reference
fi

printf '{"card":"%s","scenario":"AC-001","selector":"%s","result":"pass"}\n' "$card" "$selector"

