#!/usr/bin/env bash
#
# Acceptance program for card AUR-547.
#
# WHAT THIS PROVES
#
#   tests/acceptance/AUR-443.sh, AUR-448.sh, AUR-449.sh, AUR-458.sh,
#   AUR-466.sh and AUR-481.sh (all done cards) still materialized
#   cmd/regenerate-docs, internal/pipeline and internal/documentation/* --
#   removed from the product by commit 670c7f6 ("Focus AurumCode on code
#   review", 2026-09-12) -- and stopped as infrastructure before proving
#   anything. This card fixed each script's materialization against
#   `go list -deps ./cmd/aurumcode`'s real answer, realigned what AUR-490
#   (and, for AUR-449, also AUR-458) revoked, and left genuinely out-of-path
#   RED causes documented in docs/specs/AUR-547.md rather than masked.
#
# SELECTORS
#   all             run every scenario below, including the MUT
#   AC-001          every one of the six scripts either exits 0 (fully
#                   fixed) or exits with the exact measured RED tag this
#                   card documented in docs/specs/AUR-547.md; AND every
#                   other selector each script exposes (unit/integration/
#                   e2e/mutation) is run on its own and checked against
#                   the exact rc and tag docs/specs/AUR-547.md pins --
#                   so a claimed partial green is proved, not inferred
#   AC-002          none of the six scripts' stage_source/required_inputs
#                   copies a package this product no longer has
#   AC-003          tests/unit/AUR-449.go and tests/integration/AUR-449.go
#                   no longer assert the pre-AUR-490 rule ("quality review
#                   skipped" never appears without --seguranca); proved by
#                   both the absence of the old assertion text and a live
#                   run of TestAUR449/IntegrationAUR449 through
#                   tests/acceptance/AUR-449.sh
#   AC-002-MUT-001  reintroducing a removed package copy into a staged copy
#                   of an acceptance script's stage_source makes it fail as
#                   infrastructure (exit 79), never a behavioral verdict
#
# EXIT CODES (tests/acceptance/EXIT_CODE_CONVENTION.md):
#   0  = the promised property holds
#   1  = behavioral RED (including a surviving mutation)
#   64 = unknown selector
#   79 = inconclusive / infrastructure. Never valid red evidence.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-547'
selector="${1:-all}"

case "$selector" in
  all|AC-001|AC-002|AC-003|AC-002-MUT-001) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$selector" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$selector" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$selector" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go

readonly -a scripts=(AUR-443 AUR-448 AUR-449 AUR-458 AUR-466 AUR-481)

required_inputs=(go.mod go.sum cmd/aurumcode docs/specs/AUR-547.md
  tests/integration/AUR-449.go tests/unit/AUR-449.go)
for name in "${scripts[@]}"; do
  required_inputs+=("tests/acceptance/$name.sh")
done
for input in "${required_inputs[@]}"; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a547.XXXXXX")" || infra mktemp
cleanup_root() { chmod -R u+w -- "$1" >/dev/null 2>&1 || true; rm -rf -- "$1" >/dev/null 2>&1 || true; }
trap 'cleanup_root "$run_dir"' EXIT INT TERM HUP

mkdir -p "$run_dir/gocache" "$run_dir/gotmp"
export GOCACHE="$run_dir/gocache" GOTMPDIR="$run_dir/gotmp"
export GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOFLAGS='-mod=mod -p=1'
export TMPDIR="$run_dir"

# run_nested runs one tests/acceptance/<name>.sh with the given selector,
# from repo_root, exactly as any caller would. Globals n_rc/n_out carry the
# result -- never read a function's own exit status through a `$(...)`
# command substitution (that subshell would swallow it).
run_nested() {
  local name="$1" sel="$2"
  n_out="$run_dir/nested-$name-$sel.out"
  set +e
  ( cd "$repo_root" && bash "tests/acceptance/$name.sh" "$sel" ) >"$n_out" 2>&1
  n_rc=$?
  set -e
}

