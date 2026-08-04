#!/usr/bin/env bash
set -euo pipefail
manifest='tests/characterization/legacy/aurumcode-ignore/manifest.tsv'
[[ -s "$manifest" && ! -L "$manifest" ]] || { printf 'AUR-354/AC-001: local ignore manifest absent\n' >&2; exit 1; }
grep -Eq $'^\.aurumcode/\.gitignore\t[^\t]+\tpatterns_digest=[^\t]+\tdisposition=' "$manifest" || { printf 'AUR-354/AC-001: local ignore disposition absent\n' >&2; exit 1; }
printf '{"card":"AUR-354","scenario":"AC-001","result":"pass"}\n'
