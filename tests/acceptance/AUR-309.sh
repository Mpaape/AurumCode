#!/usr/bin/env bash
set -euo pipefail

# Retired acceptance. The characterization fixture this program exercised
# (tests/characterization/legacy/pipeline) was deleted together with the legacy
# extractor pipeline it observed, so the original proof can no longer run.
# Exit 69 states that plainly: it is neither a pass nor a functional rejection.
# The historical evidence stays in the card record under .board/.

readonly card='AUR-309'

case "${1:-AC-001}" in
  AC-001|TestAUR309|ContractAUR309|IntegrationAUR309|E2EAUR309) ;;
  *) printf '%s/unknown-selector\n' "$card" >&2; exit 64 ;;
esac

printf '%s/retired: legacy pipeline characterization fixture was removed; this acceptance can no longer be proven and is not a pass\n' "$card" >&2
exit 69
