#!/usr/bin/env bash
# AUR-520 acceptance: an approved exception (repo+rule+path, owner, reason,
# expires) removes one exact finding from AUR-519's policy gate, shown as
# accepted; an expired exception stops applying and says so; a mismatched
# repo/rule/path never matches; under a central policy a repository's own
# exception for a policy rule is dropped with a warning; and a malformed
# exception fails the config closed, before any model call. See
# docs/specs/AUR-520.md for the full account.
#
# Selectors:
#   all             run every behavior test below
#   AC-001          a valid exception for the exact finding makes the check
#                   pass and the finding is shown accepted with owner/validity
#   AC-002          an expired exception no longer applies; the check fails
#                   and the output says the exception expired
#   AC-003          an exception for a different repo/rule/path never matches
#   AC-004          under a central policy, a repo-declared exception for a
#                   policy rule is ignored with a warning
#   AC-005          an exception missing owner/reason/expires, or a malformed
#                   expires date, fails the config closed
#   AC-002-MUT-001  blank the expiry comparison so an expired exception is
#                   always read as still active; AC-002 must FAIL (RED) --
#                   this is MUT-001: ignoring the expiry date must not
#                   silently keep an exception applying past its own date
# Unknown selectors exit 64; infrastructure failures exit 79; behavioral
# failures exit 1.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-520'
selector="${1:-all}"

case "$selector" in
  all|AC-001|AC-002|AC-003|AC-004|AC-005|AC-002-MUT-001) ;;
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
[[ -f "$repo_root/internal/config/exceptions.go" ]] || infra missing-source
[[ -f "$repo_root/internal/config/gate.go" ]] || infra missing-source
[[ -f "$repo_root/cmd/aurumcode/aur520.go" ]] || infra missing-source
[[ -f "$repo_root/cmd/aurumcode/policygate.go" ]] || infra missing-source
[[ -f "$repo_root/cmd/aurumcode/aur520_test.go" ]] || infra missing-behavior-test

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a520.XXXXXX")" || infra mktemp
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

# MUT-001: blank the one condition that turns "today is past the
# exception's own expires date" into exceptionExpired (matchException,
# cmd/aurumcode/aur520.go). With the condition forced false, an expired
# exception is read as exceptionActive forever -- exactly the bypass
# AC-002 exists to refuse: ignoring the expiry date must never silently
# keep an exception applying past its own date. Anchored on the stable
# condition; the token is split so this file cannot match its own edit,
# and a missing anchor is infrastructure, never a silent no-op.
apply_mutation() {
  local target="$run_dir/root/cmd/aurumcode/aur520.go"
  local anchor='if truncateToUTCDate(now).After(expires) {'
  grep -Fq "$anchor" "$target" || infra mutation-anchor-missing
  # "&" is sed's whole-match placeholder in a replacement, so it is
  # escaped here -- an unescaped "&&" would make sed re-insert the
  # matched text and corrupt the line instead of producing valid Go.
  local replacement='if false \&\& truncateToUTCDate(now).After(expires) {'
  sed -i "s|${anchor}|${replacement}|" "$target"
  grep -Fq "$anchor" "$target" && infra mutation-not-applied
  return 0
}

