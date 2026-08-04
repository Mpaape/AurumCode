#!/usr/bin/env bash
set -euo pipefail
manifest='tests/characterization/legacy/aurumcode-rules/manifest.tsv'
[[ -s "$manifest" && ! -L "$manifest" ]] || { printf 'AUR-351/AC-001: rules manifest absent\n' >&2; exit 1; }
find .aurumcode/rules -type f -print0 | while IFS= read -r -d '' path; do
  grep -Fq "$path" "$manifest" || { printf 'AUR-351/AC-001: rule path unmapped\n' >&2; exit 1; }
done
grep -Eq $'\ttrusted=false\tauthority=none$' "$manifest" || { printf 'AUR-351/AC-001: trust disposition absent\n' >&2; exit 1; }
printf '{"card":"AUR-351","scenario":"AC-001","result":"pass"}\n'
