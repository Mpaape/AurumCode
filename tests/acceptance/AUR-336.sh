#!/usr/bin/env bash
set -euo pipefail

readonly manifest='tests/characterization/legacy/makefile/manifest.tsv'
[[ -s "$manifest" && ! -L "$manifest" ]] || {
  printf 'AUR-336/AC-001: Makefile manifest absent\n' >&2
  exit 1
}
grep -Eq $'Makefile\t[^\t]+\t[^\t]+\texecuted=false$' "$manifest" || {
  printf 'AUR-336/AC-001: Makefile non-execution proof absent\n' >&2
  exit 1
}
printf '{"card":"AUR-336","scenario":"AC-001","result":"pass"}\n'
