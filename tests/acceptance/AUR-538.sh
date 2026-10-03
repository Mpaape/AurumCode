#!/usr/bin/env bash
# AUR-538 acceptance: closes AUR-519/520's own non-blocking review gaps.
# See docs/specs/AUR-538.md for the full account and
# .board/cards/backlog/AUR-538.md for AC-001..AC-008.
#
# Selectors:
#   all             run every behavior test below, then re-run this same
#                   script for AC-008 and AC-001-MUT-001 (sealed oci-run
#                   invokes this script with no argument, i.e. "all", so
#                   those two must not be reachable only by name)
#   AC-001          --base, a fixture with NO findings at all (no
#                   hardcoded secret), gate.inconclusive: block, model
#                   verdict "" or "changes_requested": the report never
#                   reads "**Verdict:** Approve"; the identical fixture
#                   with no gate declared still reads Approve (positive
#                   control)
#   AC-002          the parser scrubs a model-forged
#                   metadata.policy_gate_withheld key
#   AC-003          AUR-519's own decorative, never-failing assertion
#                   (TestAUR519GatePartialCoverageInconclusiveWarns) is
#                   fixed to use a secret-free fixture
#   AC-004          git-less, with a missing HEAD tree object: declared
#                   unverifiable, both with and without a git binary
#   AC-005          --base, no "origin" remote: a configured exception
#                   that would otherwise match fails closed (gate fails,
#                   identity-unavailable notice); no exceptions
#                   configured at all: no notice line
#   AC-006          an expired exception listed before a renewed, active
#                   one for the same finding: the active one applies
#   AC-007          the aurumcode/policy-gate commit status description
#                   is capped at GitHub's 140-character limit, rune-safe,
#                   result word first, breach named ahead of the
#                   inconclusive reason
#   AC-008          tests/acceptance/AUR-520.sh's own per-AC selectors
#                   each require every one of their own named tests (not
#                   just "some test in the log passed")
#   AC-001-MUT-001  remove the --base verdict pull-down/withheld marker
#                   (main.go) -- AC-001 must FAIL (RED)
# Unknown selectors exit 64; infrastructure failures exit 79; behavioral
# failures exit 1.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-538'
selector="${1:-all}"

case "$selector" in
  all|AC-001|AC-002|AC-003|AC-004|AC-005|AC-006|AC-007|AC-008|AC-001-MUT-001) ;;
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
[[ -f "$repo_root/cmd/aurumcode/status_description.go" ]] || infra missing-source
[[ -f "$repo_root/cmd/aurumcode/aur538_test.go" ]] || infra missing-behavior-test
[[ -f "$repo_root/internal/prompt/parser_aur538_test.go" ]] || infra missing-behavior-test
[[ -f "$repo_root/tests/acceptance/AUR-520.sh" ]] || infra missing-source

