#!/usr/bin/env bash
# AUR-455 E2E: actually EXECUTE demo.sh -- the card's own requirement is
# that the acceptance runs the script and checks it passes, not merely
# that the file exists (see .board/cards/ready/AUR-455.md's closing
# paragraph before "## Non-goals").
#
# This is deliberately heavier than a sibling card's E2E layer
# (tests/e2e/AUR-445.sh is pure static text/source verification): AUR-455's
# whole point is that a new developer's FIRST command actually works, so
# the proof has to run it. demo.sh builds ./cmd/regenerate-docs and
# ./cmd/aurumcode once each and then exercises all three features against
# committed, deterministic fixtures -- no network, no LLM key, no git
# binary needed (the pure-Go loose-object reader handles
# tests/fixtures/repos/git-demo).
#
# ROOT OVERRIDE
#   AURUMCODE_ROOT, when set, points this script at a staged copy instead
#   of its own checkout. tests/acceptance/AUR-455.sh's MUT-001 case uses
#   this to run demo.sh against a scratch tree with a documented flag
#   broken, proving the demo actually stops running -- without ever
#   touching the tracked demo.sh or cmd/aurumcode/main.go.
#
# RESOURCE ENVIRONMENT
#   Building two Go binaries needs a writable GOCACHE/GOTMPDIR/HOME (the
#   acceptance sandbox's rootfs is read-only) and, under the sandbox's
#   256MB memory cap, GOFLAGS=-p=1 plus GOMAXPROCS=1 -- without them the Go
#   compiler's default parallelism gets OOM-killed (measured directly
#   against this card's own bootstrap-readonly-v1 image during
#   construction; see docs/specs/AUR-455.md). This script does not set
#   those itself: tests/acceptance/AUR-455.sh exports them once for every
#   lane, matching go_lane's technique in every sibling card in this
#   office. A developer running this script directly, outside the sandbox,
#   keeps their own GOCACHE and default parallelism.
set -euo pipefail
export LC_ALL=C
readonly card=AUR-455 scenario=AC-001
selector="${1:-E2EAUR455}"

fail() { printf '%s/%s/%s\n' "$card" "$scenario" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$scenario" "$1" >&2; exit 69; }

case "$selector" in
  E2EAUR455) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$scenario" >&2; exit 64 ;;
esac

if [[ -n "${AURUMCODE_ROOT:-}" ]]; then
  repo_root="$AURUMCODE_ROOT"
else
  script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
  repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
fi
[[ -d "$repo_root" ]] || infra "root_missing:$repo_root"
command -v go >/dev/null 2>&1 || infra missing_go

demo_path="$repo_root/demo.sh"
[[ -f "$demo_path" ]] || fail 'behavior-missing:demo.sh absent'
[[ -x "$demo_path" ]] || fail 'behavior-missing:demo.sh is not executable'
[[ ! -L "$demo_path" ]] || fail 'behavior-missing:demo.sh is a symlink, refusing to execute it as one'

set +e
demo_output="$(cd "$repo_root" && bash "$demo_path" 2>&1)"
demo_rc=$?
set -e

if (( demo_rc != 0 )); then
  # A deliberately mutated documented command (MUT-001) makes demo.sh exit
  # non-zero. Content alone (a nonzero exit) cannot distinguish that from
  # this card's own pre-fix RED (demo.sh absent, caught above, or a real
  # regression) -- only the caller can, via AUR455_MUTATION, exactly
  # mirroring tests/e2e/AUR-445.sh's AUR445_MUTATION convention.
  if [[ "${AUR455_MUTATION:-}" == "MUT-001" ]]; then
    printf '%s\n' "$demo_output" >&2
    fail "MUT-001: demo.sh exited $demo_rc against the mutated tree"
  fi
  printf '%s\n' "$demo_output" >&2
  fail "behavior-missing:demo.sh exited $demo_rc"
fi

# The four labeled steps' documented markers must actually be in the
# captured output -- an exit-0 script that silently skipped a step would
# still pass a bare `(( rc == 0 ))` check.
declare -A checks=(
  [step-a-result-ok]='result=ok'
  [step-b-seguranca-ran]='security pass applied'
  [step-b-finding]='config/demo-tokens.txt:'
  [step-c-fixture-finding]='A credential-shaped value was committed in plain text (DEMO_API_TOKEN)'
  [step-d-jekyll-line]='bundle install && bundle exec jekyll build'
)
for label in "${!checks[@]}"; do
  needle="${checks[$label]}"
  if [[ "$demo_output" != *"$needle"* ]]; then
    fail "behavior-missing:demo.sh output never contained the $label marker ($needle)"
  fi
done

printf '{"card":"%s","scenario":"%s","layer":"e2e","result":"pass","exit_code":%d,"markers_checked":%d}\n' \
  "$card" "$scenario" "$demo_rc" "${#checks[@]}"
