#!/usr/bin/env bash
# AUR-476 acceptance: partial review coverage reaches the terminal and the PR,
# with the reason and the omitted paths, and the notice is pipeline-derived so
# a model response claiming complete coverage cannot erase it.
#
# Runs the real Go behavior tests in cmd/aurumcode inside the sealed, offline
# Go profile. The mutation selectors apply the card's mutation to a staged,
# writable copy of the candidate and require the corresponding behavior test to
# FAIL (RED); the versioned source is never touched. A mutation nobody notices
# is a test that asserts nothing.
#
# Selectors:
#   all             run every behavior test below
#   AC-001          ignored file -> terminal coverage notice with count + paths
#   AC-002          complete review -> no coverage notice
#   AC-003          model claims full coverage -> notice still present
#   AC-004          PR with tests omitted -> published body names the omission
#   AC-001-MUT-001  suppress the notice; AC-001 must fail (RED)
#   AC-003-MUT-001  suppress the notice; AC-003 must fail (RED)
# Unknown selectors exit 64; infrastructure failures exit 79; behavioral
# failures exit 1.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-476'
selector="${1:-all}"

case "$selector" in
  all|AC-001|AC-002|AC-003|AC-004|AC-001-MUT-001|AC-003-MUT-001) ;;
  *) printf '%s/%s/unknown-selector\n' "$card" "$selector" >&2; exit 64 ;;
esac

fail() { printf '%s/%s/%s\n' "$card" "$selector" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$selector" "$1" >&2; exit 79; }

script_dir="${0%/*}"; [[ "$script_dir" != "$0" ]] || script_dir='.'
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root
command -v go >/dev/null 2>&1 || infra missing_go

for input in go.mod go.sum cmd internal pkg; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done
[[ -f "$repo_root/cmd/aurumcode/aur476_test.go" ]] || infra missing-behavior-test

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a476.XXXXXX")" || infra mktemp
cleanup_root() {
  chmod -R u+w -- "$1" >/dev/null 2>&1 || true
  rm -rf -- "$1" >/dev/null 2>&1 || true
}
trap 'cleanup_root "$run_dir"' EXIT INT TERM HUP
mkdir -p "$run_dir/root" "$run_dir/cache" "$run_dir/gotmp"
for source in go.mod go.sum cmd internal pkg; do
  cp -R "$repo_root/$source" "$run_dir/root/$source"
done
chmod -R u+w -- "$run_dir/root"

export GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOWORK=off GOENV=off
export GOFLAGS='-mod=mod -p=1'
export GOCACHE="$run_dir/cache" GOTMPDIR="$run_dir/gotmp" TMPDIR="$run_dir"
export GOMEMLIMIT=2GiB GOMAXPROCS=1

# The card's declared mutation (MUT-001): suppress the coverage declaration so
# the partial diff stops declaring its omission. The mutation inserts an early
# return into coverageNotice so the candidate still COMPILES -- the package
# body below stays referenced, so nothing becomes an unused variable and a
# build failure is never valid red evidence -- while the promised behavior is
# gone. The signature token is split so this file cannot match its own edit.
apply_mutation() {
  local target="$run_dir/root/cmd/aurumcode/passes.go"
  local signature='func coverage'"Notice(copy reviewCopy, c reviewCoverageBreakdown) string {"
  if ! grep -Fq "$signature" "$target"; then
    infra mutation-anchor-missing
  fi
  # Insert an unconditional early return as the function's first statement.
  sed -i "s|${signature}|${signature}\n\treturn \"\"|" "$target"
  if ! grep -Fq 'return ""' "$target"; then
    infra mutation-not-applied
  fi
}

case "$selector" in
  AC-001-MUT-001|AC-003-MUT-001) apply_mutation ;;
esac

test_pattern=''
expect_fail=''
case "$selector" in
  all)           test_pattern='^TestAUR476' ;;
  AC-001)        test_pattern='^TestAUR476TerminalDeclaresIgnoredCoverage$'; expect_fail='' ;;
  AC-002)        test_pattern='^TestAUR476CompleteReviewHasNoNotice$'; expect_fail='' ;;
  AC-003)        test_pattern='^TestAUR476NoticeIsPipelineDerivedNotModelDerived$'; expect_fail='' ;;
  AC-004)        test_pattern='^TestAUR476PRDeclaresOmittedTests$'; expect_fail='' ;;
  AC-001-MUT-001) test_pattern='^TestAUR476TerminalDeclaresIgnoredCoverage$'; expect_fail=1 ;;
  AC-003-MUT-001) test_pattern='^TestAUR476NoticeIsPipelineDerivedNotModelDerived$'; expect_fail=1 ;;
esac

log="$run_dir/test.log"
set +e
(cd "$run_dir/root" && go test -mod=mod -p 1 -count=1 -timeout 300s -v ./cmd/aurumcode -run "$test_pattern") >"$log" 2>&1
status=$?
set -e
cat "$log" >&2

if [[ -n "$expect_fail" ]]; then
  # A surviving mutation is a test that asserts nothing: the mutated candidate
  # MUST fail, and the failure must be the behavior test, not a compile error.
  grep -Eq -- '^--- FAIL: TestAUR476' "$log" || fail 'mutation-survived'
  (( status != 0 )) || fail 'mutation-survived-exit-zero'
  if grep -Eq 'build failed|cannot use|undefined:|syntax error' "$log"; then
    fail 'mutation-build-failure-not-behavioral'
  fi
  printf '%s/%s/pass (mutation produced RED)\n' "$card" "$selector"
  exit 0
fi

(( status == 0 )) || fail "go-test-exit:$status"
grep -Eq -- '^--- PASS: TestAUR476' "$log" || fail 'no-test-executed'

if [[ "$selector" == all ]]; then
  for name in TerminalDeclaresIgnoredCoverage CompleteReviewHasNoNotice NoticeIsPipelineDerivedNotModelDerived PRDeclaresOmittedTests; do
    grep -q "^--- PASS: TestAUR476$name " "$log" || fail "missing-pass:$name"
  done
fi
printf '%s/%s/pass\n' "$card" "$selector"
