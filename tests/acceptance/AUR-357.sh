#!/usr/bin/env bash
set -euo pipefail
manifest='tests/characterization/legacy/gemini-instructions/manifest.tsv'
[[ -s "$manifest" && ! -L "$manifest" ]] || { printf 'AUR-357/AC-001: GEMINI instruction manifest absent\n' >&2; exit 1; }
grep -Eq $'^GEMINI\.md\t[^\t]+\tauthority=none\ttrusted=false$' "$manifest" || { printf 'AUR-357/AC-001: GEMINI trust disposition absent\n' >&2; exit 1; }
printf '{"card":"AUR-357","scenario":"AC-001","result":"pass"}\n'
