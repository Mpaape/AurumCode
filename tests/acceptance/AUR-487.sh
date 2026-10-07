#!/usr/bin/env bash
set -euo pipefail

# Retired acceptance (measured by AUR-589, docs/specs/AUR-589.md).
# This program checked the static landing page under docs/site: hero,
# sections, calls to action, flags and environment it cites, style tokens,
# anchors, and (AC-005) that the copyable workflow snippet never references
# @main. Commit d3b50e74 (make main the sole persistent integration branch)
# deliberately pointed the example workflow and the identical site snippet at
# review.yml@main, so the pin AC-005 requires was reversed by design and the
# program as a whole can no longer pass. When AUR-589 measured it, AC-001 to
# AC-004 and AC-006 held once two stale anchors were fixed (a producer piped
# into grep -q under pipefail, and flags now registered as fs.StringVar).
# Exit 69 says plainly that the original proof can no longer run: it is
# neither a pass nor a functional rejection, for every selector. The
# historical evidence stays in the card record under .board/.

readonly card='AUR-487'
readonly reason='its AC-005 forbids @main in the published workflow snippet, but d3b50e74 made main the sole branch and pinned the example and site snippet to review.yml@main on purpose; not a pass'

printf '%s/retired: %s\n' "$card" "$reason" >&2
exit 69
