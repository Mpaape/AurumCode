#!/usr/bin/env bash
#
# Acceptance program for card AUR-557 (gate as one pipeline of contributors
# shared by --base and --pr; a refactor with no behavior change).
#
# SELECTORS
#   all             AC-002, AC-003, AC-004, AC-002-MUT-001 and AC-001 over the
#                   representative subset of the existing acceptances
#   AC-001-full     AC-001 over EVERY existing acceptance the contract lists
#   AC-001          the existing acceptances' own `all` selectors, run
#                   unedited, against the refactored code (the subset named
#                   in sub_acceptances_all)
#   AC-002          --base and --pr execute the same declared pipeline
#                   (cmd/aurumcode TestAUR557PathsShareOnePipeline)
#   AC-003          a contributor that fails leaves the result inconclusive
#                   by the policy's mode, never approved (internal/gate)
#   AC-004          no function of cmd/aurumcode exceeds 150 lines (go/ast)
#   AC-002-MUT-001  removing one contributor only from the --pr path turns
#                   AC-002 red AND turns AUR-550's own acceptance red
#
# GOCACHE: every nested acceptance sets its own GOCACHE under its own
# temporary directory, so run alone each would rebuild the module cold. A
# `go` shim placed first on PATH pins one shared GOCACHE for all of them
# (the card's "shared GOCACHE" rule) without editing any nested script.
#
# EXIT CODES (tests/acceptance/EXIT_CODE_CONVENTION.md):
#   0 = holds, 1 = behavioral RED, 64 = unknown selector, 79 = infrastructure
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-557'
selector="${1:-all}"

case "$selector" in
  all|AC-001|AC-001-full|AC-002|AC-003|AC-004|AC-002-MUT-001) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$selector" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$selector" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$selector" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
real_go="$(command -v go 2>/dev/null)" || infra missing_go

# The acceptances the contract names. AUR-553 has no acceptance program in
# this repository (it is not in tests/acceptance); it is reported, not run.
readonly -a all_sub_acceptances=(518 519 520 521 522 524 533 537 538 543 548 549 550 551 552 555 556 559)
# `all` runs the subset that carries the source-anchored mutations and the
# gate wiring (measured: the 15 together take ~490 s, past the 450 s margin
# the sealed profile's 600 s limit allows for `all`); AC-001-full runs all 15.
readonly -a sub_acceptances_all=(519 537 538 543 524 548 550 556 522 533)

for input in go.mod go.sum cmd internal pkg internal/gate cmd/aurumcode/structure_test.go docs/specs/AUR-557.md; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done
for n in "${all_sub_acceptances[@]}"; do
  [[ -f "$repo_root/tests/acceptance/AUR-$n.sh" ]] || infra "missing-acceptance:AUR-$n"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a557.XXXXXX")" || infra mktemp
cleanup_root() { chmod -R u+w -- "$1" >/dev/null 2>&1 || true; rm -rf -- "$1" >/dev/null 2>&1 || true; }
trap 'cleanup_root "$run_dir"' EXIT INT TERM HUP

mkdir -p "$run_dir/gocache" "$run_dir/gotmp" "$run_dir/shim"
export GOCACHE="$run_dir/gocache" GOTMPDIR="$run_dir/gotmp" TMPDIR="$run_dir"
export GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOWORK=off GOENV=off
export GOFLAGS='-mod=mod -p=1' GOMEMLIMIT=2GiB GOMAXPROCS=1

# The shim: every `go` any nested acceptance runs uses this one GOCACHE.
printf '#!/bin/sh\nexport GOCACHE=%s\nexec %s "$@"\n' "$run_dir/gocache" "$real_go" >"$run_dir/shim/go"
chmod +x "$run_dir/shim/go"
shared_path="$run_dir/shim:$PATH"

# The sealed profile's /tmp is a 512 MB tmpfs. A cache shared by scripts that
# each build mutated variants of the package outgrows it, so: warm the cache
# once with the unmodified module, mark it, and drop every entry created
# after the mark when a nested script (or the mutation copy) is done. The
# warm entries (standard library, dependencies, the unmodified packages) stay
# shared; only variant builds are discarded.
cache_mark="$run_dir/cache.mark"
warm_cache() {
  (cd "$repo_root" && go test -mod=mod -p 1 -count=1 -run '^$' ./cmd/aurumcode/ >"$run_dir/warm.log" 2>&1) || { cat "$run_dir/warm.log" >&2; infra warm-cache; }
  : >"$cache_mark"
}
prune_cache() { find "$run_dir/gocache" -type f -newer "$cache_mark" -delete 2>/dev/null || true; }

# go_test <root> <log> <pattern> <pkg...>: run go test -v; the log is the evidence.
go_test() {
  local root="$1" log="$2" pattern="$3"; shift 3
  set +e
  (cd "$root" && go test -mod=mod -p 1 -count=1 -timeout 300s -v -run "$pattern" "$@") >"$log" 2>&1
  local status=$?
  set -e
  return $status
}

require_pass() {
  local log="$1"; shift
  local name
  for name in "$@"; do
    grep -q "^--- PASS: $name " "$log" || { cat "$log" >&2; fail "missing-pass:$name"; }
  done
}

