#!/usr/bin/env bash
set -euo pipefail

# Retired acceptance (measured by AUR-589, docs/specs/AUR-589.md).
# This Task Master era program checked the sealed OCI profile definitions
# (.board/oci/profiles and .board/locks/oci) through contract bridges pinned
# to the registry schema and image digests of its time. 4aac0b6e aligned the
# resource schema with the runner bounds, and AUR-534 (03ba3f84, f9d64956)
# and AUR-522 (83753f5d) repinned the images, so the pinned values were
# replaced on purpose; the live profiles are exercised by the sealed runner
# on every card.
# Exit 69 says plainly that the original proof can no longer run: it is
# neither a pass nor a functional rejection, for every selector. The
# historical evidence stays in the card record under .board/.

readonly card='AUR-405'
readonly reason='the sealed OCI profile registry and image locks it pins were intentionally changed by 4aac0b6e (resource schema) and repinned by AUR-534 (03ba3f84, f9d64956) and AUR-522 (83753f5d); not a pass'

printf '%s/retired: %s\n' "$card" "$reason" >&2
exit 69
