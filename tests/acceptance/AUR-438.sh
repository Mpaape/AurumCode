#!/usr/bin/env bash
set -euo pipefail

# Retired acceptance (measured by AUR-589, docs/specs/AUR-589.md).
# This program proved that `review --pr --publicar --na-linha` posts a
# finding on an added line inline and turns a finding outside the changed
# lines into a general pull request comment. The product now drops findings
# outside the added lines through the scope and evidence gate
# (internal/review/scope.go, OutsideAddedLines) and reports the discard on
# stderr; AUR-541 (a683b118, docs/specs/AUR-541.md) measured this exact
# scenario and recorded the discard as intended behavior. The general-comment
# promise therefore no longer describes the product; the inline, read-only
# refusal and determinism parts remain covered by later PR cards.
# Exit 69 says plainly that the original proof can no longer run: it is
# neither a pass nor a functional rejection, for every selector. The
# historical evidence stays in the card record under .board/.

readonly card='AUR-438'
readonly reason='the PR review now discards a model finding outside the added lines through the scope and evidence gate (internal/review/scope.go, OutsideAddedLines; recorded as intentional by AUR-541 a683b118) instead of posting it as a general comment; not a pass'

printf '%s/retired: %s\n' "$card" "$reason" >&2
exit 69
