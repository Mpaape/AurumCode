#!/usr/bin/env bash
# AUR-519 acceptance: a skill's "## " section becomes a citable rule
# (policy's and, when the repository opts in, its own), the policy's
# gate.fail_on/inconclusive decides what reproves the check, and an
# inconclusive review (provider failure, AUR-476 partial coverage, or a
# degraded model parse) blocks or warns per configuration -- never
# publishing as approved. See docs/specs/AUR-519.md for the full account.
#
# Selectors:
#   all             run every behavior test below
#   AC-001          a finding citing a skill section at or above fail_on's
#                   threshold fails the check and names the skill+section
#   AC-002          a finding citing an unknown skill/section is discarded
#                   and counted as unlinked
#   AC-003          gate.inconclusive: block fails the check on a provider
#                   failure; the policy-gate line names the reason and the
#                   verdict never reads as approved
#   AC-004          AUR-476 partial coverage feeds the same inconclusive
#                   configuration (here: warn passes with a visible alert)
#   AC-005          a repo-origin skill finding never fails a policy's gate,
#                   and ApplyCentralPolicy's own gate precedence
#   AC-006          the policy gate's own commit-status context name is
#                   stable and is published only when the gate is declared
#   AC-007          adding a skill section makes it citable with no code
#                   change
#   AC-008          a model reply that is not valid JSON is flagged as a
#                   degraded parse, the flag cannot be forged by a model
#                   supplying its own "metadata.parse_mode", and it feeds
#                   the same inconclusive gate
#   AC-003-MUT-001  blank the one line that turns a provider failure into
#                   the gate's "provider_failure" inconclusive reason; AC-003
#                   must fail (RED) -- this is MUT-001: treating a provider
#                   failure as if it had produced no findings must not
#                   silently pass the gate
# Unknown selectors exit 64; infrastructure failures exit 79; behavioral
# failures exit 1.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-519'
selector="${1:-all}"

case "$selector" in
  all|AC-001|AC-002|AC-003|AC-004|AC-005|AC-006|AC-007|AC-008|AC-003-MUT-001) ;;
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
[[ -f "$repo_root/internal/review/skillrules.go" ]] || infra missing-source
[[ -f "$repo_root/internal/config/gate.go" ]] || infra missing-source
[[ -f "$repo_root/cmd/aurumcode/policygate.go" ]] || infra missing-source
[[ -f "$repo_root/cmd/aurumcode/aur519_e2e_test.go" ]] || infra missing-behavior-test

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a519.XXXXXX")" || infra mktemp
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

# MUT-001: blank the one ranking branch that turns a quality-provider failure
# into the gate's "provider_failure" inconclusive reason (AUR-576: the one
# ranking, gate.RankReason in internal/gate/reason.go, serves --base and
# --pr; it used to be an assignment in runReview, main.go). With it gone, the
# reason stays "" for
# exactly TestAUR519GateInconclusiveProviderFailureBlocks's own scenario, so
# evaluateGate never enters its inconclusive branch and the policy-gate line
# that test asserts on never appears -- the gate silently treats the
# provider's failure as if it had produced no findings at all, which is
# exactly what AC-003 exists to refuse. Anchored on the stable assignment;
# the token is split so this file cannot match its own edit, and a missing
# anchor is infrastructure, never a silent no-op.
apply_mutation() {
  local target="$run_dir/root/internal/gate/reason.go"
  local anchor='return ReasonProviderFailure'
  [[ "$(grep -Fc "$anchor" "$target")" == 1 ]] || infra mutation-anchor-missing
  sed -i "s|${anchor}|return ReasonNone|" "$target"
  grep -Fq "$anchor" "$target" && infra mutation-not-applied
  return 0
}

test_pattern=''
expect_fail=''
pkgs='./internal/review/... ./internal/config/... ./internal/prompt/... ./cmd/aurumcode/...'
case "$selector" in
  all)            test_pattern='^TestAUR519' ;;
  AC-001)         test_pattern='^TestAUR519GateSeverityBreachFailsCheck$' ;;
  AC-002)         test_pattern='^TestAUR519EnforceRuleCitationsDynamic$' ;;
  AC-003)         test_pattern='^TestAUR519GateInconclusiveProviderFailureBlocks$' ;;
  AC-004)         test_pattern='^TestAUR519GatePartialCoverageInconclusiveWarns$' ;;
  AC-005)         test_pattern='^(TestAUR519EvaluateGateRepoOriginNeverFails|TestAUR519ApplyCentralPolicyGate)$' ;;
  AC-006)         test_pattern='^TestAUR519PolicyGateStatusContextIsStable$' ;;
  AC-007)         test_pattern='^TestAUR519ParseSkillSections$' ;;
  AC-008)         test_pattern='^(TestAUR519DegradedParseDetection|TestAUR519EvaluateGateInconclusiveBlockAndWarn)$' ;;
  AC-003-MUT-001) test_pattern='^TestAUR519GateInconclusiveProviderFailureBlocks$'; expect_fail=1; apply_mutation ;;
esac

log="$run_dir/test.log"
set +e
# shellcheck disable=SC2086
(cd "$run_dir/root" && go test -mod=mod -p 1 -count=1 -timeout 300s -v $pkgs -run "$test_pattern") >"$log" 2>&1
status=$?
set -e
cat "$log" >&2

if [[ -n "$expect_fail" ]]; then
  grep -Eq -- '^--- FAIL: TestAUR519' "$log" || fail 'mutation-survived'
  (( status != 0 )) || fail 'mutation-survived-exit-zero'
  if grep -Eq 'build failed|cannot use|undefined:|syntax error' "$log"; then
    fail 'mutation-build-failure-not-behavioral'
  fi
  printf '%s/%s/pass (mutation produced RED)\n' "$card" "$selector"
  exit 0
fi

(( status == 0 )) || fail "go-test-exit:$status"
grep -Eq -- '^--- PASS: TestAUR519' "$log" || fail 'no-test-executed'

if [[ "$selector" == all ]]; then
  for name in ParseSkillSections EnforceRuleCitationsDynamic GateConfigThreshold GateConfigInconclusive \
      ApplyCentralPolicyGate DegradedParseDetection EvaluateGateNoGateDeclared EvaluateGateSeverityBreach \
      EvaluateGateRepoOriginNeverFails EvaluateGateInconclusiveBlockAndWarn MergedRuleCatalogIDs \
      GateSeverityBreachFailsCheck GateInconclusiveProviderFailureBlocks GatePartialCoverageInconclusiveWarns \
      PolicyGateStatusContextIsStable NoGateConfiguredStaysUntouched \
      GateSeverityBreachFailsCheckWithProfiles EvaluateGateWarnStillFailsOnBreach \
      EvaluateGateRuleSeverityFloorsModel EvaluateGateFailOnWithoutInconclusiveBlocks \
      MergeDynamicRulesPolicyWins ResolveRuleBuiltinWinsOverDynamic ProfilePassesCarryDegradedMetadata \
      DegradedParseNeverCached SkillsConfiguredNoGateStaysSafe PRGateWarnStillFailsOnBreach \
      PRGateInconclusiveBlockTable PartialCoverageNeverCachedUnderBlock ProfilePassesCoverageTakesWorstCase \
      NoGatePublishesApproveDespiteModelCommentVerdict VerdictWithheldAcrossModelVerdicts \
      BaseVerdictWithheldAcrossModelVerdicts ReviewerCopiesCoverageMetadata; do
    grep -q "^--- PASS: TestAUR519$name " "$log" || fail "missing-pass:$name"
  done
fi
printf '%s/%s/pass\n' "$card" "$selector"
