#!/usr/bin/env bash
# AUR-456 E2E: the "grep de verificacao estatica sobre a documentacao de
# raiz" command the card's AC-001 names, run as a standalone, offline,
# black-box script -- independent of tests/unit/AUR-456.go and
# tests/integration/AUR-456.go's Go implementation, so a single algorithmic
# bug in one layer cannot fake a pass in every layer.
#
# WHAT IT CHECKS
#   1. None of the fourteen dead paths the 2026-08-14 audit found
#      (.board/cards/ready/AUR-456.md, "Achados medidos") survive:
#      .aurumcode/rules/ (whole dir), six of the seven files under
#      .aurumcode/prompts/ (everything except documentation/welcome-page.md),
#      configs/ (whole dir), index.md and _config.yml AT THE REPOSITORY
#      ROOT, _api/ (whole dir), .github/actions/aurumcode-docs/ (whole dir,
#      including its README.md), pages-fix.md, test-jekyll.sh.
#   2. The one live exception, .aurumcode/prompts/documentation/welcome-page.md,
#      is present, non-empty, and really is the path
#      internal/documentation/welcome/generator.go's defaultPromptPath names
#      -- doc-vs-code, not doc-vs-doc.
#   3. The live rules catalog internal/review/rules/security.yml -- grepped
#      directly, never the deleted .aurumcode/rules/security.yml -- carries
#      the singular, patterned security/hardcoded-secret id and none of the
#      dead, patternless security/hardcoded-secrets (plural) id. This is the
#      divergence proof MUT-001 exploits.
#   4. scripts/action-entrypoint.sh, the Dockerfile's real ENTRYPOINT
#      target, is present and non-empty (the Dockerfile itself is outside
#      this card's read_paths, so the direct Dockerfile-vs-script binding is
#      host-run evidence recorded in docs/specs/AUR-456.md, not asserted
#      here).
#
# SCOPE
#   This never executes `aurumcode` or `regenerate-docs`; it is static
#   filesystem and text verification, matching the sandbox's no-network,
#   no-binary-execution constraint.
#
# ROOT OVERRIDE
#   AURUMCODE_ROOT, when set, is scanned instead of this script's own
#   repository checkout. tests/acceptance/AUR-456.sh's MUT-001 case uses this
#   to point the same, unmodified script at a scratch copy with the dead
#   .aurumcode/rules/security.yml reintroduced, proving the mutation is
#   caught without ever touching the tracked (already-deleted) path.
#
# AUR456_MUTATION
#   Set to "MUT-001" ONLY by the acceptance's MUT-001 case, alongside
#   AURUMCODE_ROOT pointing at its mutated scratch copy. It exists because
#   the reintroduced .aurumcode/rules/security.yml is byte-identical to the
#   pre-deletion tracked file -- content alone cannot tell a deliberate
#   MUT-001 run apart from this card's own pre-fix RED (where nothing has
#   been deleted yet). Without this marker set, a present dead copy always
#   reads as behavior-missing, including on the untouched pre-fix tree; only
#   the deliberate MUT-001 run sees the MUT-001 label.
set -euo pipefail
export LC_ALL=C
readonly card=AUR-456 scenario=AC-001
selector="${1:-E2EAUR456}"

fail() { printf '%s/%s/%s\n' "$card" "$scenario" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$scenario" "$1" >&2; exit 69; }

case "$selector" in
  E2EAUR456) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$scenario" >&2; exit 64 ;;
esac

if [[ -n "${AURUMCODE_ROOT:-}" ]]; then
  repo_root="$AURUMCODE_ROOT"
else
  script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
  repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
fi
[[ -d "$repo_root" ]] || infra "root_missing:$repo_root"

# dead[]: "path|mutatable(0/1)". Only .aurumcode/rules is mutatable: MUT-001
# is specifically "reintroduzir uma copia morta de regra" (the card's own
# words), so it is the only entry that can legitimately reappear under a
# recorded mutation marker.
dead=(
  ".aurumcode/rules|1"
  ".aurumcode/prompts/changelog-generation.md|0"
  ".aurumcode/prompts/review.md|0"
  ".aurumcode/prompts/documentation.md|0"
  ".aurumcode/prompts/test.md|0"
  ".aurumcode/prompts/summary.md|0"
  ".aurumcode/prompts/documentation-generation.md|0"
  "configs|0"
  "index.md|0"
  "_config.yml|0"
  "_api|0"
  ".github/actions/aurumcode-docs|0"
  "pages-fix.md|0"
  "test-jekyll.sh|0"
)

for entry in "${dead[@]}"; do
  IFS='|' read -r rel mutatable <<<"$entry"
  if [[ -e "$repo_root/$rel" || -L "$repo_root/$rel" ]]; then
    if [[ "$mutatable" == "1" && "${AUR456_MUTATION:-}" == "MUT-001" ]]; then
      fail "MUT-001: $rel was reintroduced"
    fi
    fail "behavior-missing:dead copy still present: $rel"
  fi
done

# The one live exception: present, non-empty, and grepped straight out of
# the generator source that reads it.
override="$repo_root/.aurumcode/prompts/documentation/welcome-page.md"
[[ -s "$override" ]] || fail 'behavior-missing:.aurumcode/prompts/documentation/welcome-page.md absent or empty'

generator="$repo_root/internal/documentation/welcome/generator.go"
[[ -s "$generator" ]] || fail 'behavior-missing:internal/documentation/welcome/generator.go absent'
grep -Fq 'defaultPromptPath = ".aurumcode/prompts/documentation/welcome-page.md"' "$generator" \
  || fail 'behavior-missing:generator.go no longer names the surviving override path'

# The live rules catalog: grep proves both directions of the divergence
# MUT-001 exploits -- the dead plural, patternless id must be absent from
# the LIVE file, and the live singular, patterned id must be present.
live_rules="$repo_root/internal/review/rules/security.yml"
[[ -s "$live_rules" ]] || fail 'behavior-missing:internal/review/rules/security.yml absent'
if grep -Eq '^\s*-\s*id:\s*security/hardcoded-secrets\s*$' "$live_rules"; then
  fail 'behavior-missing:internal/review/rules/security.yml regressed to the dead plural id security/hardcoded-secrets'
fi
grep -Eq '^\s*-\s*id:\s*security/hardcoded-secret\s*$' "$live_rules" \
  || fail 'behavior-missing:internal/review/rules/security.yml does not declare the live singular id security/hardcoded-secret'
# pattern: must appear in the same rule block (between this id and the next
# "- id:" line, or end of file).
awk '
  /^[[:space:]]*-[[:space:]]*id:[[:space:]]*security\/hardcoded-secret[[:space:]]*$/ { seen=1; next }
  seen && /^[[:space:]]*-[[:space:]]*id:/ { exit }
  seen && /^[[:space:]]*pattern:[[:space:]]*[^[:space:]]/ { found=1; exit }
  END { exit(found ? 0 : 1) }
' "$live_rules" || fail 'behavior-missing:security/hardcoded-secret rule carries no pattern: key'

# The Dockerfile's real ENTRYPOINT target (Dockerfile itself is outside
# read_paths; see docs/specs/AUR-456.md for the host-run binding proof).
entrypoint="$repo_root/scripts/action-entrypoint.sh"
[[ -s "$entrypoint" ]] || fail 'behavior-missing:scripts/action-entrypoint.sh absent or empty'

printf '{"card":"%s","scenario":"%s","layer":"e2e","result":"pass","dead_paths_checked":%d,"live_kept":2}\n' \
  "$card" "$scenario" "${#dead[@]}"
