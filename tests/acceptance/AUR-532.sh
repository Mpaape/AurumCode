#!/usr/bin/env bash
# AUR-532 acceptance: the feedback loop improves the policy from real use,
# through one reviewed pull request, never by itself. GitHub is a local fake.
#
# Selectors:
#   all        AC-001..AC-007, MUT-001
#   AC-001     dismissed false positive becomes a signal with its evidence
#   AC-002     finding fixed between audit records is a true positive
#   AC-003     /aurum perdeu is an escaped defect with a candidate case
#   AC-004     one PR, proposals without a cited signal discarded
#   AC-005     before/after measurement, regression highlighted
#   AC-006     rerun without new signals opens nothing
#   AC-007     signals redacted before the model and the PR
#   MUT-001    counting a false positive as a true positive turns AC-001 RED
# Unknown selector exits 64; infrastructure 79; behavioral failure 1.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-532'
selector="${1:-all}"
case "$selector" in
  all|AC-001|AC-002|AC-003|AC-004|AC-005|AC-006|AC-007|MUT-001) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$selector" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$selector" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$selector" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root

readonly pkgs=(./internal/feedback/ ./cmd/aurumcode/)
readonly ac1='^TestAUR532AC001DismissedFalsePositiveIsASignal$'
readonly ac2='^TestAUR532AC002FixedFindingIsATruePositive$'
readonly ac3='^TestAUR532AC003LostCommandIsAnEscapedDefect$'
readonly ac4='^(TestAUR532AC004ProposalWithoutSignalIsDiscarded|TestAUR532AC004AC006SinglePullRequestAndIdempotentRerun)$'
readonly ac5='^(TestAUR532AC005MeasurementHighlightsRegression|TestAUR532AC005MeasureCommand)$'
readonly ac6='^(TestAUR532AC006RerunWithoutNewSignalsPlansNothing|TestAUR532AC004AC006SinglePullRequestAndIdempotentRerun)$'
readonly ac7='^TestAUR532AC007SignalsAreRedacted$'
readonly fp_rule='newSignal(filter, KindFalsePositive, repo, "alert:"'

for input in go.mod go.sum cmd internal pkg; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a532.XXXXXX")" || infra mktemp
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
  file="$root/internal/feedback/alerts.go"
  grep -Fq -- "$fp_rule" "$file" || infra fp-rule-absent
  # A dismissed false positive is recorded as a true positive.
  sed -i 's/newSignal(filter, KindFalsePositive, repo, "alert:"/newSignal(filter, KindTruePositive, repo, "alert:"/' "$file"
  ! grep -Fq -- "$fp_rule" "$file" || infra mutation-not-applied
  expect_red "$root" "$run_dir/mut1.log" "$ac1"
  printf '%s/MUT-001/rejected\n' "$card"
}

case "$selector" in
  AC-001) run_ac AC-001 "$ac1" 1 ;;
  AC-002) run_ac AC-002 "$ac2" 1 ;;
  AC-003) run_ac AC-003 "$ac3" 1 ;;
  AC-004) run_ac AC-004 "$ac4" 2 ;;
  AC-005) run_ac AC-005 "$ac5" 2 ;;
  AC-006) run_ac AC-006 "$ac6" 2 ;;
  AC-007) run_ac AC-007 "$ac7" 1 ;;
  MUT-001) run_mut001 ;;
  all)
    run_ac AC-001 "$ac1" 1
    run_ac AC-002 "$ac2" 1
    run_ac AC-003 "$ac3" 1
    run_ac AC-004 "$ac4" 2
    run_ac AC-005 "$ac5" 2
    run_ac AC-006 "$ac6" 2
    run_ac AC-007 "$ac7" 1
    run_mut001
    printf '%s/all/pass\n' "$card"
    ;;
esac
