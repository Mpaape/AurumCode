#!/usr/bin/env bash
# AUR-535 acceptance: the direct Docker Action can apply a central policy
# that lives outside the reviewed tree (/github/home, fed by
# $RUNNER_TEMP/_github_home), a policy inside the reviewed tree is refused on
# the --pr path before any model call, and a symlink outside the tree that
# points into it is refused as well.
#
# Selectors:
#   all      every AC plus both mutations
#   AC-001   policy under /github/home applied, in the workspace refused;
#            the docs and action.yml describe that recipe
#   AC-002   --pr: policy inside the reviewed tree fails closed
#   AC-003   symlink outside the tree pointing into it is refused
#   MUT-001  containment check removed from the --pr path; AC-002 must be RED
#   MUT-002  symlink resolution disabled; AC-003 must be RED
# Unknown selectors exit 64; infrastructure failures exit 79; behavioral
# failures exit 1.
set -euo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-535'
selector="${1:-all}"

fail() { printf '%s/%s/%s\n' "$card" "$selector" "$1" >&2; exit 1; }
infra() { printf '%s/%s/infrastructure/%s\n' "$card" "$selector" "$1" >&2; exit 79; }

readonly known='all AC-001 AC-002 AC-003 MUT-001 MUT-002'
if [[ " $known " != *" $selector "* ]]; then
  printf '%s/%s/unknown-selector\n' "$card" "$selector" >&2
  exit 64
fi

script_dir="${0%/*}"
if [[ "$script_dir" == "$0" ]]; then
  script_dir='.'
fi
repo_root="$(CDPATH='' cd -- "$script_dir/../.." && pwd -P)" || infra repo_root

if [[ "$selector" == all ]]; then
  for sub in AC-001 AC-002 AC-003 MUT-001 MUT-002; do
    bash "$0" "$sub" || fail "sub-selector:$sub"
  done
  printf '%s/all/pass\n' "$card"
  exit 0
fi

command -v go >/dev/null 2>&1 || infra missing_go
for input in go.mod go.sum cmd internal pkg action.yml docs/configuration.md; do
  [[ -e "$repo_root/$input" ]] || infra "missing-input:$input"
done
[[ -f "$repo_root/cmd/aurumcode/aur535_test.go" ]] || infra missing-behavior-test
[[ -f "$repo_root/internal/config/central_test.go" ]] || infra missing-unit-test

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a535.XXXXXX")" || infra mktemp
cleanup_root() {
  chmod -R u+w -- "$1" >/dev/null 2>&1 || true
  rm -rf -- "$1" >/dev/null 2>&1 || true
}
trap 'cleanup_root "$run_dir"' EXIT INT TERM HUP
mkdir -p "$run_dir/root" "$run_dir/gotmp"
for source in go.mod go.sum cmd internal pkg; do
  cp -R "$repo_root/$source" "$run_dir/root/$source"
done
chmod -R u+w -- "$run_dir/root"

export GOPROXY=off GOSUMDB=off GOTOOLCHAIN=local GOWORK=off GOENV=off
export GOFLAGS='-mod=mod -p=1 -buildvcs=false'
: "${GOCACHE:=$run_dir/cache}"
export GOCACHE GOTMPDIR="$run_dir/gotmp"

# A mutation edits the copied tree only. The anchor tokens are split so this
# script can never match its own text, and a missing or unapplied anchor is
# infrastructure, never a silent no-op.
mutate() {
  local target="$run_dir/root/$1" anchor="$2" replacement="$3"
  grep -Fq -- "$anchor" "$target" || infra "mutation-anchor-missing:$1"
  local content
  content="$(cat -- "$target")" || infra "mutation-read:$1"
  printf '%s\n' "${content/"$anchor"/"$replacement"}" >"$target"
  if grep -Fq -- "$anchor" "$target"; then
    infra "mutation-not-applied:$1"
  fi
}

pkg=''
test_pattern=''
expect_fail=''
if [[ "$selector" == AC-001 ]]; then
  pkg='./cmd/aurumcode/'
  test_pattern='^TestAUR535ActionPolicyFromGithubHomeApplies$'
elif [[ "$selector" == AC-002 ]]; then
  pkg='./cmd/aurumcode/'
  test_pattern='^TestAUR535PRPolicyInsideReviewedTreeFailsClosed$'
elif [[ "$selector" == AC-003 ]]; then
  pkg='./internal/config/'
  test_pattern='^TestAUR535PolicySymlinkIntoReviewedTreeRefused$'
elif [[ "$selector" == MUT-001 ]]; then
  pkg='./cmd/aurumcode/'
  test_pattern='^TestAUR535PRPolicyInsideReviewedTreeFailsClosed$'
  expect_fail=1
  mutate cmd/aurumcode/review_pr_inputs.go \
    'ValidatePolicyOutsideReviewedTree(p.opts.policyDir, cwd); err '"!= nil {" \
    'ValidatePolicyOutsideReviewedTree(p.opts.policyDir, cwd); false && err != nil {'
elif [[ "$selector" == MUT-002 ]]; then
  pkg='./internal/config/'
  test_pattern='^TestAUR535PolicySymlinkIntoReviewedTreeRefused$'
  expect_fail=1
  mutate internal/config/central.go \
    'filepath.EvalSymlinks(abs); err '"== nil {" \
    'abs, error(nil); false && err == nil {'
fi

# AC-001 is also a documentation contract: the recipe the docs give must be
# the one the behavior test exercises, and no stale claim that the policy
# can live in the workspace may remain.
if [[ "$selector" == AC-001 ]]; then
  doc="$repo_root/docs/configuration.md"
  grep -Fq '_github_home' "$doc" || fail 'doc-missing-runner-temp-home'
  grep -Fq 'policy_path: /github/home/' "$doc" || fail 'doc-missing-github-home-policy-path'
  if grep -Fq 'já no workspace' "$doc"; then
    fail 'doc-still-claims-workspace-policy'
  fi
  grep -Fq '/github/home' "$repo_root/action.yml" || fail 'action-description-missing-github-home'
  if grep -Fq 'inside the container workspace' "$repo_root/action.yml"; then
    fail 'action-description-still-workspace'
  fi
fi

log="$run_dir/test.log"
set +e
(cd "$run_dir/root" && go test -count=1 -timeout 300s -v "$pkg" -run "$test_pattern") >"$log" 2>&1
status=$?
set -e
cat "$log" >&2

if [[ -n "$expect_fail" ]]; then
  if grep -Eq 'build failed|cannot use|undefined:|syntax error|declared and not used' "$log"; then
    fail 'mutation-build-failure-not-behavioral'
  fi
  grep -Eq -- '^--- FAIL: TestAUR535' "$log" || fail 'mutation-survived'
  (( status != 0 )) || fail 'mutation-survived-exit-zero'
  printf '%s/%s/pass (mutation produced RED)\n' "$card" "$selector"
  exit 0
fi

(( status == 0 )) || fail "go-test-exit:$status"
grep -Eq -- '^--- PASS: TestAUR535' "$log" || fail 'no-test-executed'
printf '%s/%s/pass\n' "$card" "$selector"
