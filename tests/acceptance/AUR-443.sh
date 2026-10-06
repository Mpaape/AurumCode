#!/usr/bin/env bash
set -euo pipefail

# Retired acceptance (measured by AUR-589, docs/specs/AUR-589.md).
# This program proved the first-use surface of the binary: the top-level
# --help listing review and docs, usage errors for both subcommands, and a
# provider-missing review that exits 1 with a fixture-shaped hint. Commit
# 670c7f6d (Focus AurumCode on code review) removed the docs subcommand, and
# decd6cb2 (share local and pull request context) made a review without an
# LLM provider run the deterministic analysis and exit 0 with an explicit
# notice. Both changes were intentional, so the original proof can no longer
# hold as written.
# Exit 69 says plainly that the original proof can no longer run: it is
# neither a pass nor a functional rejection, for every selector. The
# historical evidence stays in the card record under .board/.

readonly card='AUR-443'
readonly reason='the first-use surface it pinned changed in the product: the docs subcommand was removed in 670c7f6d and a review without a provider now degrades to a deterministic-only report with exit 0 (decd6cb2); not a pass'

printf '%s/retired: %s\n' "$card" "$reason" >&2
exit 69
