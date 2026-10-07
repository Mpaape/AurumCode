#!/usr/bin/env bash
set -euo pipefail

# Retired acceptance (measured by AUR-589, docs/specs/AUR-589.md).
# This program cross-checked that delivered specs described the source of
# their time: a docs subcommand in cmd/aurumcode, a --pr flag,
# extractFilePath at fixed lines of internal/git/githubclient/client.go and
# ref: v1 in code-review.yml. Commit 670c7f6d (Focus AurumCode on code
# review) removed the docs subcommand and the following review cards reshaped
# the flags, the client and the workflow, so every anchor it corroborates is
# gone on purpose.
# Exit 69 says plainly that the original proof can no longer run: it is
# neither a pass nor a functional rejection, for every selector. The
# historical evidence stays in the card record under .board/.

readonly card='AUR-446'
readonly reason='the delivered behavior its spec cross-checks pinned (docs subcommand, --pr flag, client.go line anchors, ref v1 workflow) was removed or reshaped in 670c7f6d and later review cards; not a pass'

printf '%s/retired: %s\n' "$card" "$reason" >&2
exit 69
