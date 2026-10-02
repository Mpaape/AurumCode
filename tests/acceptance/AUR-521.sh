#!/usr/bin/env bash
# AUR-521 acceptance: a policy-governed review writes a compliance audit
# record (policy digest, workflow/reviewed SHA, model, verdict, gate
# decision, blocking findings, exceptions applied, coverage) and a SARIF
# 2.1.0 document whose per-finding fingerprint is the single, canonical
# finding identity (internal/render.FindingFingerprint) also shared with
# AUR-494. See docs/specs/AUR-521.md for the full account.
#
# Selectors:
#   all             run every behavior test below
#   AC-001          the audit record carries every required field and the
#                   policy digest changes when the policy's own config or a
#                   referenced skill file changes
#   AC-002          the SARIF document carries the 2.1.0-required fields
#                   (rule, severity, file, line) and the per-finding
#                   fingerprint is stable across repeated computation/runs
#                   and changes with the finding's own content
#   AC-003          a finding marked as excepted (synthetic until AUR-520
#                   lands) renders into SARIF's own suppressions shape with
#                   its justification
#   AC-004          an inconclusive run's audit record names the
#                   inconclusive decision and the SARIF invocation marks
#                   executionSuccessful=false with a notification naming why
#   AC-005          a secret canary present in a finding's own text never
#                   reaches the written audit record or SARIF document
#   AC-002-MUT-001  fold a per-call nonce (wall-clock time) into the
#                   fingerprint payload; AC-002's own stability test must go
#                   RED -- this is MUT-001: a fingerprint that moves for the
#                   SAME finding across runs defeats every consumer that
#                   deduplicates or tracks a finding by it
# Unknown selectors exit 64; infrastructure failures exit 79; behavioral
# failures exit 1.
set -Eeuo pipefail
export LC_ALL=C
umask 077

readonly card='AUR-521'
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
for source in \
  internal/render/finding_identity.go \
  internal/render/audit.go \
  internal/render/sarif.go \
  cmd/aurumcode/aur521.go \
  cmd/aurumcode/policygate.go; do
  [[ -f "$repo_root/$source" ]] || infra "missing-source:$source"
done
for behavior in \
  internal/render/finding_identity_test.go \
  internal/render/audit_test.go \
  internal/render/sarif_test.go \
  cmd/aurumcode/aur521_test.go; do
  [[ -f "$repo_root/$behavior" ]] || infra "missing-behavior-test:$behavior"
done

run_dir="$(mktemp -d "${TMPDIR:-/tmp}/aurum-a521.XXXXXX")" || infra mktemp
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

# MUT-001: fold a per-call, wall-clock-derived nonce into the fingerprint
# payload (internal/render/finding_identity.go's own anchor line, `nonce :=
# ""`). With it mutated, FindingFingerprint stops being pure: the exact same
# finding identity hashes to a different value on every call, which is
# exactly the defect AC-002's own stability test exists to catch. Anchored
# on the stable `"strconv"` import line (to add "time") and the `nonce :=
# ""` assignment; either missing is infrastructure, never a silent no-op.
apply_mutation() {
  local target="$run_dir/root/internal/render/finding_identity.go"
  local anchor_import='"strconv"'
  local anchor_nonce='nonce := ""'
  grep -Fq "$anchor_import" "$target" || infra mutation-anchor-missing
  grep -Fq "$anchor_nonce" "$target" || infra mutation-anchor-missing
  sed -i "s|${anchor_import}|${anchor_import}\n\t\"time\"|" "$target"
  sed -i "s|${anchor_nonce}|nonce := strconv.FormatInt(time.Now().UnixNano(), 10)|" "$target"
  grep -Fq "$anchor_nonce" "$target" && infra mutation-not-applied
  grep -Fq '"time"' "$target" || infra mutation-import-not-applied
  return 0
}

test_pattern=''
expect_fail=''
pkgs='./internal/render/... ./cmd/aurumcode/...'
case "$selector" in
  all)   test_pattern='^TestAUR521' ;;
  AC-001)
    test_pattern='^(TestAUR521PolicyDigestChangesWhenPolicyChanges|TestAUR521AuditAndSARIFOnGateBreach|TestAUR521PRPathWritesComplianceArtifacts)$'
    ;;
  AC-002)
    test_pattern='^(TestAUR521SARIFRequiredFields|TestAUR521SARIFFingerprintStableAcrossWrites|TestAUR521SARIFOmitsRegionForLinelessFinding|TestAUR521FindingFingerprintStableAcrossRuns|TestAUR521FindingFingerprintChangesWithContent|TestAUR521FindingFingerprintNormalizesWhitespaceAndPath|TestAUR521AuditFingerprintStableAcrossTwoRuns)$'
    ;;
  AC-003) test_pattern='^TestAUR521SARIFSuppressionForExceptedFinding$' ;;
  AC-004)
    test_pattern='^(TestAUR521SARIFInconclusiveRun|TestAUR521AuditRecordInconclusiveMarksOmittedFiles|TestAUR521AuditInconclusiveListsOmittedFiles)$'
    ;;
  AC-005)
    test_pattern='^(TestAUR521WriteAuditRecordRedactsSecretCanary|TestAUR521SARIFRedactsSecretCanary|TestAUR521RedactsSecretCanaryEndToEnd)$'
    ;;
  AC-002-MUT-001)
    test_pattern='^TestAUR521FindingFingerprintStableAcrossRuns$'
    expect_fail=1
    apply_mutation
    ;;
esac

log="$run_dir/test.log"
set +e
# shellcheck disable=SC2086
(cd "$run_dir/root" && go test -mod=mod -p 1 -count=1 -timeout 300s -v $pkgs -run "$test_pattern") >"$log" 2>&1
status=$?
set -e
cat "$log" >&2

if [[ -n "$expect_fail" ]]; then
  grep -Eq -- '^--- FAIL: TestAUR521' "$log" || fail 'mutation-survived'
  (( status != 0 )) || fail 'mutation-survived-exit-zero'
  if grep -Eq 'build failed|cannot use|undefined:|syntax error' "$log"; then
    fail 'mutation-build-failure-not-behavioral'
  fi
  printf '%s/%s/pass (mutation produced RED)\n' "$card" "$selector"
  exit 0
fi

(( status == 0 )) || fail "go-test-exit:$status"
grep -Eq -- '^--- PASS: TestAUR521' "$log" || fail 'no-test-executed'

if [[ "$selector" == all ]]; then
  for name in \
    FindingFingerprintStableAcrossRuns FindingFingerprintChangesWithContent \
    FindingFingerprintNormalizesWhitespaceAndPath \
    PolicyDigestChangesWhenPolicyChanges AuditRecordInconclusiveMarksOmittedFiles \
    WriteAuditRecordRedactsSecretCanary SARIFRequiredFields \
    SARIFFingerprintStableAcrossWrites SARIFSuppressionForExceptedFinding \
    SARIFOmitsRegionForLinelessFinding SARIFInconclusiveRun SARIFRedactsSecretCanary \
    AuditAndSARIFOnGateBreach AuditFingerprintStableAcrossTwoRuns \
    AuditInconclusiveListsOmittedFiles RedactsSecretCanaryEndToEnd \
    PRPathWritesComplianceArtifacts; do
    grep -q "^--- PASS: TestAUR521$name " "$log" || fail "missing-pass:$name"
  done
fi
printf '%s/%s/pass\n' "$card" "$selector"
