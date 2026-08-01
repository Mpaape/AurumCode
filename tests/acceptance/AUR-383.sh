#!/usr/bin/env bash
set -euo pipefail

export LC_ALL=C
umask 077

readonly card='AUR-383'
selector="${1:-AC-001}"
case "$selector" in
  AC-001|ContractAUR383|IntegrationAUR383|E2EAUR383) ;;
  *) printf '%s/AC-001/unknown-selector\n' "$card" >&2; exit 64 ;;
esac

repo_root="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd -P)"
source_file="$repo_root/docs/README.md"

fail() {
  printf '%s/AC-001/%s\n' "$card" "$1" >&2
  exit 1
}

[[ -f "$source_file" && ! -L "$source_file" ]] || fail guide_claim_unmapped
bytes="$(wc -c <"$source_file" | tr -d ' ')"
(( bytes <= 2097152 )) || fail input_limit_exceeded
claims="$repo_root/tests/characterization/legacy/guide-claims/AUR-383/claims.tsv"
[[ -s "$claims" && ! -L "$claims" ]] || fail guide_claim_unmapped
source_sha="$(sha256sum "$source_file" | awk '{print $1}')"
awk -F '\t' -v target='docs/README.md' -v digest="$source_sha" '
  $1 == target && $2 == digest && $0 ~ /(evidence|qualify|remove|keep)/ { found=1; count++ }
  END { exit(found && count <= 256 ? 0 : 1) }
' "$claims" || fail claim_matrix_mismatch
if grep -Eq $'\t(unverified|unsupported)\t[^\t]*$' "$claims"; then
  fail claim_without_disposition
fi

printf '{"card":"%s","scenario":"AC-001","selector":"%s","result":"pass"}\n' "$card" "$selector"

