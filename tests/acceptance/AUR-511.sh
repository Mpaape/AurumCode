#!/usr/bin/env bash
#
# Acceptance program for card AUR-511 (AC-001..AC-006, MUT-001).
#
# It runs the versioned review-quality benchmark harness in tests/benchmark
# against the frozen corpus in tests/benchmark/testdata/corpus.json and refuses
# a selector that matched no test. Fixtures only validate the harness; they are
# never presented as the score of a real model.
#
# Exit codes: 0 pass, 1 behavioral RED, 64 unknown selector, 79 infrastructure.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-511'
selector="${1:-all}"

case "$selector" in
  all|AC-001|AC-002|AC-003|AC-004|AC-005|AC-006|MUT-001) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$selector" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$selector" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$selector" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go

owned_inputs=(tests/benchmark tests/benchmark/testdata/corpus.json)
for input in "${owned_inputs[@]}"; do
  [[ -e "$repo_root/$input" ]] || fail "behavior-missing:$input"
done
[[ -f "$repo_root/go.mod" ]] || infra missing-go-mod
[[ -f "$repo_root/go.sum" ]] || infra missing-go-sum

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a511.XXXXXX")" || infra mktemp
cleanup_root() {
  chmod -R u+w -- "$1" >/dev/null 2>&1 || true
  rm -rf -- "$1" >/dev/null 2>&1 || true
}
trap 'cleanup_root "$run_dir"' EXIT INT TERM HUP

export GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOWORK=off GOENV=off
export GOFLAGS='-buildvcs=false -p=1'
export GOCACHE="$run_dir/gocache" GOTMPDIR="$run_dir/gotmp" TMPDIR="$run_dir"
export GOMAXPROCS=1
mkdir -p "$GOCACHE" "$GOTMPDIR"

pattern=''
case "$selector" in
  all)    pattern='^TestAUR511' ;;
  AC-001) pattern='^TestAUR511CorpusCoverageFrozen$' ;;
  AC-002) pattern='^TestAUR511HarnessMetricsAndRounds$' ;;
  AC-003) pattern='^TestAUR511LocalPilotFixtureIsNotRealScore$' ;;
  AC-004) pattern='^TestAUR511ComparisonSeparatesAndReportsUncertainty$' ;;
  AC-005) pattern='^TestAUR511ContextAndPromptVariants$' ;;
  AC-006) pattern='^TestAUR511SuggestionApplicabilityAndHumanDecision$' ;;
  MUT-001) pattern='^TestAUR511Mutation' ;;
esac

log="$run_dir/go-test.log"
set +e
(cd "$repo_root" && go test -mod=mod -count=1 -v -timeout 300s ./tests/benchmark -run "$pattern") >"$log" 2>&1
status=$?
set -e
cat "$log" >&2
((status == 0)) || fail "go-test-exit:$status"

grep -Eq -- '^--- PASS: TestAUR511' "$log" || fail 'no-test-executed'
grep -Eq '(^|[[:space:]])ok[[:space:]].*tests/benchmark' "$log" || fail 'package-not-ok'

printf '%s/%s/pass\n' "$card" "$selector"
