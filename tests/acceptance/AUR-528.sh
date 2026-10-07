#!/usr/bin/env bash
# AUR-528 acceptance: a package the OSV base marks malicious (MAL-) fails the
# check whatever fail_on says and cannot be excepted; the model points out a
# typosquat suspicion only with evidence from the live registry metadata.
#
# Selectors:
#   all        AC-001, AC-002, AC-003, AC-004, AC-005, MUT-001
#   AC-001     MAL- advisory fails even with an empty fail_on
#   AC-002     exception for a MAL- advisory is refused with a warning
#   AC-003     unreachable base or registry is inconclusive, never clears
#   AC-004     grounded suspicion with metadata evidence follows fail_on
#   AC-005     suspicion without metadata evidence is discarded
#   MUT-001    applying fail_on to a MAL- advisory turns AC-001 RED
# Unknown selector exits 64; infrastructure 79; behavioral failure 1.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-528'
selector="${1:-all}"
case "$selector" in
  all|AC-001|AC-002|AC-003|AC-004|AC-005|MUT-001) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$selector" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$selector" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$selector" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root

readonly pkgs=(./internal/gate/ ./internal/dependencies/)
readonly ac_001='^(TestAUR528AC001MaliciousAlwaysFails)$'
readonly ac_002='^(TestAUR528AC002MaliciousExceptionRefused)$'
readonly ac_003='^(TestAUR528AC003UnreachableIsInconclusive|TestAUR528AC003UnreachableNeverClears|TestUnvettedPackageInconclusiveUnderFailOn)$'
readonly ac_004='^(TestAUR528AC004SuspicionFollowsFailOn|TestAUR528AC004GroundedSuspicion)$'
readonly ac_005='^(TestAUR528AC005UngroundedSuspicionDiscarded)$'

for input in go.mod go.sum cmd internal pkg; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a528.XXXXXX")" || infra mktemp
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

# mutate replaces the one line equal to old (after the leading tabs) with new.
mutate() {
  local file="$1" old="$2" new="$3" count
  [[ -f "$file" ]] || infra "mutation-target-missing:${file##*/}"
  count="$(grep -Fxc -- "$old" "$file" || true)"
  [[ "$count" == 1 ]] || infra "mutation-anchor-count-$count:${file##*/}"
  awk -v old="$old" -v new="$new" '$0 == old { print new; next } { print }' "$file" >"$file.mut" || infra mutation-awk
  mv -- "$file.mut" "$file"
  ! grep -Fxq -- "$old" "$file" || infra mutation-not-applied
}

run_mut001() {
  local root="$run_dir/root-mut-001"
  stage "$root"
  # applying fail_on to a MAL- advisory turns AC-001 RED
  mutate "$root/internal/gate/dependencies_policy.go" '	return f.Vuln.Malicious()' '	return p.gated && p.cfg.Fails(f.Vuln.Severity)'
  expect_red "$root" "$run_dir/mut-001.log" "$ac_001"
  printf '%s/MUT-001/rejected\n' "$card"
}

case "$selector" in
  AC-001) run_ac AC-001 "$ac_001" 1 ;;
  AC-002) run_ac AC-002 "$ac_002" 1 ;;
  AC-003) run_ac AC-003 "$ac_003" 3 ;;
  AC-004) run_ac AC-004 "$ac_004" 2 ;;
  AC-005) run_ac AC-005 "$ac_005" 1 ;;
  MUT-001) run_mut001 ;;
  all)
    run_ac AC-001 "$ac_001" 1
    run_ac AC-002 "$ac_002" 1
    run_ac AC-003 "$ac_003" 3
    run_ac AC-004 "$ac_004" 2
    run_ac AC-005 "$ac_005" 1
    run_mut001
    printf '%s/all/pass\n' "$card"
    ;;
esac
