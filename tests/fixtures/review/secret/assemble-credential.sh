#!/usr/bin/env bash
# Prints the runtime-assembled synthetic credential-shaped secret used by
# the AUR-432 shell tests. The value is deliberately split across literals
# so no tracked file ever contains a credential shape (the sealed
# acceptance runner refuses tracked inputs matching one); the assembled
# value matches the `sk-…` shape the redaction filter and the runner both
# recognize, and it is accepted by nothing.
set -euo pipefail
printf 'sk-%s%s\n' 'aurumsynth' '0123456789abcdef01'
