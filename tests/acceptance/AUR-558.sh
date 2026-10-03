#!/usr/bin/env bash
# AUR-558 -- cmd/aurumcode organised by responsibility, with a structural guard.
#
# Selectors:
#   AC-001          no production file of cmd/aurumcode is named aurNNN.go
#   AC-002          the four reanchored acceptances (519 537 538 543), unedited
#                   in behaviour, run against the refactored code
#   AC-002-full     AUR-557.sh AC-001-full: every acceptance AUR-557 lists
#   AC-003          docs/architecture.md cites every internal/ package
#   AC-004          no function above 150 lines; internal/gate imported by
#                   cmd/aurumcode only from the pipeline assembly file
#   AC-001-MUT-001  in a copy, a file renamed back to aurNNN.go: AC-001 fails
#   AC-002-MUT-002  in a copy, AUR-538's mutation anchored on a line with no
#                   effect: its mutation selector no longer produces RED
#   AC-004-MUT-001  in a copy, internal/gate imported from another file: fails
#   all             AC-001, AC-003, AC-004, both mutations above, and the
#                   reanchored mutation selectors of 519, 537, 538 and 543
#
# EXIT CODES (tests/acceptance/EXIT_CODE_CONVENTION.md):
#   0 = holds, 1 = behavioral RED, 64 = unknown selector, 79 = infrastructure
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-558'
selector="${1:-all}"

case "$selector" in
  all|AC-001|AC-002|AC-002-full|AC-003|AC-004|AC-001-MUT-001|AC-002-MUT-002|AC-004-MUT-001) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$selector" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$selector" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$selector" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
real_go="$(command -v go 2>/dev/null)" || infra missing_go

for input in go.mod go.sum cmd internal pkg docs/architecture.md cmd/aurumcode/structure_test.go \
  tests/acceptance/AUR-519.sh tests/acceptance/AUR-537.sh tests/acceptance/AUR-538.sh tests/acceptance/AUR-543.sh tests/acceptance/AUR-557.sh; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a558.XXXXXX")" || infra mktemp
cleanup_root() { chmod -R u+w -- "$1" >/dev/null 2>&1 || true; rm -rf -- "$1" >/dev/null 2>&1 || true; }
trap 'cleanup_root "$run_dir"' EXIT INT TERM HUP

mkdir -p "$run_dir/gocache" "$run_dir/gotmp" "$run_dir/shim"
export GOCACHE="$run_dir/gocache" GOTMPDIR="$run_dir/gotmp" TMPDIR="$run_dir"
export GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOWORK=off GOENV=off
export GOFLAGS='-mod=mod -p=1' GOMEMLIMIT=2GiB GOMAXPROCS=1

# One GOCACHE for every nested script (each sets its own otherwise).
printf '#!/bin/sh\nexport GOCACHE=%s\nexec %s "$@"\n' "$run_dir/gocache" "$real_go" >"$run_dir/shim/go"
chmod +x "$run_dir/shim/go"
shared_path="$run_dir/shim:$PATH"

# The sealed profile's /tmp is a small tmpfs: warm the cache once, mark it,
# and drop what variant builds add after the mark.
cache_mark="$run_dir/cache.mark"
warm_cache() {
  (cd "$repo_root" && go test -mod=mod -p 1 -count=1 -run '^$' ./cmd/aurumcode/ ./internal/gate/ >"$run_dir/warm.log" 2>&1) || { cat "$run_dir/warm.log" >&2; infra warm-cache; }
  : >"$cache_mark"
}
prune_cache() { find "$run_dir/gocache" -type f -newer "$cache_mark" -delete 2>/dev/null || true; }

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

ac001() {
  local log="$run_dir/ac001.log"
  go_test "$repo_root" "$log" '^TestAUR558NoProductionFileNamedByCard$' ./cmd/aurumcode/ || { cat "$log" >&2; fail go-test-failed; }
  require_pass "$log" TestAUR558NoProductionFileNamedByCard
}

ac003() {
  local log="$run_dir/ac003.log"
  go_test "$repo_root" "$log" '^TestAUR558ArchitectureDocCitesEveryInternalPackage$' ./cmd/aurumcode/ || { cat "$log" >&2; fail go-test-failed; }
  require_pass "$log" TestAUR558ArchitectureDocCitesEveryInternalPackage
}

ac004() {
  local log="$run_dir/ac004.log"
  go_test "$repo_root" "$log" '^(TestAUR557NoFunctionExceedsLineLimit|TestAUR557EntryPointsAreShort|TestAUR558GateImportedOnlyByAssembly)$' ./cmd/aurumcode/ || { cat "$log" >&2; fail go-test-failed; }
  require_pass "$log" TestAUR557NoFunctionExceedsLineLimit TestAUR557EntryPointsAreShort TestAUR558GateImportedOnlyByAssembly
}

make_copy() {
  local root="$1"
  mkdir -p "$root/tests"
  cp -R "$repo_root/go.mod" "$repo_root/go.sum" "$repo_root/cmd" "$repo_root/internal" "$repo_root/pkg" "$root/"
  cp -R "$repo_root/docs" "$root/docs"
  cp -R "$repo_root/tests/acceptance" "$root/tests/acceptance"
  [[ -d "$repo_root/scripts" ]] && cp -R "$repo_root/scripts" "$root/scripts"
  [[ -d "$repo_root/tests/benchmark" ]] && cp -R "$repo_root/tests/benchmark" "$root/tests/benchmark"
  chmod -R u+w "$root"
}

