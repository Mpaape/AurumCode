#!/usr/bin/env bash
# AUR-518 acceptance: a central policy, when declared with --politica (or
# AURUMCODE_POLICY), takes exclusive authority over a repository's own
# rules and ignore patterns -- the repository cannot disable a policy rule,
# loosen its severity, or hide a path from it -- while the repository's own
# context (skills/docs) still reaches the model alongside the policy's, and
# a missing/invalid policy fails the command before any model call.
#
# Selectors:
#   all             run every behavior test below
#   AC-001 .. AC-006
#   AC-001-MUT-001  apply the repo override after the policy; AC-001 must
#                   fail (RED)
# Unknown selectors exit 64; infrastructure failures exit 79; behavioral
# failures exit 1.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-518'
selector="${1:-all}"

case "$selector" in
  all|AC-001|AC-002|AC-003|AC-004|AC-005|AC-006|AC-001-MUT-001) ;;
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
[[ -f "$repo_root/internal/config/central.go" ]] || infra missing-source
[[ -f "$repo_root/internal/config/central_test.go" ]] || infra missing-unit-test
[[ -f "$repo_root/cmd/aurumcode/aur518_test.go" ]] || infra missing-behavior-test

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a518.XXXXXX")" || infra mktemp
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

# MUT-001: "apply the repo override after the policy" -- ApplyCentralPolicy
# stops taking the policy's own Rules and takes the repository's instead,
# so a repository's rule override (disable or severity) survives exactly
# the case AC-001 exists to refuse. Anchored on the stable field
# assignment; the token is split so this file cannot match its own edit,
# and a missing anchor is infrastructure, never a silent no-op.
apply_mutation() {
  local target="$run_dir/root/internal/config/central.go"
  local anchor='effective.Rules = '"central.Rules"
  grep -Fq "$anchor" "$target" || infra mutation-anchor-missing
  sed -i "s|${anchor}|effective.Rules = repo.Rules|" "$target"
  grep -Fq "$anchor" "$target" && infra mutation-not-applied
  return 0
}

test_pattern=''
expect_fail=''
pkgs='./internal/config/... ./cmd/aurumcode/...'
case "$selector" in
  all)            test_pattern='^TestAUR518' ;;
  AC-001)         test_pattern='^TestAUR518PolicyKeepsRuleDespiteRepoDisable$' ;;
  AC-002)         test_pattern='^TestAUR518PolicySeverityOverrideIgnored$' ;;
  AC-003)         test_pattern='^TestAUR518PolicyIgnoreWinsOverRepo$' ;;
  AC-004)         test_pattern='^TestAUR518PolicyAndRepoSkillsBothReachPrompt$' ;;
  AC-005)         test_pattern='^TestAUR518MissingOrInvalidPolicyFailsClosed$' ;;
  AC-006)         test_pattern='^TestAUR518NoPolicyKeepsRepoRuleOverride$' ;;
  AC-001-MUT-001) test_pattern='^TestAUR518PolicyKeepsRuleDespiteRepoDisable$'; expect_fail=1; apply_mutation ;;
esac

log="$run_dir/test.log"
set +e
# shellcheck disable=SC2086
(cd "$run_dir/root" && go test -mod=mod -p 1 -count=1 -timeout 300s -v $pkgs -run "$test_pattern") >"$log" 2>&1
status=$?
set -e
cat "$log" >&2

if [[ -n "$expect_fail" ]]; then
  grep -Eq -- '^--- FAIL: TestAUR518' "$log" || fail 'mutation-survived'
  (( status != 0 )) || fail 'mutation-survived-exit-zero'
  if grep -Eq 'build failed|cannot use|undefined:|syntax error' "$log"; then
    fail 'mutation-build-failure-not-behavioral'
  fi
  printf '%s/%s/pass (mutation produced RED)\n' "$card" "$selector"
  exit 0
fi

(( status == 0 )) || fail "go-test-exit:$status"
grep -Eq -- '^--- PASS: TestAUR518' "$log" || fail 'no-test-executed'

if [[ "$selector" == all ]]; then
  for name in PolicyKeepsRuleDespiteRepoDisable PolicySeverityOverrideIgnored PolicyIgnoreWinsOverRepo PolicyAndRepoSkillsBothReachPrompt MissingOrInvalidPolicyFailsClosed NoPolicyKeepsRepoRuleOverride; do
    grep -q "^--- PASS: TestAUR518$name " "$log" || fail "missing-pass:$name"
  done
fi
printf '%s/%s/pass\n' "$card" "$selector"
