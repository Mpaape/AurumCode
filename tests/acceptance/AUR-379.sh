#!/usr/bin/env bash
set -euo pipefail

export LC_ALL=C
umask 077

readonly card='AUR-379'
selector="${1:-AC-001}"
case "$selector" in
  AC-001|ContractAUR379|IntegrationAUR379|E2EAUR379) ;;
  *) printf '%s/AC-001/unknown-selector\n' "$card" >&2; exit 64 ;;
esac

repo_root="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd -P)"
source_file="$repo_root/LITELLM_QUICKSTART.md"

fail() {
  printf '%s/AC-001/%s\n' "$card" "$1" >&2
  exit 1
}

[[ -f "$source_file" && ! -L "$source_file" ]] || fail claim_without_disposition
bytes="$(wc -c <"$source_file" | tr -d ' ')"
(( bytes <= 2097152 )) || fail input_limit_exceeded
claims="$repo_root/tests/migration/legacy/public-docs/AUR-379/claims.tsv"
[[ -s "$claims" && ! -L "$claims" ]] || fail claim_without_disposition
source_sha="$(sha256sum "$source_file" | awk '{print $1}')"
awk -F '\t' -v target='LITELLM_QUICKSTART.md' -v digest="$source_sha" '
  $1 == target && $2 == digest && $0 ~ /(evidence|qualify|remove|keep)/ { found=1; count++ }
  END { exit(found && count <= 256 ? 0 : 1) }
' "$claims" || fail claim_matrix_mismatch
if grep -Eq $'\t(unverified|unsupported)\t[^\t]*$' "$claims"; then
  fail claim_without_disposition
fi

printf '{"card":"%s","scenario":"AC-001","selector":"%s","result":"pass"}\n' "$card" "$selector"