# AC-008 proves tests/acceptance/AUR-520.sh's own per-selector fix is
# actually load-bearing, not merely present. It builds ONE self-contained
# copy (go.mod/go.sum/cmd/internal/pkg plus the script itself, at the same
# relative layout, in a mktemp dir -- N2: never a log or a copy left
# under the repository root) so the mutations below can edit the COPIED
# script without ever touching the real one.
#
# Positive control: the unmodified copy's own seven selectors must all
# still exit 0 -- this is the SAME loop the pre-fix version of this file
# ran, kept so a regression in AUR-520.sh itself is still caught here.
#
# Negative control (B1): merely checking "exit 0" cannot tell "the
# per-selector names loop is wired up" apart from "AUR-520.sh happens to
# exit 0 for some other reason" -- reverting the AC-008 fix in
# AUR-520.sh (back to a single `grep -Eq -- '^--- PASS: TestAUR520' "$log"`
# check) would pass the positive control too, since both of an AC's named
# tests still run and pass. So for each AC-00N (001..005), this drops ONE
# of that selector's own two named tests from test_pattern (a literal,
# fixed-string edit -- never touching `names`, which still names BOTH),
# so only one of the two tests actually executes, and requires the
# now-mutated copy to exit 1 with stderr naming exactly the dropped test
# via AUR-520.sh's own "missing-pass:<name>" line. Only the fix this card
# added could ever produce that: the old, unfixed AUR-520.sh accepted any
# single PASS and would have read this as green.
if [[ "$selector" == AC-008 ]]; then
  outer="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a538-508.XXXXXX")" || infra mktemp
  trap 'chmod -R u+w -- "$outer" >/dev/null 2>&1 || true; rm -rf -- "$outer" >/dev/null 2>&1 || true' EXIT INT TERM HUP
  mkdir -p "$outer/tests/acceptance"
  for source in go.mod go.sum cmd internal pkg; do
    cp -R "$repo_root/$source" "$outer/$source"
  done
  cp "$repo_root/tests/acceptance/AUR-520.sh" "$outer/tests/acceptance/AUR-520.sh"
  chmod -R u+w -- "$outer"

  # Performance only, never behavior: this card's own go-unit-offline-v1
  # profile budgets 600s/2GB for the WHOLE accept run, and "all" (below)
  # now runs this selector too, so AC-008's twelve otherwise-independent
  # AUR-520.sh invocations (each copying go.mod/go.sum/cmd/internal/pkg
  # into ITS OWN fresh mktemp and building from scratch) must not each
  # pay a full, cold rebuild. The copy's own `export GOCACHE="$run_dir/
  # cache"` line is patched to prefer an inherited GOCACHE when the
  # caller (this block) sets one, so all twelve share one populated
  # build cache -- the first invocation warms it, the other eleven reuse
  # it. AUR-520.sh's own behavior (what it tests, how it decides
  # pass/fail) is untouched; only where its build cache lives changes.
  grep -Fq 'export GOCACHE="$run_dir/cache" GOTMPDIR="$run_dir/gotmp" TMPDIR="$run_dir"' "$outer/tests/acceptance/AUR-520.sh" ||
    infra cache-share-anchor-missing
  sed -i 's#export GOCACHE="$run_dir/cache" GOTMPDIR="$run_dir/gotmp" TMPDIR="$run_dir"#export GOCACHE="${AUR538_SHARED_GOCACHE:-$run_dir/cache}" GOTMPDIR="$run_dir/gotmp" TMPDIR="$run_dir"#' "$outer/tests/acceptance/AUR-520.sh"
  grep -Fq 'AUR538_SHARED_GOCACHE' "$outer/tests/acceptance/AUR-520.sh" || infra cache-share-patch-failed
  export AUR538_SHARED_GOCACHE="$outer/sharedcache"
  mkdir -p "$AUR538_SHARED_GOCACHE"

  pristine="$outer/tests/acceptance/AUR-520.sh.pristine"
  cp "$outer/tests/acceptance/AUR-520.sh" "$pristine"

  for sub in all AC-001 AC-002 AC-003 AC-004 AC-005 AC-002-MUT-001; do
    bash "$outer/tests/acceptance/AUR-520.sh" "$sub" >"$outer/positive.log" 2>&1 ||
      { cat "$outer/positive.log" >&2; fail "AUR-520.sh-selector-failed:$sub"; }
  done

  # selector:droppedFullTestName:droppedShortName
  for triple in \
      'AC-001:TestAUR520PRValidExceptionPasses:PRValidExceptionPasses' \
      'AC-002:TestAUR520EvaluateGateExpiredExceptionStillBreaches:EvaluateGateExpiredExceptionStillBreaches' \
      'AC-003:TestAUR520MatchExceptionExactFieldsRequired:MatchExceptionExactFieldsRequired' \
      'AC-004:TestAUR520ApplyCentralPolicyException:ApplyCentralPolicyException' \
      'AC-005:TestAUR520ExceptionConfigValidateRequiresEveryField:ExceptionConfigValidateRequiresEveryField'; do
    sub="${triple%%:*}"
    rest="${triple#*:}"
    dropped_full="${rest%%:*}"
    dropped_short="${rest#*:}"

    cp "$pristine" "$outer/tests/acceptance/AUR-520.sh"
    grep -Fq "|$dropped_full" "$outer/tests/acceptance/AUR-520.sh" || infra "mutation-anchor-missing:$sub"
    sed -i "s#|$dropped_full##" "$outer/tests/acceptance/AUR-520.sh"
    grep -Fq "|$dropped_full" "$outer/tests/acceptance/AUR-520.sh" && infra "mutation-not-applied:$sub"

    set +e
    bash "$outer/tests/acceptance/AUR-520.sh" "$sub" >"$outer/negative.log" 2>&1
    neg_status=$?
    set -e
    (( neg_status == 1 )) || { cat "$outer/negative.log" >&2; fail "negative-control-wrong-exit:$sub:$neg_status"; }
    grep -Fq "missing-pass:$dropped_short" "$outer/negative.log" ||
      { cat "$outer/negative.log" >&2; fail "negative-control-missing-stderr:$sub"; }
  done

  printf '%s/%s/pass\n' "$card" "$selector"
  exit 0
