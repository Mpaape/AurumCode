#!/usr/bin/env bash
set -euo pipefail

export LC_ALL=C
umask 077

readonly card='AUR-339'
selector="${1:-AC-001}"
case "$selector" in
  AC-001|ContractAUR339|IntegrationAUR339|E2EAUR339) ;;
  *) printf '%s/AC-001/unknown-selector\n' "$card" >&2; exit 64 ;;
esac

repo_root="$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd -P)"
fail() {
  printf '%s/AC-001/%s\n' "$card" "$1" >&2
  exit 1
}

expected_csv='AUR-386,AUR-387,AUR-388,AUR-389'
suite="$repo_root/tests/gates/legacy/AUR-339/suite.tsv"
[[ -s "$suite" && ! -L "$suite" ]] || fail setting_manifest_missing
expected_count="$(tr ',' '\n' <<< "$expected_csv" | awk 'NF { count++ } END { print count+0 }')"

awk -F '\t' -v expected="$expected_csv" -v expected_count="$expected_count" '
  BEGIN {
    count=split(expected, ids, ",")
    for (i=1; i<=count; i++) want[ids[i]]=1
  }
  NF != 5 { bad=1; next }
  !($1 in want) || seen[$1]++ { bad=1; next }
  $2 != "pass" { bad=1 }
  $3 !~ /^sha256:[0-9a-f]{64}$/ || $4 !~ /^sha256:[0-9a-f]{64}$/ || $5 !~ /^sha256:[0-9a-f]{64}$/ { bad=1 }
  identity == "" { identity=$3 }
  identity != $3 { bad=1 }
  END {
    if (NR != expected_count) bad=1
    for (id in want) if (!seen[id]) bad=1
    exit(bad ? 1 : 0)
  }
' "$suite" || fail suite_manifest_invalid

printf '{"card":"%s","scenario":"AC-001","selector":"%s","children":%s,"result":"pass"}\n' "$card" "$selector" "$expected_count"
