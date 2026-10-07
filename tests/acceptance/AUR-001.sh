#!/usr/bin/env bash
set -euo pipefail

# Retired acceptance (measured by AUR-589, docs/specs/AUR-589.md).
# This Task Master era program inventoried and pinned the behavior of the
# legacy product (pre-reconstruction commands, extractors and pipeline).
# Commit 670c7f6d removed that legacy surface, so the baseline it compares
# against no longer exists in the product.
# Exit 69 says plainly that the original proof can no longer run: it is
# neither a pass nor a functional rejection, for every selector. The
# historical evidence stays in the card record under .board/.

readonly card='AUR-001'
readonly reason='the legacy product it inventoried and characterized was removed in 670c7f6d (Focus AurumCode on code review); not a pass'

printf '%s/retired: %s\n' "$card" "$reason" >&2
exit 69
