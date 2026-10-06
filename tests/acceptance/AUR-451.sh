#!/usr/bin/env bash
set -euo pipefail

# Retired acceptance (measured by AUR-589, docs/specs/AUR-589.md).
# This program proved the --seguranca, --fail-on, --modelo and --limite flags
# on `review --pr` through a bridge that pins exactly one comment POST per
# finding and the `No issues found.` empty-review text. The PR path now
# publishes a review summary after the findings (two POSTs for one finding)
# and prints the published-count line on an empty review; dd3716f0 (review:
# --pr path as explicit phases) is where that publication contract lives
# today. The usage-error, --limite and --modelo subcases still pass, but the
# pinned publication shape no longer describes the product.
# Exit 69 says plainly that the original proof can no longer run: it is
# neither a pass nor a functional rejection, for every selector. The
# historical evidence stays in the card record under .board/.

readonly card='AUR-451'
readonly reason='the PR publication contract changed: the --pr path posts a review summary after the findings and prints the published-count line instead of No issues found. (phases restructured in dd3716f0); not a pass'

printf '%s/retired: %s\n' "$card" "$reason" >&2
exit 69
