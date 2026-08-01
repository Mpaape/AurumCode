#!/usr/bin/env bash
set -euo pipefail
manifest='tests/characterization/legacy/changelog/manifest.tsv'
[[ -s "$manifest" && ! -L "$manifest" ]] || { printf 'AUR-356/AC-001: changelog manifest absent\n' >&2; exit 1; }
grep -Eq $'^CHANGELOG\.md\t[^\t]+\townership=(manual|mixed)\t' "$manifest" || { printf 'AUR-356/AC-001: changelog ownership absent\n' >&2; exit 1; }
printf '{"card":"AUR-356","scenario":"AC-001","result":"pass"}\n'
