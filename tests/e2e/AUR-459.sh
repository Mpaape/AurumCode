#!/usr/bin/env bash
#
# Retired E2E program for card AUR-459, selector E2EAUR459.
#
# It proved that a finding the model reported only under "line_comments"
# closed the --fail-on gate. That path no longer exists: "line_comments"
# carried no evidence, impact or verification, so every finding converted
# from it was discarded by the evidence gate, and the review template never
# taught it. The parser now treats a reply whose only list is
# "line_comments" as inconclusive and drops (and announces) line comments
# beside a real "issues" list. Proving the old conversion would mean
# admitting findings without evidence, so this program is retired
# explicitly. The decision and the measurement are in docs/specs/AUR-546.md;
# the new behavior is proven by tests/acceptance/AUR-546.sh.
#
# EXIT CODES (tests/acceptance/EXIT_CODE_CONVENTION.md):
#   64 = unknown selector
#   69 = retired: neither a pass nor a functional rejection
set -Eeuo pipefail

readonly card='AUR-459'
readonly scenario='E2E'
selector="${1:-E2EAUR459}"
case "$selector" in
  E2EAUR459) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$scenario" >&2; exit 64 ;;
esac

printf '%s/%s/retired: line_comments is no longer a findings schema (see docs/specs/AUR-546.md); the conversion this program proved was removed and is not a pass\n' "$card" "$scenario" >&2
exit 69
