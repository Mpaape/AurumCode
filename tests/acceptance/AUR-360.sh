#!/usr/bin/env bash
set -euo pipefail

# Retired acceptance (measured by AUR-589, docs/specs/AUR-589.md).
# This Task Master era program verified the bootstrap lockset under
# .board/bootstrap/locks (trust root, Go toolchain, Actions, scanners,
# parsers, documentation tools) byte for byte against the digests pinned when
# it was written. AUR-534 (03ba3f84) moved the toolchain from Go 1.21 to Go
# 1.27 and AUR-522 (83753f5d) repinned the sealed image, so those pins were
# replaced on purpose; the live lockset is exercised by the sealed runner on
# every card.
# Exit 69 says plainly that the original proof can no longer run: it is
# neither a pass nor a functional rejection, for every selector. The
# historical evidence stays in the card record under .board/.

readonly card='AUR-360'
readonly reason='the bootstrap lockset and toolchain pins it verifies were intentionally repinned by AUR-534 (03ba3f84, Go 1.21 to 1.27) and AUR-522 (83753f5d); not a pass'

printf '%s/retired: %s\n' "$card" "$reason" >&2
exit 69
