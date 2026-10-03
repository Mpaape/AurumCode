#!/usr/bin/env bash
#
# Acceptance program for card AUR-573 (CI green; done cards' acceptances run
# again on main).
#
# SELECTORS
#   all                 AC-001, AC-002, AC-003 and AC-001-MUT-001
#   AC-001              go test of tests/benchmark and internal/artifacts with
#                       a clean environment (no GOFLAGS) and an invalid GIT_DIR
#                       passes: the harnesses pass -buildvcs=false themselves
#   AC-002              the quick repaired acceptances exit 0
#   AC-002-full         the slow repaired acceptances exit 0 (the full run is
#                       longer than the 600 s sealed budget; run it on its own)
#   AC-003              docs/specs/AUR-573.md records each script's measured cause
#   AC-001-MUT-001      removing -buildvcs=false from the benchmark harness makes
#                       AC-001 fail when VCS is unavailable (simulated)
#
# EXIT CODES: 0 holds; 1 behavioral RED; 64 unknown selector; 79 infrastructure.
set -Eeuo pipefail
export LC_ALL=C

readonly card='AUR-573'
selector="${1:-all}"
case "$selector" in
  all|AC-001|AC-002|AC-002-full|AC-003|AC-001-MUT-001) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$selector" >&2; exit 64 ;;
esac

fail()  { printf '%s/%s/%s\n' "$card" "$selector" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$selector" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go

# Clean environment: no GOFLAGS from the caller, VCS unavailable.
clean_go_test() {
  local dir="$1"; shift
  ( cd "$dir" && env -u GOFLAGS GIT_DIR=/nonexistent go test -count=1 "$@" )
}

ac001() {
  local out
  out="$(clean_go_test "$repo_root" ./tests/benchmark ./internal/artifacts 2>&1)" || {
    printf '%s\n' "$out" >&2
    grep -Fq 'error obtaining VCS status' <<<"$out" && fail 'vcs-status-dependency'
    fail 'go-test'
  }
  grep -Fq 'github.com/Mpaape/AurumCode/tests/benchmark' <<<"$out" || fail 'benchmark-not-run'
  grep -Fq 'github.com/Mpaape/AurumCode/internal/artifacts' <<<"$out" || fail 'artifacts-not-run'
  # No exec of go may rely on the environment for -buildvcs.
  local bad
  bad="$(find "$repo_root/tests" "$repo_root/internal" "$repo_root/cmd" -name '*.go' -type f -exec grep -nE 'exec\.Command\("go", "build",' {} + | grep -vF -e '-buildvcs=false' || true)"
  [[ -z "$bad" ]] || { printf '%s\n' "$bad" >&2; fail 'go-build-without-buildvcs'; }
}

# run_acceptance ID [SELECTOR]: exit must be 0.
run_acceptance() {
  local id="$1" sel="${2:-all}" rc=0 out
  [[ -f "$repo_root/tests/acceptance/$id.sh" ]] || infra "missing:$id"
  out="$(cd "$repo_root" && timeout 900 bash "tests/acceptance/$id.sh" "$sel" 2>&1)" || rc=$?
  if (( rc != 0 )); then
    printf '%s\n' "$out" | tail -n 15 >&2
    fail "$id/$sel/exit:$rc"
  fi
  printf '%s/%s/%s ok\n' "$card" "$id" "$sel"
}

quick=(AUR-493 AUR-496 AUR-500 AUR-561 AUR-562)
slow=(AUR-468 AUR-473 AUR-479 AUR-504 AUR-505)
# AUR-450 has no `all` selector (64): AC-001 is what AUR-473 AC-003 runs.

ac002() { local id; for id in "${quick[@]}"; do run_acceptance "$id"; done; }
ac002_full() {
  local id
  for id in "${slow[@]}"; do run_acceptance "$id"; done
  run_acceptance AUR-450 AC-001
}

ac003() {
  local spec="$repo_root/docs/specs/AUR-573.md" id
  [[ -f "$spec" ]] || fail 'spec-missing'
  for id in AUR-468 AUR-473 AUR-479 AUR-491 AUR-493 AUR-496 AUR-497 AUR-500 AUR-504 AUR-505 AUR-561 AUR-562 AUR-534; do
    grep -Fq -- "$id" "$spec" || fail "spec-lacks:$id"
  done
  grep -Fq 'Causa medida' "$spec" || fail 'spec-lacks-cause-heading'
}

mutation_001() {
  local stage; stage="$(mktemp -d)"
  trap 'chmod -R u+w -- "$stage" 2>/dev/null || true; rm -rf -- "$stage"' RETURN
  ( cd "$repo_root" && tar --exclude=.git -cf - . ) | tar -xf - -C "$stage"
  mkdir "$stage/.git"   # present but unusable: with GIT_DIR invalid, VCS stamping errors
  local target="$stage/tests/benchmark/aur523.go"
  grep -Fq '"go", "build", "-buildvcs=false", "-o", bin' "$target" || infra 'mutation-anchor-missing'
  sed -i 's/"go", "build", "-buildvcs=false", "-o", bin/"go", "build", "-o", bin/' "$target"
  grep -Fq -- '-buildvcs=false' "$target" && infra 'mutation-not-applied'
  local out rc=0
  out="$(clean_go_test "$stage" ./tests/benchmark 2>&1)" || rc=$?
  (( rc != 0 )) || fail 'MUT-001/survived'
  grep -Fq 'error obtaining VCS status' <<<"$out" || fail 'MUT-001/wrong-failure-mode'
  printf '%s/AC-001-MUT-001/rejected\n' "$card"
}

case "$selector" in
  all) ac001; ac002; ac003; mutation_001 ;;
  AC-001) ac001 ;;
  AC-002) ac002 ;;
  AC-002-full) ac002_full ;;
  AC-003) ac003 ;;
  AC-001-MUT-001) mutation_001 ;;
esac
printf '%s/%s/ok\n' "$card" "$selector"
