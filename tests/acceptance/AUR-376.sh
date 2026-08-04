#!/usr/bin/env bash
set -euo pipefail

export LC_ALL=C
umask 077

readonly card='AUR-376'
selector="${1:-AC-001}"
case "$selector" in
  AC-001|ContractAUR376|IntegrationAUR376|E2EAUR376) ;;
  *) printf '%s/AC-001/unknown-selector\n' "$card" >&2; exit 64 ;;
esac

repo_root="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd -P)"
source_file="$repo_root/test-jekyll.sh"

fail() {
  printf '%s/AC-001/%s\n' "$card" "$1" >&2
  exit 1
}

infra() {
  printf '%s/AC-001/infrastructure: %s\n' "$card" "$1" >&2
  exit 79
}

[[ -f "$source_file" && ! -L "$source_file" ]] || fail legacy_source_uncharacterized
bytes="$(wc -c <"$source_file" | tr -d ' ')"
(( bytes <= 1048576 )) || infra input_limit_exceeded
manifest="$repo_root/tests/characterization/legacy/script-files/AUR-376/manifest.tsv"
[[ -s "$manifest" && ! -L "$manifest" ]] || fail legacy_source_uncharacterized
source_sha="$(sha256sum "$source_file" | awk '{print $1}')"
awk -F '\t' -v target='test-jekyll.sh' -v digest="$source_sha" '
  $1 == target && $2 == digest && $0 ~ /commands_executed=0/ { found=1 }
  END { exit(found ? 0 : 1) }
' "$manifest" || fail manifest_row_mismatch

printf '{"card":"%s","scenario":"AC-001","selector":"%s","result":"pass"}\n' "$card" "$selector"