# AC-001: AUR-449 is fully fixed (exits 0); the other five are fixed at the
# materialization and acceptance-shell level but stay RED for a measured,
# out-of-path cause (a sibling tests/unit/*.go, tests/integration/*.go or
# tests/e2e/*.sh this card's paths do not include) -- recorded here as the
# exact last-line tag docs/specs/AUR-547.md documents for each. A different
# red cause, or an unexpected green, both fail this scenario: either means
# the documented spec no longer describes the real, current state.
expected_tag() {
  case "$1" in
    AUR-443) printf 'selector:TestAUR443:exit:1' ;;
    AUR-448) printf 's3-grounded-finding-missing' ;;
    AUR-458) printf 'e2e-failed:AUR-458/E2E/pr-path-must-refuse-the-flag:want-2-got-1' ;;
    AUR-466) printf 'selector:IntegrationAUR466:exit:1' ;;
    AUR-481) printf 'selector:IntegrationAUR481:exit:1' ;;
  esac
}

# detail_checks pins, per script, the exact outcome of every OTHER
# selector that script exposes -- not just the coarse AC-001 tag --
# so a claimed partial green (e.g. AUR-443's e2e_case/mutation_case, or
# AUR-466/481's e2e_case) is actually proved by running it, and a claimed
# red's inner cause (unit/integration) is pinned precisely enough that a
# DIFFERENT red cause is distinguishable from the one measured here (B3,
# independent review of this card).
#  script   selector            expected_rc  expected_tag_substring
readonly -a detail_checks=(
  'AUR-443|TestAUR443|1|selector:TestAUR443:exit:1'
  'AUR-443|IntegrationAUR443|1|selector:IntegrationAUR443:exit:1'
  'AUR-443|E2EAUR443|0|E2EAUR443/ok'
  'AUR-443|AC-001-MUT-001|0|MUT-001/rejected'
  'AUR-448|TestAUR448|1|selector:TestAUR448:exit:1'
  'AUR-448|IntegrationAUR448|1|selector:IntegrationAUR448:exit:1'
  'AUR-448|E2EAUR448|0|E2EAUR448/ok'
  'AUR-448|AC-001-MUT-001|1|MUT-001/not-rejected'
  'AUR-466|IntegrationAUR466|1|selector:IntegrationAUR466:exit:1'
  'AUR-466|E2EAUR466|0|e2e-ok'
  'AUR-481|IntegrationAUR481|1|selector:IntegrationAUR481:exit:1'
  'AUR-481|E2EAUR481|0|e2e-ok'
)

# has_git is true only when a git BINARY is on PATH. The sealed
# go-unit-offline-v1 profile's own image has none (measured directly: its
# own tests/integration/AUR-443.go subtest "git-binary backend" prints
# "no git binary on PATH in this environment: skipping..."), while the
# go-shared dev container this card was built against does. That one
# difference changes TestAUR443IntegrationBridge's own outcome (it
# compares pure-Go vs git-binary output; with no git binary the
# git-binary-backend half of the comparison -- and the subtest this card
# measured RED against, "a_valid_ref_still_resolves_and_reviews_normally_
# on_both_backends" -- never runs the branch that hits the AUR-490 stdout
# mismatch), so IntegrationAUR443 is GREEN under the sealed profile and
# RED under go-shared. Both are real, both are out of this card's paths
# (tests/integration/AUR-443.go) either way -- this just keeps the pin
# honest across the two environments this card is run in, documented in
# docs/specs/AUR-547.md.
has_git() { command -v git >/dev/null 2>&1; }

run_detail_checks() {
  local any_bad=0 entry name sel want_rc want_tag last
  for entry in "${detail_checks[@]}"; do
    IFS='|' read -r name sel want_rc want_tag <<<"$entry"
    if [[ "$name/$sel" == 'AUR-443/IntegrationAUR443' ]] && ! has_git; then
      want_rc=0; want_tag='ok'
    fi
    run_nested "$name" "$sel"
    if [[ "$n_rc" -eq 79 || "$n_rc" -eq 69 ]]; then
      cat "$n_out" >&2
      infra "detail-infra:$name:$sel:$n_rc"
    fi
    if [[ "$n_rc" != "$want_rc" ]]; then
      cat "$n_out" >&2
      printf '%s/%s/detail-wrong-exit:%s:%s:want:%s:got:%s\n' \
        "$card" "$selector" "$name" "$sel" "$want_rc" "$n_rc" >&2
      any_bad=1
      continue
    fi
    last="$(tail -n1 "$n_out")"
    if ! grep -Fq "$want_tag" <<<"$last"; then
      cat "$n_out" >&2
      printf '%s/%s/detail-different-cause:%s:%s:want:%s:got:%s\n' \
        "$card" "$selector" "$name" "$sel" "$want_tag" "$last" >&2
      any_bad=1
    fi
  done
  [[ "$any_bad" -eq 0 ]]
}

