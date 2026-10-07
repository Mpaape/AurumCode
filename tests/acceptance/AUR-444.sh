#!/usr/bin/env bash
set -euo pipefail

# Retired acceptance (measured by AUR-589, docs/specs/AUR-589.md).
# This program proved that the published GitHub Action runs the review it
# advertises, through bridge programs under tests/unit, tests/integration and
# tests/e2e. Commit 670c7f6d deleted those bridges together with the
# documentation pipeline the Action image then shipped, so the proof has no
# program left to run. The Action contract is exercised by later cards that
# read action.yml (AUR-534, AUR-576, AUR-578).
# Exit 69 says plainly that the original proof can no longer run: it is
# neither a pass nor a functional rejection, for every selector. The
# historical evidence stays in the card record under .board/.

readonly card='AUR-444'
readonly reason='its unit, integration and e2e bridges (tests/unit/AUR-444.go, tests/integration/AUR-444.go, tests/e2e/AUR-444.sh) were deleted with the legacy surface in 670c7f6d (Focus AurumCode on code review); not a pass'

printf '%s/retired: %s\n' "$card" "$reason" >&2
exit 69
