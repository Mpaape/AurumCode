#!/usr/bin/env bash
set -euo pipefail

# Retired acceptance. When AUR-586 re-ran it in a clean tree with cmd, internal
# and pkg copied whole, the behavior it observed was red for a reason that is
# not a staging gap: it asserts rule-citation behavior over fixtures whose findings the current scope-and-evidence gate now discards ("sem evidencia concreta"), so the original proof no longer holds.
# Rewriting it would mean rewriting the behavior it proved, so it is retired
# explicitly. Exit 69 states that plainly: it is neither a pass nor a
# functional rejection. The historical evidence stays in the card record under
# .board/, and the measurement is in docs/specs/AUR-586.md.

readonly card='AUR-434'

case "${1:-AC-001}" in
  AC-001|TestAUR434|IntegrationAUR434|E2EAUR434|AC-001-MUT-001) ;;
  *) printf '%s/unknown-selector\n' "$card" >&2; exit 64 ;;
esac

printf '%s/retired: the behavior this acceptance proved changed in the product (see docs/specs/AUR-586.md); it can no longer be proven as written and is not a pass\n' "$card" >&2
exit 69