fi

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a538.XXXXXX")" || infra mktemp
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

# AC-001-MUT-001: remove the one line that marks a review as
# gate-withheld (main.go, right after "if gateResult.Fail ||
# gateResult.Inconclusive"). Without it, formalReviewEvent/
# canonicalVerdict (pr.go/passes.go) have nothing engine-owned left to
# read, and a clean, zero-issue result falls through to "approve" even
# under a blocking gate -- exactly the bypass AC-001 exists to refuse.
# Anchored on the exact statement so a missing anchor is infrastructure,
# never a silent no-op.
apply_mutation() {
  local target="$run_dir/root/internal/gate/outcome.go"
  local anchor='			result.Metadata[prompt.PolicyGateWithheldKey] = "true"'
  grep -Fq "$anchor" "$target" || infra mutation-anchor-missing
  grep -Fv "$anchor" "$target" >"$target.tmp"
  mv "$target.tmp" "$target"
  grep -Fq "$anchor" "$target" && infra mutation-not-applied
  return 0
}

all_names='TestAUR538BaseCleanFixtureVerdictWithheldUnderBlock TestAUR538BaseCleanFixtureApprovesWithoutGate
  TestAUR538ParseScrubsPolicyGateWithheldKey
  TestAUR519GatePartialCoverageInconclusiveWarns
  TestAUR538MissingTreeObjectIsUnverifiable
  TestAUR538BaseNoOriginExceptionNeverMatches TestAUR538BaseNoOriginNoExceptionsNoNotice
  TestAUR538MatchExceptionActivePreferredOverExpiredSameFinding
  TestAUR538CapStatusDescriptionRuneSafe TestAUR538PublishPolicyGateStatusDescriptionCapped
  TestAUR538OrderedGateReasonsBreachBeforeExceptionBeforeInconclusive TestAUR538PublishPolicyGateStatusWordAndStateTable
  TestAUR538OrderedGateReasonsExactExceptionMarkersNotBareWord'

