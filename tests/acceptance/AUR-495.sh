#!/usr/bin/env bash
# AUR-495 acceptance: a dependency changed by the change is read by the model,
# grounded in the diff, cross-checked with the scanner when it knows the file,
# and checked against OSV (a local fake server, no network) on both sides:
# introduced, pre-existing or fixed; a failing source is inconclusive.
#
# Selectors:
#   all        AC-001, AC-002, AC-003, AC-004, AC-005, AC-006, AC-007, AC-008, MUT-001, MUT-002
#   AC-001     bump to an advisory version: introduced, with ids, fix and link
#   AC-002     advisory on both sides is pre-existing
#   AC-003     fixing the version records the fix
#   AC-004     unknown format read by the model; divergence declared; scanner evidence kept
#   AC-005     range consulted by package; unresolved is inconclusive
#   AC-006     unreachable/stale source or missing scanner is inconclusive
#   AC-007     monorepo lockfiles reported separately
#   AC-008     a dependency absent from the diff is discarded
#   MUT-001    a failed OSV query read as an empty list turns AC-006 RED
#   MUT-002    labelling a pre-existing advisory as introduced turns AC-002 RED
# Unknown selector exits 64; infrastructure 79; behavioral failure 1.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-495'
selector="${1:-all}"
case "$selector" in
  all|AC-001|AC-002|AC-003|AC-004|AC-005|AC-006|AC-007|AC-008|MUT-001|MUT-002) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$selector" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$selector" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$selector" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root

readonly pkgs=(./internal/dependencies/ ./internal/gate/ ./internal/config/)
readonly ac_001='^(TestAUR495AC001IntroducedAdvisory|TestAUR495AC001FindingLine|TestDependenciesSectionParse|TestDependenciesGovernedByPolicy)$'
readonly ac_002='^(TestAUR495AC002Preexisting)$'
readonly ac_003='^(TestAUR495AC003Fixed)$'
readonly ac_004='^(TestAUR495AC004UnknownFormatAndDivergence|TestAUR495AC004ScannerEvidenceKept)$'
readonly ac_005='^(TestAUR495AC005Range)$'
readonly ac_006='^(TestAUR495AC006SourceFailures|TestAUR495AC006InconclusiveFollowsPolicy)$'
readonly ac_007='^(TestAUR495AC007Monorepo)$'
readonly ac_008='^(TestAUR495AC008InventedDependencyDiscarded)$'

for input in go.mod go.sum cmd internal pkg; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a495.XXXXXX")" || infra mktemp
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
  # a failed OSV query read as an empty list turns AC-006 RED
  mutate "$root/internal/dependencies/osv.go" '			return nil, err' '			return nil, nil'
  expect_red "$root" "$run_dir/mut-001.log" "$ac_006"
  printf '%s/MUT-001/rejected\n' "$card"
}

run_mut002() {
  local root="$run_dir/root-mut-002"
  stage "$root"
  # labelling a pre-existing advisory as introduced turns AC-002 RED
  mutate "$root/internal/dependencies/classify.go" '			status = StatusPreexisting' '			status = StatusIntroduced'
  expect_red "$root" "$run_dir/mut-002.log" "$ac_002"
  printf '%s/MUT-002/rejected\n' "$card"
}

case "$selector" in
  AC-001) run_ac AC-001 "$ac_001" 4 ;;
  AC-002) run_ac AC-002 "$ac_002" 1 ;;
  AC-003) run_ac AC-003 "$ac_003" 1 ;;
  AC-004) run_ac AC-004 "$ac_004" 2 ;;
  AC-005) run_ac AC-005 "$ac_005" 1 ;;
  AC-006) run_ac AC-006 "$ac_006" 2 ;;
  AC-007) run_ac AC-007 "$ac_007" 1 ;;
  AC-008) run_ac AC-008 "$ac_008" 1 ;;
  MUT-001) run_mut001 ;;
  MUT-002) run_mut002 ;;
  all)
    run_ac AC-001 "$ac_001" 4
    run_ac AC-002 "$ac_002" 1
    run_ac AC-003 "$ac_003" 1
    run_ac AC-004 "$ac_004" 2
    run_ac AC-005 "$ac_005" 1
    run_ac AC-006 "$ac_006" 2
    run_ac AC-007 "$ac_007" 1
    run_ac AC-008 "$ac_008" 1
    run_mut001
    run_mut002
    printf '%s/all/pass\n' "$card"
    ;;
esac