run_ac001() {
  local any_bad=0 name last
  for name in "${scripts[@]}"; do
    run_nested "$name" AC-001
    if [[ "$name" == AUR-449 ]]; then
      if [[ "$n_rc" -ne 0 ]]; then
        cat "$n_out" >&2
        printf '%s/%s/AUR-449-regressed:%s\n' "$card" "$selector" "$n_rc" >&2
        any_bad=1
      fi
      continue
    fi
    if [[ "$n_rc" -eq 0 ]]; then
      cat "$n_out" >&2
      printf '%s/%s/unexpectedly-fixed:%s\n' "$card" "$selector" "$name" >&2
      any_bad=1
      continue
    fi
    if [[ "$n_rc" -eq 79 || "$n_rc" -eq 69 ]]; then
      cat "$n_out" >&2
      infra "ac001-infra:$name:$n_rc"
    fi
    last="$(tail -n1 "$n_out")"
    if ! grep -Fq "$(expected_tag "$name")" <<<"$last"; then
      cat "$n_out" >&2
      printf '%s/%s/different-red-cause:%s:%s\n' "$card" "$selector" "$name" "$last" >&2
      any_bad=1
    fi
  done
  [[ "$any_bad" -eq 0 ]]
}

# AC-002: none of the six scripts' stage_source/required_inputs copies a
# package this product no longer has.
run_ac002() {
  local any_bad=0 name hit
  for name in "${scripts[@]}"; do
    hit="$(grep -v '^[[:space:]]*#' "$repo_root/tests/acceptance/$name.sh" | \
      grep -En 'cmd/regenerate-docs|internal/pipeline|internal/documentation' || true)"
    if [[ -n "$hit" ]]; then
      printf '%s/%s/removed-package-still-copied:%s:%s\n' "$card" "$selector" "$name" "$hit" >&2
      any_bad=1
    fi
  done
  [[ "$any_bad" -eq 0 ]]
}

# AC-003: tests/unit/AUR-449.go and tests/integration/AUR-449.go no longer
# assert the pre-AUR-490 rule. Checked two ways: the exact old assertion
# text is gone (static), and the live selectors still pass through the
# real binary (behavioral) -- a static-only check could pass vacuously if
# the assertion were deleted rather than corrected.
run_ac003() {
  local any_bad=0
  if grep -Fq 'the AUR-449 skip note must never appear without --seguranca' \
    "$repo_root/tests/unit/AUR-449.go" "$repo_root/tests/integration/AUR-449.go"; then
    printf '%s/%s/old-rule-still-asserted\n' "$card" "$selector" >&2
    any_bad=1
  fi
  grep -Fq 'quality review skipped' "$repo_root/tests/unit/AUR-449.go" || {
    printf '%s/%s/new-rule-missing:unit\n' "$card" "$selector" >&2; any_bad=1
  }
  grep -Fq 'quality review skipped' "$repo_root/tests/integration/AUR-449.go" || {
    printf '%s/%s/new-rule-missing:integration\n' "$card" "$selector" >&2; any_bad=1
  }

  run_nested AUR-449 TestAUR449
  if [[ "$n_rc" -eq 79 || "$n_rc" -eq 69 ]]; then cat "$n_out" >&2; infra 'ac003-infra:unit'; fi
  if [[ "$n_rc" -ne 0 ]]; then cat "$n_out" >&2; printf '%s/%s/unit-selector-failed:%s\n' "$card" "$selector" "$n_rc" >&2; any_bad=1; fi

  run_nested AUR-449 IntegrationAUR449
  if [[ "$n_rc" -eq 79 || "$n_rc" -eq 69 ]]; then cat "$n_out" >&2; infra 'ac003-infra:integration'; fi
  if [[ "$n_rc" -ne 0 ]]; then cat "$n_out" >&2; printf '%s/%s/integration-selector-failed:%s\n' "$card" "$selector" "$n_rc" >&2; any_bad=1; fi

  [[ "$any_bad" -eq 0 ]]
}

