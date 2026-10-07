#!/usr/bin/env bash
set -euo pipefail

# Retired acceptance (measured by AUR-589, docs/specs/AUR-589.md).
# This program proved the first-use surface of the binary: the top-level
# --help listing review and docs, usage errors for both subcommands, and a
# provider-missing review hint. Commit 670c7f6d (Focus AurumCode on code
# review) removed the docs subcommand on purpose, so the surface it pins no
# longer exists as written. Whether a review without a provider should exit
# 0 or 1 is measured by AUR-590 (AUR-448), not decided here.
# Exit 69 says plainly that the original proof can no longer run: it is
# neither a pass nor a functional rejection, for every selector. The
# historical evidence stays in the card record under .board/.

readonly card='AUR-443'
readonly reason='the first-use surface it pinned (review and docs subcommands) no longer exists: the docs subcommand was removed in 670c7f6d (Focus AurumCode on code review); not a pass'

printf '%s/retired: %s\n' "$card" "$reason" >&2
exit 69
