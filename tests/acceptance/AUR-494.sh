#!/usr/bin/env bash
#
# Acceptance program for card AUR-494: a new review round of the same pull
# request reuses the findings' identity, does not repeat comments and does
# not keep blocking a fixed bug. See docs/specs/AUR-494.md.
#
# SELECTORS
#   all      every selector below
#   AC-001   two real --pr rounds over the same diff (fake GitHub that keeps
#            the conversation): the second posts zero comments, says so, and
#            keeps the same review event
#   AC-002   moving the same code keeps the identity (no new comment); a
#            second defect on the same line is commented
#   AC-003   a person's reply reaches the prompt but does not switch the
#            rule off; fixing the code removes REQUEST_CHANGES and the body
#            records the resolution
#   AC-004   a changed model input finds a new defect, commented without
#            repeating the equivalent earlier one
#   MUT-001  disabling the identity check (every finding is posted) turns
#            AC-001 red with a duplicated comment
#
# EXIT CODES: 0 pass, 1 behavioral RED, 64 unknown selector, 79 infrastructure.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-494'
selector="${1:-all}"
case "$selector" in
  all|AC-001|AC-002|AC-003|AC-004|MUT-001)
    ;;
  *)
    printf '%s/%s/unknown-selector\n' "$card" "$selector" >&2
    exit 64
    ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$selector" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$selector" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go

for input in go.mod go.sum cmd internal pkg docs/specs/AUR-494.md docs/review-quality.md; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a494.XXXXXX")" || infra mktemp
trap 'chmod -R u+w -- "$run_dir" >/dev/null 2>&1 || true; rm -rf -- "$run_dir" >/dev/null 2>&1 || true' EXIT INT TERM HUP
mkdir -p "$run_dir/gocache" "$run_dir/gotmp"
: "${GOCACHE:=$run_dir/gocache}"
: "${GOTMPDIR:=$run_dir/gotmp}"
export GOCACHE GOTMPDIR GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOFLAGS=-mod=mod

# stage copies the module into a fresh root so a mutation never touches the
# checkout.
stage() {
  local root="$1" top
  mkdir -p "$root"
  for top in go.mod go.sum cmd internal pkg; do
    cp -R "$repo_root/$top" "$root/$top"
  done
  chmod -R u+w -- "$root"
}

# go_test_v runs the named tests of pkg in root verbosely and requires each
# to PASS.
go_test_v() {
  local root="$1" pkg="$2" pattern="$3" label="$4" name
  (cd "$root" && go test -buildvcs=false -count=1 -p 1 -v -run "$pattern" "$pkg") >"$run_dir/gotest.out" 2>&1 || { cat "$run_dir/gotest.out" >&2; fail "$label"; }
  for name in ${pattern//|/ }; do
    grep -q "^--- PASS: $name " "$run_dir/gotest.out" || fail "not-run:$name"
  done
}

run_scenario() {
  local id="$1" cmd_tests="$2" unit_tests="$3"
  stage "$run_dir/$id"
  go_test_v "$run_dir/$id" ./cmd/aurumcode "$cmd_tests" "$id-rounds"
  if [[ -n "$unit_tests" ]]; then
    go_test_v "$run_dir/$id" ./internal/review/rounds "$unit_tests" "$id-unit"
  fi
}

run_mut001() {
  local root="$run_dir/mut001" target anchor
  stage "$root"
  target="$root/cmd/aurumcode/pr_rounds.go"
  anchor='return i >= len(r.plan.Post) || r.plan.Post[i]'
  grep -Fq "$anchor" "$target" || infra mut001-anchor
  sed -i 's/return i >= len(r.plan.Post) || r.plan.Post\[i\]/return true/' "$target"
  if grep -Fq "$anchor" "$target"; then infra mut001-not-applied; fi
  set +e
  (cd "$root" && go test -buildvcs=false -count=1 -p 1 -run 'TestAUR494AC001SameDiffTwiceNoDuplicateComment' ./cmd/aurumcode) >"$run_dir/mut.out" 2>&1
  local rc=$?
  set -e
  [[ "$rc" -ne 0 ]] || fail mutant-survived
  if grep -Eq 'build failed|undefined:|syntax error|cannot use' "$run_dir/mut.out"; then
    cat "$run_dir/mut.out" >&2
    fail mutant-build-failure
  fi
  grep -Fq 'AUR-494 duplicate comment' "$run_dir/mut.out" || { cat "$run_dir/mut.out" >&2; fail mutant-other-cause; }
  printf '%s/MUT-001/red (a segunda rodada repetiu o comentario)\n' "$card"
}

ac001() { run_scenario AC-001 'TestAUR494AC001SameDiffTwiceNoDuplicateComment' 'TestMarkerRoundTrip|TestPlanRoundCountsRepeatsAndResolved'; }
ac002() { run_scenario AC-002 'TestAUR494AC002MovedCodeKeepsIdentityNewDefectStays' ''; }
ac003() { run_scenario AC-003 'TestAUR494AC003FixRemovesBlockAndTextDisablesNothing|TestAUR494ForgedMarkerFromAnotherAuthorIsIgnored|TestAUR494InconclusiveRunNamesNothingResolved' 'TestPublishedIgnoresReplies|TestPlanRoundCondensedIsNeverResolved'; }
ac004() { run_scenario AC-004 'TestAUR494AC004NewContextFindsNewDefectWithoutRepeating' ''; }

case "$selector" in
  AC-001)
    ac001
    ;;
  AC-002)
    ac002
    ;;
  AC-003)
    ac003
    ;;
  AC-004)
    ac004
    ;;
  MUT-001)
    run_mut001
    ;;
  all)
    ac001
    ac002
    ac003
    ac004
    run_mut001
    ;;
esac
printf '%s/%s/pass\n' "$card" "$selector"
