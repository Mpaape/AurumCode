#!/usr/bin/env bash
set -euo pipefail

# Retired acceptance (measured by AUR-589, docs/specs/AUR-589.md).
# This program proved behavior of the documentation generator:
# internal/documentation (extractors, normalizer, site, welcome),
# internal/pipeline and cmd/regenerate-docs. Commit 670c7f6d (Focus AurumCode
# on code review and publish interactive documentation) removed that product
# surface; the project site is now built by MkDocs (AUR-560..564), not by the
# product. Rewriting this acceptance would mean re-adding the removed product.
# Exit 69 says plainly that the original proof can no longer run: it is
# neither a pass nor a functional rejection, for every selector. The
# historical evidence stays in the card record under .board/.

readonly card='AUR-426'
readonly reason='the documentation generator it proved (internal/documentation, cmd/regenerate-docs, welcome page, site scaffold) was removed from the product in 670c7f6d (Focus AurumCode on code review); not a pass'

printf '%s/retired: %s\n' "$card" "$reason" >&2
exit 69