# expect_red <root> <log> <pattern> <test name>: the guard test must fail by
# its own assertion, not by a build error.
expect_red() {
  local root="$1" log="$2" pattern="$3" name="$4" survived=0
  go_test "$root" "$log" "$pattern" ./cmd/aurumcode/ && survived=1
  prune_cache
  (( survived == 0 )) || { cat "$log" >&2; fail "mutation-survived:$name"; }
  grep -Eq -- "^--- FAIL: $name" "$log" || { cat "$log" >&2; fail "mutation-not-behavioral:$name"; }
}

ac001_mut001() {
  local root="$run_dir/mut1"
  make_copy "$root"
  [[ -f "$root/cmd/aurumcode/cmd_sbom.go" ]] || infra mutation-anchor-missing
  mv "$root/cmd/aurumcode/cmd_sbom.go" "$root/cmd/aurumcode/aur549.go"
  expect_red "$root" "$run_dir/mut1.log" '^TestAUR558NoProductionFileNamedByCard$' TestAUR558NoProductionFileNamedByCard
  rm -rf "$root"
}

ac004_mut001() {
  local root="$run_dir/mut4"
  make_copy "$root"
  cat >"$root/cmd/aurumcode/leak_gate.go" <<'GO'
package main

import "github.com/Mpaape/AurumCode/internal/gate"

var _ = gate.OriginRepo
GO
  expect_red "$root" "$run_dir/mut4.log" '^TestAUR558GateImportedOnlyByAssembly$' TestAUR558GateImportedOnlyByAssembly
  rm -rf "$root"
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

# The mutation selectors that were anchored on moved code. Each must still
# prove RED (the nested script exits 0 only when its mutation produced RED).
readonly -a mutation_selectors=(519:AC-003-MUT-001 537:AC-001-MUT-001 537:AC-001-MUT-002 538:AC-001-MUT-001 543:AC-001-MUT-001 543:AC-001-MUT-002 543:N1-MUT-001)
ac002_mutations() {
  local item bad=0
  for item in "${mutation_selectors[@]}"; do
    run_nested "$repo_root" "AUR-${item%%:*}" "${item#*:}"
    if (( n_rc != 0 )); then
      cat "$n_out" >&2
      printf '%s/%s/mutation-not-red:%s:exit:%s\n' "$card" "$selector" "$item" "$n_rc" >&2
      bad=1
    else
      printf '%s/%s/mutation-red:%s\n' "$card" "$selector" "$item" >&2
    fi
  done
  (( bad == 0 )) || exit 1
}

# MUT-002: AUR-538's mutation anchored on a line whose removal changes
# nothing. Its mutation selector must stop producing RED (exit non-zero).
ac002_mut002() {
  local root="$run_dir/mut2"
  make_copy "$root"
  local script="$root/tests/acceptance/AUR-538.sh" before
  before="$(grep -Fc "local anchor='			result.Metadata[prompt.PolicyGateWithheldKey] = \"true\"'" "$script")"
  [[ "$before" == 1 ]] || infra mutation-anchor-missing
  sed -i "s|local anchor='			result.Metadata\[prompt.PolicyGateWithheldKey\] = \"true\"'|local anchor='// ApplyOutcome publishes the decision'\"'\"'s lines (stderr and the review'\"'\"'s'|" "$script"
  grep -Fq "ApplyOutcome publishes the decision" "$script" || infra mutation-not-applied
  run_nested "$root" AUR-538 AC-001-MUT-001
  if (( n_rc == 0 )); then cat "$n_out" >&2; fail 'mutation-survived:AUR-538-still-red'; fi
  if (( n_rc == 79 || n_rc == 64 )); then cat "$n_out" >&2; infra "mutation-infra:AUR-538:$n_rc"; fi
  rm -rf "$root"
}

case "$selector" in
  AC-001)         ac001 ;;
  AC-002)         warm_cache; ac002_mutations
                  for n in 519 537 538 543; do
                    run_nested "$repo_root" "AUR-$n" all
                    (( n_rc == 0 )) || { cat "$n_out" >&2; fail "nested-red:AUR-$n:exit:$n_rc"; }
                  done ;;
  AC-002-full)    warm_cache
                  set +e
                  ( cd "$repo_root" && PATH="$shared_path" bash tests/acceptance/AUR-557.sh AC-001-full ) >"$run_dir/full.out" 2>&1
                  rc=$?
                  set -e
                  cat "$run_dir/full.out" >&2
                  (( rc == 0 )) || fail "aur-557-ac-001-full:exit:$rc" ;;
  AC-003)         ac003 ;;
  AC-004)         ac004 ;;
  AC-001-MUT-001) warm_cache; ac001_mut001 ;;
  AC-002-MUT-002) warm_cache; ac002_mut002 ;;
  AC-004-MUT-001) warm_cache; ac004_mut001 ;;
  all)
    warm_cache
    ac001; ac003; ac004
    ac001_mut001; ac004_mut001
    ac002_mut002
    ac002_mutations
    ;;
esac
printf '%s/%s/pass\n' "$card" "$selector"
