#!/usr/bin/env bash
set -euo pipefail

f='.board/research/secure-code-review.md'
[[ -s "$f" ]] || { printf 'AUR-015/AC-001: secure review baseline absent\n' >&2; exit 1; }
for needle in 'OWASP' 'threat' 'trust boundary' 'fail closed'; do
  grep -Fiq "$needle" "$f" || { printf 'AUR-015/AC-001: missing %s\n' "$needle" >&2; exit 1; }
done
printf '{"card":"AUR-015","scenario":"AC-001","result":"pass"}\n'