# AUR-538 AC-008: every selector below (not just "all") names the exact
# set of tests it requires, in `names` -- the same list the final check
# loops over to require EACH ONE's own PASS line, never just "at least
# one test in the -run pattern passed". Before this fix a per-AC selector
# could read green with only one of its two named tests actually
# executed (a typo'd -run pattern, a test silently skipped) because the
# only check was "some TestAUR520... passed somewhere in the log".
all_names='ExceptionConfigValidateRequiresEveryField ExceptionConfigValidateRejectsMalformedExpires
  ExceptionConfigExpiresOnIsUTCMidnight ValidateExceptionsNamesTheFailingIndex
  ParseRejectsMalformedException ParseAcceptsValidExceptions ApplyCentralPolicyException
  MatchExceptionExactFieldsRequired MatchExceptionRepoCaseInsensitive
  MatchExceptionUnknownRepoNeverMatches MatchExceptionExpiryBoundary
  MatchExceptionTimezoneCannotBypassExpiry TruncateToUTCDateDropsTimeOfDay
  EvaluateGateExceptionSkipsBreach EvaluateGateExpiredExceptionStillBreaches
  EvaluateGateInconclusiveBlockNeverAppliesException EvaluateGateIgnoresModelFreeTextFields
  EvaluateGateNoExceptionsIsByteIdentical BaseValidExceptionPasses BaseExpiredExceptionStillFails
  BaseMismatchedExceptionNeverApplies CentralPolicyDropsRepoException BaseMalformedExceptionFailsClosed
  LocalRepoIdentityFromOriginRemote PRValidExceptionPasses'

test_pattern=''
expect_fail=''
names=''
pkgs='./internal/config/... ./cmd/aurumcode/...'
case "$selector" in
  all)            test_pattern='^TestAUR520'; names="$all_names" ;;
  AC-001)         test_pattern='^(TestAUR520BaseValidExceptionPasses|TestAUR520PRValidExceptionPasses)$'; names='BaseValidExceptionPasses PRValidExceptionPasses' ;;
  AC-002)         test_pattern='^(TestAUR520BaseExpiredExceptionStillFails|TestAUR520EvaluateGateExpiredExceptionStillBreaches)$'; names='BaseExpiredExceptionStillFails EvaluateGateExpiredExceptionStillBreaches' ;;
  AC-003)         test_pattern='^(TestAUR520BaseMismatchedExceptionNeverApplies|TestAUR520MatchExceptionExactFieldsRequired)$'; names='BaseMismatchedExceptionNeverApplies MatchExceptionExactFieldsRequired' ;;
  AC-004)         test_pattern='^(TestAUR520CentralPolicyDropsRepoException|TestAUR520ApplyCentralPolicyException)$'; names='CentralPolicyDropsRepoException ApplyCentralPolicyException' ;;
  AC-005)         test_pattern='^(TestAUR520BaseMalformedExceptionFailsClosed|TestAUR520ExceptionConfigValidateRequiresEveryField)$'; names='BaseMalformedExceptionFailsClosed ExceptionConfigValidateRequiresEveryField' ;;
  AC-002-MUT-001) test_pattern='^(TestAUR520BaseExpiredExceptionStillFails|TestAUR520EvaluateGateExpiredExceptionStillBreaches)$'; expect_fail=1; apply_mutation ;;
esac

log="$run_dir/test.log"
set +e
# shellcheck disable=SC2086
(cd "$run_dir/root" && go test -mod=mod -p 1 -count=1 -timeout 300s -v $pkgs -run "$test_pattern") >"$log" 2>&1
status=$?
set -e
cat "$log" >&2

if [[ -n "$expect_fail" ]]; then
  grep -Eq -- '^--- FAIL: TestAUR520' "$log" || fail 'mutation-survived'
  (( status != 0 )) || fail 'mutation-survived-exit-zero'
  if grep -Eq 'build failed|cannot use|undefined:|syntax error' "$log"; then
    fail 'mutation-build-failure-not-behavioral'
  fi
  printf '%s/%s/pass (mutation produced RED)\n' "$card" "$selector"
  exit 0
fi

(( status == 0 )) || fail "go-test-exit:$status"
grep -Eq -- '^--- PASS: TestAUR520' "$log" || fail 'no-test-executed'

# AC-008: every selector (not only "all") requires EVERY one of its own
# named tests to show its own PASS line -- never just "something in the
# log passed".
for name in $names; do
  grep -q "^--- PASS: TestAUR520$name " "$log" || fail "missing-pass:$name"
done
printf '%s/%s/pass\n' "$card" "$selector"
