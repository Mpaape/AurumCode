#!/usr/bin/env bash
# AUR-599 acceptance: the CI status a review publishes rests on facts of
# this execution. Running checks and statuses this product published on an
# earlier round never reach the model as facts nor become analysis items; a
# scanner that did not run never appears; a concluded failure stays.
#
# Selectors:
#   all        AC-001, AC-002, AC-003, MUT-001, MUT-002
#   AC-001     running checks and own statuses withheld and discarded
#   AC-002     a scanner that did not run is discarded
#   AC-003     a concluded failure stays in the prompt and in the review
#   MUT-001    passing the own status to the model as a fact turns AC-001 RED
#   MUT-002    keeping a scanner that did not run turns AC-002 RED
# Unknown selector exits 64; infrastructure 79; behavioral failure 1.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-599'
selector="${1:-all}"
case "$selector" in
  all|AC-001|AC-002|AC-003|MUT-001|MUT-002) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$selector" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$selector" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$selector" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root

readonly pkgs=(./internal/review/cistatus/ ./cmd/aurumcode/)
readonly ac1='^(TestAC001RunningChecksAndOwnStatusesAreNotFacts|TestAUR599AC001RunningAndOwnStatusesAreNotPublished)$'
readonly ac2='^(TestAC002ScannerNotRunNeverAppears|TestAUR599AC002ScannerNotRunIsNotPublished)$'
readonly ac3='^(TestAC003ConcludedFailureStays|TestAUR599AC003ConcludedFailureIsPublished)$'
readonly own_rule='case c.Own(name):'
readonly scanner_rule='ok && !executed[engine] {'

for input in go.mod go.sum cmd internal pkg; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a599.XXXXXX")" || infra mktemp
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
  for source in go.mod go.sum cmd internal pkg; do
    cp -R "$repo_root/$source" "$root/$source"
  done
  chmod -R u+w -- "$root"
}

go_test() {
  local root="$1" log="$2" pattern="$3"
  ( cd "$root" && go test -buildvcs=false -count=1 -p 1 -v -run "$pattern" "${pkgs[@]}" ) >"$log" 2>&1
}

run_ac() {
  local name="$1" pattern="$2" root="$run_dir/root" log="$run_dir/$1.log" passes
  command -v go >/dev/null 2>&1 || infra missing_go
  [[ -d "$root" ]] || stage "$root"
  go_test "$root" "$log" "$pattern" || { cat "$log" >&2; fail "go-test-failed:$name"; }
  passes="$(grep -Ec -- '^--- PASS: ' "$log" || true)"
  [[ "$passes" == 2 ]] || { cat "$log" >&2; fail "want-2-passes-got-$passes"; }
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
  file="$root/internal/review/cistatus/context.go"
  grep -Fq -- "$own_rule" "$file" || infra own-rule-missing
  # The own status is treated like any other producer's check.
  sed -i 's/case c.Own(name):/case false:/' "$file"
  ! grep -Fq -- "$own_rule" "$file" || infra mutation-not-applied
  expect_red "$root" "$run_dir/mut1.log" "$ac1"
  printf '%s/MUT-001/rejected\n' "$card"
}

run_mut002() {
  local root="$run_dir/root-mut2" file
  stage "$root"
  file="$root/internal/review/cistatus/filter.go"
  grep -Fq -- "$scanner_rule" "$file" || infra scanner-rule-missing
  # Every scanner counts as executed.
  sed -i 's/ok \&\& !executed\[engine\] {/ok \&\& !executed[engine] \&\& engine == "" {/' "$file"
  ! grep -Fq -- "$scanner_rule" "$file" || infra mutation-not-applied
  expect_red "$root" "$run_dir/mut2.log" "$ac2"
  printf '%s/MUT-002/rejected\n' "$card"
}

case "$selector" in
  AC-001) run_ac AC-001 "$ac1" ;;
  AC-002) run_ac AC-002 "$ac2" ;;
  AC-003) run_ac AC-003 "$ac3" ;;
  MUT-001) run_mut001 ;;
  MUT-002) run_mut002 ;;
  all)
    run_ac AC-001 "$ac1"
    run_ac AC-002 "$ac2"
    run_ac AC-003 "$ac3"
    run_mut001
    run_mut002
    printf '%s/all/pass\n' "$card"
    ;;
esac
