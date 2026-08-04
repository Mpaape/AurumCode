#!/usr/bin/env bash
set -euo pipefail

export LC_ALL=C
umask 077

readonly card='AUR-391'
selector="${1:-AC-001}"
case "$selector" in
  AC-001|ContractAUR391|IntegrationAUR391|E2EAUR391) ;;
  *) printf '%s/AC-001/unknown-selector\n' "$card" >&2; exit 64 ;;
esac

repo_root="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd -P)"
source_file="$repo_root/.dockerignore"

fail() {
  printf '%s/AC-001/%s\n' "$card" "$1" >&2
  exit 1
}

infra() {
  printf '%s/AC-001/infrastructure: %s\n' "$card" "$1" >&2
  exit 79
}

[[ -f "$source_file" && ! -L "$source_file" ]] || fail ignore_semantics_incomplete
manifest="$repo_root/tests/characterization/legacy/root-ignores/AUR-391/manifest.tsv"
[[ -s "$manifest" && ! -L "$manifest" ]] || fail ignore_semantics_incomplete
source_sha="$(sha256sum "$source_file" | awk '{print $1}')"
awk -F '\t' -v target='.dockerignore' -v digest="$source_sha" '
  $1 == target && $2 == digest && $0 ~ /semantic_probe=pass/ && $0 ~ /cases=32/ { found=1 }
  END { exit(found ? 0 : 1) }
' "$manifest" || fail semantic_probe_mismatch

printf '{"card":"%s","scenario":"AC-001","selector":"%s","result":"pass"}\n' "$card" "$selector"

