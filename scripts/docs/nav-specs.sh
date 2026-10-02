#!/usr/bin/env bash
# Prints the "Specs" nav block (YAML, 4-space indent) from docs/specs/AUR-*.md,
# in natural order. Paste it under "- Specs:" in mkdocs.yml when specs change;
# tests/acceptance/AUR-560.sh fails if mkdocs.yml is missing any spec.
set -Eeuo pipefail
cd "$(dirname "$0")/../../docs/specs"
printf '    - specs/README.md\n'
for f in $(ls AUR-*.md | sort -V); do printf '    - specs/%s\n' "$f"; done