ac002() {
  local log="$run_dir/ac002.log"
  go_test "$repo_root" "$log" '^TestAUR557PathsShareOnePipeline$' ./cmd/aurumcode/ || { cat "$log" >&2; fail go-test-failed; }
  require_pass "$log" TestAUR557PathsShareOnePipeline
}

ac003() {
  local log="$run_dir/ac003.log"
  go_test "$repo_root" "$log" '^(TestPipelineAppliesContributorsInDeclaredOrder|TestPipelineContributorErrorIsInconclusiveNeverApproved|TestPipelineFatalErrorAborts|TestResultMergeJoinsReasonsAndIgnoresInactive)$' ./internal/gate/ || { cat "$log" >&2; fail go-test-failed; }
  require_pass "$log" TestPipelineAppliesContributorsInDeclaredOrder TestPipelineContributorErrorIsInconclusiveNeverApproved TestPipelineFatalErrorAborts TestResultMergeJoinsReasonsAndIgnoresInactive
}

ac004() {
  local log="$run_dir/ac004.log"
  go_test "$repo_root" "$log" '^(TestAUR557NoFunctionExceedsLineLimit|TestAUR557EntryPointsAreShort)$' ./cmd/aurumcode/ || { cat "$log" >&2; fail go-test-failed; }
  require_pass "$log" TestAUR557NoFunctionExceedsLineLimit TestAUR557EntryPointsAreShort
}

# run_nested <root> <AUR-nnn> <selector>: n_rc/n_out carry the result.
run_nested() {
  local root="$1" name="$2" sel="$3"
  n_out="$run_dir/nested-$name-$sel.out"
  set +e
  ( cd "$root" && PATH="$shared_path" bash "tests/acceptance/$name.sh" "$sel" ) >"$n_out" 2>&1
  n_rc=$?
  set -e
  prune_cache
}

ac001() {
  local n bad=0
  for n in "$@"; do
    run_nested "$repo_root" "AUR-$n" all
    if (( n_rc != 0 )); then
      cat "$n_out" >&2
      printf '%s/%s/nested-red:AUR-%s:exit:%s\n' "$card" "$selector" "$n" "$n_rc" >&2
      bad=1
    else
      printf '%s/%s/nested-green:AUR-%s\n' "$card" "$selector" "$n" >&2
    fi
  done
  (( bad == 0 )) || exit 1
}

# MUT-001: in a copy, the --pr path alone drops the last contributor of the
# pipeline it executes. AC-002 must go red, and so must AUR-550's own
# acceptance (the dropped contributor is the Dependency-Track one).
ac002_mut001() {
  local root="$run_dir/mut"
  mkdir -p "$root/tests"
  cp -R "$repo_root/go.mod" "$repo_root/go.sum" "$repo_root/cmd" "$repo_root/internal" "$repo_root/pkg" "$root/" 2>/dev/null || true
  cp -R "$repo_root/tests/acceptance" "$root/tests/acceptance" 2>/dev/null || true
  chmod -R u+w "$root"
  # AUR-576: both paths run the session's one gate phase (review_gate.go);
  # the mutation drops the contributor for the --pr source only.
  local target="$root/cmd/aurumcode/review_gate.go"
  local anchor='res, ok := s.executeGate(pipeline, reason)'
  [[ "$(grep -Fc "$anchor" "$target")" == 1 ]] || infra mutation-anchor-missing
  sed -i "s|${anchor}|if s.source.Label == \"--pr\" { pipeline = gate.NewPipeline(pipeline.Contributors()[:len(pipeline.Contributors())-1]...) }; res, ok := s.executeGate(pipeline, reason)|" "$target"
  grep -Fq 'if s.source.Label == "--pr" { pipeline = gate.NewPipeline(' "$target" || infra mutation-not-applied

  local log="$run_dir/mut-ac002.log"
  local survived=0
  go_test "$root" "$log" '^TestAUR557PathsShareOnePipeline$' ./cmd/aurumcode/ && survived=1
  prune_cache
  (( survived == 0 )) || { cat "$log" >&2; fail 'mutation-survived:ac-002'; }
  grep -Eq -- '^--- FAIL: TestAUR557PathsShareOnePipeline' "$log" || { cat "$log" >&2; fail 'mutation-not-behavioral:ac-002'; }

  run_nested "$root" AUR-550 all
  if (( n_rc == 0 )); then cat "$n_out" >&2; fail 'mutation-survived:AUR-550'; fi
  if (( n_rc == 79 || n_rc == 69 )); then cat "$n_out" >&2; infra "mutation-infra:AUR-550:$n_rc"; fi
  rm -rf "$root"
}

case "$selector" in
  AC-001)       warm_cache; ac001 "${sub_acceptances_all[@]}" ;;
  AC-001-full)  warm_cache; ac001 "${all_sub_acceptances[@]}" ;;
  AC-002)       ac002 ;;
  AC-003)       ac003 ;;
  AC-004)       ac004 ;;
  AC-002-MUT-001) warm_cache; ac002_mut001 ;;
  all)
    warm_cache
    ac002; ac003; ac004; ac002_mut001
    ac001 "${sub_acceptances_all[@]}"
    ;;
esac
printf '%s/%s/pass\n' "$card" "$selector"
