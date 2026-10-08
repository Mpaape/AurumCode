#!/usr/bin/env bash
# AUR-509 acceptance: the merge requires a useful, concise changelog entry.
#
# Selectors:
#   all        AC-001, AC-002, AC-003, AC-004, MUT-001, MUT-002
#   AC-001     missing, whitespace-only and no-information changes fail
#   AC-002     agent logs and long entries fail; release consolidation passes
#   AC-003     stable check, fails closed, mode read from the base
#   AC-004     suggestion stays separate; entry text is data
#   MUT-001    approving a missing entry turns AC-001 RED
#   MUT-002    turning a diff error into success turns AC-003 RED
# Unknown selector exits 64; infrastructure 79; behavioral failure 1.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-509'
selector="${1:-all}"
case "$selector" in
  all|AC-001|AC-002|AC-003|AC-004|MUT-001|MUT-002) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$selector" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$selector" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$selector" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root

readonly pkgs=(./internal/changelog/ ./internal/config/ ./cmd/aurumcode/)
readonly ac1='^TestAUR509AC001RequiredModeRefusesUselessChanges$'
readonly ac2='^TestAUR509AC002EntryIsConciseAndUserFacing$'
readonly ac3='^(TestAUR509AC003CheckFailsClosed|TestAUR509AC003PullRequestCannotSwitchItOff|TestAUR509AC003CentralPolicyDecides|TestAUR509ChangelogCheckSection)$'
readonly ac4='^(TestAUR509AC004SuggestionStaysSeparate|TestAUR509AC004EntryTextIsData)$'
readonly missing_rule='return fail(ReasonMissing, "a PR não altera "+r.File)'
readonly diff_rule='não foi possível obter o diff'

for input in go.mod go.sum cmd internal pkg; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a509.XXXXXX")" || infra mktemp
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
  file="$root/internal/changelog/require.go"
  grep -Fq -- "$missing_rule" "$file" || infra missing-rule-absent
  # A pull request that does not touch the changelog is approved.
  sed -i 's/return fail(ReasonMissing, "a PR não altera "+r.File)/return Verdict{OK: true, Reason: ReasonOK}/' "$file"
  ! grep -Fq -- "$missing_rule" "$file" || infra mutation-not-applied
  expect_red "$root" "$run_dir/mut1.log" "$ac1"
  printf '%s/MUT-001/rejected\n' "$card"
}

run_mut002() {
  local root="$run_dir/root-mut2" file line
  stage "$root"
  file="$root/cmd/aurumcode/cmd_changelog.go"
  line="$(grep -nF -- "$diff_rule" "$file" | cut -d: -f1 | head -n1)"
  [[ -n "$line" ]] || infra diff-rule-absent
  line=$((line + 1))
  sed -n "${line}p" "$file" | grep -Fq 'return 1' || infra diff-rule-shape
  # An unreadable diff is reported, then treated as success.
  sed -i "${line}s/return 1/return 0/" "$file"
  sed -n "${line}p" "$file" | grep -Fq 'return 0' || infra mutation-not-applied
  expect_red "$root" "$run_dir/mut2.log" "$ac3"
  printf '%s/MUT-002/rejected\n' "$card"
}

case "$selector" in
  AC-001) run_ac AC-001 "$ac1" 1 ;;
  AC-002) run_ac AC-002 "$ac2" 1 ;;
  AC-003) run_ac AC-003 "$ac3" 4 ;;
  AC-004) run_ac AC-004 "$ac4" 2 ;;
  MUT-001) run_mut001 ;;
  MUT-002) run_mut002 ;;
  all)
    run_ac AC-001 "$ac1" 1
    run_ac AC-002 "$ac2" 1
    run_ac AC-003 "$ac3" 4
    run_ac AC-004 "$ac4" 2
    run_mut001
    run_mut002
    printf '%s/all/pass\n' "$card"
    ;;
esac
