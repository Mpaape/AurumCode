#!/usr/bin/env bash
set -euo pipefail

f='.board/research/quality-standards.md'
[[ -s "$f" ]] || { printf 'AUR-016/AC-001: quality evidence baseline absent\n' >&2; exit 1; }
grep -Fq 'ISO/IEC 25010:2023' "$f" || exit 1
grep -Fiq 'does not claim conformity' "$f" || exit 1
printf '{"card":"AUR-016","scenario":"AC-001","result":"pass"}\n'
