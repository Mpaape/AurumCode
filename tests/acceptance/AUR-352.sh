#!/usr/bin/env bash
set -euo pipefail
manifest='tests/characterization/legacy/aurumcode-bash-docs/manifest.tsv'
[[ -s "$manifest" && ! -L "$manifest" ]] || { printf 'AUR-352/AC-001: bash-doc manifest absent\n' >&2; exit 1; }
find .aurumcode/bash -type f -print0 | while IFS= read -r -d '' path; do
  grep -Fq "$path" "$manifest" || { printf 'AUR-352/AC-001: bash-doc path unmapped\n' >&2; exit 1; }
done
grep -Eq $'\texecuted=false$' "$manifest" || { printf 'AUR-352/AC-001: non-execution proof absent\n' >&2; exit 1; }
printf '{"card":"AUR-352","scenario":"AC-001","result":"pass"}\n'