# AC-002-MUT-001: stage a WRITABLE copy of tests/acceptance/AUR-449.sh
# (never the committed file) into a scratch root, reintroduce a removed
# package copy line into its stage_source(), and run it. The copy() helper
# every one of these scripts shares calls this file's own infra() the
# instant a materialization source does not exist on disk -- exactly what
# happens the moment cmd/regenerate-docs is asked for again, since the
# product does not have it any more. A surviving mutant (anything but exit
# 79) means a future copy-paste of a removed package would silently stop
# being caught as an environment gap.
run_mut001() {
  local root="$run_dir/root-mut001"
  # tests/acceptance/AUR-443.sh resolves its OWN repo_root from $0's
  # location (two directories up), so the mutated copy has to sit at the
  # same relative depth as the real file, not loose in a flat scratch
  # dir. Everything it might read is symlinked from the real tree (cheap,
  # no copying -- the mutated script's own stage_source() does the actual
  # cp -R'ing into a further-nested scratch root of its own); only
  # tests/acceptance is a REAL directory, holding just the mutated file,
  # so this never writes into the real tests/acceptance/. AUR-443.sh is
  # chosen because its copy() helper raises infra() itself the instant a
  # source path does not exist (AUR-449.sh's copy() has no such guard, so
  # a missing cp source there would surface as a raw shell errexit, not
  # this card's own infra()).
  mkdir -p "$root/tests" "$root/tests/acceptance"
  local top
  for top in go.mod go.sum cmd internal pkg docs action.yml; do
    [[ -e "$repo_root/$top" ]] && ln -s "$repo_root/$top" "$root/$top"
  done
  for top in unit integration e2e fixtures; do
    [[ -e "$repo_root/tests/$top" ]] && ln -s "$repo_root/tests/$top" "$root/tests/$top"
  done
  cp "$repo_root/tests/acceptance/AUR-443.sh" "$root/tests/acceptance/AUR-443.sh"
  chmod u+w -- "$root/tests/acceptance/AUR-443.sh"

  grep -Fxq '  copy "$root" pkg' "$root/tests/acceptance/AUR-443.sh" || infra 'MUT-001/anchor-absent'
  sed -i 's#^  copy "\$root" pkg$#  copy "$root" cmd/regenerate-docs\n  copy "$root" pkg#' "$root/tests/acceptance/AUR-443.sh"
  grep -Fq 'copy "$root" cmd/regenerate-docs' "$root/tests/acceptance/AUR-443.sh" || infra 'MUT-001/mutation-not-applied'

  local out="$run_dir/mut001.out" rc
  set +e
  ( cd "$root" && bash "tests/acceptance/AUR-443.sh" AC-001 ) >"$out" 2>&1
  rc=$?
  set -e

  if [[ "$rc" -eq 0 ]]; then
    cat "$out" >&2
    printf '%s/%s/MUT-001/mutation-survived-as-pass\n' "$card" "$selector" >&2
    return 1
  fi
  if [[ "$rc" -ne 79 ]]; then
    cat "$out" >&2
    printf '%s/%s/MUT-001/mutation-survived-as-behavioral:%s\n' "$card" "$selector" "$rc" >&2
    return 1
  fi
  grep -Fq 'missing_input:cmd/regenerate-docs' "$out" || {
    cat "$out" >&2
    printf '%s/%s/MUT-001/wrong-infra-cause\n' "$card" "$selector" >&2
    return 1
  }
  return 0
}

case "$selector" in
  AC-001)
    run_ac001 || fail AC-001
    run_detail_checks || fail AC-001
    ;;
  AC-002) run_ac002 || fail AC-002 ;;
  AC-003) run_ac003 || fail AC-003 ;;
  AC-002-MUT-001) run_mut001 || fail AC-002-MUT-001 ;;
  all)
    run_ac001 || fail AC-001
    run_detail_checks || fail AC-001
    run_ac002 || fail AC-002
    run_ac003 || fail AC-003
    run_mut001 || fail AC-002-MUT-001
    ;;
esac

printf '%s/%s/pass\n' "$card" "$selector"
exit 0