test_pattern=''
expect_fail=''
names=''
pkgs='./internal/prompt/... ./cmd/aurumcode/...'
case "$selector" in
  all)    test_pattern='^(TestAUR538|TestAUR519GatePartialCoverageInconclusiveWarns)'; names="$all_names" ;;
  AC-001) test_pattern='^(TestAUR538BaseCleanFixtureVerdictWithheldUnderBlock|TestAUR538BaseCleanFixtureApprovesWithoutGate)$'
          names='TestAUR538BaseCleanFixtureVerdictWithheldUnderBlock TestAUR538BaseCleanFixtureApprovesWithoutGate' ;;
  AC-002) test_pattern='^TestAUR538ParseScrubsPolicyGateWithheldKey$'
          names='TestAUR538ParseScrubsPolicyGateWithheldKey' ;;
  AC-003) test_pattern='^TestAUR519GatePartialCoverageInconclusiveWarns$'
          names='TestAUR519GatePartialCoverageInconclusiveWarns' ;;
  AC-004) test_pattern='^TestAUR538MissingTreeObjectIsUnverifiable$'
          names='TestAUR538MissingTreeObjectIsUnverifiable' ;;
  AC-005) test_pattern='^(TestAUR538BaseNoOriginExceptionNeverMatches|TestAUR538BaseNoOriginNoExceptionsNoNotice)$'
          names='TestAUR538BaseNoOriginExceptionNeverMatches TestAUR538BaseNoOriginNoExceptionsNoNotice' ;;
  AC-006) test_pattern='^TestAUR538MatchExceptionActivePreferredOverExpiredSameFinding$'
          names='TestAUR538MatchExceptionActivePreferredOverExpiredSameFinding' ;;
  AC-007) test_pattern='^(TestAUR538CapStatusDescriptionRuneSafe|TestAUR538PublishPolicyGateStatusDescriptionCapped|TestAUR538OrderedGateReasonsBreachBeforeExceptionBeforeInconclusive|TestAUR538PublishPolicyGateStatusWordAndStateTable|TestAUR538OrderedGateReasonsExactExceptionMarkersNotBareWord)$'
          names='TestAUR538CapStatusDescriptionRuneSafe TestAUR538PublishPolicyGateStatusDescriptionCapped TestAUR538OrderedGateReasonsBreachBeforeExceptionBeforeInconclusive TestAUR538PublishPolicyGateStatusWordAndStateTable TestAUR538OrderedGateReasonsExactExceptionMarkersNotBareWord' ;;
  AC-001-MUT-001)
          test_pattern='^(TestAUR538BaseCleanFixtureVerdictWithheldUnderBlock|TestAUR538BaseCleanFixtureApprovesWithoutGate)$'
          expect_fail=1; apply_mutation ;;
esac

log="$run_dir/test.log"
set +e
# shellcheck disable=SC2086
(cd "$run_dir/root" && go test -mod=mod -p 1 -count=1 -timeout 300s -v $pkgs -run "$test_pattern") >"$log" 2>&1
status=$?
set -e
cat "$log" >&2

if [[ -n "$expect_fail" ]]; then
  grep -Eq -- '^--- FAIL: TestAUR538' "$log" || fail 'mutation-survived'
  (( status != 0 )) || fail 'mutation-survived-exit-zero'
  if grep -Eq 'build failed|cannot use|undefined:|syntax error' "$log"; then
    fail 'mutation-build-failure-not-behavioral'
  fi
  printf '%s/%s/pass (mutation produced RED)\n' "$card" "$selector"
  exit 0
fi

(( status == 0 )) || fail "go-test-exit:$status"
grep -Eq -- '^--- PASS: Test' "$log" || fail 'no-test-executed'

# Every selector (not only "all") requires EVERY one of its own named
# tests to show its own PASS line -- never just "something in the log
# passed" (the same discipline AC-008 requires of AUR-520.sh).
for name in $names; do
  grep -q -- "^--- PASS: $name " "$log" || fail "missing-pass:$name"
done

# B1 (sealed-gate blocker): oci-run's go-unit-offline-v1 profile invokes
# this script with NO argument at all (selector "all"). Before this,
# "all" only ran the Go test pattern above, which never touches AC-008
# or AC-001-MUT-001 -- so AUR-520.sh's own AC-008 fix, or this card's own
# verdict-withheld mutation guard, could be reverted and the SEALED gate
# would still read green. "all" now re-execs this same script (by its
# own path, "$0") for each, mapping that child's own exit code back
# through this script's own convention: 79 (infrastructure) stays 79,
# any other non-zero is this script's own behavioral failure (1, via
# fail()), 0 is silently fine.
if [[ "$selector" == all ]]; then
  for nested in AC-008 AC-001-MUT-001; do
    nested_status=0
    bash "$0" "$nested" || nested_status=$?
    case "$nested_status" in
      0) ;;
      79) infra "nested-selector-failed:$nested" ;;
      *) fail "nested-selector-failed:$nested" ;;
    esac
  done
fi
printf '%s/%s/pass\n' "$card" "$selector"
