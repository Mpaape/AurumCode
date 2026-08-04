#!/usr/bin/env bash
set -euo pipefail

readonly manifest='tests/characterization/legacy/aurumcode-prompts/manifest.tsv'
[[ -s "$manifest" && ! -L "$manifest" ]] || {
  printf 'AUR-334/AC-001: prompt disposition manifest absent\n' >&2
  exit 1
}
find .aurumcode/prompts -type f -print0 | while IFS= read -r -d '' path; do
  grep -Fq "$path" "$manifest" || {
    printf 'AUR-334/AC-001: prompt path unmapped\n' >&2
    exit 1
  }
done
grep -Eq $'\t(keep|migrate|quarantine|remove)\ttrusted=false$' "$manifest" || {
  printf 'AUR-334/AC-001: disposition vocabulary absent\n' >&2
  exit 1
}
printf '{"card":"AUR-334","scenario":"AC-001","result":"pass"}\n'
