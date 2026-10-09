#!/usr/bin/env bash
# AUR-600 acceptance: with a declared gate, "blocking" in the published
# review means exactly what the gate fails on, and the review's verdict
# follows the gate; without a gate the historical text stays. A CI status
# whose every item was discarded says in one line that nothing failed.
#
# Selectors:
#   all        AC-001, AC-002, AC-003, AC-004, MUT-001, MUT-002
#   AC-001     fail_on error + only warnings: no "blocking", no "changes requested"
#   AC-002     fail_on error + one error: one blocking finding, changes requested
#   AC-003     no gate declared: historical text preserved
#   AC-004     CI status with every item discarded shows the one line
#   MUT-001    counting a warning as blocking under fail_on error turns AC-001 RED
#   MUT-002    a verdict that ignores the gate turns AC-001 RED
# Unknown selector exits 64; infrastructure 79; behavioral failure 1.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-600'
selector="${1:-all}"
case "$selector" in
  all|AC-001|AC-002|AC-003|AC-004|MUT-001|MUT-002) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$selector" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$selector" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$selector" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root

readonly pkgs=(./internal/review/blocking/ ./cmd/aurumcode/)
readonly ac1='^(TestRuleFollowsDeclaredGate|TestAUR600WarningBelowGateIsNotBlocking|TestAUR600LocalVerdictFollowsGate)$'
readonly ac2='^(TestAUR600GateBreachIsBlocking)$'
readonly ac3='^(TestAUR600WithoutGateKeepsHistoricalText)$'
readonly ac4='^(TestAUR600CIStatusAllDiscardedShowsOneLine)$'
readonly verdict_rule='if n := rule.Count(result.Issues); n > 0 {'

for input in go.mod go.sum cmd internal pkg; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a600.XXXXXX")" || infra mktemp
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
  file="$root/internal/review/blocking/rule.go"
  grep -Fq -- 'return len(r.keys)' "$file" || infra count-rule-missing
  # Count ignores the gate and counts every error and warning as blocking.
  sed -i '/^func (r Rule) Count/,/^}/{/if r.declared {/,/^\t}/d}' "$file"
  ! grep -Fq -- 'return len(r.keys)' "$file" || infra mutation-not-applied
  expect_red "$root" "$run_dir/mut1.log" "$ac1"
  printf '%s/MUT-001/rejected\n' "$card"
}

run_mut002() {
  local root="$run_dir/root-mut2" file
  stage "$root"
  file="$root/cmd/aurumcode/pr_summary_format.go"
  [[ "$(grep -Fc -- "$verdict_rule" "$file")" == 1 ]] || infra verdict-rule-missing
  # The document's decision ignores the gate: every error and warning blocks.
  sed -i 's/if n := rule.Count(result.Issues); n > 0 {/if n := blocking.Ungated().Count(result.Issues); n > 0 {/' "$file"
  ! grep -Fq -- "$verdict_rule" "$file" || infra mutation-not-applied
  expect_red "$root" "$run_dir/mut2.log" "$ac1"
  printf '%s/MUT-002/rejected\n' "$card"
}

case "$selector" in
  AC-001) run_ac AC-001 "$ac1" 3 ;;
  AC-002) run_ac AC-002 "$ac2" 1 ;;
  AC-003) run_ac AC-003 "$ac3" 1 ;;
  AC-004) run_ac AC-004 "$ac4" 1 ;;
  MUT-001) run_mut001 ;;
  MUT-002) run_mut002 ;;
  all)
    run_ac AC-001 "$ac1" 3
    run_ac AC-002 "$ac2" 1
    run_ac AC-003 "$ac3" 1
    run_ac AC-004 "$ac4" 1
    run_mut001
    run_mut002
    printf '%s/all/pass\n' "$card"
    ;;
esac
