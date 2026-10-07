#!/usr/bin/env bash
set -euo pipefail

# Retired acceptance (measured by AUR-589, docs/specs/AUR-589.md).
# This program proved the example pull request workflow
# (.github/workflows/examples/code-review.yml) through a bridge that required
# the pull-requests write grant on a publishing job and never at the workflow
# level. Commit 2cd157bc (publish structured pull request reviews) turned the
# example into a single job that calls the reusable
# .github/workflows/review.yml, with the grants at the caller level, so the
# shape this proof pinned was replaced on purpose.
# Exit 69 says plainly that the original proof can no longer run: it is
# neither a pass nor a functional rejection, for every selector. The
# historical evidence stays in the card record under .board/.

readonly card='AUR-440'
readonly reason='the example workflow it pinned became a single-job caller of the reusable review.yml in 2cd157bc, so the job-level pull-requests grant it required no longer exists by design; not a pass'

printf '%s/retired: %s\n' "$card" "$reason" >&2
exit 69
