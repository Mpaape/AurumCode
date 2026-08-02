#!/usr/bin/env bash
set -euo pipefail

export LC_ALL=C
umask 077

readonly card='AUR-388'
selector="${1:-AC-001}"
case "$selector" in
  AC-001|ContractAUR388|IntegrationAUR388|E2EAUR388) ;;
  *) printf '%s/AC-001/unknown-selector\n' "$card" >&2; exit 64 ;;
esac

repo_root="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd -P)"
source_file="$repo_root/.mcp.json"

fail() {
  printf '%s/AC-001/%s\n' "$card" "$1" >&2
  exit 1
}

infra() {
  printf '%s/AC-001/infrastructure: %s\n' "$card" "$1" >&2
  exit 79
}

[[ -f "$source_file" && ! -L "$source_file" ]] || fail settings_parse_inconclusive
bytes="$(wc -c <"$source_file" | tr -d ' ')"
(( bytes <= 1048576 )) || infra input_limit_exceeded
manifest="$repo_root/tests/characterization/legacy/agent-settings/AUR-388/manifest.tsv"
[[ -s "$manifest" && ! -L "$manifest" ]] || fail settings_parse_inconclusive
source_sha="$(sha256sum "$source_file" | awk '{print $1}')"
awk -F '\t' -v target='.mcp.json' -v digest="$source_sha" '
  $1 == target && $2 == digest && $0 ~ /secret_values=0/ && $0 ~ /commands_executed=0/ { found=1 }
  END { exit(found ? 0 : 1) }
' "$manifest" || fail settings_manifest_mismatch

printf '{"card":"%s","scenario":"AC-001","selector":"%s","result":"pass"}\n' "$card" "$selector"

