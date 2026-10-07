#!/usr/bin/env bash
# AUR-512 acceptance: the consumer QA (tests/consumer) covers the contract and
# its verifier never lets a regressed product, an anonymous run or an
# infrastructure failure pass. The live run against the consumer repository
# (tests/consumer/run.sh) is the owner's; this script proves the harness.
#
# Selectors:
#   all        AC-001..AC-005, MUT-001
#   AC-001     both installations pinned by SHA, comments/review, inline
#   AC-002     model absent/inconclusive, publication, permissions, CI failing
#   AC-003     fork scenario declared, never pull_request_target
#   AC-004     changelog gate, repeated round, fix clears the block
#   AC-005     evidence names repo/SHA/run; infrastructure is "nao_medido"
#   MUT-001    ignoring conclusion and statuses lets the regressed product pass
# Unknown selector exits 64; infrastructure 79; behavioral failure 1.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-512'
selector="${1:-all}"
case "$selector" in
  all|AC-001|AC-002|AC-003|AC-004|AC-005|MUT-001) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$selector" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$selector" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$selector" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root

readonly pkgs=(./tests/consumer/)
readonly table='^TestAUR512ScenarioTableCoversTheContract$'
readonly regress='^TestAUR512RegressedProductFailsQA$'
readonly evidence='^TestAUR512EvidenceIdentifiesTheRunAndInfraIsNotMeasured$'
readonly conclusion_rule='if e.Conclusion != x.Conclusion {'
readonly status_rule='if got := e.Statuses[k]; got != x.Statuses[k] {'

for input in go.mod go.sum cmd internal pkg tests/consumer; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a512.XXXXXX")" || infra mktemp
trap 'chmod -R u+w -- "$run_dir" >/dev/null 2>&1 || true; rm -rf -- "$run_dir" >/dev/null 2>&1 || true' EXIT INT TERM HUP
mkdir -p "$run_dir/gotmp"

export GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOWORK=off GOENV=off
export GOFLAGS='-mod=mod -p=1 -buildvcs=false'
: "${GOCACHE:=$run_dir/gocache}"
export GOCACHE GOTMPDIR="$run_dir/gotmp" TMPDIR="$run_dir"
export GOMEMLIMIT=2GiB GOMAXPROCS=1

# stage copies the whole module (never enumerated packages) to a fresh root.
stage() {
  local root="$1" source
  mkdir -p "$root"
  mkdir -p "$root/tests"
  for source in go.mod go.sum cmd internal pkg tests/consumer; do
    cp -R "$repo_root/$source" "$root/$source"
  done
  chmod -R u+w -- "$root"
}

go_test() {
  local root="$1" log="$2" pattern="$3"
  ( cd "$root" && go test -buildvcs=false -count=1 -p 1 -v -run "$pattern" "${pkgs[@]}" ) >"$log" 2>&1
}

run_ac() {
  local name="$1" pattern="$2" want="$3" root="$run_dir/root" log="$run_dir/$1.log" passes
  command -v go >/dev/null 2>&1 || infra missing_go
  [[ -d "$root" ]] || stage "$root"
  go_test "$root" "$log" "$pattern" || { cat "$log" >&2; fail "go-test-failed:$name"; }
  passes="$(grep -Ec -- '^--- PASS: ' "$log" || true)"
  [[ "$passes" == "$want" ]] || { cat "$log" >&2; fail "want-$want-passes-got-$passes"; }
  printf '%s/%s/pass\n' "$card" "$name"
}

expect_red() {
  local root="$1" log="$2" pattern="$3"
  if go_test "$root" "$log" "$pattern"; then
    cat "$log" >&2; fail mutation-survived
  fi
  if grep -Eq 'build failed|cannot use|undefined:|syntax error|declared and not used' "$log"; then
    cat "$log" >&2; fail mutation-did-not-compile
  fi
  grep -Eq -- '^--- FAIL: ' "$log" || { cat "$log" >&2; fail mutation-not-behavioral; }
  grep -E -- '^--- FAIL: |_test\.go:[0-9]+:' "$log" | sed -n '1,4p' >&2
}

run_mut001() {
  local root="$run_dir/root-mut1" file
  stage "$root"
  file="$root/tests/consumer/consumer.go"
  grep -Fq -- "$conclusion_rule" "$file" || infra conclusion-rule-absent
  grep -Fq -- "$status_rule" "$file" || infra status-rule-absent
  # The QA stops looking at the conclusion and at the gate statuses.
  sed -i 's/if e.Conclusion != x.Conclusion {/if false \&\& e.Conclusion != x.Conclusion {/' "$file"
  sed -i 's/if got := e.Statuses\[k\]; got != x.Statuses\[k\] {/if got := e.Statuses[k]; false \&\& got != x.Statuses[k] {/' "$file"
  ! grep -Fq -- "$conclusion_rule" "$file" || infra mutation-not-applied
  ! grep -Fq -- "$status_rule" "$file" || infra mutation-not-applied
  expect_red "$root" "$run_dir/mut1.log" "$regress"
  printf '%s/MUT-001/rejected\n' "$card"
}

case "$selector" in
  AC-001|AC-002|AC-003) run_ac "$selector" "$table" 1 ;;
  AC-004) run_ac AC-004 "$regress" 1 ;;
  AC-005) run_ac AC-005 "$evidence" 1 ;;
  MUT-001) run_mut001 ;;
  all)
    run_ac AC-001 "$table" 1
    run_ac AC-004 "$regress" 1
    run_ac AC-005 "$evidence" 1
    run_mut001
    printf '%s/all/pass\n' "$card"
    ;;
esac
