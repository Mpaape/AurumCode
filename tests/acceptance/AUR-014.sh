#!/usr/bin/env bash
set -euo pipefail

f='.board/research/code-review-standards.md'
for needle in 'NIST SP 800-218' 'Google Engineering Practices' \
  'ISO/IEC 20246:2017' 'ISO/IEC 25010:2023' 'IEEE 1028-2008' \
  'CR-REV-001' 'CR-EVD-002'; do
  grep -Fq "$needle" "$f" || { printf 'AUR-014/AC-001: missing %s\n' "$needle" >&2; exit 1; }
done
printf '{"card":"AUR-014","scenario":"AC-001","result":"pass"}\n'
