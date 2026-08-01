#!/usr/bin/env bash
set -euo pipefail
manifest='tests/characterization/legacy/iso-weights/manifest.tsv'
[[ -s "$manifest" && ! -L "$manifest" ]] || { printf 'AUR-353/AC-001: ISO weights manifest absent\n' >&2; exit 1; }
grep -Eq $'^\.aurumcode/iso25010-weights\.yml\t[^\t]+\t(normalized|migrate|quarantine)\tweight_sum=' "$manifest" || { printf 'AUR-353/AC-001: ISO weight disposition absent\n' >&2; exit 1; }
printf '{"card":"AUR-353","scenario":"AC-001","result":"pass"}\n'
